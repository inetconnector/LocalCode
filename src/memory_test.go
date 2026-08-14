// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryRememberListForget(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	cfg := defaultConfig()
	state := &AppState{Config: cfg}

	stored, err := state.executeMemoryAction(project, AgentAction{Action: "memory_remember", Content: "Use concise German summaries for this project.", Scope: "project"})
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if !strings.Contains(stored, "MEMORY STORED") {
		t.Fatalf("unexpected remember result: %s", stored)
	}
	if len(state.Config.Memories) != 1 {
		t.Fatalf("expected one memory, got %d", len(state.Config.Memories))
	}
	id := state.Config.Memories[0].ID

	listed, err := state.executeMemoryAction(project, AgentAction{Action: "memory_list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(listed, id) || !strings.Contains(listed, "concise German") {
		t.Fatalf("memory not listed: %s", listed)
	}

	deleted, err := state.executeMemoryAction(project, AgentAction{Action: "memory_forget", MemoryID: id})
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}
	if !strings.Contains(deleted, "MEMORY DELETED") {
		t.Fatalf("unexpected delete result: %s", deleted)
	}
	if len(state.Config.Memories) != 0 {
		t.Fatalf("expected no memories after delete, got %d", len(state.Config.Memories))
	}
}

func TestMemoryContextScopesAndFiltering(t *testing.T) {
	project := t.TempDir()
	otherProject := t.TempDir()
	nowCfg := defaultConfig()
	if _, err := rememberInConfig(&nowCfg, project, "Project build command is go test ./...", "project"); err != nil {
		t.Fatal(err)
	}
	if _, err := rememberInConfig(&nowCfg, otherProject, "Other project uses npm test.", "project"); err != nil {
		t.Fatal(err)
	}
	if _, err := rememberInConfig(&nowCfg, project, "Prefer direct root-cause fixes.", "global"); err != nil {
		t.Fatal(err)
	}

	context := memoryContextForAgent(nowCfg, project)
	if !strings.Contains(context, "Project build command") {
		t.Fatalf("project memory missing from context: %s", context)
	}
	if !strings.Contains(context, "Prefer direct root-cause fixes") {
		t.Fatalf("global memory missing from context: %s", context)
	}
	if strings.Contains(context, "Other project uses npm test") {
		t.Fatalf("other project memory leaked into context: %s", context)
	}

	filtered := filterMemories(nowCfg, project, "project", "build")
	if len(filtered) != 1 || !strings.Contains(filtered[0].Content, "build command") {
		t.Fatalf("unexpected filtered memories: %s", memoriesJSON(filtered))
	}
}

func TestMemoryRejectsSecrets(t *testing.T) {
	cfg := defaultConfig()
	if _, err := rememberInConfig(&cfg, t.TempDir(), "api_key = abc123", "project"); err == nil {
		t.Fatal("expected secret-like memory to be rejected")
	}
	if len(cfg.Memories) != 0 {
		t.Fatalf("secret-like memory was stored: %s", memoriesJSON(cfg.Memories))
	}
}

func TestParseAgentActionMemoryArguments(t *testing.T) {
	a, err := parseAgentAction(`{"action":"memory_forget","message":"forget","arguments":{"memory_id":"abc123"}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.MemoryID != "abc123" {
		t.Fatalf("memory_id was not normalized from arguments: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"memory_remember","message":"remember"}`); err == nil {
		t.Fatal("memory_remember without content must be rejected")
	}
}

func TestDirectMemoryPromptStoresGlobalWithoutModel(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	state := NewAppState(defaultConfig(), nil)
	state.Project = project
	state.Model = "local-model"

	err := state.StartAgentForThread("Mache immer Ursachen statt Symptome beheben, merke dir das.", "local-model", nil, project, "")
	if err != nil {
		t.Fatal(err)
	}
	if state.Running {
		t.Fatal("direct memory request must not start a model run")
	}
	if len(state.Config.Memories) != 1 {
		t.Fatalf("expected one memory, got %d", len(state.Config.Memories))
	}
	entry := state.Config.Memories[0]
	if entry.Scope != memoryScopeGlobal {
		t.Fatalf("always memory should be global, got %#v", entry)
	}
	if !strings.Contains(entry.Content, "Ursachen statt Symptome") || strings.Contains(strings.ToLower(entry.Content), "merke dir") {
		t.Fatalf("unexpected remembered content: %q", entry.Content)
	}
	context := memoryContextForAgent(state.Config, filepath.Join(project, "sub"))
	if !strings.Contains(context, "Ursachen statt Symptome") {
		t.Fatalf("global memory not injected across projects: %s", context)
	}
}

func TestDirectMemoryPromptListsAndDeletesByQuery(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	state := NewAppState(defaultConfig(), nil)
	state.Project = project
	state.Model = "local-model"
	if _, err := state.executeMemoryAction(project, AgentAction{Action: "memory_remember", Content: "Kurze Antworten bevorzugen.", Scope: "global"}); err != nil {
		t.Fatal(err)
	}

	if err := state.StartAgentForThread("Zeige gemerkt Liste", "local-model", nil, project, ""); err != nil {
		t.Fatal(err)
	}
	if state.Running {
		t.Fatal("direct memory list must not start a model run")
	}
	if !strings.Contains(state.LastSummary, "Kurze Antworten") {
		t.Fatalf("list result missing memory: %s", state.LastSummary)
	}

	if err := state.StartAgentForThread("Entferne aus der gemerkt Liste kurze Antworten", "local-model", nil, project, ""); err != nil {
		t.Fatal(err)
	}
	if len(state.Config.Memories) != 0 {
		t.Fatalf("expected memory to be deleted, got %s", memoriesJSON(state.Config.Memories))
	}
}

func TestDetectDirectMemoryRequestScopes(t *testing.T) {
	req, ok := detectDirectMemoryRequest("Merke dir für dieses Projekt: build läuft mit go test ./...")
	if !ok || req.Kind != "remember" || req.Scope != memoryScopeProject || !strings.Contains(req.Content, "build läuft") {
		t.Fatalf("unexpected project memory request: %#v ok=%t", req, ok)
	}
	req, ok = detectDirectMemoryRequest("Always answer concise, remember that")
	if !ok || req.Kind != "remember" || req.Scope != memoryScopeGlobal {
		t.Fatalf("unexpected global memory request: %#v ok=%t", req, ok)
	}
	req, ok = detectDirectMemoryRequest("Lösche Erinnerung abcdef1234567890")
	if !ok || req.Kind != "forget" || req.MemoryID != "abcdef1234567890" {
		t.Fatalf("unexpected forget request: %#v ok=%t", req, ok)
	}
}

func memoriesJSON(entries []MemoryEntry) string {
	data, _ := json.MarshalIndent(entries, "", "  ")
	return string(data)
}
