// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestMultiProjectConcurrentRunTracking(t *testing.T) {
	tempRoot := t.TempDir()
	projA := filepath.Join(tempRoot, "ProjectAlpha")
	projB := filepath.Join(tempRoot, "ProjectBeta")

	cfg := Config{
		RootProjectDir: tempRoot,
		LastProject:    projA,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	if len(state.GetRunningProjects()) != 0 {
		t.Fatalf("expected 0 running projects initially, got %d", len(state.GetRunningProjects()))
	}
	if state.IsProjectRunning(projA) {
		t.Fatalf("expected projA not running initially")
	}

	// Register run on Project A
	runA := &ActiveAgentRun{
		ID:             "run-A",
		Project:        projA,
		ThreadID:       "thread-A",
		Model:          "model-1",
		Phase:          "working",
		StartedAt:      time.Now(),
		LastProgressAt: time.Now(),
	}
	state.RegisterActiveRun(runA)

	if !state.IsProjectRunning(projA) {
		t.Errorf("expected projA to be running")
	}
	if state.IsProjectRunning(projB) {
		t.Errorf("expected projB NOT to be running")
	}
	if len(state.GetRunningProjects()) != 1 {
		t.Errorf("expected 1 running project, got %d", len(state.GetRunningProjects()))
	}

	// Test IsThreadRunning and GetActiveRunForThread
	if !state.IsThreadRunning("thread-A") {
		t.Errorf("expected thread-A to be running")
	}
	if state.IsThreadRunning("thread-B") {
		t.Errorf("expected thread-B NOT to be running")
	}
	if state.IsThreadRunning("") != true {
		t.Errorf("expected empty threadID to return true when state is running")
	}
	activeA := state.GetActiveRunForThread("thread-A")
	if activeA == nil || activeA.ID != "run-A" {
		t.Errorf("expected active run-A for thread-A, got %v", activeA)
	}
	if state.GetActiveRunForThread("non-existent") != nil {
		t.Errorf("expected nil active run for non-existent thread")
	}

	// Register run on Project B concurrently
	runB := &ActiveAgentRun{
		ID:             "run-B",
		Project:        projB,
		ThreadID:       "thread-B",
		Model:          "model-2",
		Phase:          "working",
		StartedAt:      time.Now(),
		LastProgressAt: time.Now(),
	}
	state.RegisterActiveRun(runB)

	if !state.IsProjectRunning(projA) {
		t.Errorf("expected projA to still be running")
	}
	if !state.IsProjectRunning(projB) {
		t.Errorf("expected projB to also be running concurrently")
	}
	if len(state.GetRunningProjects()) != 2 {
		t.Errorf("expected 2 running projects, got %d", len(state.GetRunningProjects()))
	}
	if !state.IsThreadRunning("thread-B") {
		t.Errorf("expected thread-B to be running")
	}

	// Unregister run on Project A
	state.UnregisterActiveRun(runA.ID)

	if state.IsProjectRunning(projA) {
		t.Errorf("expected projA to be finished")
	}
	if !state.IsProjectRunning(projB) {
		t.Errorf("expected projB to still be running")
	}
	if len(state.GetRunningProjects()) != 1 {
		t.Errorf("expected 1 running project left, got %d", len(state.GetRunningProjects()))
	}

	// Test nil and corner cases
	state.RegisterActiveRun(nil)
	state.RegisterActiveRun(&ActiveAgentRun{ID: ""})
	state.UnregisterActiveRun("")
	state.UnregisterActiveRun("unknown-id")

	// Unregister run on Project B
	state.UnregisterActiveRun(runB.ID)
	if state.IsProjectRunning(projB) {
		t.Errorf("expected projB to be finished")
	}
	if len(state.GetRunningProjects()) != 0 {
		t.Errorf("expected 0 running projects, got %d", len(state.GetRunningProjects()))
	}
}

