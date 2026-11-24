// SPDX-License-Identifier: Apache-2.0

// localcode-relay is a self-hostable, zero-knowledge HTTP/HTTPS relay binary
// that forwards opaque end-to-end encrypted frames between LocalCode workstations
// and paired mobile clients without ever seeing keys, code, prompts, or diffs.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	protocolVersion      = 1
	domainSeparation     = "localcode-e2ee-relay-v1"
	nonceBytes           = 12
	proofMaxClockSkewSec = 60
	defaultMaxQueueDepth = 256
	defaultMaxFrameBytes = 1 << 20 // 1 MiB
	defaultSessionTTL    = 15 * time.Minute
	defaultMaxSessions   = 4096
	defaultMaxPoll       = 30 * time.Second
	defaultRateLimit     = 60.0
	defaultRateBurst     = 120
)

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

type opaqueEnvelope struct {
	Version    int    `json:"v"`
	SessionID  string `json:"sid"`
	SenderRole string `json:"from"`
	Direction  string `json:"dir"`
	Seq        uint64 `json:"seq"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ct"`
	Timestamp  int64  `json:"ts"`
}

type relayConfig struct {
	MaxQueueDepth   int
	MaxFrameBytes   int64
	SessionIdleTTL  time.Duration
	MaxSessions     int
	MaxPollTimeout  time.Duration
	RateLimitPerSec float64
	RateBurst       int
}

type rateBucket struct {
	tokens   float64
	lastFill time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*rateBucket
}

func newRateLimiter(rate float64, burst int) *rateLimiter {
	if rate <= 0 {
		rate = defaultRateLimit
	}
	if burst <= 0 {
		burst = defaultRateBurst
	}
	return &rateLimiter{
		rate:    rate,
		burst:   float64(burst),
		buckets: make(map[string]*rateBucket),
	}
}

func (l *rateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &rateBucket{tokens: l.burst - 1, lastFill: now}
		return true
	}
	elapsed := now.Sub(b.lastFill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastFill = now
	}
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (l *rateLimiter) PurgeIdle(cutoff time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if b.lastFill.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

type sessionMailbox struct {
	mu                sync.Mutex
	authTokenHash     [sha256.Size]byte
	lastActivity      time.Time
	toWorkstation     []json.RawMessage
	toMobile          []json.RawMessage
	notifyWorkstation chan struct{}
	notifyMobile      chan struct{}
}

type relayServer struct {
	cfg       relayConfig
	mu        sync.RWMutex
	sessions  map[string]*sessionMailbox
	limiter   *rateLimiter
	closedCh  chan struct{}
	closeOnce sync.Once
}

func newRelayServer(cfg relayConfig) *relayServer {
	if cfg.MaxQueueDepth <= 0 {
		cfg.MaxQueueDepth = defaultMaxQueueDepth
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = defaultMaxFrameBytes
	}
	if cfg.SessionIdleTTL <= 0 {
		cfg.SessionIdleTTL = defaultSessionTTL
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = defaultMaxSessions
	}
	if cfg.MaxPollTimeout <= 0 {
		cfg.MaxPollTimeout = defaultMaxPoll
	}
	s := &relayServer{
		cfg:      cfg,
		sessions: make(map[string]*sessionMailbox),
		limiter:  newRateLimiter(cfg.RateLimitPerSec, cfg.RateBurst),
		closedCh: make(chan struct{}),
	}
	go s.sweepLoop()
	return s
}

func (s *relayServer) Close() {
	s.closeOnce.Do(func() {
		close(s.closedCh)
	})
}

func (s *relayServer) sweepLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.closedCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-s.cfg.SessionIdleTTL)
			s.mu.Lock()
			for sid, box := range s.sessions {
				box.mu.Lock()
				expired := box.lastActivity.Before(cutoff)
				box.mu.Unlock()
				if expired {
					delete(s.sessions, sid)
				}
			}
			s.mu.Unlock()
			s.limiter.PurgeIdle(cutoff)
		}
	}
}

func verifyProof(token, sid, role, method, proof string, now time.Time) bool {
	if token == "" || proof == "" {
		return false
	}
	parts := strings.Split(strings.TrimSpace(proof), ".")
	if len(parts) != 2 {
		return false
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	skew := now.UTC().Unix() - ts
	if skew < -proofMaxClockSkewSec || skew > proofMaxClockSkewSec {
		return false
	}
	providedMAC, err := hex.DecodeString(parts[1])
	if err != nil || len(providedMAC) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(domainSeparation + "|req|" + sid + "|" + role + "|" + strings.ToUpper(method) + "|" + parts[0]))
	return hmac.Equal(providedMAC, mac.Sum(nil))
}

