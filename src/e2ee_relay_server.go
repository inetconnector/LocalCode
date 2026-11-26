// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRelayMaxQueueDepth   = 256
	DefaultRelayMaxFrameBytes   = 1 << 20 // 1 MiB
	DefaultRelaySessionIdleTTL  = 15 * time.Minute
	DefaultRelayMaxSessions     = 4096
	DefaultRelayMaxPollTimeout  = 30 * time.Second
	DefaultRelayRateLimitPerSec = 60.0
	DefaultRelayRateBurst       = 120
)

// ZeroKnowledgeRelayConfig configures bounds and rate limits for ZeroKnowledgeRelayServer.
type ZeroKnowledgeRelayConfig struct {
	MaxQueueDepth   int
	MaxFrameBytes   int64
	SessionIdleTTL  time.Duration
	MaxSessions     int
	MaxPollTimeout  time.Duration
	RateLimitPerSec float64
	RateBurst       int
	SweepInterval   time.Duration
}

type relayRateBucket struct {
	tokens   float64
	lastFill time.Time
}

type relayRateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*relayRateBucket
}

func newRelayRateLimiter(rate float64, burst int) *relayRateLimiter {
	if rate <= 0 {
		rate = DefaultRelayRateLimitPerSec
	}
	if burst <= 0 {
		burst = DefaultRelayRateBurst
	}
	return &relayRateLimiter{
		rate:    rate,
		burst:   float64(burst),
		buckets: make(map[string]*relayRateBucket),
	}
}

