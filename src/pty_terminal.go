// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxPTYSessions = 8
	DefaultPTYIdleTimeout = 15 * time.Minute
	DefaultPTYCols        = 80
	DefaultPTYRows        = 24
	minPTYCols            = 2
	maxPTYCols            = 500
	minPTYRows            = 1
	maxPTYRows            = 200
	maxPTYInputBytes      = 64 * 1024
	ptyReplayMaxEvents    = 256
	ptyReplayMaxBytes     = 256 * 1024
)

var (
	ErrMaxPTYSessions     = errors.New("maximum PTY sessions limit reached")
	ErrPTYSessionNotFound = errors.New("PTY session not found")
	ErrPTYSessionClosed   = errors.New("PTY session closed")
	ErrPTYPathEscapesRoot = errors.New("PTY workspace path escapes allowed root")
	ptySessionIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,64}$`)
)

// ptyBackend abstracts OS-specific PTY implementations (Windows ConPTY, Unix /dev/ptmx, or pipe fallback).
type ptyBackend interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	Close() error
	Wait() (int, error)
	BackendName() string
}

// PTYEvent represents an event emitted on a PTY session stream.
type PTYEvent struct {
	Seq       uint64    `json:"seq"`
	Type      string    `json:"type"` // "output", "resize", "exit", "error"
	Data      string    `json:"data,omitempty"`
	Cols      int       `json:"cols,omitempty"`
	Rows      int       `json:"rows,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// PTYSessionInfo is a serializable snapshot of a PTY session's metadata.
type PTYSessionInfo struct {
	ID            string    `json:"id"`
	WorkspaceRoot string    `json:"workspace_root"`
	Cols          int       `json:"cols"`
	Rows          int       `json:"rows"`
	Backend       string    `json:"backend"`
	CreatedAt     time.Time `json:"created_at"`
	LastActivity  time.Time `json:"last_activity"`
	Closed        bool      `json:"closed"`
	ExitCode      *int      `json:"exit_code,omitempty"`
}

// PTYSession represents a single interactive pseudo-terminal process.
type PTYSession struct {
	ID            string
	WorkspaceRoot string
	CreatedAt     time.Time

	mu           sync.Mutex
	cols         int
	rows         int
	lastActivity time.Time
	backend      ptyBackend
	backendName  string
	closed       bool
	exitCode     *int
	nextSeq      uint64
	replay       []PTYEvent
	replayBytes  int
	subscribers  map[uint64]chan PTYEvent
	nextSubID    uint64
	done         chan struct{}
	closeOnce    sync.Once
}

func newPTYSession(id, workspaceRoot string, cols, rows int, backend ptyBackend) *PTYSession {
	now := time.Now().UTC()
	s := &PTYSession{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		CreatedAt:     now,
		cols:          cols,
		rows:          rows,
		lastActivity:  now,
		backend:       backend,
		backendName:   backend.BackendName(),
		subscribers:   make(map[uint64]chan PTYEvent),
		done:          make(chan struct{}),
	}
	go s.readLoop()
	return s
}

func (s *PTYSession) Info() PTYSessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	var codeCopy *int
	if s.exitCode != nil {
		v := *s.exitCode
		codeCopy = &v
	}
	return PTYSessionInfo{
		ID:            s.ID,
		WorkspaceRoot: s.WorkspaceRoot,
		Cols:          s.cols,
		Rows:          s.rows,
		Backend:       s.backendName,
		CreatedAt:     s.CreatedAt,
		LastActivity:  s.lastActivity,
		Closed:        s.closed,
		ExitCode:      codeCopy,
	}
}

func (s *PTYSession) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) > maxPTYInputBytes {
		return fmt.Errorf("input exceeds maximum size of %d bytes", maxPTYInputBytes)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrPTYSessionClosed
	}
	backend := s.backend
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()

	_, err := backend.Write(data)
	return err
}