func (s *relayServer) authenticate(r *http.Request, sid, role string, now time.Time) (*sessionMailbox, int, string) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, http.StatusUnauthorized, "missing bearer relay auth token"
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if len(token) < 32 {
		return nil, http.StatusUnauthorized, "invalid relay auth token"
	}
	if !verifyProof(token, sid, role, r.Method, strings.TrimSpace(r.Header.Get("X-Relay-Proof")), now) {
		return nil, http.StatusUnauthorized, "invalid or expired relay hmac proof"
	}
	tokenHash := sha256.Sum256([]byte(token))

	s.mu.RLock()
	box, ok := s.sessions[sid]
	s.mu.RUnlock()
	if ok {
		if subtle.ConstantTimeCompare(box.authTokenHash[:], tokenHash[:]) != 1 {
			return nil, http.StatusUnauthorized, "relay auth token mismatch"
		}
		box.mu.Lock()
		box.lastActivity = now
		box.mu.Unlock()
		return box, http.StatusOK, ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if box, ok := s.sessions[sid]; ok {
		if subtle.ConstantTimeCompare(box.authTokenHash[:], tokenHash[:]) != 1 {
			return nil, http.StatusUnauthorized, "relay auth token mismatch"
		}
		box.mu.Lock()
		box.lastActivity = now
		box.mu.Unlock()
		return box, http.StatusOK, ""
	}
	if len(s.sessions) >= s.cfg.MaxSessions {
		return nil, http.StatusServiceUnavailable, "relay session capacity reached"
	}
	box = &sessionMailbox{
		authTokenHash:     tokenHash,
		lastActivity:      now,
		notifyWorkstation: make(chan struct{}, 1),
		notifyMobile:      make(chan struct{}, 1),
	}
	s.sessions[sid] = box
	return box, http.StatusOK, ""
}

func (s *relayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	if r.URL.Path == "/relay/v1/health" {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.mu.RLock()
		active := len(s.sessions)
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"active_sessions": active,
			"max_frame_bytes": s.cfg.MaxFrameBytes,
			"max_queue_depth": s.cfg.MaxQueueDepth,
			"zero_knowledge":  true,
		})
		return
	}

	const prefix = "/relay/v1/sessions/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	sid := rest[:slash]
	action := rest[slash+1:]
	if !sessionIDPattern.MatchString(sid) {
		writeJSONError(w, http.StatusBadRequest, "invalid session id")
		return
	}

	now := time.Now()
	if !s.limiter.Allow("ip:"+clientIP(r), now) || !s.limiter.Allow("sid:"+sid, now) {
		w.Header().Set("Retry-After", "1")
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	switch action {
	case "send":
		s.handleSend(w, r, sid, now)
	case "poll":
		s.handlePoll(w, r, sid, now)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func (s *relayServer) handleSend(w http.ResponseWriter, r *http.Request, sid string, now time.Time) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limited := http.MaxBytesReader(w, r.Body, s.cfg.MaxFrameBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "encrypted frame exceeds maximum size")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read frame")
		return
	}
	if int64(len(body)) == 0 || int64(len(body)) > s.cfg.MaxFrameBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "invalid frame size")
		return
	}
	var env opaqueEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeJSONError(w, http.StatusBadRequest, "malformed envelope json")
		return
	}
	if err := validateEnvelope(&env, sid); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	box, status, msg := s.authenticate(r, sid, env.SenderRole, now)
	if status != http.StatusOK {
		writeJSONError(w, status, msg)
		return
	}
	cleanFrame, err := json.Marshal(&env)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "envelope serialization error")
		return
	}

	box.mu.Lock()
	box.lastActivity = now
	if env.SenderRole == "workstation" {
		if len(box.toMobile) >= s.cfg.MaxQueueDepth {
			box.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			writeJSONError(w, http.StatusTooManyRequests, "mobile queue full")
			return
		}
		box.toMobile = append(box.toMobile, json.RawMessage(cleanFrame))
		select {
		case box.notifyMobile <- struct{}{}:
		default:
		}
	} else {
		if len(box.toWorkstation) >= s.cfg.MaxQueueDepth {
			box.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			writeJSONError(w, http.StatusTooManyRequests, "workstation queue full")
			return
		}
		box.toWorkstation = append(box.toWorkstation, json.RawMessage(cleanFrame))
		select {
		case box.notifyWorkstation <- struct{}{}:
		default:
		}
	}
	box.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "sid": sid, "seq": env.Seq})
}

