// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIDEStopTaskRequiresExactActiveIdentity(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{}`, 400}, {`{"thread_id":"t","run_id":"r","extra":1}`, 400},
		{`{"thread_id":"t","run_id":"r"} {}`, 400},
		{`{"thread_id":"wrong","run_id":"r"}`, 409},
		{`{"thread_id":"t","run_id":"old"}`, 409},
		{`{"thread_id":"t","run_id":"r"}`, 200},
		{strings.Repeat("x", 1025), 400},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		state := &AppState{Running: true, RunID: "r", CurrentThread: "t", Cancel: cancel}
		server := NewServer(state)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/stop-task", strings.NewReader(tc.body)))
		if rr.Code != tc.status || (ctx.Err() != nil) != (tc.status == 200) {
			t.Fatalf("body=%s status=%d cancelled=%v", tc.body, rr.Code, ctx.Err())
		}
		cancel()
	}
}

func TestIDETransportUsesDesktopSecurityBoundary(t *testing.T) {
	for _, route := range []string{"stop-task", "steer"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req := httptest.NewRequest(method, "http://127.0.0.1/api/"+route, strings.NewReader(`{}`))
			if method == http.MethodPost {
				req.Header.Set("Origin", "https://untrusted.example")
			}
			rr := httptest.NewRecorder()
			NewServer(&AppState{}).ServeHTTP(rr, req)
			want := 405
			if method == http.MethodPost {
				want = 403
			}
			if rr.Code != want {
				t.Fatalf("%s %s status=%d", method, route, rr.Code)
			}
		}
	}
}

func steeringTestState() *AppState {
	s := &AppState{Running: true, RunID: "r", CurrentThread: "t", Config: Config{Language: "en"}}
	s.beginAgentSteering("r")
	return s
}

