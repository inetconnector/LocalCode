// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAutomaticParallelPreflightSwarmRunsConcurrentlyAndPopulatesRoster(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html><body><canvas id=\"gameCanvas\"></canvas><script src=\"game.js\"></script></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.js"), []byte("window.startGame = function() { return true; };\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var syncMu sync.Mutex
	var syncedBody map[string]any
	syncServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/swarm/sync" {
			data, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			if json.Unmarshal(data, &parsed) == nil {
				syncMu.Lock()
				syncedBody = parsed
				syncMu.Unlock()
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer syncServer.Close()
	t.Setenv("AGENTOS_URL", syncServer.URL)

	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = dir

	brief := state.runAutomaticParallelPreflightSwarm(context.Background(), dir, "Erstelle ein vollstaendiges spielbares Spiel", "test-model", cfg)
	if !strings.Contains(brief, "[ANTIGRAVITY 2.0 · PARALLEL SUBAGENT SWARM WAVE 1]") {
		t.Fatalf("expected Wave 1 swarm header in brief, got:\n%s", brief)
	}

	state.mu.RLock()
	wave, active, agents := state.snapshotSwarmRosterLocked()
	state.mu.RUnlock()

	if wave != 1 {
		t.Fatalf("expected wave 1, got %d", wave)
	}
	if active != 0 {
		t.Fatalf("expected 0 running subagents after wave completion, got %d", active)
	}
	if len(agents) != 3 {
		t.Fatalf("expected 3 parallel Wave 1 subagents, got %d", len(agents))
	}
	names := map[string]bool{}
	for _, a := range agents {
		names[a.Name] = true
		if a.Status != "completed" {
			t.Fatalf("expected subagent %s status completed, got %s", a.Name, a.Status)
		}
	}
	for _, want := range []string{"@explorer", "@architect", "@qa-planner"} {
		if !names[want] {
			t.Fatalf("expected subagent %s in roster, got %#v", want, agents)
		}
	}
	syncMu.Lock()
	_ = syncedBody
	syncMu.Unlock()
}

func TestAutomaticParallelVerifierSwarmDetectsViolationsAndPassesClean(t *testing.T) {
	dir := t.TempDir()
	badTest := filepath.Join(dir, "test.js")
	if err := os.WriteFile(badTest, []byte("const fn = jest.fn();\njest.mock('fs');\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = dir
	t.Setenv("AGENTOS_URL", "http://127.0.0.1:1")

	issues := state.runAutomaticParallelVerifierSwarm(context.Background(), dir, "test-model", cfg, map[string]bool{"test.js": true})
	if len(issues) == 0 {
		t.Fatalf("expected Wave 3 verifier swarm to catch offline Jest violation")
	}

	cleanTest := "const test = require('node:test');\nconst assert = require('node:assert/strict');\ntest('ok', () => assert.equal(1 + 1, 2));\n"
	if err := os.WriteFile(badTest, []byte(cleanTest), 0o644); err != nil {
		t.Fatal(err)
	}
	issues = state.runAutomaticParallelVerifierSwarm(context.Background(), dir, "test-model", cfg, map[string]bool{"test.js": true})
	if len(issues) != 0 {
		t.Fatalf("expected Wave 3 verifier swarm to pass clean file, got issues: %#v", issues)
	}
}

func TestExecuteSpawnSubagentsActionFansOutWorkersInParallel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("function init() { return 42; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = dir
	t.Setenv("AGENTOS_URL", "http://127.0.0.1:1")

	out, err := state.executeSpawnSubagentsAction(context.Background(), dir, "test-model", cfg, AgentAction{
		Action: "spawn_subagents",
		Args:   []string{"Analysiere Architektur", "Pruefe Test-Setup"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "PARALLEL SUBAGENT SWARM RESULTS (2 workers)") {
		t.Fatalf("expected 2 parallel workers output, got:\n%s", out)
	}
	if !strings.Contains(out, "@worker-1") || !strings.Contains(out, "@worker-2") {
		t.Fatalf("expected @worker-1 and @worker-2 sections, got:\n%s", out)
	}
}

func TestNomaSupermodelHeaderAndBodyToggle(t *testing.T) {
	var mu sync.Mutex
	var gotHeader string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		mu.Lock()
		gotHeader = r.Header.Get("X-Noma-Supermodel")
		gotBody = parsed
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"action\":\"finish\",\"message\":\"ok\"}"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	client := NewOllamaClient()
	client.BaseURL = srv.URL
	client.NomaSupermodelEnabled = false
	if _, err := client.ChatStream(context.Background(), "noma-mica", []OllamaMessage{{Role: "user", Content: "ping"}}, nil, nil); err != nil {
		t.Fatalf("ChatStream (disabled) failed: %v", err)
	}
	mu.Lock()
	h0, b0 := gotHeader, gotBody
	mu.Unlock()
	if h0 != "0" {
		t.Fatalf("expected X-Noma-Supermodel=0 when disabled, got %q", h0)
	}
	if sm, ok := b0["supermodel"].(bool); !ok || sm {
		t.Fatalf("expected body supermodel=false when disabled, got %#v", b0["supermodel"])
	}

	client.NomaSupermodelEnabled = true
	if _, err := client.ChatStream(context.Background(), "noma-mica", []OllamaMessage{{Role: "user", Content: "ping"}}, nil, nil); err != nil {
		t.Fatalf("ChatStream (enabled) failed: %v", err)
	}
	mu.Lock()
	h1, b1 := gotHeader, gotBody
	mu.Unlock()
	if h1 != "1" {
		t.Fatalf("expected X-Noma-Supermodel=1 when enabled, got %q", h1)
	}
	if sm, ok := b1["supermodel"].(bool); !ok || !sm {
		t.Fatalf("expected body supermodel=true when enabled, got %#v", b1["supermodel"])
	}
}