func (s *relayServer) handlePoll(w http.ResponseWriter, r *http.Request, sid string, now time.Time) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "workstation" && role != "mobile" {
		writeJSONError(w, http.StatusBadRequest, "invalid role query parameter")
		return
	}
	box, status, msg := s.authenticate(r, sid, role, now)
	if status != http.StatusOK {
		writeJSONError(w, status, msg)
		return
	}

	maxFrames := 32
	if rawMax := strings.TrimSpace(r.URL.Query().Get("max")); rawMax != "" {
		if n, err := strconv.Atoi(rawMax); err == nil && n > 0 {
			maxFrames = n
		}
	}
	if maxFrames > s.cfg.MaxQueueDepth {
		maxFrames = s.cfg.MaxQueueDepth
	}
	timeout := time.Duration(0)
	if rawTimeout := strings.TrimSpace(r.URL.Query().Get("timeout")); rawTimeout != "" {
		if d, err := time.ParseDuration(rawTimeout); err == nil && d > 0 {
			timeout = d
		} else if secs, err := strconv.Atoi(rawTimeout); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	if timeout > s.cfg.MaxPollTimeout {
		timeout = s.cfg.MaxPollTimeout
	}

	drain := func() []json.RawMessage {
		box.mu.Lock()
		defer box.mu.Unlock()
		box.lastActivity = time.Now()
		q := &box.toMobile
		if role == "workstation" {
			q = &box.toWorkstation
		}
		if len(*q) == 0 {
			return nil
		}
		n := len(*q)
		if n > maxFrames {
			n = maxFrames
		}
		out := make([]json.RawMessage, n)
		copy(out, (*q)[:n])
		rem := len(*q) - n
		if rem == 0 {
			*q = nil
		} else {
			copy(*q, (*q)[n:])
			*q = (*q)[:rem]
		}
		return out
	}

	frames := drain()
	if len(frames) == 0 && timeout > 0 {
		notifyCh := box.notifyMobile
		if role == "workstation" {
			notifyCh = box.notifyWorkstation
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for len(frames) == 0 {
			select {
			case <-notifyCh:
				frames = drain()
			case <-timer.C:
				goto respond
			case <-r.Context().Done():
				return
			case <-s.closedCh:
				goto respond
			}
		}
	}

respond:
	if frames == nil {
		frames = []json.RawMessage{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sid":    sid,
		"role":   role,
		"frames": frames,
	})
}

func validateEnvelope(env *opaqueEnvelope, expectedSID string) error {
	if env.Version != protocolVersion {
		return errors.New("unsupported envelope version")
	}
	if env.SessionID != expectedSID {
		return errors.New("envelope session id mismatch")
	}
	switch env.SenderRole {
	case "workstation":
		if env.Direction != "workstation->mobile" {
			return errors.New("invalid direction for workstation sender")
		}
	case "mobile":
		if env.Direction != "mobile->workstation" {
			return errors.New("invalid direction for mobile sender")
		}
	default:
		return errors.New("invalid sender role")
	}
	if env.Seq == 0 {
		return errors.New("sequence must be positive")
	}
	if nb, err := base64.RawURLEncoding.DecodeString(env.Nonce); err != nil || len(nb) != nonceBytes {
		return errors.New("invalid opaque nonce")
	}
	if ct, err := base64.RawURLEncoding.DecodeString(env.Ciphertext); err != nil || len(ct) < 16 {
		return errors.New("invalid opaque ciphertext")
	}
	return nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("localcode-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)

	listenAddr := fs.String("listen", ":8443", "Listen address (e.g. :8443 or 127.0.0.1:8443)")
	tlsCert := fs.String("tls-cert", "", "Path to TLS certificate PEM file")
	tlsKey := fs.String("tls-key", "", "Path to TLS private key PEM file")
	maxFrame := fs.Int64("max-frame-bytes", defaultMaxFrameBytes, "Maximum encrypted frame size in bytes")
	maxQueue := fs.Int("max-queue", defaultMaxQueueDepth, "Maximum queued frames per direction per session")
	sessionTTL := fs.Duration("session-ttl", defaultSessionTTL, "Idle session expiration duration")
	maxSessions := fs.Int("max-sessions", defaultMaxSessions, "Maximum concurrent relay sessions")
	rateLimit := fs.Float64("rate-limit", defaultRateLimit, "Sustained requests per second per IP/session")
	rateBurst := fs.Int("rate-burst", defaultRateBurst, "Burst request capacity per IP/session")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*tlsCert == "") != (*tlsKey == "") {
		fmt.Fprintln(stderr, "error: --tls-cert and --tls-key must be provided together")
		return 2
	}

	relay := newRelayServer(relayConfig{
		MaxQueueDepth:   *maxQueue,
		MaxFrameBytes:   *maxFrame,
		SessionIdleTTL:  *sessionTTL,
		MaxSessions:     *maxSessions,
		MaxPollTimeout:  defaultMaxPoll,
		RateLimitPerSec: *rateLimit,
		RateBurst:       *rateBurst,
	})
	defer relay.Close()

	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           relay,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       90 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := log.New(stdout, "[localcode-relay] ", log.LstdFlags|log.LUTC)
	errCh := make(chan error, 1)
	go func() {
		if *tlsCert != "" && *tlsKey != "" {
			logger.Printf("listening on https://%s (zero-knowledge E2EE relay, max_queue=%d, max_frame=%d)", *listenAddr, *maxQueue, *maxFrame)
			errCh <- srv.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			logger.Printf("listening on http://%s (zero-knowledge E2EE relay, max_queue=%d, max_frame=%d)", *listenAddr, *maxQueue, *maxFrame)
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "server error: %v\n", err)
			return 1
		}
		return 0
	case <-ctx.Done():
		logger.Printf("shutdown signal received; draining connections...")
		relay.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "graceful shutdown error: %v\n", err)
			return 1
		}
		logger.Printf("stopped cleanly")
		return 0
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