func (s *PTYSession) Resize(cols, rows int) error {
	cols, rows = clampPTYSize(cols, rows)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrPTYSessionClosed
	}
	backend := s.backend
	s.cols = cols
	s.rows = rows
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()

	if err := backend.Resize(cols, rows); err != nil {
		return err
	}
	s.publish(PTYEvent{
		Type:      "resize",
		Cols:      cols,
		Rows:      rows,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

func (s *PTYSession) Subscribe() (<-chan PTYEvent, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bufSize := len(s.replay) + 128
	if bufSize < 256 {
		bufSize = 256
	}
	ch := make(chan PTYEvent, bufSize)
	for _, ev := range s.replay {
		ch <- ev
	}
	if s.closed {
		close(ch)
		return ch, func() {}
	}
	subID := s.nextSubID
	s.nextSubID++
	s.subscribers[subID] = ch

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subscribers, subID)
			s.mu.Unlock()
		})
	}
	return ch, unsub
}

func (s *PTYSession) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		backend := s.backend
		s.mu.Unlock()
		if backend != nil {
			closeErr = backend.Close()
		}
	})
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
	return closeErr
}

func (s *PTYSession) idleDuration(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.lastActivity)
}

func (s *PTYSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *PTYSession) publish(ev PTYEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishLocked(ev)
}

func (s *PTYSession) publishLocked(ev PTYEvent) {
	s.nextSeq++
	ev.Seq = s.nextSeq
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	s.lastActivity = ev.Timestamp

	s.replay = append(s.replay, ev)
	s.replayBytes += len(ev.Data)
	for len(s.replay) > ptyReplayMaxEvents || (s.replayBytes > ptyReplayMaxBytes && len(s.replay) > 1) {
		s.replayBytes -= len(s.replay[0].Data)
		s.replay = s.replay[1:]
	}

	for _, ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
			// Drop oldest queued event for slow consumer rather than stalling the PTY reader.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func (s *PTYSession) readLoop() {
	defer close(s.done)

	buf := make([]byte, 4096)
	for {
		n, err := s.backend.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			s.publish(PTYEvent{
				Type:      "output",
				Data:      chunk,
				Timestamp: time.Now().UTC(),
			})
		}
		if err != nil {
			break
		}
	}

	code, _ := s.backend.Wait()
	_ = s.backend.Close()

	s.mu.Lock()
	if !s.closed {
		exitVal := code
		s.exitCode = &exitVal
		s.publishLocked(PTYEvent{
			Type:      "exit",
			ExitCode:  &exitVal,
			Timestamp: time.Now().UTC(),
		})
		s.closed = true
		for id, ch := range s.subscribers {
			close(ch)
			delete(s.subscribers, id)
		}
	}
	s.mu.Unlock()
}

// PTYManager manages lifecycle, concurrency limits, path boundaries, and idle eviction of PTY sessions.
type PTYManager struct {
	AllowedRoot string
	MaxSessions int
	IdleTimeout time.Duration

	mu          sync.Mutex
	sessions    map[string]*PTYSession
	stopSweep   chan struct{}
	sweepOnce   sync.Once
	closedAll   bool
	startFunc   func(workspaceRoot string, cols, rows int) (ptyBackend, error)
}

// NewPTYManager creates a PTYManager bounded by allowedRoot (when non-empty),
// enforcing DefaultMaxPTYSessions (8) and DefaultPTYIdleTimeout (15m).
func NewPTYManager(allowedRoot string) *PTYManager {
	m := &PTYManager{
		AllowedRoot: strings.TrimSpace(allowedRoot),
		MaxSessions: DefaultMaxPTYSessions,
		IdleTimeout: DefaultPTYIdleTimeout,
		sessions:    make(map[string]*PTYSession),
		stopSweep:   make(chan struct{}),
		startFunc:   startPTYProcess,
	}
	go m.sweepLoop(30 * time.Second)
	return m
}

