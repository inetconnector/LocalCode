// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelSubagentCapabilitySetRejectsMutations(t *testing.T) {
	for _, action := range []AgentAction{
		{Action: "write_file", Path: "x.txt", Content: "bad"},
		{Action: "delete_file", Path: "x.txt"},
		{Action: "run_command", Command: "echo bad"},
		{Action: "git", Args: []string{"status"}},
		{Action: "web_search", Query: "bad"},
	} {
		if err := validateModelSubagentAction(action); err == nil {
			t.Fatalf("mutation/non-read-only action unexpectedly allowed: %#v", action)
		}
	}
	if err := validateModelSubagentAction(AgentAction{Action: "read_file", Path: "x.txt"}); err != nil {
		t.Fatalf("read_file should be allowed: %v", err)
	}
}

func TestModelSubagentUsesBoundedReadOnlyToolLoop(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	original := "package demo\n\nfunc Important() int { return 42 }\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		step := calls.Add(1)
		var action string
		switch step {
		case 1:
			action = `{"action":"read_file","message":"Inspect the implementation","path":"main.go"}`
		default:
			action = `{"action":"finish","message":"Relevant file: main.go; symbol: Important; preserve return contract; verify with targeted Go tests."}`
		}
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{
			Message: OllamaMessage{Role: "assistant", Content: action},
			Done:    true,
		})
	}))
	defer server.Close()

	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	cfg.ApprovalMode = "strict"
	ollama := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	state := NewAppState(cfg, ollama)
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Model = "fake-local-model"
	state.mu.Unlock()

	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, "inspect Important and identify change risks")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls=%d want 2", calls.Load())
	}
	for _, marker := range []string{"MODEL READ-ONLY SUBAGENT HANDOFF", "main.go", "Important", "READ-ONLY TRACE", "read_file"} {
		if !strings.Contains(result, marker) {
			t.Fatalf("handoff missing %q:\n%s", marker, result)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("subagent changed project file: %q", data)
	}
}
