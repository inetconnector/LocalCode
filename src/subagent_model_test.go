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

func TestNativeChildCapabilitySetRejectsMutationAndExternalActions(t *testing.T) {
	for _, action := range []nativeChildAction{
		{Action: "write_file", Path: "x.txt"},
		{Action: "delete_file", Path: "x.txt"},
		{Action: "run_command"},
		{Action: "git"},
		{Action: "web_search"},
		{Action: "mcp_call_tool"},
		{Action: "subagent_analyze"},
	} {
		if err := validateNativeChildAction(action); err == nil {
			t.Fatalf("non-read-only child action unexpectedly allowed: %#v", action)
		}
	}
	if err := validateNativeChildAction(nativeChildAction{Action: "read_file", Path: "x.txt"}); err != nil {
		t.Fatalf("read_file should be allowed: %v", err)
	}
}

func TestNativeChildValidationRequiresInputsAndStructuredFinish(t *testing.T) {
	for _, action := range []nativeChildAction{
		{Action: "read_file"},
		{Action: "search_text"},
		{Action: "finish", Message: "done"},
	} {
		if err := validateNativeChildAction(action); err == nil {
			t.Fatalf("missing input unexpectedly accepted: %#v", action)
		}
	}
	valid := []nativeChildAction{
		{Action: "list_files"},
		{Action: "search_text", Query: "Important"},
		{Action: "finish", Message: "done", Result: &AgentResult{Summary: "handoff"}},
	}
	for _, action := range valid {
		if err := validateNativeChildAction(action); err != nil {
			t.Fatalf("valid child action rejected: %#v: %v", action, err)
		}
	}
}

func TestAgentRolesAndBudgetsAreBounded(t *testing.T) {
	cfg := defaultConfig()
	for input, want := range map[string]AgentRole{"": AgentRoleExplorer, "explore": AgentRoleExplorer, "planner": AgentRolePlanner, "review": AgentRoleReviewer, "builder": AgentRoleBuilder} {
		got, err := normalizeAgentRole(input)
		if err != nil || got != want {
			t.Fatalf("normalizeAgentRole(%q)=%q,%v want %q", input, got, err, want)
		}
	}
	if _, err := normalizeReadOnlyMissionRole("builder"); err == nil {
		t.Fatal("mutation-capable builder role must not execute in a read-only mission")
	}
	budget := normalizeAgentBudget(AgentBudget{ModelCalls: 999, ToolCalls: 999, EstimatedTokenBudget: 1 << 40, TimeSeconds: 9999}, AgentRoleExplorer, cfg)
	defaults := defaultAgentBudget(AgentRoleExplorer, cfg)
	if budget != defaults {
		t.Fatalf("oversized budget was not clamped: %#v want %#v", budget, defaults)
	}
	caps := capabilitiesForAgentRole(AgentRoleReviewer)
	if len(caps) != 3 || caps[0] != AgentCapabilityRepositoryRead || caps[1] != AgentCapabilityLSP || caps[2] != AgentCapabilityReview {
		t.Fatalf("unexpected reviewer capabilities: %#v", caps)
	}
}

func TestExecuteNativeChildReadOnlyActions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n\nfunc Important() int { return 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)

	listed, err := state.executeNativeChildAction(context.Background(), root, cfg, nativeChildAction{Action: "list_files", MaxDepth: 2})
	if err != nil || !strings.Contains(listed, "main.go") {
		t.Fatalf("list_files failed: %v\n%s", err, listed)
	}
	read, err := state.executeNativeChildAction(context.Background(), root, cfg, nativeChildAction{Action: "read_file", Path: "main.go"})
	if err != nil || !strings.Contains(read, "Important") {
		t.Fatalf("read_file failed: %v\n%s", err, read)
	}
	searched, err := state.executeNativeChildAction(context.Background(), root, cfg, nativeChildAction{Action: "search_text", Query: "Important"})
	if err != nil || !strings.Contains(searched, "main.go") {
		t.Fatalf("search_text failed: %v\n%s", err, searched)
	}
	if _, err := state.executeNativeChildAction(context.Background(), root, cfg, nativeChildAction{Action: "write_file"}); err == nil {
		t.Fatal("unsupported child action unexpectedly executed")
	}
}

func TestMandatoryReliabilityPreflightRemainsDeterministic(t *testing.T) {
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

	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, deterministicSubagentTaskPrefix+"inspect Important", "explorer")
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