func (m *PTYManager) maxSessionsLimit() int {
	if m.MaxSessions <= 0 {
		return DefaultMaxPTYSessions
	}
	return m.MaxSessions
}

func (m *PTYManager) idleTimeoutDuration() time.Duration {
	if m.IdleTimeout <= 0 {
		return DefaultPTYIdleTimeout
	}
	return m.IdleTimeout
}

func (m *PTYManager) sweepLoop(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopSweep:
			return
		case now := <-ticker.C:
			m.EvictIdle(now)
		}
	}
}

// EvictIdle closes and removes sessions whose idle duration exceeds IdleTimeout.
func (m *PTYManager) EvictIdle(now time.Time) int {
	timeout := m.idleTimeoutDuration()
	var expired []*PTYSession

	m.mu.Lock()
	for id, sess := range m.sessions {
		if sess.isClosed() || sess.idleDuration(now) >= timeout {
			expired = append(expired, sess)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	for _, sess := range expired {
		_ = sess.Close()
	}
	return len(expired)
}

// SetAllowedRoot updates the workspace root boundary safely across goroutines.
func (m *PTYManager) SetAllowedRoot(root string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.AllowedRoot = strings.TrimSpace(root)
	m.mu.Unlock()
}

func (m *PTYManager) resolveWorkspaceDir(workspaceRoot, cwd string) (string, error) {
	m.mu.Lock()
	allowedRoot := strings.TrimSpace(m.AllowedRoot)
	m.mu.Unlock()

	ws := strings.TrimSpace(workspaceRoot)
	if ws == "" {
		ws = allowedRoot
	}
	if ws == "" {
		return "", errors.New("workspace_root is required")
	}

	var validatedWS string
	var err error
	if allowedRoot != "" {
		validatedWS, err = ensureExistingProjectDir(allowedRoot, ws)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrPTYPathEscapesRoot, err)
		}
	} else {
		validatedWS, err = ensureExistingProjectDir(ws, "")
		if err != nil {
			return "", err
		}
	}

	if strings.TrimSpace(cwd) == "" {
		return validatedWS, nil
	}

	resolvedCWD, err := ensureExistingProjectDir(validatedWS, cwd)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPTYPathEscapesRoot, err)
	}
	if allowedRoot != "" {
		if _, err := ensureExistingProjectDir(allowedRoot, resolvedCWD); err != nil {
			return "", fmt.Errorf("%w: %v", ErrPTYPathEscapesRoot, err)
		}
	}
	return resolvedCWD, nil
}

// CreateSession spawns a new interactive PTY rooted at workspaceRoot.
func (m *PTYManager) CreateSession(workspaceRoot string, cols, rows int) (*PTYSession, error) {
	return m.CreateSessionInDir(workspaceRoot, "", cols, rows)
}

// CreateSessionInDir spawns a new interactive PTY at cwd inside workspaceRoot.
func (m *PTYManager) CreateSessionInDir(workspaceRoot, cwd string, cols, rows int) (*PTYSession, error) {
	cols, rows = clampPTYSize(cols, rows)
	dir, err := m.resolveWorkspaceDir(workspaceRoot, cwd)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.closedAll {
		m.mu.Unlock()
		return nil, ErrPTYSessionClosed
	}
	// Prune already-exited sessions before checking capacity.
	for id, sess := range m.sessions {
		if sess.isClosed() {
			delete(m.sessions, id)
		}
	}
	if len(m.sessions) >= m.maxSessionsLimit() {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w (max %d)", ErrMaxPTYSessions, m.maxSessionsLimit())
	}
	startFn := m.startFunc
	if startFn == nil {
		startFn = startPTYProcess
	}
	// Reserve a slot pessimistically via placeholder check after backend start under lock.
	m.mu.Unlock()

	backend, err := startFn(dir, cols, rows)
	if err != nil {
		return nil, err
	}

	id := "pty-" + newID()
	sess := newPTYSession(id, dir, cols, rows, backend)

	m.mu.Lock()
	if m.closedAll || len(m.sessions) >= m.maxSessionsLimit() {
		m.mu.Unlock()
		_ = sess.Close()
		if m.closedAll {
			return nil, ErrPTYSessionClosed
		}
		return nil, fmt.Errorf("%w (max %d)", ErrMaxPTYSessions, m.maxSessionsLimit())
	}
	m.sessions[id] = sess
	m.mu.Unlock()

	return sess, nil
}