func TestFallbackActiveRunTracking(t *testing.T) {
	tempRoot := t.TempDir()
	cfg := Config{
		RootProjectDir: tempRoot,
		LastProject:    tempRoot,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	// Simulate legacy single-run fields
	cancelCalled := false
	state.mu.Lock()
	state.Running = true
	state.CurrentThread = "legacy-thread-1"
	state.RunID = "legacy-run-1"
	state.Project = tempRoot
	state.Model = "legacy-model"
	state.RunPhase = "thinking"
	state.Cancel = func() { cancelCalled = true }
	state.mu.Unlock()

	if !state.IsThreadRunning("legacy-thread-1") {
		t.Errorf("expected legacy-thread-1 to be reported as running")
	}
	if state.IsThreadRunning("other-thread") {
		t.Errorf("expected other-thread NOT to be running")
	}

	run := state.GetActiveRunForThread("legacy-thread-1")
	if run == nil || run.ID != "legacy-run-1" {
		t.Fatalf("expected legacy run, got %v", run)
	}
	if run.Cancel != nil {
		run.Cancel()
	}
	if !cancelCalled {
		t.Errorf("expected cancel function to be hooked")
	}

	// Test IsProjectRunning with empty and mismatch
	if state.IsProjectRunning("") {
		t.Errorf("expected empty project not running")
	}
	if !state.IsProjectRunning(tempRoot) {
		t.Errorf("expected tempRoot to be running under fallback")
	}
}

func TestBuildUDPDiscoveryPayload(t *testing.T) {
	tempRoot := t.TempDir()
	cfg := Config{
		RootProjectDir: tempRoot,
		LastProject:    tempRoot,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	state.RegisterActiveRun(&ActiveAgentRun{
		ID:       "run-udp",
		Project:  filepath.Join(tempRoot, "ActiveProj"),
		ThreadID: "thread-udp",
	})

	payload := buildUDPDiscoveryPayload(32146, "LocalCode-PC", "FP123", "https://192.168.1.94:32146/remote", []string{"https://192.168.1.94:32146/remote"}, state)
	if payload.App != "LocalCode Remote" {
		t.Errorf("expected App='LocalCode Remote', got %q", payload.App)
	}
	if payload.Port != 32146 {
		t.Errorf("expected Port=32146, got %d", payload.Port)
	}
	if payload.TLSFingerprint != "FP123" {
		t.Errorf("expected TLSFingerprint='FP123', got %q", payload.TLSFingerprint)
	}
	if len(payload.RemoteURLs) != 1 || payload.RemoteURLs[0] != "https://192.168.1.94:32146/remote" {
		t.Errorf("unexpected RemoteURLs: %v", payload.RemoteURLs)
	}
	if len(payload.RunningProjects) == 0 {
		t.Errorf("expected at least 1 running project in UDP payload")
	}
}

func TestAgentActionValidationAndHelpers(t *testing.T) {
	// Test stringMapArg and intMapArg
	args := map[string]any{
		"str":     "hello",
		"intNum":  42,
		"fltNum":  float64(100),
		"strNum":  "200",
		"jsonNum": json.Number("300"),
	}
	if stringMapArg(nil, "str") != "" || stringMapArg(args, "missing") != "" || stringMapArg(args, "str") != "hello" {
		t.Errorf("unexpected stringMapArg result")
	}
	if intMapArg(nil, "intNum") != 0 || intMapArg(args, "missing") != 0 {
		t.Errorf("unexpected intMapArg empty result")
	}
	if intMapArg(args, "intNum") != 42 || intMapArg(args, "fltNum") != 100 || intMapArg(args, "strNum") != 200 || intMapArg(args, "jsonNum") != 300 {
		t.Errorf("unexpected intMapArg parsed result")
	}

	// Test validateAgentAction
	actions := []struct {
		action  AgentAction
		wantErr bool
	}{
		{action: AgentAction{Action: "read_file", Path: "file.txt"}, wantErr: false},
		{action: AgentAction{Action: "read_file", Path: ""}, wantErr: true},
		{action: AgentAction{Action: "delete_file", Path: "file.txt"}, wantErr: false},
		{action: AgentAction{Action: "search_text", Query: "foo"}, wantErr: false},
		{action: AgentAction{Action: "search_text", Query: ""}, wantErr: true},
		{action: AgentAction{Action: "lsp", Path: "a.go", Name: "symbols"}, wantErr: false},
		{action: AgentAction{Action: "lsp", Path: "a.go", Name: "definition", Line: 1, Character: 1}, wantErr: false},
		{action: AgentAction{Action: "lsp", Path: "a.go", Name: "definition", Line: 0, Character: 1}, wantErr: true},
		{action: AgentAction{Action: "lsp", Path: "a.go", Name: "unknown_op"}, wantErr: true},
		{action: AgentAction{Action: "replace_text", Path: "a.go", OldText: "old"}, wantErr: false},
		{action: AgentAction{Action: "write_file", Path: "a.go", Content: "code"}, wantErr: false},
		{action: AgentAction{Action: "create_svg_asset", Path: "a.svg", Content: "<svg></svg>"}, wantErr: false},
		{action: AgentAction{Action: "convert_image_asset", Source: "a.png", Destination: "a.webp"}, wantErr: false},
		{action: AgentAction{Action: "subagent_analyze", Task: "check"}, wantErr: false},
		{action: AgentAction{Action: "command_read", Name: "cmd"}, wantErr: false},
		{action: AgentAction{Action: "run_tool", Tool: "tool1"}, wantErr: false},
		{action: AgentAction{Action: "run_command", Command: "dir"}, wantErr: false},
		{action: AgentAction{Action: "web_fetch", URL: "https://example.com"}, wantErr: false},
		{action: AgentAction{Action: "mcp_call_tool", Server: "srv", Tool: "tool"}, wantErr: false},
		{action: AgentAction{Action: "skill_read", Skill: "sk"}, wantErr: false},
		{action: AgentAction{Action: "skill_list_resources", Skill: "sk"}, wantErr: false},
		{action: AgentAction{Action: "skill_read_resource", Skill: "sk", Resource: "res"}, wantErr: false},
		{action: AgentAction{Action: "skill_copy_resource", Skill: "sk", Resource: "res", Destination: "dst"}, wantErr: false},
		{action: AgentAction{Action: "skill_run_script", Skill: "sk", Script: "run.sh"}, wantErr: false},
		{action: AgentAction{Action: "memory_remember", Content: "fact"}, wantErr: false},
		{action: AgentAction{Action: "memory_forget", MemoryID: "mem1"}, wantErr: false},
		{action: AgentAction{Action: "other_custom_action"}, wantErr: false},
	}

	for _, tc := range actions {
		err := validateAgentAction(tc.action)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateAgentAction(%s): err=%v, wantErr=%v", tc.action.Action, err, tc.wantErr)
		}
	}

	// Test agentContextEvent helpers
	if !agentContextEventIsTransient("status") || !agentContextEventIsTransient("progress") || agentContextEventIsTransient("final") {
		t.Errorf("unexpected agentContextEventIsTransient results")
	}
	if !agentContextEventShouldIncludeDetail("final") || !agentContextEventShouldIncludeDetail("question") || agentContextEventShouldIncludeDetail("status") {
		t.Errorf("unexpected agentContextEventShouldIncludeDetail results")
	}
	if agentContextEventLabel(UIEvent{Type: "user"}) != "Nutzer" {
		t.Errorf("unexpected agentContextEventLabel user")
	}
	if agentContextEventLabel(UIEvent{Type: "final"}) != "Letzte Antwort" {
		t.Errorf("unexpected agentContextEventLabel final")
	}
	if agentContextEventLabel(UIEvent{Type: "tool_result", Action: "read_file"}) != "Werkzeug read_file" {
		t.Errorf("unexpected agentContextEventLabel tool_result")
	}
	if agentContextEventLabel(UIEvent{Type: "tool_error"}) != "Fehler" {
		t.Errorf("unexpected agentContextEventLabel tool_error")
	}
	if agentContextEventLabel(UIEvent{Type: "warning"}) != "Warnung" {
		t.Errorf("unexpected agentContextEventLabel warning")
	}
}

func TestDefaultActionDescription(t *testing.T) {
	cfgDE := Config{Language: "de"}
	cfgEN := Config{Language: "en"}

	tests := []struct {
		action AgentAction
		wantDE string
		wantEN string
	}{
		{action: AgentAction{Action: "read_file", Path: "main.go"}, wantDE: "Datei lesen: main.go", wantEN: "Read file: main.go"},
		{action: AgentAction{Action: "read_file"}, wantDE: "Datei lesen", wantEN: "Read file"},
		{action: AgentAction{Action: "write_file", Path: "out.go"}, wantDE: "Datei schreiben: out.go", wantEN: "Write file: out.go"},
		{action: AgentAction{Action: "write_file"}, wantDE: "Datei schreiben", wantEN: "Write file"},
		{action: AgentAction{Action: "replace_text", Path: "fix.go"}, wantDE: "Datei bearbeiten: fix.go", wantEN: "Edit file: fix.go"},
		{action: AgentAction{Action: "replace_text"}, wantDE: "Datei bearbeiten", wantEN: "Edit file"},
		{action: AgentAction{Action: "search_text", Query: "foo"}, wantDE: "Text suchen: foo", wantEN: "Search text: foo"},
		{action: AgentAction{Action: "search_text"}, wantDE: "Code durchsuchen", wantEN: "Search code"},
		{action: AgentAction{Action: "list_files", Path: "dir"}, wantDE: "Dateien auflisten: dir", wantEN: "List files: dir"},
		{action: AgentAction{Action: "list_files"}, wantDE: "Dateien auflisten", wantEN: "List files"},
		{action: AgentAction{Action: "run_command", Command: "go test"}, wantDE: "Befehl ausführen: go test", wantEN: "Run command: go test"},
		{action: AgentAction{Action: "run_command"}, wantDE: "Befehl ausführen", wantEN: "Run command"},
		{action: AgentAction{Action: "run_tool", Tool: "toolA"}, wantDE: "Werkzeug ausführen: toolA", wantEN: "Run tool: toolA"},
		{action: AgentAction{Action: "run_tool"}, wantDE: "Werkzeug ausführen", wantEN: "Run tool"},
		{action: AgentAction{Action: "git", Args: []string{"status"}}, wantDE: "Git: status", wantEN: "Git: status"},
		{action: AgentAction{Action: "git"}, wantDE: "Git", wantEN: "Git"},
		{action: AgentAction{Action: "git_commit"}, wantDE: "Git Commit erstellen", wantEN: "Create Git commit"},
		{action: AgentAction{Action: "web_search", Query: "docs"}, wantDE: "Websuche: docs", wantEN: "Web search: docs"},
		{action: AgentAction{Action: "web_search"}, wantDE: "Websuche", wantEN: "Web search"},
		{action: AgentAction{Action: "web_fetch", URL: "https://foo"}, wantDE: "Webseite aufrufen: https://foo", wantEN: "Fetch URL: https://foo"},
		{action: AgentAction{Action: "web_fetch"}, wantDE: "Webseite aufrufen", wantEN: "Fetch URL"},
		{action: AgentAction{Action: "project_info"}, wantDE: "Projektstruktur analysieren", wantEN: "Analyze project structure"},
		{action: AgentAction{Action: "build_project"}, wantDE: "Projekt bauen", wantEN: "Build project"},
		{action: AgentAction{Action: "deploy_android"}, wantDE: "Android App bereitstellen", wantEN: "Deploy Android app"},
		{action: AgentAction{Action: "subagent_analyze", Task: "deep analysis"}, wantDE: "Subagent Analyse: deep analysis", wantEN: "Subagent analysis: deep analysis"},
		{action: AgentAction{Action: "subagent_analyze"}, wantDE: "Subagent Analyse", wantEN: "Subagent analysis"},
		{action: AgentAction{Action: "aider_edit", Task: "refactor"}, wantDE: "Code Engine: refactor", wantEN: "Code engine: refactor"},
		{action: AgentAction{Action: "aider_edit"}, wantDE: "Code Engine ausführen", wantEN: "Execute code engine"},
		{action: AgentAction{Action: "custom_act"}, wantDE: "custom_act", wantEN: "custom_act"},
		{action: AgentAction{}, wantDE: "Arbeite...", wantEN: "Working..."},
	}

	for _, tc := range tests {
		gotDE := defaultActionDescription(tc.action, cfgDE)
		if gotDE != tc.wantDE {
			t.Errorf("defaultActionDescription DE (%s): got %q, want %q", tc.action.Action, gotDE, tc.wantDE)
		}
		gotEN := defaultActionDescription(tc.action, cfgEN)
		if gotEN != tc.wantEN {
			t.Errorf("defaultActionDescription EN (%s): got %q, want %q", tc.action.Action, gotEN, tc.wantEN)
		}
	}
}

func TestAgentActionParsingAndContext(t *testing.T) {
	// Valid JSON direct
	act, err := parseAgentAction(`{"action":"read_file","path":"foo.go"}`)
	if err != nil || act.Action != "read_file" || act.Path != "foo.go" {
		t.Errorf("expected parsed action, got %v, err=%v", act, err)
	}

	// Markdown or surrounded JSON
	act2, err := parseAgentAction("Some markdown prefix\n```json\n{\"action\":\"write_file\",\"path\":\"bar.go\",\"content\":\"code\"}\n```\nsuffix")
	if err != nil || act2.Action != "write_file" || act2.Path != "bar.go" || act2.Content != "code" {
		t.Errorf("expected parsed fenced action, got %v, err=%v", act2, err)
	}

	// Action with arguments map
	act3, err := parseAgentAction(`{"action":"search_text","arguments":{"query":"search_term"}}`)
	if err != nil || act3.Action != "search_text" || act3.Query != "search_term" {
		t.Errorf("expected argument-filled action, got %v, err=%v", act3, err)
	}

	// Invalid actions
	if _, err := parseAgentAction(`{"action":""}`); err == nil {
		t.Errorf("expected error for empty action")
	}
	if _, err := parseAgentAction(`invalid json without braces`); err == nil {
		t.Errorf("expected error for non-json")
	}

	// chooseMoreSpecificAction
	a1 := AgentAction{Action: "read_file", Path: "a.go"}
	a2 := AgentAction{Action: "write_file", Path: "b.go"}
	if chooseMoreSpecificAction(a1, a2).Action != "write_file" {
		t.Errorf("expected a2 to be chosen")
	}
	if chooseMoreSpecificAction(a1, AgentAction{}).Action != "read_file" {
		t.Errorf("expected a1 to be chosen when second is empty")
	}

	// actionForModelContext
	cfgSmall := Config{ContextLength: 8192}
	cfgLarge := Config{ContextLength: 128000}

	longContent := make([]byte, 5000)
	for i := range longContent {
		longContent[i] = 'x'
	}
	actLong := AgentAction{
		Action:  "write_file",
		Path:    "large.txt",
		Content: string(longContent),
		OldText: string(longContent),
		NewText: string(longContent),
	}

	actTruncSmall := actionForModelContext(actLong, cfgSmall)
	if len(actTruncSmall.Content) > 500 {
		t.Errorf("expected content to be omitted/truncated for small context")
	}
	if len(actTruncSmall.OldText) > 500 || len(actTruncSmall.NewText) > 500 {
		t.Errorf("expected old/new text omitted")
	}

	actTruncLarge := actionForModelContext(actLong, cfgLarge)
	if actTruncLarge.Content != string(longContent) {
		t.Errorf("expected 5000 bytes content preserved under 65k+ context")
	}
}

func TestPublicOnlyDialContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Block private IPs
	_, err := publicOnlyDialContext(ctx, "tcp", "127.0.0.1:80")
	if err == nil {
		t.Errorf("expected blocked private IP for 127.0.0.1")
	}
	_, err = publicOnlyDialContext(ctx, "tcp", "192.168.1.1:443")
	if err == nil {
		t.Errorf("expected blocked private IP for 192.168.1.1")
	}
	_, err = publicOnlyDialContext(ctx, "tcp", "10.0.0.1:80")
	if err == nil {
		t.Errorf("expected blocked private IP for 10.0.0.1")
	}

	// Invalid address format
	_, err = publicOnlyDialContext(ctx, "tcp", "invalid-address-without-port")
	if err == nil {
		t.Errorf("expected error for invalid address without port")
	}
}