func (l *relayRateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &relayRateBucket{tokens: l.burst - 1, lastFill: now}
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

func (l *relayRateLimiter) PurgeIdle(cutoff time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if b.lastFill.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

type relaySessionMailbox struct {
	mu                sync.Mutex
	authTokenHash     [sha256.Size]byte
	lastActivity      time.Time
	toWorkstation     []json.RawMessage
	toMobile          []json.RawMessage
	notifyWorkstation chan struct{}
	notifyMobile      chan struct{}
}

func newRelaySessionMailbox(tokenHash [sha256.Size]byte, now time.Time) *relaySessionMailbox {
	return &relaySessionMailbox{
		authTokenHash:     tokenHash,
		lastActivity:      now,
		notifyWorkstation: make(chan struct{}, 1),
		notifyMobile:      make(chan struct{}, 1),
	}
}

// ZeroKnowledgeRelayServer forwards opaque E2EE envelopes between workstation and mobile
// endpoints without ever possessing decryption keys or observing plaintext payloads.
type ZeroKnowledgeRelayServer struct {
	cfg       ZeroKnowledgeRelayConfig
	mu        sync.RWMutex
	sessions  map[string]*relaySessionMailbox
	limiter   *relayRateLimiter
	closedCh  chan struct{}
	closeOnce sync.Once
	nowFn     func() time.Time
}

// NewZeroKnowledgeRelayServer creates a bounded, rate-limited zero-knowledge relay server.
func NewZeroKnowledgeRelayServer(cfg ZeroKnowledgeRelayConfig) *ZeroKnowledgeRelayServer {
	if cfg.MaxQueueDepth <= 0 {
		cfg.MaxQueueDepth = DefaultRelayMaxQueueDepth
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = DefaultRelayMaxFrameBytes
	}
	if cfg.SessionIdleTTL <= 0 {
		cfg.SessionIdleTTL = DefaultRelaySessionIdleTTL
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = DefaultRelayMaxSessions
	}
	if cfg.MaxPollTimeout <= 0 {
		cfg.MaxPollTimeout = DefaultRelayMaxPollTimeout
	}
	if cfg.RateLimitPerSec <= 0 {
		cfg.RateLimitPerSec = DefaultRelayRateLimitPerSec
	}
	if cfg.RateBurst <= 0 {
		cfg.RateBurst = DefaultRelayRateBurst
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = time.Minute
	}

	s := &ZeroKnowledgeRelayServer{
		cfg:      cfg,
		sessions: make(map[string]*relaySessionMailbox),
		limiter:  newRelayRateLimiter(cfg.RateLimitPerSec, cfg.RateBurst),
		closedCh: make(chan struct{}),
		nowFn:    time.Now,
	}
	go s.sweepLoop()
	return s
}

// Close stops background cleanup and unblocks any active long-poll requests.
func (s *ZeroKnowledgeRelayServer) Close() {
	s.closeOnce.Do(func() {
		close(s.closedCh)
	})
}

func (s *ZeroKnowledgeRelayServer) sweepLoop() {
	ticker := time.NewTicker(s.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.closedCh:
			return
		case <-ticker.C:
			s.PurgeExpiredSessions()
		}
	}
}

// PurgeExpiredSessions removes idle sessions exceeding SessionIdleTTL.
func (s *ZeroKnowledgeRelayServer) PurgeExpiredSessions() int {
	now := s.nowFn()
	cutoff := now.Add(-s.cfg.SessionIdleTTL)
	removed := 0

	s.mu.Lock()
	for sid, box := range s.sessions {
		box.mu.Lock()
		expired := box.lastActivity.Before(cutoff)
		box.mu.Unlock()
		if expired {
			delete(s.sessions, sid)
			removed++
		}
	}
	s.mu.Unlock()

	s.limiter.PurgeIdle(cutoff)
	return removed
}

// RegisterSession pre-binds a session ID to its HMAC-derived relay auth token hash.
func (s *ZeroKnowledgeRelayServer) RegisterSession(sessionID, relayAuthToken string) error {
	if err := ValidateRelaySessionID(sessionID); err != nil {
		return err
	}
	if len(relayAuthToken) < 32 {
		return errors.New("relay: auth token too short")
	}
	tokenHash := sha256.Sum256([]byte(relayAuthToken))
	now := s.nowFn()

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok {
		if subtle.ConstantTimeCompare(existing.authTokenHash[:], tokenHash[:]) != 1 {
			return errors.New("relay: session already bound to different auth token")
		}
		existing.mu.Lock()
		existing.lastActivity = now
		existing.mu.Unlock()
		return nil
	}
	if len(s.sessions) >= s.cfg.MaxSessions {
		return errors.New("relay: max active sessions reached")
	}
	s.sessions[sessionID] = newRelaySessionMailbox(tokenHash, now)
	return nil
}

// ActiveSessionCount returns the number of currently tracked relay sessions.
func (s *ZeroKnowledgeRelayServer) ActiveSessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// ServeHTTP dispatches /relay/v1/health, /relay/v1/sessions/{sid}/send, and /relay/v1/sessions/{sid}/poll.
func (s *ZeroKnowledgeRelayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	if r.URL.Path == "/relay/v1/health" {
		if r.Method != http.MethodGet {
			writeRelayError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"active_sessions": s.ActiveSessionCount(),
			"max_frame_bytes": s.cfg.MaxFrameBytes,
			"max_queue_depth": s.cfg.MaxQueueDepth,
			"zero_knowledge":  true,
		})
		return
	}

	const prefix = "/relay/v1/sessions/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeRelayError(w, http.StatusNotFound, "not found")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		writeRelayError(w, http.StatusNotFound, "not found")
		return
	}
	sid := rest[:slash]
	action := rest[slash+1:]
	if err := ValidateRelaySessionID(sid); err != nil {
		writeRelayError(w, http.StatusBadRequest, "invalid session id")
		return
	}

	now := s.nowFn()
	ipKey := "ip:" + relayClientIP(r)
	sidKey := "sid:" + sid
	if !s.limiter.Allow(ipKey, now) || !s.limiter.Allow(sidKey, now) {
		w.Header().Set("Retry-After", "1")
		writeRelayError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	switch action {
	case "send":
		s.handleSend(w, r, sid, now)
	case "poll":
		s.handlePoll(w, r, sid, now)
	default:
		writeRelayError(w, http.StatusNotFound, "not found")
	}
}

func (s *ZeroKnowledgeRelayServer) authenticateRequest(r *http.Request, sid string, role RelayRole, now time.Time) (*relaySessionMailbox, int, string) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, http.StatusUnauthorized, "missing bearer relay auth token"
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if len(token) < 32 {
		return nil, http.StatusUnauthorized, "invalid relay auth token"
	}
	proof := strings.TrimSpace(r.Header.Get("X-Relay-Proof"))
	if !VerifyRelayRequestProof(token, sid, role, r.Method, proof, now) {
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
	box = newRelaySessionMailbox(tokenHash, now)
	s.sessions[sid] = box
	return box, http.StatusOK, ""
}