func (m *PTYManager) GetSession(id string) (*PTYSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

func (m *PTYManager) ListSessions() []PTYSessionInfo {
	m.mu.Lock()
	sessions := make([]*PTYSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	out := make([]PTYSessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (m *PTYManager) Write(id string, data []byte) error {
	sess, ok := m.GetSession(id)
	if !ok {
		return ErrPTYSessionNotFound
	}
	return sess.Write(data)
}

func (m *PTYManager) Resize(id string, cols, rows int) error {
	sess, ok := m.GetSession(id)
	if !ok {
		return ErrPTYSessionNotFound
	}
	return sess.Resize(cols, rows)
}

func (m *PTYManager) ReadEvents(id string) (<-chan PTYEvent, func(), error) {
	sess, ok := m.GetSession(id)
	if !ok {
		return nil, nil, ErrPTYSessionNotFound
	}
	ch, unsub := sess.Subscribe()
	return ch, unsub, nil
}

func (m *PTYManager) Close(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return ErrPTYSessionNotFound
	}
	return sess.Close()
}

func (m *PTYManager) CloseAll() {
	m.sweepOnce.Do(func() {
		if m.stopSweep != nil {
			close(m.stopSweep)
		}
	})
	m.mu.Lock()
	m.closedAll = true
	sessions := make([]*PTYSession, 0, len(m.sessions))
	for id, sess := range m.sessions {
		sessions = append(sessions, sess)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, sess := range sessions {
		_ = sess.Close()
	}
}

func clampPTYSize(cols, rows int) (int, int) {
	if cols <= 0 {
		cols = DefaultPTYCols
	}
	if rows <= 0 {
		rows = DefaultPTYRows
	}
	if cols < minPTYCols {
		cols = minPTYCols
	} else if cols > maxPTYCols {
		cols = maxPTYCols
	}
	if rows < minPTYRows {
		rows = minPTYRows
	} else if rows > maxPTYRows {
		rows = maxPTYRows
	}
	return cols, rows
}

func ptyRequireLoopback(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if !loopbackRequestHost(r.Host) {
		http.Error(w, "forbidden host: loopback required", http.StatusForbidden)
		return false
	}
	if r.RemoteAddr != "" && !loopbackRequestHost(r.RemoteAddr) {
		http.Error(w, "forbidden remote address: loopback required", http.StatusForbidden)
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if origin := r.Header.Get("Origin"); origin != "" && !trustedBrowserOrigin(origin) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return false
		}
		fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
		if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
			http.Error(w, "forbidden fetch site", http.StatusForbidden)
			return false
		}
	}
	return true
}

// RegisterPTYRoutes mounts all loopback-only PTY terminal HTTP/SSE endpoints on mux:
//   - POST   /api/pty/sessions
//   - GET    /api/pty/sessions
//   - POST   /api/pty/sessions/{id}/input
//   - POST   /api/pty/sessions/{id}/resize
//   - GET    /api/pty/sessions/{id}/stream
//   - DELETE /api/pty/sessions/{id}
func RegisterPTYRoutes(mux *http.ServeMux, manager *PTYManager) {
	if mux == nil || manager == nil {
		return
	}

	mux.HandleFunc("/api/pty/sessions", func(w http.ResponseWriter, r *http.Request) {
		if !ptyRequireLoopback(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"sessions": manager.ListSessions()})
		case http.MethodPost:
			var req struct {
				WorkspaceRoot string `json:"workspace_root"`
				Project       string `json:"project"`
				Cwd           string `json:"cwd"`
				Cols          int    `json:"cols"`
				Rows          int    `json:"rows"`
			}
			if err := readJSON(io.LimitReader(r.Body, 64*1024), &req); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
				return
			}
			ws := strings.TrimSpace(req.WorkspaceRoot)
			if ws == "" {
				ws = strings.TrimSpace(req.Project)
			}
			sess, err := manager.CreateSessionInDir(ws, req.Cwd, req.Cols, req.Rows)
			if err != nil {
				switch {
				case errors.Is(err, ErrPTYPathEscapesRoot):
					http.Error(w, err.Error(), http.StatusForbidden)
				case errors.Is(err, ErrMaxPTYSessions):
					http.Error(w, err.Error(), http.StatusTooManyRequests)
				default:
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = writeJSON(w, sess.Info())
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/pty/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if !ptyRequireLoopback(w, r) {
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/pty/sessions/")
		rest = strings.TrimSuffix(rest, "/")
		if rest == "" {
			http.Error(w, "missing session id", http.StatusNotFound)
			return
		}
		parts := strings.Split(rest, "/")
		id := parts[0]
		if !ptySessionIDPattern.MatchString(id) {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				sess, ok := manager.GetSession(id)
				if !ok {
					http.Error(w, ErrPTYSessionNotFound.Error(), http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = writeJSON(w, sess.Info())
			case http.MethodDelete:
				if err := manager.Close(id); err != nil {
					if errors.Is(err, ErrPTYSessionNotFound) {
						http.Error(w, err.Error(), http.StatusNotFound)
						return
					}
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = writeJSON(w, map[string]any{"ok": true, "id": id})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) != 2 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		action := parts[1]
		switch action {
		case "input":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Data     string `json:"data"`
				Encoding string `json:"encoding"`
			}
			if err := readJSON(io.LimitReader(r.Body, maxPTYInputBytes*2), &req); err != nil {
				http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
				return
			}
			var payload []byte
			if strings.EqualFold(strings.TrimSpace(req.Encoding), "base64") {
				decoded, err := base64.StdEncoding.DecodeString(req.Data)
				if err != nil {
					http.Error(w, "invalid base64 input", http.StatusBadRequest)
					return
				}
				payload = decoded
			} else {
				payload = []byte(req.Data)
			}
			if err := manager.Write(id, payload); err != nil {
				switch {
				case errors.Is(err, ErrPTYSessionNotFound):
					http.Error(w, err.Error(), http.StatusNotFound)
				case errors.Is(err, ErrPTYSessionClosed):
					http.Error(w, err.Error(), http.StatusGone)
				default:
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"ok": true, "bytes": len(payload)})

		case "resize":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			}
			if err := readJSON(io.LimitReader(r.Body, 4096), &req); err != nil {
				http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
				return
			}
			if err := manager.Resize(id, req.Cols, req.Rows); err != nil {
				switch {
				case errors.Is(err, ErrPTYSessionNotFound):
					http.Error(w, err.Error(), http.StatusNotFound)
				case errors.Is(err, ErrPTYSessionClosed):
					http.Error(w, err.Error(), http.StatusGone)
				default:
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			cols, rows := clampPTYSize(req.Cols, req.Rows)
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"ok": true, "cols": cols, "rows": rows})

		case "stream":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			events, unsub, err := manager.ReadEvents(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			defer unsub()

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming unsupported", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, ": connected\n\n")
			flusher.Flush()

			keepalive := time.NewTicker(15 * time.Second)
			defer keepalive.Stop()

			for {
				select {
				case <-r.Context().Done():
					return
				case <-keepalive.C:
					if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
						return
					}
					flusher.Flush()
				case ev, open := <-events:
					if !open {
						return
					}
					encoded, err := json.Marshal(ev)
					if err != nil {
						continue
					}
					if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Seq, encoded); err != nil {
						return
					}
					flusher.Flush()
					if ev.Type == "exit" {
						return
					}
				}
			}

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
}
