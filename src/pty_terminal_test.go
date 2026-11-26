// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPTYSessionRealLifecycleEchoResizeClose(t *testing.T) {
	workspace := t.TempDir()
	mgr := NewPTYManager(workspace)
	defer mgr.CloseAll()

	sess, err := mgr.CreateSession(workspace, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	info := sess.Info()
	if info.Cols != 80 || info.Rows != 24 {
		t.Fatalf("expected initial size 80x24, got %dx%d", info.Cols, info.Rows)
	}
	if runtime.GOOS == "windows" && info.Backend != "conpty" {
		t.Logf("note: running on windows with backend=%q", info.Backend)
	}

	events, unsub, err := mgr.ReadEvents(sess.ID)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}
	defer unsub()

	if err := mgr.Resize(sess.ID, 120, 40); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	afterResize := sess.Info()
	if afterResize.Cols != 120 || afterResize.Rows != 40 {
		t.Fatalf("expected resized dimensions 120x40, got %dx%d", afterResize.Cols, afterResize.Rows)
	}

	if err := mgr.Write(sess.ID, []byte("echo NOMAMICA_PTY_OK\r\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var combined strings.Builder
	sawMarker := false
	sawResize := false
	deadline := time.After(10 * time.Second)
	for !sawMarker || !sawResize {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed before seeing marker (sawMarker=%v, sawResize=%v, output=%q)", sawMarker, sawResize, combined.String())
			}
			if ev.Type == "resize" && ev.Cols == 120 && ev.Rows == 40 {
				sawResize = true
			}
			if ev.Type == "output" {
				combined.WriteString(ev.Data)
				if strings.Contains(combined.String(), "NOMAMICA_PTY_OK") {
					sawMarker = true
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for PTY output marker (sawMarker=%v, sawResize=%v, output=%q)", sawMarker, sawResize, combined.String())
		}
	}

	if err := mgr.Close(sess.ID); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, ok := mgr.GetSession(sess.ID); ok {
		t.Fatalf("session %s still registered after Close", sess.ID)
	}
	if err := mgr.Write(sess.ID, []byte("echo after\r\n")); !errors.Is(err, ErrPTYSessionNotFound) {
		t.Fatalf("expected ErrPTYSessionNotFound after Close, got %v", err)
	}
}

func TestPTYPathEscapeBlocked(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "workspace")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	mgr := NewPTYManager(allowed)
	defer mgr.CloseAll()

	// Relative traversal escape
	if _, err := mgr.CreateSessionInDir(allowed, filepath.Join("..", "outside"), 80, 24); !errors.Is(err, ErrPTYPathEscapesRoot) {
		t.Fatalf("expected ErrPTYPathEscapesRoot for relative traversal, got %v", err)
	}
	// Absolute path outside root
	if _, err := mgr.CreateSession(outside, 80, 24); !errors.Is(err, ErrPTYPathEscapesRoot) {
		t.Fatalf("expected ErrPTYPathEscapesRoot for outside absolute path, got %v", err)
	}

	// Verify HTTP route returns 403 Forbidden for escape attempts
	mux := http.NewServeMux()
	RegisterPTYRoutes(mux, mgr)

	body := bytes.NewBufferString(`{"workspace_root":"` + filepath.ToSlash(outside) + `","cols":80,"rows":24}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/pty/sessions", body)
	req.Host = "127.0.0.1:32145"
	req.RemoteAddr = "127.0.0.1:51234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 for outside workspace_root, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPTYRoutesEnforceStrictLoopback(t *testing.T) {
	workspace := t.TempDir()
	mgr := NewPTYManager(workspace)
	defer mgr.CloseAll()

	mux := http.NewServeMux()
	RegisterPTYRoutes(mux, mgr)

	// Non-loopback Host
	req1 := httptest.NewRequest(http.MethodGet, "http://evil.example/api/pty/sessions", nil)
	req1.Host = "evil.example"
	req1.RemoteAddr = "127.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback Host, got %d", rec1.Code)
	}

	// Non-loopback RemoteAddr
	req2 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/pty/sessions", nil)
	req2.Host = "127.0.0.1:32145"
	req2.RemoteAddr = "192.168.1.44:54321"
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback RemoteAddr, got %d", rec2.Code)
	}

	// Cross-origin POST
	req3 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/pty/sessions", strings.NewReader(`{}`))
	req3.Host = "127.0.0.1:32145"
	req3.RemoteAddr = "127.0.0.1:12345"
	req3.Header.Set("Origin", "https://attacker.example")
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for untrusted Origin, got %d", rec3.Code)
	}
}

type fakePTYBackend struct {
	mu        sync.Mutex
	pr        *io.PipeReader
	pw        *io.PipeWriter
	cols      int
	rows      int
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
}

func newFakePTYBackend(cols, rows int) *fakePTYBackend {
	pr, pw := io.Pipe()
	return &fakePTYBackend{pr: pr, pw: pw, cols: cols, rows: rows, done: make(chan struct{})}
}

func (f *fakePTYBackend) BackendName() string { return "fake" }
func (f *fakePTYBackend) Read(p []byte) (int, error) {
	return f.pr.Read(p)
}
func (f *fakePTYBackend) Write(p []byte) (int, error) {
	return f.pw.Write(p)
}
func (f *fakePTYBackend) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return ErrPTYSessionClosed
	}
	f.cols = cols
	f.rows = rows
	return nil
}
func (f *fakePTYBackend) Wait() (int, error) {
	<-f.done
	return 0, nil
}
func (f *fakePTYBackend) Close() error {
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
		_ = f.pw.Close()
		_ = f.pr.Close()
		close(f.done)
	})
	return nil
}

func TestPTYManagerMaxSessionsAndIdleEviction(t *testing.T) {
	workspace := t.TempDir()
	mgr := NewPTYManager(workspace)
	defer mgr.CloseAll()
	mgr.IdleTimeout = 50 * time.Millisecond
	mgr.startFunc = func(_ string, cols, rows int) (ptyBackend, error) {
		return newFakePTYBackend(cols, rows), nil
	}

	if mgr.maxSessionsLimit() != DefaultMaxPTYSessions || DefaultMaxPTYSessions != 8 {
		t.Fatalf("expected default max sessions = 8, got %d", mgr.maxSessionsLimit())
	}

	var ids []string
	for i := 0; i < DefaultMaxPTYSessions; i++ {
		sess, err := mgr.CreateSession(workspace, 80, 24)
		if err != nil {
			t.Fatalf("CreateSession #%d failed: %v", i+1, err)
		}
		ids = append(ids, sess.ID)
	}

	// 9th session must be rejected
	if _, err := mgr.CreateSession(workspace, 80, 24); !errors.Is(err, ErrMaxPTYSessions) {
		t.Fatalf("expected ErrMaxPTYSessions on 9th session, got %v", err)
	}

	// Closing one frees a slot
	if err := mgr.Close(ids[0]); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	sess, err := mgr.CreateSession(workspace, 80, 24)
	if err != nil {
		t.Fatalf("expected session creation to succeed after closing one, got %v", err)
	}
	ids = append(ids[1:], sess.ID)

	// Idle eviction closes sessions past IdleTimeout
	evicted := mgr.EvictIdle(time.Now().UTC().Add(time.Second))
	if evicted != len(ids) {
		t.Fatalf("expected %d evicted sessions, got %d", len(ids), evicted)
	}
	if len(mgr.ListSessions()) != 0 {
		t.Fatalf("expected 0 active sessions after idle eviction, got %d", len(mgr.ListSessions()))
	}
}

func TestPTYHTTPAndSSEEndToEnd(t *testing.T) {
	workspace := t.TempDir()
	mgr := NewPTYManager(workspace)
	defer mgr.CloseAll()

	mux := http.NewServeMux()
	RegisterPTYRoutes(mux, mgr)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Create session via POST /api/pty/sessions
	createBody, _ := json.Marshal(map[string]any{
		"workspace_root": workspace,
		"cols":           80,
		"rows":           24,
	})
	resp, err := http.Post(srv.URL+"/api/pty/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/pty/sessions failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(raw))
	}
	var created PTYSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode session info: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// 2. Open SSE stream
	streamCtx, cancelStream := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancelStream()
	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/pty/sessions/"+created.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("GET stream failed: %v", err)
	}
	defer streamResp.Body.Close()
	if !strings.HasPrefix(streamResp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", streamResp.Header.Get("Content-Type"))
	}

	// 3. Resize via POST /api/pty/sessions/{id}/resize
	resizeResp, err := http.Post(srv.URL+"/api/pty/sessions/"+created.ID+"/resize", "application/json", strings.NewReader(`{"cols":100,"rows":30}`))
	if err != nil {
		t.Fatalf("POST resize failed: %v", err)
	}
	resizeResp.Body.Close()
	if resizeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on resize, got %d", resizeResp.StatusCode)
	}

	// 4. Write command via POST /api/pty/sessions/{id}/input
	inputResp, err := http.Post(srv.URL+"/api/pty/sessions/"+created.ID+"/input", "application/json", strings.NewReader(`{"data":"echo NOMAMICA_PTY_OK\r\n"}`))
	if err != nil {
		t.Fatalf("POST input failed: %v", err)
	}
	inputResp.Body.Close()
	if inputResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on input, got %d", inputResp.StatusCode)
	}

	// 5. Read SSE events until marker appears
	scanner := bufio.NewScanner(streamResp.Body)
	var output strings.Builder
	foundMarker := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var ev PTYEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			continue
		}
		if ev.Type == "output" {
			output.WriteString(ev.Data)
			if strings.Contains(output.String(), "NOMAMICA_PTY_OK") {
				foundMarker = true
				break
			}
		}
	}
	if !foundMarker {
		t.Fatalf("did not receive NOMAMICA_PTY_OK over SSE stream (err=%v, output=%q)", scanner.Err(), output.String())
	}

	// 6. Delete session via DELETE /api/pty/sessions/{id}
	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/pty/sessions/"+created.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE session failed: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on DELETE, got %d", delResp.StatusCode)
	}
}