func (s *ZeroKnowledgeRelayServer) handleSend(w http.ResponseWriter, r *http.Request, sid string, now time.Time) {
	if r.Method != http.MethodPost {
		writeRelayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limited := http.MaxBytesReader(w, r.Body, s.cfg.MaxFrameBytes)
	rawBody, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeRelayError(w, http.StatusRequestEntityTooLarge, "encrypted frame exceeds maximum size")
			return
		}
		writeRelayError(w, http.StatusBadRequest, "failed to read frame")
		return
	}
	if int64(len(rawBody)) == 0 || int64(len(rawBody)) > s.cfg.MaxFrameBytes {
		writeRelayError(w, http.StatusRequestEntityTooLarge, "invalid frame size")
		return
	}

	// Parse ONLY outer routing metadata; ciphertext remains 100% opaque to the relay.
	var env RelayEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		writeRelayError(w, http.StatusBadRequest, "malformed envelope json")
		return
	}
	if err := validateOpaqueEnvelope(&env, sid); err != nil {
		writeRelayError(w, http.StatusBadRequest, err.Error())
		return
	}

	senderRole := RelayRole(env.SenderRole)
	box, status, msg := s.authenticateRequest(r, sid, senderRole, now)
	if status != http.StatusOK {
		writeRelayError(w, status, msg)
		return
	}

	// Re-marshal only the strict opaque RelayEnvelope struct so no extra unvalidated fields pass through.
	cleanFrame, err := json.Marshal(&env)
	if err != nil {
		writeRelayError(w, http.StatusInternalServerError, "envelope serialization error")
		return
	}

	box.mu.Lock()
	box.lastActivity = now
	if senderRole == RelayRoleWorkstation {
		if len(box.toMobile) >= s.cfg.MaxQueueDepth {
			box.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			writeRelayError(w, http.StatusTooManyRequests, "mobile queue full")
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
			writeRelayError(w, http.StatusTooManyRequests, "workstation queue full")
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
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":  true,
		"sid": sid,
		"seq": env.Seq,
	})
}

func (s *ZeroKnowledgeRelayServer) handlePoll(w http.ResponseWriter, r *http.Request, sid string, now time.Time) {
	if r.Method != http.MethodGet {
		writeRelayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	role := RelayRole(strings.TrimSpace(r.URL.Query().Get("role")))
	if role != RelayRoleWorkstation && role != RelayRoleMobile {
		writeRelayError(w, http.StatusBadRequest, "invalid role query parameter")
		return
	}

	box, status, msg := s.authenticateRequest(r, sid, role, now)
	if status != http.StatusOK {
		writeRelayError(w, status, msg)
		return
	}

	maxFrames := 32
	if rawMax := strings.TrimSpace(r.URL.Query().Get("max")); rawMax != "" {
		if parsed, err := strconv.Atoi(rawMax); err == nil && parsed > 0 {
			maxFrames = parsed
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

	frames := s.drainQueue(box, role, maxFrames)
	if len(frames) == 0 && timeout > 0 {
		notifyCh := box.notifyMobile
		if role == RelayRoleWorkstation {
			notifyCh = box.notifyWorkstation
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for len(frames) == 0 {
			select {
			case <-notifyCh:
				frames = s.drainQueue(box, role, maxFrames)
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
		"role":   string(role),
		"frames": frames,
	})
}

func (s *ZeroKnowledgeRelayServer) drainQueue(box *relaySessionMailbox, role RelayRole, maxFrames int) []json.RawMessage {
	box.mu.Lock()
	defer box.mu.Unlock()
	box.lastActivity = s.nowFn()

	var q *[]json.RawMessage
	if role == RelayRoleWorkstation {
		q = &box.toWorkstation
	} else {
		q = &box.toMobile
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
	remaining := len(*q) - n
	if remaining == 0 {
		*q = nil
	} else {
		copy(*q, (*q)[n:])
		*q = (*q)[:remaining]
	}
	return out
}

func validateOpaqueEnvelope(env *RelayEnvelope, expectedSID string) error {
	if env.Version != relayProtocolVersion {
		return errors.New("unsupported envelope version")
	}
	if env.SessionID != expectedSID {
		return errors.New("envelope session id mismatch")
	}
	switch RelayRole(env.SenderRole) {
	case RelayRoleWorkstation:
		if env.Direction != relayDirWorkstationToMobile {
			return errors.New("invalid direction for workstation sender")
		}
	case RelayRoleMobile:
		if env.Direction != relayDirMobileToWorkstation {
			return errors.New("invalid direction for mobile sender")
		}
	default:
		return errors.New("invalid sender role")
	}
	if env.Seq == 0 {
		return errors.New("sequence must be positive")
	}
	if nonceBytes, err := base64.RawURLEncoding.DecodeString(env.Nonce); err != nil || len(nonceBytes) != relayNonceBytes {
		return errors.New("invalid opaque nonce")
	}
	if ctBytes, err := base64.RawURLEncoding.DecodeString(env.Ciphertext); err != nil || len(ctBytes) < 16 {
		return errors.New("invalid opaque ciphertext")
	}
	return nil
}

func relayClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func writeRelayError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
	})
}