func TestModelExplorerUsesBoundedStructuredReadOnlyLoop(t *testing.T) {
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
		action := `{"action":"read_file","message":"Inspect implementation","path":"main.go"}`
		if step > 1 {
			action = `{"action":"finish","message":"Return independent evidence","result":{"summary":"Important is implemented in main.go and should preserve its return contract.","findings":[{"category":"symbol","summary":"Important returns 42","path":"main.go","symbol":"Important"}],"tests":[{"name":"go test ./...","status":"recommended"}],"risks":[{"severity":"low","summary":"Changing the return contract may affect callers."}]}}`
		}
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Role: "assistant", Content: action}, Done: true})
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

	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, "inspect Important and identify change risks", "explorer")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls=%d want 2", calls.Load())
	}
	for _, marker := range []string{"STRUCTURED CHILD AGENT RESULT", `"role": "explorer"`, `"status": "completed"`, "Important", `"model_calls": 2`} {
		if !strings.Contains(result, marker) {
			t.Fatalf("structured handoff missing %q:\n%s", marker, result)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("child agent changed project file: %q", data)
	}
}

func TestPlannerCanReturnStructuredTaskProposalsWithoutExecutingThem(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := `{"action":"finish","message":"Plan ready","result":{"summary":"Split implementation and verification.","suggested_tasks":[{"id":"build","role":"builder","objective":"Implement the narrow code change","dependencies":[],"capabilities":["repository-read"]},{"id":"review","role":"reviewer","objective":"Review the resulting diff","dependencies":["build"]}]}}`
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Role: "assistant", Content: action}, Done: true})
	}))
	defer server.Close()
	cfg := defaultConfig()
	ollama := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	state := NewAppState(cfg, ollama)
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Model = "fake-local-model"
	state.mu.Unlock()

	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, "plan a safe change", "planner")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`"role": "planner"`, `"suggested_tasks"`, `"id": "build"`, `"task_graph"`, `"state": "ready"`, `"requested_capabilities"`, "Review the resulting diff"} {
		if !strings.Contains(result, marker) {
			t.Fatalf("planner result missing %q:\n%s", marker, result)
		}
	}
}

func TestNativeChildHardModelBudgetStopsFurtherInference(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		action := `{"action":"list_files","message":"Inspect tree","max_depth":2}`
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Role: "assistant", Content: action}, Done: true})
	}))
	defer server.Close()
	cfg := defaultConfig()
	ollama := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	state := NewAppState(cfg, ollama)
	t.Cleanup(state.Close)
	task, err := newReadOnlyAgentTask(root, "fake-local-model", "inspect repository", "explorer", cfg)
	if err != nil {
		t.Fatal(err)
	}
	task.Budget = AgentBudget{ModelCalls: 1, ToolCalls: 1, EstimatedTokenBudget: 100000, TimeSeconds: 60}
	result, err := state.runNativeReadOnlyAgentTask(context.Background(), root, cfg, task)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("model calls=%d want hard limit 1", calls.Load())
	}
	if result.Status != AgentResultBudgetExhausted {
		t.Fatalf("status=%q want %q; result=%#v", result.Status, AgentResultBudgetExhausted, result)
	}
	if result.Usage.ModelCalls != 1 || result.Usage.ToolCalls != 1 {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
	if len(result.Findings) == 0 || result.Findings[0].Category != "deterministic-fallback" {
		t.Fatalf("budget exhaustion must return deterministic evidence: %#v", result)
	}
}
func TestPlannerRejectsInvalidTaskGraphAndRequestsCorrection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := calls.Add(1)
		action := `{"action":"finish","message":"Plan ready","result":{"summary":"Plan.","suggested_tasks":[{"id":"a","role":"builder","objective":"A","dependencies":["missing"]}]}}`
		if step > 1 {
			action = `{"action":"finish","message":"Corrected plan","result":{"summary":"Plan.","suggested_tasks":[{"id":"a","role":"builder","objective":"A"},{"id":"b","role":"reviewer","objective":"B","dependencies":["a"]}]}}`
		}
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Role: "assistant", Content: action}, Done: true})
	}))
	defer server.Close()
	cfg := defaultConfig()
	ollama := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	state := NewAppState(cfg, ollama)
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Model = "fake-local-model"
	state.mu.Unlock()
	result, err := state.runReadOnlyModelSubagent(context.Background(), root, cfg, "plan a safe change", "planner")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls=%d want 2 after invalid graph correction", calls.Load())
	}
	for _, marker := range []string{`"task_graph"`, `"id": "a"`, `"id": "b"`, `"dependencies": [`, `"a"`} {
		if !strings.Contains(result, marker) {
			t.Fatalf("corrected planner handoff missing %q:\n%s", marker, result)
		}
	}
}
