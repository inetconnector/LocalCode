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

func TestModelSubagentValidationRequiresInputs(t *testing.T) {
	cases := []AgentAction{
		{Action: "read_file"},
		{Action: "search_text"},
		{Action: "finish"},
	}
	for _, action := range cases {
		if err := validateModelSubagentAction(action); err == nil {
			t.Fatalf("missing input unexpectedly accepted: %#v", action)
		}
	}
	for _, action := range []AgentAction{
		{Action: "list_files"},
		{Action: "search_text", Query: "Important"},
		{Action: "finish", Message: "handoff"},
	} {
		if err := validateModelSubagentAction(action); err != nil {
			t.Fatalf("valid read-only action rejected: %#v: %v", action, err)
		}
	}
}

func TestExecuteModelSubagentReadOnlyActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n\nfunc Important() int { return 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)

	listed, err := state.executeModelSubagentAction(context.Background(), root, cfg, AgentAction{Action: "list_files", MaxDepth: 2})
	if err != nil || !strings.Contains(listed, "main.go") {
		t.Fatalf("list_files failed: %v\n%s", err, listed)
	}
	read, err := state.executeModelSubagentAction(context.Background(), root, cfg, AgentAction{Action: "read_file", Path: "main.go"})
	if err != nil || !strings.Contains(read, "Important") {
		t.Fatalf("read_file failed: %v\n%s", err, read)
	}
	searched, err := state.executeModelSubagentAction(context.Background(), root, cfg, AgentAction{Action: "search_text", Query: "Important"})
	if err != nil || !strings.Contains(searched, "main.go") {
		t.Fatalf("search_text failed: %v\n%s", err, searched)
	}
	if _, err := state.executeModelSubagentAction(context.Background(), root, cfg, AgentAction{Action: "write_file"}); err == nil {
		t.Fatal("unsupported action unexpectedly executed")
	}
}

func TestMandatoryReliabilityPreflightDoesNotConsumeModelTurns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n\nfunc Important() int { return 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "deterministic preflight must not call model", http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	ollama := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	state := NewAppState(cfg, ollama)
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Model = "fake-local-model"
	state.mu.Unlock()

	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, deterministicSubagentTaskPrefix+"inspect Important")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("mandatory reliability preflight consumed %d model calls", calls.Load())
	}
	if !strings.Contains(result, "READ-ONLY SUBAGENT HANDOFF") || !strings.Contains(result, "Important") {
		t.Fatalf("deterministic preflight handoff missing expected evidence:\n%s", result)
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
