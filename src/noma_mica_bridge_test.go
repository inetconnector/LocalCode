// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFastOllamaFormatSimplifiesComplexObjectSchema(t *testing.T) {
	if got := fastOllamaFormat(actionSchema); got != "json" {
		t.Fatalf("fastOllamaFormat(actionSchema) = %#v, want \"json\"", got)
	}
	if got := fastOllamaFormat(nil); got != nil {
		t.Fatalf("fastOllamaFormat(nil) = %#v, want nil", got)
	}
	if got := fastOllamaFormat("json"); got != "json" {
		t.Fatalf("fastOllamaFormat(\"json\") = %#v, want \"json\"", got)
	}
}

func TestExtractLiveStreamPreviewUnescapesCodeContent(t *testing.T) {
	raw := `{"action":"write_file","message":"Write engine","path":"src/engine.js","content":"const x = 1;\nfunction step() {\n\treturn \"ok\";\n`
	got := extractLiveStreamPreview(raw)
	if !strings.Contains(got, "const x = 1;\nfunction step()") || !strings.Contains(got, `return "ok";`) {
		t.Fatalf("unexpected live preview: %q", got)
	}
}

func TestOllamaChatStreamEmitsLiveDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"{\"action\":\"write_file\","},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"\"message\":\"ok\",\"path\":\"a.js\",\"content\":\"const a=1;\"}"},"done":true,"done_reason":"stop","eval_count":12}`)
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	var lastDelta string
	var maxTokens int
	out, err := client.ChatStream(context.Background(), "qwen2.5-coder:14b", []OllamaMessage{{Role: "user", Content: "hi"}}, actionSchema, func(acc string, tokens int, _ float64) {
		lastDelta = acc
		if tokens > maxTokens {
			maxTokens = tokens
		}
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	if !strings.Contains(out, `"write_file"`) || maxTokens < 2 || !strings.Contains(lastDelta, "const a=1;") {
		t.Fatalf("unexpected stream output: out=%q delta=%q tokens=%d", out, lastDelta, maxTokens)
	}
}

func TestInactiveThreadActionsSucceedWhileAnotherThreadRuns(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project

	t1, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	// t2 is now CurrentThread and actively running; t1 is inactive.
	state.mu.Lock()
	state.Running = true
	state.CurrentThread = t2.ID
	state.mu.Unlock()

	if err := state.RenameChat(t1.ID, "Renamed Inactive"); err != nil {
		t.Fatalf("RenameChat on inactive thread failed while active thread running: %v", err)
	}
	if err := state.ArchiveChat(t1.ID, true); err != nil {
		t.Fatalf("ArchiveChat on inactive thread failed while active thread running: %v", err)
	}
	if err := state.ArchiveChat(t1.ID, false); err != nil {
		t.Fatalf("Unarchive on inactive thread failed: %v", err)
	}
	if err := state.DeleteChat(t1.ID); err != nil {
		t.Fatalf("DeleteChat on inactive thread failed while active thread running: %v", err)
	}
	// Active running thread t2 must still be protected from direct DeleteChat.
	if err := state.DeleteChat(t2.ID); err == nil {
		t.Fatal("expected DeleteChat on currently running thread to fail")
	}
}

func TestOrderedTaskTargetFilesAndStepPlan(t *testing.T) {
	project := filepath.Join(t.TempDir(), "product")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	task := "Create product/README.md, product/index.html, product/tests/engine.test.js, product/src/game.js, product/src/renderer.js, product/src/engine.js."
	got := orderedTaskTargetFiles(project, task)
	want := []string{"src/engine.js", "src/renderer.js", "src/game.js", "index.html", "tests/engine.test.js", "README.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("orderedTaskTargetFiles = %v, want %v", got, want)
	}
	badge, title, _, directive := nomaNextStepPlan(project, task, map[string]bool{}, false, false, 3, 70)
	if !strings.Contains(badge, "Modul 1/6") || !strings.Contains(title, "src/engine.js") || !strings.Contains(directive, `"src/engine.js"`) {
		t.Fatalf("unexpected step plan: badge=%q title=%q directive=%q", badge, title, directive)
	}
	// When all target files exist and verification failed, NOMA must enter repair mode (not blind re-test loop).
	for _, rel := range want {
		full := filepath.Join(project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(strings.Repeat("x", 64)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	allChanged := map[string]bool{}
	for _, rel := range want {
		allChanged[rel] = true
	}
	repBadge, _, _, repDirective := nomaNextStepPlan(project, task, allChanged, true, true, 10, 70)
	if !strings.Contains(repBadge, "Reparatur") || !strings.Contains(repDirective, "REPARATUR-STEUERUNG") {
		t.Fatalf("expected repair mode after failed verification, got badge=%q directive=%q", repBadge, repDirective)
	}
}

func TestOfflinePackageInstallGuardBlocksNpmInstallWhenNetworkDisabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	if msg := offlinePackageInstallBlocked(cfg, AgentAction{Action: "open_terminal", Command: "npm install canvas"}); !strings.Contains(msg, "OFFLINE-POLICY BLOCKIERT") {
		t.Fatalf("expected offline install block for open_terminal, got %q", msg)
	}
	if msg := offlinePackageInstallBlocked(cfg, AgentAction{Action: "run_tool", Tool: "npm", Args: []string{"install", "canvas"}}); !strings.Contains(msg, "OFFLINE-POLICY BLOCKIERT") {
		t.Fatalf("expected offline install block for run_tool, got %q", msg)
	}
	if msg := offlinePackageInstallBlocked(cfg, AgentAction{Action: "run_tool", Tool: "node", Args: []string{"--test", "tests/engine.test.js"}}); msg != "" {
		t.Fatalf("node --test should not be blocked, got %q", msg)
	}
	cfg.NetworkEnabled = true
	if msg := offlinePackageInstallBlocked(cfg, AgentAction{Action: "open_terminal", Command: "npm install canvas"}); msg != "" {
		t.Fatalf("online config should not trigger offline block, got %q", msg)
	}
}

func TestEffectiveCompactionContextLengthMatchesLocalOllamaNumCtx(t *testing.T) {
	localCfg := defaultConfig()
	localCfg.OllamaURL = "http://127.0.0.1:11434"
	localCfg.ContextLength = 32768
	if got := effectiveCompactionContextLength(localCfg); got != 6144 {
		t.Fatalf("local Ollama compaction context = %d, want 6144", got)
	}
	defaultLocalCfg := defaultConfig()
	defaultLocalCfg.OllamaURL = ""
	defaultLocalCfg.ContextLength = 32768
	if got := effectiveCompactionContextLength(defaultLocalCfg); got != 6144 {
		t.Fatalf("default empty OllamaURL compaction context = %d, want 6144", got)
	}
	if got := postCompactionTokenTarget(localCfg); got >= 4000 {
		t.Fatalf("local Ollama post-compaction target = %d, want < 4000", got)
	}
	remoteCfg := localCfg
	remoteCfg.OllamaURL = "https://computemesh.example.com"
	if got := effectiveCompactionContextLength(remoteCfg); got != 32768 {
		t.Fatalf("remote compaction context = %d, want 32768", got)
	}
	if len(systemPromptForConfig(localCfg)) >= len(agentSystemPrompt)/2 {
		t.Fatalf("expected compact local system prompt (<50%% of full prompt), got %d vs %d chars", len(systemPromptForConfig(localCfg)), len(agentSystemPrompt))
	}
	if systemPromptForConfig(remoteCfg) != agentSystemPrompt {
		t.Fatal("expected full agentSystemPrompt for remote large-context config")
	}
}

func TestMicaPostActionVerifyFlagsDestructiveShrinkAndRule7Stubs(t *testing.T) {
	project := t.TempDir()
	srcDir := filepath.Join(project, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(srcDir, "engine.js")
	if err := os.WriteFile(target, []byte("module.exports = { reset: () => ({}) };\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := runMicaPostActionVerify(context.Background(), nil, project, AgentAction{Action: "write_file", Path: "src/engine.js"}, defaultConfig(), 2400)
	if !strings.Contains(note, "DESTRUCTIVE_SHRINK_WARNING") {
		t.Fatalf("expected DESTRUCTIVE_SHRINK_WARNING, got %q", note)
	}
	stubCode := "function step() {\n  // Implement game logic here\n}\nmodule.exports = { step };\n"
	if err := os.WriteFile(target, []byte(stubCode), 0o600); err != nil {
		t.Fatal(err)
	}
	stubNote := runMicaPostActionVerify(context.Background(), nil, project, AgentAction{Action: "write_file", Path: "src/engine.js"}, defaultConfig(), 0)
	if !strings.Contains(stubNote, "Rule-7 Zero-Stub Verstoß") {
		t.Fatalf("expected Rule-7 stub violation note, got %q", stubNote)
	}
	if isPathCompletedInRun(project, "src/engine.js", map[string]bool{"src/engine.js": true}) {
		t.Fatal("expected file containing Rule-7 placeholder stub to not count as completed")
	}
}

func TestActionSignatureDistinguishesChangedWriteContent(t *testing.T) {
	a1 := AgentAction{Action: "write_file", Path: "src/engine.js", Content: "function step() { // TODO }\n"}
	a2 := AgentAction{Action: "write_file", Path: "src/engine.js", Content: "function step() { return 1; }\n"}
	a1Dup := AgentAction{Action: "write_file", Path: "src/engine.js", Content: "function step() { // TODO }\n"}
	if actionSignature(a1) == actionSignature(a2) {
		t.Fatal("write_file with different content must not produce identical actionSignature")
	}
	if actionSignature(a1) != actionSignature(a1Dup) {
		t.Fatal("identical write_file actions must still share the same actionSignature")
	}
	if err := validateAgentAction(AgentAction{Action: "write_file", Path: "tests/engine.test.js", Content: "[omitted from model history: 2736 bytes of content for tests/engine.test.js; use read_file if exact content is needed]"}); err == nil {
		t.Fatal("expected validateAgentAction to reject '[omitted from model history:...]' placeholder content")
	}
}

func TestIndexHTMLIncludesMarkdownTabsFastRenderAndNomaMicaEngine(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("static", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, needle := range []string{"renderMarkdownTabs", "renderMarkdownHTML", ">Vorschau<", ">Quelltext<", "updateLiveStreamInPlace", "computeChatSignature", "LocalCode · NomaMica", "renderGroupedChatEvents", "trajectory-group-panel"} {
		if !strings.Contains(html, needle) {
			t.Fatalf("static/index.html missing expected marker %q", needle)
		}
	}
}

func TestParseAgentActionSplitsCompoundRunToolString(t *testing.T) {
	act, err := parseAgentAction(`{"action":"run_tool","tool":"node --check src/engine.js","message":"Check syntax"}`)
	if err != nil {
		t.Fatalf("parseAgentAction error: %v", err)
	}
	if act.Tool != "node" || len(act.Args) != 2 || act.Args[0] != "--check" || act.Args[1] != "src/engine.js" {
		t.Fatalf("unexpected parsed run_tool action: tool=%q args=%v", act.Tool, act.Args)
	}
	wAct, err := parseAgentAction("{\"action\":\"write_file\",\"path\":\"tests/engine.test.js\",\"message\":\"Write test\",\"content\":\"```js\\nconst a = 1;\\n```\"}")
	if err != nil {
		t.Fatalf("parseAgentAction write_file error: %v", err)
	}
	if wAct.Content != "const a = 1;" {
		t.Fatalf("expected outer markdown fence stripped from non-md write_file, got %q", wAct.Content)
	}
}

func TestRunNomaMicaPreflightAndModuleHintsCoverage(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project

	fakeRoot := t.TempDir()
	nomaDir := filepath.Join(fakeRoot, "NOMA", "noma_runtime", "coding")
	if err := os.MkdirAll(nomaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nomaDir, "problem_framing.py"), []byte("# stub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOMAMICA_ROOT", fakeRoot)
	if gotRoot := discoverNomaMicaRoot(project); gotRoot != fakeRoot {
		t.Fatalf("discoverNomaMicaRoot = %q, want %q", gotRoot, fakeRoot)
	}
	plan := runNomaMicaPreflight(context.Background(), state, project, "Implement src/engine.js, src/renderer.js, src/game.js, index.html, tests/engine.test.js, README.md, style.css", cfg)
	if !strings.Contains(plan, "NOMA × MICA × AGENTOS ARCHITEKTUR-") {
		t.Fatalf("unexpected preflight plan: %q", plan)
	}
	for _, mod := range []string{"src/engine.js", "src/renderer.js", "src/game.js", "tests/engine.test.js", "index.html", "README.md", "style.css", "other.txt"} {
		if describeModuleHint(mod) == "" {
			t.Fatalf("empty module hint for %s", mod)
		}
	}
	preview := extractLiveStreamPreview(`{"action":"write_file","content":"line1\n\t\"quoted\"\\slash"}`)
	if !strings.Contains(preview, "line1\n\t\"quoted\"\\slash") {
		t.Fatalf("unexpected preview: %q", preview)
	}
}