func TestStopAgentForThread(t *testing.T) {
	tempRoot := t.TempDir()
	cfg := Config{
		RootProjectDir: tempRoot,
		LastProject:    tempRoot,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	// Stopping non-existent thread
	if state.StopAgentForThread("non-existent-thread") {
		t.Errorf("expected StopAgentForThread to return false for non-existent thread")
	}

	// Register active run
	cancelled := false
	state.RegisterActiveRun(&ActiveAgentRun{
		ID:       "run-cancel-test",
		Project:  tempRoot,
		ThreadID: "thread-cancel-test",
		Cancel:   func() { cancelled = true },
	})

	if !state.StopAgentForThread("thread-cancel-test") {
		t.Errorf("expected StopAgentForThread to return true for active thread")
	}
	if !cancelled {
		t.Errorf("expected Cancel hook to be invoked")
	}
}

func TestMainFlagHelpers(t *testing.T) {
	if !hasTrayFlag([]string{"--tray"}) || !hasTrayFlag([]string{"/tray"}) || !hasTrayFlag([]string{"-tray"}) {
		t.Errorf("expected hasTrayFlag to detect tray flags")
	}
	if hasTrayFlag([]string{"--other", "-v"}) {
		t.Errorf("expected hasTrayFlag to return false for non-tray flags")
	}

	t.Setenv("LOCALCODE_FAST_START", "1")
	if !fastStartupRequested() {
		t.Errorf("expected fastStartupRequested to be true")
	}
	t.Setenv("LOCALCODE_FAST_START", "0")
	if fastStartupRequested() {
		t.Errorf("expected fastStartupRequested to be false for '0'")
	}
	t.Setenv("LOCALCODE_FAST_START", "false")
	if fastStartupRequested() {
		t.Errorf("expected fastStartupRequested to be false for 'false'")
	}

	res := fastStartupBootstrap(Config{OllamaURL: "http://localhost:11434", ContextLength: 16000})
	if res.Ollama == nil || res.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("unexpected fastStartupBootstrap Ollama client")
	}
}
