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
	"time"
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
	agentosDir := filepath.Join(fakeRoot, "AgentOS", "src", "agentos", "integration")
	if err := os.MkdirAll(nomaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(agentosDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nomaDir, "problem_framing.py"), []byte("# stub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentosDir, "utp_v1.py"), []byte("# stub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOMAMICA_ROOT", fakeRoot)
	if gotRoot := discoverNomaMicaRoot(project); gotRoot != fakeRoot {
		t.Fatalf("discoverNomaMicaRoot = %q, want %q", gotRoot, fakeRoot)
	}
	plan := runNomaMicaSuitePreflight(context.Background(), state, project, "Implement src/engine.js, src/renderer.js, src/game.js, index.html, tests/engine.test.js, README.md, style.css", cfg)
	if !strings.Contains(plan, "NOMA × MICA × AGENTOS ARCHITEKTUR-") {
		t.Fatalf("unexpected preflight plan: %q", plan)
	}
	for _, mod := range []string{"src/engine.js", "src/renderer.js", "src/game.js", "tests/engine.test.js", "index.html", "README.md", "style.css", "other.txt"} {
		if describeTargetFileRole(mod) == "" || genericModuleContract(mod) == "" {
			t.Fatalf("empty module role or contract for %s", mod)
		}
	}
	preview := extractLiveStreamPreview(`{"action":"write_file","content":"line1\n\t\"quoted\"\\slash"}`)
	if !strings.Contains(preview, "line1\n\t\"quoted\"\\slash") {
		t.Fatalf("unexpected preview: %q", preview)
	}
	if extractLiveStreamPreview("") != "" || len(extractLiveStreamPreview(strings.Repeat("x", 1500))) != 1200 {
		t.Fatal("unexpected extractLiveStreamPreview fallback")
	}
	if stripOuterMarkdownFence("plain") != "plain" || stripOuterMarkdownFence("```no-newline```") != "```no-newline```" {
		t.Fatal("unexpected stripOuterMarkdownFence fallback")
	}
	rAct, err := parseAgentAction("{\"action\":\"replace_text\",\"path\":\"src/a.js\",\"old_text\":\"```js\\nold();\\n```\",\"new_text\":\"```js\\nnew();\\n```\"}")
	if err != nil || rAct.OldText != "old();" || rAct.NewText != "new();" {
		t.Fatalf("unexpected replace_text fence strip: err=%v old=%q new=%q", err, rAct.OldText, rAct.NewText)
	}
	jsPath := filepath.Join(project, "src", "ext.js")
	if err := os.MkdirAll(filepath.Dir(jsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsPath, []byte("const x = require('express');\nconst builtin = require('fs');\nconst rel = require('./local');\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.NetworkEnabled = false
	offNote := runMicaPostActionVerify(context.Background(), state, project, AgentAction{Action: "write_file", Path: "src/ext.js"}, cfg, 0)
	if !strings.Contains(offNote, "Verbotene externe Offline-Abhängigkeit") {
		t.Fatalf("expected offline dependency violation note, got %q", offNote)
	}
	if err := os.WriteFile(jsPath, []byte(strings.Repeat("var ok = 1;\n", 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	okNote := runMicaPostActionVerify(context.Background(), state, project, AgentAction{Action: "write_file", Path: "src/ext.js"}, cfg, 0)
	if !strings.Contains(okNote, "MICA ZERTIFIKAT") {
		t.Fatalf("expected MICA ZERTIFIKAT note, got %q", okNote)
	}
	verBadge, _, _, verDir := nomaNextStepPlan(project, "Create src/ext.js", map[string]bool{"src/ext.js": true}, true, false, 5, 70)
	if !strings.Contains(verBadge, "Verifikation") || !strings.Contains(verDir, "VERIFIKATIONS-STEUERUNG") {
		t.Fatalf("expected verification plan, got %q / %q", verBadge, verDir)
	}
	gateBadge, _, _, gateDir := nomaNextStepPlan(project, "Create src/ext.js", map[string]bool{"src/ext.js": true}, false, false, 6, 70)
	if !strings.Contains(gateBadge, "Abschluss-Gate") || !strings.Contains(gateDir, "ABSCHLUSS-FREIGABE") {
		t.Fatalf("expected completion gate plan, got %q / %q", gateBadge, gateDir)
	}
}

func TestBuildWorkspaceContractManifestCoversHTMLJSGoPyRust(t *testing.T) {
	project := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(project, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", `<!doctype html><html><body><canvas id="stage" width="640" height="480"></canvas><button id="startBtn">Start</button><script src="src/core.js"></script><script type="module" src="src/mod.mjs"></script></body></html>`)
	write("src/core.js", "class Engine {}\nfunction createState(seed) { return { seed }; }\nwindow.CoreAPI = { Engine, createState };\nexports.helper = () => 1;\nif (typeof module !== 'undefined' && module.exports) { module.exports = { Engine, createState }; }\n")
	write("src/mod.mjs", "export function boot() { return true; }\n")
	write("tools/calc.py", "class Solver:\n    pass\ndef run_calc(x):\n    return x\n")
	write("pkg/service.go", "package pkg\ntype Runner struct{}\nfunc RunAll() {}\n")
	write("crates/lib.rs", "pub struct Engine {}\npub fn tick() {}\n")
	write("node_modules/ignored/index.js", "window.ShouldNotAppear = 1;\n")

	manifest := buildWorkspaceContractManifest(project)
	for _, want := range []string{
		"NOMA LIVE WORKSPACE-MANIFEST",
		"index.html: scripts=[src/core.js, src/mod.mjs[module]] ids=[canvas#stage, button#startBtn]",
		"src/core.js:",
		"globals=[CoreAPI]",
		"exports=[Engine, createState, helper]",
		"class Engine",
		"createState(seed)",
		"src/mod.mjs: exports=[boot]",
		"tools/calc.py: defs=[Solver, run_calc]",
		"pkg/service.go: exported=[Runner, RunAll]",
		"crates/lib.rs: pub=[Engine, tick]",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "ShouldNotAppear") {
		t.Fatalf("manifest should ignore node_modules:\n%s", manifest)
	}
	if buildWorkspaceContractManifest(t.TempDir()) != "" {
		t.Fatal("expected empty manifest for empty directory")
	}
}

func TestDetectMicaRule7StubExtendedPatterns(t *testing.T) {
	cases := []struct {
		path    string
		code    string
		wantHit bool
	}{
		{"src/app.js", "function update(dt) {\n  // comment only\n}\n", true},
		{"src/app.js", "function noop() {}\nfunction step(x) { return x + 1; }\n", false},
		{"tests/app.test.js", "function dummyMock() {}\n", false},
		{"src/app.js", "function f() { throw new Error('Not implemented yet'); }\n", true},
		{"src/app.py", "def f():\n    raise NotImplementedError\n", true},
		{"src/lib.rs", "pub fn f() { todo!() }\n", true},
		{"src/app.js", "function f() {\n  // ... rest of logic\n  return 1;\n}\n", true},
		{"src/app.js", "function f(x) { return x * 2; }\n", false},
		{"README.md", "// TODO: docs\n", false},
	}
	for _, tc := range cases {
		got := detectMicaRule7Stub(tc.path, tc.code)
		if tc.wantHit && got == "" {
			t.Fatalf("expected stub detection for %s:\n%s", tc.path, tc.code)
		}
		if !tc.wantHit && got != "" {
			t.Fatalf("unexpected stub detection %q for %s:\n%s", got, tc.path, tc.code)
		}
	}
}

func TestVerifyWebAppIntegrityCatchesCommonWhiteScreenBugs(t *testing.T) {
	cfg := defaultConfig()
	cfg.NetworkEnabled = false

	// 1. Clean working browser app with dual-export + canvas draw calls passes static + Node vm smoke test.
	goodDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goodDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(goodDir, "style.css"), []byte("body { background: #111; }\n"), 0o600)
	_ = os.WriteFile(filepath.Join(goodDir, "index.html"), []byte(`<!doctype html><html><head><link rel="stylesheet" href="style.css"></head><body><canvas id="stage" width="320" height="240"></canvas><div id="hud">0</div><script src="src/core.js"></script><script src="src/app.js"></script></body></html>`), 0o600)
	_ = os.WriteFile(filepath.Join(goodDir, "src", "core.js"), []byte(`(function(root){
function createState() { return { score: 10 }; }
root.AppCore = { createState };
if (typeof module !== 'undefined' && module.exports) { module.exports = { createState }; }
})(typeof window !== 'undefined' ? window : globalThis);
`), 0o600)
	_ = os.WriteFile(filepath.Join(goodDir, "src", "app.js"), []byte(`document.addEventListener('DOMContentLoaded', () => {
  const canvas = document.getElementById('stage') || document.getElementById('fallback');
  const hud = document.querySelector('#hud');
  const dyn = document.createElement('div');
  dyn.id = 'dynNote';
  document.body.appendChild(dyn);
  const foundDyn = document.getElementById('dynNote');
  const ctx = canvas.getContext('2d');
  const st = window.AppCore.createState();
  hud.textContent = String(st.score) + (foundDyn ? '' : '!');
  function frame() {
    ctx.fillStyle = '#0f0';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
});
`), 0o600)
	if issues := verifyWebAppIntegrity(context.Background(), goodDir, cfg, true); len(issues) > 0 {
		t.Fatalf("expected clean web app to pass integrity + smoke check, got: %v", issues)
	}

	// 2. Static violations: external CDN, missing css/js, unguarded require(), bare export, missing DOM ID.
	badDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(badDir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(badDir, "index.html"), []byte(`<!doctype html><html><head>
<link rel="stylesheet" href="https://cdn.example.com/a.css">
<link rel="stylesheet" href="missing.css">
</head><body>
<canvas id="stage"></canvas>
<script src="https://cdn.example.com/lib.js"></script>
<script src="src/missing.js"></script>
<script src="src/bad_req.js"></script>
<script src="src/bad_esm.js"></script>
</body></html>`), 0o600)
	_ = os.WriteFile(filepath.Join(badDir, "src", "bad_req.js"), []byte("const core = require('./core');\nconst el = document.getElementById('nonExistentId');\n"), 0o600)
	_ = os.WriteFile(filepath.Join(badDir, "src", "bad_esm.js"), []byte("export function boot() { return 1; }\n"), 0o600)
	badIssues := strings.Join(verifyWebAppIntegrity(context.Background(), badDir, cfg, false), "\n")
	for _, needle := range []string{
		"externes Stylesheet",
		"fehlendes lokales Stylesheet",
		"externes Skript",
		"fehlendes lokales Skript",
		"ReferenceError: require is not defined",
		"ES-Modul-Syntax",
		"nonExistentId",
	} {
		if !strings.Contains(badIssues, needle) {
			t.Fatalf("expected issue containing %q, got:\n%s", needle, badIssues)
		}
	}

	// 3. Runtime ReferenceError during browser boot smoke test.
	crashDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(crashDir, "index.html"), []byte(`<!doctype html><html><body><canvas id="stage"></canvas><script src="boot.js"></script></body></html>`), 0o600)
	_ = os.WriteFile(filepath.Join(crashDir, "boot.js"), []byte("window.addEventListener('load', () => { undefinedSymbol(); });\n"), 0o600)
	crashIssues := strings.Join(verifyWebAppIntegrity(context.Background(), crashDir, cfg, true), "\n")
	if !strings.Contains(crashIssues, "ReferenceError") {
		t.Fatalf("expected runtime ReferenceError from smoke test, got:\n%s", crashIssues)
	}

	// 4. Blank canvas (0 draw calls) detected by smoke test.
	blankDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(blankDir, "index.html"), []byte(`<!doctype html><html><body><canvas id="stage"></canvas><script>const c = document.getElementById('stage'); c.getContext('2d');</script></body></html>`), 0o600)
	blankIssues := strings.Join(verifyWebAppIntegrity(context.Background(), blankDir, cfg, true), "\n")
	if !strings.Contains(blankIssues, "0 Zeichenaufrufe") {
		t.Fatalf("expected blank canvas warning from smoke test, got:\n%s", blankIssues)
	}

	// 5. Bundler project skips classic script require/ESM checks.
	viteDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(viteDir, "package.json"), []byte(`{"devDependencies":{"vite":"^5.0.0"}}`), 0o600)
	_ = os.WriteFile(filepath.Join(viteDir, "index.html"), []byte(`<!doctype html><html><body><div id="app"></div><script src="main.js"></script></body></html>`), 0o600)
	_ = os.WriteFile(filepath.Join(viteDir, "main.js"), []byte("import './x';\nexport const ok = true;\n"), 0o600)
	if viteIssues := verifyWebAppIntegrity(context.Background(), viteDir, cfg, true); len(viteIssues) > 0 {
		t.Fatalf("expected bundler project to skip classic script errors, got: %v", viteIssues)
	}
}

func TestNomaNextStepPlanDynamicPromptWithoutExplicitFilesAndStubFocus(t *testing.T) {
	project := t.TempDir()
	task := "Erstelle eine spielbare Web-Anwendung mit HTML5 Canvas und Tests."

	// 1. Empty workspace without explicit file paths emits dynamic architecture directive.
	badge, title, _, dir := nomaNextStepPlan(project, task, map[string]bool{}, false, false, 1, 60)
	if !strings.Contains(badge, "Architektur") || title == "" || !strings.Contains(dir, "DYNAMISCHE ARCHITEKTUR-STEUERUNG") {
		t.Fatalf("unexpected dynamic architecture directive: badge=%q title=%q dir=%q", badge, title, dir)
	}

	// 2. Non-edit intent without target files returns empty directive.
	_, _, _, readDir := nomaNextStepPlan(project, "Analysiere dieses Repository", map[string]bool{}, false, false, 1, 60)
	if readDir != "" {
		t.Fatalf("expected empty directive for read-only intent, got %q", readDir)
	}

	// 3. Pending stub file takes top priority.
	_ = os.MkdirAll(filepath.Join(project, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(project, "src", "core.js"), []byte("function step() {}\nwindow.Core = { step };\n"), 0o600)
	stubBadge, _, _, stubDir := nomaNextStepPlan(project, task, map[string]bool{"src/core.js": true}, true, false, 2, 60)
	if !strings.Contains(stubBadge, "Rule-7") || !strings.Contains(stubDir, "RULE-7 VERVOLLSTÄNDIGUNGS-STEUERUNG") || !strings.Contains(stubDir, "NOMA LIVE WORKSPACE-MANIFEST") {
		t.Fatalf("expected stub focus directive with manifest, got badge=%q dir=%q", stubBadge, stubDir)
	}

	// 4. After fixing stub, failed verification enters repair mode with live manifest.
	_ = os.WriteFile(filepath.Join(project, "src", "core.js"), []byte("function step(v) { return v + 1; }\nwindow.Core = { step };\n"), 0o600)
	_ = os.MkdirAll(filepath.Join(project, "tests"), 0o755)
	_ = os.WriteFile(filepath.Join(project, "tests", "core.test.js"), []byte("const test = require('node:test');\n"), 0o600)
	repBadge, _, _, repDir := nomaNextStepPlan(project, task, map[string]bool{"src/core.js": true}, true, true, 3, 60)
	if !strings.Contains(repBadge, "Reparatur") || !strings.Contains(repDir, "globals=[Core]") {
		t.Fatalf("expected repair mode with manifest, got badge=%q dir=%q", repBadge, repDir)
	}

	// 5. Changed since verification mentions discovered test file + manifest.
	progBadge, _, _, progDir := nomaNextStepPlan(project, task, map[string]bool{"src/core.js": true}, true, false, 4, 60)
	if !strings.Contains(progBadge, "Umsetzung") || !strings.Contains(progDir, "tests/core.test.js") || !strings.Contains(progDir, "globals=[Core]") {
		t.Fatalf("expected progress/verification directive with test file, got badge=%q dir=%q", progBadge, progDir)
	}

	// 6. Verified state still provides manifest.
	_, _, _, okDir := nomaNextStepPlan(project, task, map[string]bool{"src/core.js": true}, false, false, 5, 60)
	if !strings.Contains(okDir, "NOMA LIVE WORKSPACE-MANIFEST") {
		t.Fatalf("expected status directive with manifest, got %q", okDir)
	}
}

func TestNomaArchitectureBlueprintAndNodeTestDOMGuard(t *testing.T) {
	project := t.TempDir()
	longTask := "Erzeuge eine vollständige modulare Web-Anwendung mit HTML5 Canvas, Physik-Engine, Soundeffekten, Punkte-/Levelsystem und Offline-Unit-Tests."
	if !shouldDeriveNomaArchitectureBlueprint(project, longTask, classifyTaskIntent(longTask)) {
		t.Fatal("expected blueprint derivation for empty workspace with long plain edit prompt")
	}
	if shouldDeriveNomaArchitectureBlueprint(project, "Kurze Aufgabe", classifyTaskIntent("Kurze Aufgabe")) {
		t.Fatal("short prompt should not trigger blueprint derivation")
	}
	if shouldDeriveNomaArchitectureBlueprint(project, "Create src/engine.js and index.html with full tests and rendering logic.", classifyTaskIntent("Create src/engine.js")) {
		t.Fatal("prompt with explicit filenames should not trigger blueprint derivation")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"message":{"role":"assistant","content":"{\"summary\":\"Modular web app\",\"modules\":[{\"path\":\"tests/engine.test.js\",\"role\":\"Unit tests\",\"features\":[\"Verify state\"],\"exports\":[]},{\"path\":\"index.html\",\"role\":\"Entry HTML\",\"features\":[\"Canvas & HUD\"],\"exports\":[]},{\"path\":\"src/renderer.js\",\"role\":\"Canvas renderer\",\"features\":[\"Draw entities\"],\"exports\":[\"render\"]},{\"path\":\"src/engine.js\",\"role\":\"DOM-free core\",\"features\":[\"State machine\",\"Scoring\"],\"exports\":[\"createState\",\"step\"]},{\"path\":\"../escape.js\",\"role\":\"Bad\",\"features\":[\"Nope\"]}]}"}}`
		fmt.Fprintln(w, resp)
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	state := NewAppState(cfg, client)
	state.Project = project

	plan := state.nomaPlanProjectArchitecture(context.Background(), "qwen2.5-coder:14b", project, longTask, cfg)
	if plan == nil || len(plan.Modules) != 4 {
		t.Fatalf("expected 4 sanitized modules (escape.js filtered), got %#v", plan)
	}
	if plan.Modules[0].Path != "src/engine.js" || plan.Modules[1].Path != "src/renderer.js" || plan.Modules[2].Path != "index.html" || plan.Modules[3].Path != "tests/engine.test.js" {
		t.Fatalf("unexpected blueprint module order: %#v", plan.Modules)
	}
	badge, title, _, dir := nomaNextStepPlanWithBlueprint(project, longTask, plan, map[string]bool{}, false, false, 1, 60)
	if !strings.Contains(badge, "Modul 1/4") || !strings.Contains(title, "src/engine.js") || !strings.Contains(dir, "PFLICHT-FEATURES") || !strings.Contains(dir, "State machine") || !strings.Contains(dir, "createState, step") {
		t.Fatalf("unexpected blueprint directive: badge=%q title=%q dir=%q", badge, title, dir)
	}

	// Node test DOM guard catches unmocked document usage in tests, allows mocked or pure tests.
	if msg := detectNodeTestDOMViolation("tests/app.test.js", "const c = document.createElement('canvas');\n"); !strings.Contains(msg, "ReferenceError: document is not defined") {
		t.Fatalf("expected Node test DOM violation, got %q", msg)
	}
	if msg := detectNodeTestDOMViolation("tests/app.test.js", "global.document = {}; const c = document.createElement('canvas');\n"); msg != "" {
		t.Fatalf("expected mocked global.document to be allowed, got %q", msg)
	}
	if msg := detectNodeTestDOMViolation("src/app.js", "const c = document.createElement('canvas');\n"); msg != "" {
		t.Fatalf("browser source file should not trigger Node test DOM violation, got %q", msg)
	}
	badTest := filepath.Join(project, "tests", "bad.test.js")
	_ = os.MkdirAll(filepath.Dir(badTest), 0o755)
	_ = os.WriteFile(badTest, []byte("const c = document.createElement('canvas');\n"), 0o600)
	note := runMicaPostActionVerify(context.Background(), state, project, AgentAction{Action: "write_file", Path: "tests/bad.test.js"}, cfg, 0)
	if !strings.Contains(note, "document is not defined") {
		t.Fatalf("expected NODE_TEST_DOM_VIOLATION in post-action verify, got %q", note)
	}
	if isPathCompletedInRun(project, "tests/bad.test.js", map[string]bool{"tests/bad.test.js": true}) {
		t.Fatal("test file with Node DOM violation must not count as completed")
	}

	// Empty class methods & comment-only control blocks must be flagged by Rule 7.
	if hit := detectMicaRule7Stub("src/core.js", "class S {\n  initialize() {\n    // Setup initial state\n  }\n}\n"); !strings.Contains(hit, "leere Klassenmethode") {
		t.Fatalf("expected empty class method violation, got %q", hit)
	}
	if hit := detectMicaRule7Stub("src/core.js", "class S {\n  update() {\n    if (true) {\n      // Handle collision\n    }\n  }\n}\n"); !strings.Contains(hit, "Kommentar-Block") {
		t.Fatalf("expected comment-only block violation, got %q", hit)
	}

	// Fake test runner that catches errors and exits 0 must be caught.
	fakeTest := "function test(name, fn) { try { fn(); } catch (err) { console.error('FAIL: ' + err.message); } }\n"
	if msg := detectFakeNodeTestRunner("tests/fake.test.js", fakeTest); !strings.Contains(msg, "node:test") {
		t.Fatalf("expected fake test runner violation, got %q", msg)
	}
	if msg := detectFakeNodeTestRunner("tests/real.test.js", "const test = require('node:test');\nconst assert = require('node:assert/strict');\n"); msg != "" {
		t.Fatalf("real node:test file should not trigger fake runner violation, got %q", msg)
	}

	// Classic .js file with bare ES export without bundler or type:module must be flagged immediately.
	if msg := detectClassicScriptESMSyntax(project, "src/audio.js", "export const audio = {};\n"); !strings.Contains(msg, "ES-Modul-Syntax") {
		t.Fatalf("expected classic script ESM violation, got %q", msg)
	}
	// Missing local media asset (.mp3) referenced by browser script must be caught by verifyWebAppIntegrity.
	mediaDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(mediaDir, "index.html"), []byte(`<!doctype html><html><body><canvas id="c"></canvas><script src="app.js"></script></body></html>`), 0o600)
	_ = os.WriteFile(filepath.Join(mediaDir, "app.js"), []byte(`const url = 'sounds/jump.mp3'; document.getElementById('c').getContext('2d').fillRect(0,0,1,1);`), 0o600)
	mediaIssues := strings.Join(verifyWebAppIntegrity(context.Background(), mediaDir, cfg, false), "\n")
	if !strings.Contains(mediaIssues, "sounds/jump.mp3") {
		t.Fatalf("expected missing local media asset warning, got %q", mediaIssues)
	}

	// Class methods & constructor fields must appear in Live Workspace Manifest.
	clsDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(clsDir, "engine.js"), []byte("class Engine {\n  constructor() {\n    this.score = 0;\n    this.lives = 3;\n  }\n  step(dt) {\n    this.score += dt;\n  }\n}\nwindow.Engine = Engine;\n"), 0o600)
	clsManifest := buildWorkspaceContractManifest(clsDir)
	for _, want := range []string{"methods=[.step(dt)]", "fields=[score, lives]"} {
		if !strings.Contains(clsManifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, clsManifest)
		}
	}

	// Bracketed placeholder description in write_file must be rejected for non-JSON files.
	if err := validateAgentAction(AgentAction{Action: "write_file", Path: "src/engine.js", Content: "[complete and valid JavaScript code for src/engine.js]"}); err == nil {
		t.Fatal("expected bracketed placeholder write_file content to be rejected")
	}
	if err := validateAgentAction(AgentAction{Action: "write_file", Path: "data.json", Content: "[1, 2, 3]"}); err != nil {
		t.Fatalf("valid JSON array file should not be rejected: %v", err)
	}

	// Stub context excerpt must include line numbers around detected stub lines.
	exDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(exDir, "a.js"), []byte("line1();\nline2();\n// Placeholder\nline4();\n"), 0o600)
	exHit := detectMicaRule7Stub("a.js", "line1();\nline2();\n// Placeholder\nline4();\n")
	excerpt := extractStubContextExcerpt(exDir, "a.js", exHit)
	if !strings.Contains(excerpt, "3: // Placeholder") || !strings.Contains(excerpt, "2: line2();") {
		t.Fatalf("unexpected stub context excerpt: %q", excerpt)
	}

	// Unguarded window assignment in dual-export module must be caught; guarded must pass.
	if msg := detectUnguardedWindowInDualModule("src/engine.js", "module.exports = { a: 1 };\nwindow.App = { a: 1 };\n"); !strings.Contains(msg, "ReferenceError: window is not defined") {
		t.Fatalf("expected unguarded window violation, got %q", msg)
	}
	if msg := detectUnguardedWindowInDualModule("src/engine.js", "module.exports = { a: 1 };\nif (typeof window !== 'undefined') {\n  window.App = { a: 1 };\n}\n"); msg != "" {
		t.Fatalf("guarded window assignment should pass, got %q", msg)
	}

	// Jest API hallucinations in node:test files must be caught.
	if msg := detectJestAPIInNodeTest(project, "tests/engine.test.js", "const fn = jest.fn();\n"); !strings.Contains(msg, "Jest-API") {
		t.Fatalf("expected Jest API violation, got %q", msg)
	}
	if msg := detectJestAPIInNodeTest(project, "tests/engine.test.js", "const test = require('node:test');\nconst assert = require('node:assert/strict');\n"); msg != "" {
		t.Fatalf("clean node:test file should pass, got %q", msg)
	}
}

func TestNomaWebResearchAndWorldModuleOrderingAndRetryTemperature(t *testing.T) {
	if got := temperatureForContext(context.Background(), 0.1); got != 0.1 {
		t.Fatalf("default temperature = %v, want 0.1", got)
	}
	if got := temperatureForContext(withRetryTemperature(context.Background(), 0.35), 0.1); got != 0.35 {
		t.Fatalf("retry temperature = %v, want 0.35", got)
	}

	// Mock DuckDuckGo HTML endpoint & webHTTPClient for deterministic offline test.
	ddgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<a class="result__a" href="https://example.org/spec">Reference Layout Specification</a><a class="result__snippet">Six tilted platforms connected by ladders with top goal platform and rolling obstacles.</a>`)
	}))
	defer ddgSrv.Close()
	origDDG := duckDuckGoSearchEndpoint
	origClient := webHTTPClient
	duckDuckGoSearchEndpoint = ddgSrv.URL
	webHTTPClient = func(timeout time.Duration, maxRedirects int) *http.Client { return ddgSrv.Client() }
	defer func() {
		duckDuckGoSearchEndpoint = origDDG
		webHTTPClient = origClient
	}()

	callIdx := 0
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callIdx++
		w.Header().Set("Content-Type", "application/json")
		if callIdx == 1 {
			fmt.Fprintln(w, `{"message":{"role":"assistant","content":"{\"query\":\"classic platformer arcade layout rules\"}"}}`)
			return
		}
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"{\"summary\":\"Split world+engine web app\",\"modules\":[{\"path\":\"tests/engine.test.js\",\"role\":\"Tests\",\"features\":[\"Verify physics\"]},{\"path\":\"src/engine.js\",\"role\":\"Core simulation\",\"features\":[\"Physics\",\"Scoring\"]},{\"path\":\"src/world.js\",\"role\":\"Layout & constants\",\"features\":[\"Tilted platforms\",\"Ladders\"]},{\"path\":\"index.html\",\"role\":\"Entry\",\"features\":[\"Canvas\"]}]}"}}`)
	}))
	defer ollamaSrv.Close()

	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.NetworkEnabled = true
	cfg.WebSearchProvider = "duckduckgo"
	client := NewOllamaClient()
	client.BaseURL = ollamaSrv.URL
	state := NewAppState(cfg, client)
	state.Project = project

	task := "Erzeuge eine vollständige modulare Web-Anwendung mit geneigten Plattformen, Leitern, Physik, Punkten und Unit-Tests."
	plan := state.nomaPlanProjectArchitecture(context.Background(), "qwen2.5-coder:14b", project, task, cfg)
	if plan == nil || len(plan.Modules) != 4 {
		t.Fatalf("expected 4 planned modules, got %#v", plan)
	}
	if plan.Modules[0].Path != "src/world.js" || plan.Modules[1].Path != "src/engine.js" {
		t.Fatalf("expected src/world.js before src/engine.js, got %#v", plan.Modules)
	}
	if !strings.Contains(plan.ResearchBrief, "Reference Layout Specification") {
		t.Fatalf("expected researched reference brief in plan, got %q", plan.ResearchBrief)
	}
	_, _, _, dir := nomaNextStepPlanWithBlueprint(project, task, plan, map[string]bool{}, false, false, 1, 60)
	if !strings.Contains(dir, "NOMA WEB-RECHERCHE-REFERENZ") {
		t.Fatalf("expected web research reference injected into module 1 directive, got %q", dir)
	}

	// Default CommonJS export must appear as exportDefault=Foo (distinct from destructured exports=[Foo]).
	expDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(expDir, "world.js"), []byte("class World {}\nif (typeof window !== 'undefined') window.World = World;\nif (typeof module !== 'undefined' && module.exports) module.exports = World;\n"), 0o600)
	expManifest := buildWorkspaceContractManifest(expDir)
	if !strings.Contains(expManifest, "exportDefault=World") {
		t.Fatalf("expected exportDefault=World in manifest, got:\n%s", expManifest)
	}

	// Unguarded require() inside a dual-export browser module must be flagged immediately.
	if msg := detectUnguardedRequireInDualModule(expDir, "src/engine.js", "const World = require('./world');\nif (typeof window !== 'undefined') window.Engine = {};\n"); !strings.Contains(msg, "ReferenceError: require is not defined") {
		t.Fatalf("expected unguarded dual-module require violation, got %q", msg)
	}
	if msg := detectUnguardedRequireInDualModule(expDir, "src/engine.js", "const World = (typeof module !== 'undefined' && module.exports) ? require('./world') : window.World;\n"); msg != "" {
		t.Fatalf("guarded dual-module require should pass, got %q", msg)
	}
}

func TestExactSceneMCPServerDefaultAndAttachmentHelpers(t *testing.T) {
	found := false
	for _, srv := range defaultMCPServers() {
		if srv.Name == "exactscene" {
			found = true
			if srv.Transport != "stdio" || srv.Enabled {
				t.Fatalf("exactscene MCP server must be opt-in stdio, got %+v", srv)
			}
			if len(srv.Args) < 2 || srv.Args[0] != "-m" || srv.Args[1] != "exactscene.mcp_server" {
				t.Fatalf("unexpected exactscene MCP args: %v", srv.Args)
			}
		}
	}
	if !found {
		t.Fatal("defaultMCPServers() missing exactscene entry")
	}

	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	if got := runExactSceneWebImageReference(context.Background(), t.TempDir(), cfg, "test diagram"); got != "" {
		t.Fatalf("expected empty web image reference when network disabled, got %q", got)
	}
	if got := runExactSceneAttachmentAnalysis(context.Background(), t.TempDir(), cfg, "", nil); got != "" {
		t.Fatalf("expected empty attachment analysis for nil images, got %q", got)
	}
	env := exactScenePythonEnv(t.TempDir())
	hasUTF8 := false
	for _, kv := range env {
		if kv == "PYTHONIOENCODING=utf-8" {
			hasUTF8 = true
			break
		}
	}
	if !hasUTF8 {
		t.Fatal("exactScenePythonEnv missing PYTHONIOENCODING=utf-8")
	}
}