func TestSteeringFIFOIdempotenceBoundsAndTerminalAdmission(t *testing.T) {
	s := steeringTestState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.setSteeringModelCancel("r", cancel)
	if err := s.queueAgentSteering("t", "r", "1", "first"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("active model was not interrupted")
	}
	if err := s.queueAgentSteering("t", "r", "1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := s.queueAgentSteering("t", "r", "1", "changed"); err == nil {
		t.Fatal("id collision accepted")
	}
	if err := s.queueAgentSteering("t", "r", "2", "second"); err != nil {
		t.Fatal(err)
	}
	if s.admitActionAfterSteering("r", true) {
		t.Fatal("finish overtook pending instructions")
	}
	items := s.drainAgentSteering("r")
	if len(items) != 2 || items[0].Message != "first" || items[1].Message != "second" {
		t.Fatalf("items=%v", items)
	}
	if !s.admitActionAfterSteering("r", true) {
		t.Fatal("terminal admission blocked")
	}
	if err := s.queueAgentSteering("t", "r", "3", "too late"); err == nil {
		t.Fatal("late instruction accepted")
	}
	for _, tc := range []struct{ thread, run, id, message string }{{"other", "r", "a", "x"}, {"t", "old", "a", "x"}, {"t", "r", "", "x"}, {"t", "r", "a", " "}, {"t", "r", "a", strings.Repeat("x", 32769)}} {
		if err := steeringTestState().queueAgentSteering(tc.thread, tc.run, tc.id, tc.message); err == nil {
			t.Fatalf("invalid accepted: %#v", tc)
		}
	}
	s = steeringTestState()
	for i := 0; i < 16; i++ {
		if err := s.queueAgentSteering("t", "r", string(rune('a'+i)), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.queueAgentSteering("t", "r", "overflow", "x"); err == nil {
		t.Fatal("queue overflow accepted")
	}
	s = steeringTestState()
	for i := 0; i < 4; i++ {
		if err := s.queueAgentSteering("t", "r", string(rune('a'+i)), strings.Repeat("x", 32768)); err != nil {
			t.Fatal(err)
		}
		s.drainAgentSteering("r")
	}
	if err := s.queueAgentSteering("t", "r", "overflow", "x"); err == nil {
		t.Fatal("cumulative byte budget reset by drain")
	}
}

func TestSteeringConcurrentFinishHasOneWinner(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := steeringTestState()
		var wg sync.WaitGroup
		wg.Add(2)
		var accepted, finished bool
		go func() { defer wg.Done(); accepted = s.queueAgentSteering("t", "r", "1", "correction") == nil }()
		go func() { defer wg.Done(); finished = s.admitActionAfterSteering("r", true) }()
		wg.Wait()
		if accepted == finished {
			t.Fatalf("accepted=%v finished=%v", accepted, finished)
		}
	}
}

func TestSteeringRejectsStaleApprovalWithoutGrantingAuthority(t *testing.T) {
	s := steeringTestState()
	s.Pending = &PendingAction{ID: "pending", Result: make(chan ApprovalDecision, 1)}
	if err := s.queueAgentSteering("t", "r", "correction", "Do not modify that file."); err != nil {
		t.Fatal(err)
	}
	select {
	case decision := <-s.Pending.Result:
		if decision.Approved || decision.Persist {
			t.Fatal("steering granted authority")
		}
	default:
		t.Fatal("stale approval wait was not released")
	}
	if s.admitActionAfterSteering("r", false) {
		t.Fatal("stale tool admitted before steering")
	}
}

func TestSteeringTransportStrictRequestAndRemoteIsolation(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	for _, route := range []string{"/remote/api/steer", "/remote/api/stop-task"} {
		rr := httptest.NewRecorder()
		NewRemoteServer(&AppState{}).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{}`)))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("remote route %s returned %d", route, rr.Code)
		}
		rr = httptest.NewRecorder()
		NewRemoteServer(&AppState{}).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, route, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("remote GET route %s returned %d", route, rr.Code)
		}
	}
	for _, body := range []string{`{}`, `{"thread_id":"t","run_id":"r","id":"1","message":"hello","extra":1}`, `{"thread_id":"t","run_id":"r","id":"1","message":"hello"} {}`} {
		rr := httptest.NewRecorder()
		NewServer(steeringTestState()).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/steer", strings.NewReader(body)))
		if rr.Code < 400 {
			t.Fatalf("invalid body accepted %s", body)
		}
	}
	rr := httptest.NewRecorder()
	NewServer(steeringTestState()).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/steer", strings.NewReader(`{"thread_id":"t","run_id":"r","id":"1","message":"hello"}`)))
	if rr.Code != 202 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSteeringInterruptsModelAndSupersedesItsAction(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALCODE_CACHE_HOME", t.TempDir())
	started := make(chan struct{})
	observed := make(chan string, 1)
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model"}}})
			return
		}
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Messages []OllamaMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		found := ""
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "PRIORITY USER UPDATE") {
				found = m.Content
			}
		}
		if found == "" {
			once.Do(func() { close(started) })
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		observed <- found
		_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"role": "assistant", "content": `{"action":"finish","message":"updated result"}`}, "done": true})
	}))
	defer server.Close()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.Language = "en"
	cfg.MaxAgentSteps = 5
	cfg.ModelTimeout = 15
	cfg.ContextCompactionEnabled = false
	cfg.CreateProjectDocs = false
	s := steeringTestState()
	s.Config = cfg
	s.Ollama = client
	s.Project = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan string, 1)
	go func() {
		done <- s.executeAgentLoop(ctx, "r", s.Project, "test-model", []OllamaMessage{{Role: "user", Content: "Say hello"}}, cfg, "", "Say hello")
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("model not started")
	}
	if err := s.queueAgentSteering("t", "r", "update", "Use the name Ada instead."); err != nil {
		t.Fatal(err)
	}
	select {
	case content := <-observed:
		if !strings.Contains(content, "Ada") {
			t.Fatal(content)
		}
	case <-ctx.Done():
		t.Fatal("new instruction not observed")
	}
	select {
	case outcome := <-done:
		if outcome != "done" {
			t.Fatal(outcome)
		}
	case <-ctx.Done():
		t.Fatal("loop did not finish")
	}
}
