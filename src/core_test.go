// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEnsureWithinRoot(t *testing.T) {
	root := t.TempDir()
	got, err := ensureWithinRoot(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("got %q want %q", got, root)
	}

	got, err = ensureWithinRoot(root, filepath.Join("a", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, root) {
		t.Fatalf("path escaped root: %q", got)
	}

	if _, err := ensureWithinRoot(root, filepath.Join("..", "outside.txt")); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestWriteAndReplaceFile(t *testing.T) {
	root := t.TempDir()
	if _, err := writeProjectFile(root, "x.txt", "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	diff, err := replaceText(root, "x.txt", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+three") {
		t.Fatalf("unexpected diff: %s", diff)
	}
	data, err := os.ReadFile(filepath.Join(root, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nthree\n" {
		t.Fatalf("unexpected file: %q", data)
	}
}

func TestFileActionsReportPostconditions(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	writeOut, err := executeAction(context.Background(), root, cfg, AgentAction{Action: "write_file", Path: "x.txt", Content: "one"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"POSTCONDITION", "target: exists=true", "sha256="} {
		if !strings.Contains(writeOut, want) {
			t.Fatalf("write postcondition missing %q in: %s", want, writeOut)
		}
	}
	copyOut, err := executeAction(context.Background(), root, cfg, AgentAction{Action: "copy_path", Source: "x.txt", Destination: "copy.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"source: exists=true", "destination: exists=true", "sha256="} {
		if !strings.Contains(copyOut, want) {
			t.Fatalf("copy postcondition missing %q in: %s", want, copyOut)
		}
	}
	moveOut, err := executeAction(context.Background(), root, cfg, AgentAction{Action: "move_path", Source: "copy.txt", Destination: "moved.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"source: exists=false", "destination: exists=true"} {
		if !strings.Contains(moveOut, want) {
			t.Fatalf("move postcondition missing %q in: %s", want, moveOut)
		}
	}
	deleteOut, err := executeAction(context.Background(), root, cfg, AgentAction{Action: "delete_file", Path: "moved.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deleteOut, "target: exists=false") {
		t.Fatalf("delete postcondition missing target removal: %s", deleteOut)
	}
}

func TestProjectTreeAndSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n// needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "x", "skip.js"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := projectTree(root, "", 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tree, "src/main.go") {
		t.Fatalf("missing file: %s", tree)
	}
	if strings.Contains(tree, "node_modules") {
		t.Fatalf("ignored directory included: %s", tree)
	}

	hits, err := searchProject(root, "needle", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hits, "src/main.go:2") {
		t.Fatalf("missing hit: %s", hits)
	}
	if strings.Contains(hits, "skip.js") {
		t.Fatalf("ignored hit included: %s", hits)
	}
}

func TestParseAgentAction(t *testing.T) {
	a, err := parseAgentAction(`{"action":"read_file","message":"Lese Datei","path":"README.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Action != "read_file" || a.Path != "README.md" {
		t.Fatalf("unexpected action: %#v", a)
	}
	a, err = parseAgentAction(`{"action":"write_file","message":"Schreibe Datei","arguments":{"path":"index.html","content":"<main>ok</main>"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Path != "index.html" || a.Content != "<main>ok</main>" {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"write_file","message":"Leere Datei","path":"index.html"}`); err == nil {
		t.Fatal("write_file without content must be rejected")
	}
	a, err = parseAgentAction(`{"action":"skill_read","message":"Lies Skill","arguments":{"skill":"python-test"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Skill != "python-test" {
		t.Fatalf("skill argument was not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"skill_read","message":"Lies Skill"}`); err == nil {
		t.Fatal("skill_read without skill must be rejected")
	}
	a, err = parseAgentAction(`{"action":"skill_read_resource","message":"Lies Ressource","arguments":{"skill":"asset-skill","resource":"references/palette.md"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Skill != "asset-skill" || a.Resource != "references/palette.md" {
		t.Fatalf("skill resource arguments were not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"skill_read_resource","message":"Lies Ressource","skill":"asset-skill"}`); err == nil {
		t.Fatal("skill_read_resource without resource must be rejected")
	}
	a, err = parseAgentAction(`{"action":"skill_copy_resource","message":"Kopiere Ressource","arguments":{"skill":"asset-skill","resource":"assets/icon.png","destination":"public/icon.png"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Skill != "asset-skill" || a.Resource != "assets/icon.png" || a.Destination != "public/icon.png" {
		t.Fatalf("skill copy arguments were not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"skill_copy_resource","message":"Kopiere Ressource","skill":"asset-skill","resource":"assets/icon.png"}`); err == nil {
		t.Fatal("skill_copy_resource without destination must be rejected")
	}
	a, err = parseAgentAction(`{"action":"skill_run_script","message":"Starte Skill-Skript","skill":"asset-skill","script":"scripts/build.ps1","args":["--fast"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Skill != "asset-skill" || a.Script != "scripts/build.ps1" || len(a.Args) != 1 || a.Args[0] != "--fast" {
		t.Fatalf("skill run arguments were not parsed: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"skill_run_script","message":"Starte Skill-Skript","skill":"asset-skill"}`); err == nil {
		t.Fatal("skill_run_script without script must be rejected")
	}
	a, err = parseAgentAction(`{"action":"subagent_analyze","message":"Analysiere","arguments":{"task":"Untersuche src/main.go"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Task != "Untersuche src/main.go" {
		t.Fatalf("subagent task argument was not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"subagent_analyze","message":"Analysiere"}`); err == nil {
		t.Fatal("subagent_analyze without task must be rejected")
	}
	if actionNeedsApproval(defaultConfig(), t.TempDir(), AgentAction{Action: "subagent_analyze", Task: "check"}) {
		t.Fatal("subagent_analyze must be read-only")
	}
	a, err = parseAgentAction(`{"action":"command_read","message":"Lies Command","arguments":{"name":"review"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "review" {
		t.Fatalf("command_read name argument was not normalized: %#v", a)
	}
	if _, err := parseAgentAction(`{"action":"command_read","message":"Lies Command"}`); err == nil {
		t.Fatal("command_read without name must be rejected")
	}
	if actionNeedsApproval(defaultConfig(), t.TempDir(), AgentAction{Action: "command_read", Name: "review"}) {
		t.Fatal("command_read must be read-only")
	}
}

func TestProjectCommandsListReadAndExpand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	mustWrite(t, filepath.Join(root, ".localcode", "commands", "review.md"), "---\ndescription: Review workflow\n---\nBitte prüfe {{args}} in {{project}} unter {{cwd}}.")
	mustWrite(t, filepath.Join(root, ".codex", "commands", "review.md"), "shadowed")
	mustWrite(t, filepath.Join(root, ".opencode", "commands", "plan.txt"), "Plane {{args}}.")

	commands := listProjectCommands(root)
	if len(commands) != 2 {
		t.Fatalf("unexpected command count %d: %#v", len(commands), commands)
	}
	byName := map[string]ProjectCommand{}
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}
	if byName["review"].Description != "Review workflow" || !strings.Contains(byName["review"].Source, ".localcode/commands/review.md") {
		t.Fatalf("unexpected review command: %#v", byName["review"])
	}
	if byName["plan"].Name != "plan" || byName["plan"].Scope != "project" {
		t.Fatalf("unexpected plan command: %#v", byName["plan"])
	}
	listOut := formatProjectCommandList(root)
	if !strings.Contains(listOut, "/review") || !strings.Contains(listOut, "/plan") {
		t.Fatalf("list output missing commands:\n%s", listOut)
	}
	read, err := readProjectCommand(root, "review")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read.Body, "shadowed") {
		t.Fatal("project command precedence loaded a shadowed command")
	}
	expanded, cmd, ok, err := expandSlashCommandPrompt(root, "/review src/main.go")
	if err != nil || !ok || cmd == nil {
		t.Fatalf("expand ok=%t cmd=%#v err=%v", ok, cmd, err)
	}
	for _, want := range []string{"SLASH COMMAND /review", "Arguments: src/main.go", "Bitte prüfe src/main.go", filepath.Base(root)} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded command missing %q:\n%s", want, expanded)
		}
	}
}

func TestProjectCommandReadTool(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	mustWrite(t, filepath.Join(root, ".localcode", "commands", "fix.md"), "---\ndescription: Fix workflow\n---\nBehebe {{args}}.")
	state := &AppState{Config: defaultConfig()}
	list, wait := state.handleAgentAction(context.Background(), root, AgentAction{Action: "command_list", Message: "Liste Commands"})
	if wait || !strings.Contains(list, "/fix") {
		t.Fatalf("unexpected command_list wait=%t output=\n%s", wait, list)
	}
	read, wait := state.handleAgentAction(context.Background(), root, AgentAction{Action: "command_read", Message: "Lies Command", Name: "fix"})
	if wait || !strings.Contains(read, "COMMAND /fix") || !strings.Contains(read, "Behebe {{args}}.") {
		t.Fatalf("unexpected command_read wait=%t output=\n%s", wait, read)
	}
}

func TestReadOnlySubagentHandoff(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "src", "main.go")
	before := []byte("package main\n// failing parser path\nfunc main() {}\n")
	if err := os.WriteFile(mainPath, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo\nparser notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := runReadOnlySubagent(root, defaultConfig(), "Untersuche failing parser in src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"READ-ONLY SUBAGENT HANDOFF", "Mode: read-only", "src/main.go", "failing parser", "SEARCH EVIDENCE"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing %q in report:\n%s", want, report)
		}
	}
	after, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("read-only subagent modified project file")
	}
}

func TestAgentToolHooksRunConfiguredCommands(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 15
	cfg.HookBeforeTool = "echo before-hook"
	cfg.HookAfterTool = "echo after-hook"
	before, err := runAgentToolHook(context.Background(), root, cfg, "before", AgentAction{Action: "read_file", Path: "README.md", Message: "read"})
	if err != nil || !strings.Contains(before, "before-hook") {
		t.Fatalf("before hook output=%q err=%v", before, err)
	}
	after, err := runAgentToolHook(context.Background(), root, cfg, "after", AgentAction{Action: "read_file", Path: "README.md", Message: "read"})
	if err != nil || !strings.Contains(after, "after-hook") {
		t.Fatalf("after hook output=%q err=%v", after, err)
	}
	if agentToolHookEligible(AgentAction{Action: "finish"}) || agentToolHookEligible(AgentAction{Action: "ask_user"}) {
		t.Fatal("finish and ask_user must not trigger tool hooks")
	}
}

func TestCompletionGuardBlocksPlaceholderAndUnverifiedCapabilities(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "game.js"), []byte("// placeholder\nlet score = 0;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	intent := classifyTaskIntent("Baue game.js mit Pause, Restart und teste ob es funktioniert.")
	issues := completionGuardIssues(project, intent, intent.OriginalTask, map[string]bool{"game.js": true}, true)
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"Platzhalter", "keine passende Prüfung", "Pause", "Neustart"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing issue %q in: %s", want, joined)
		}
	}
}

func TestCompletionGuardAllowsVerifiedConcreteEdit(t *testing.T) {
	project := t.TempDir()
	content := `let paused = false;
let score = 0;
let lives = 3;
function pauseGame() { paused = true; }
function restartGame() { score = 0; lives = 3; }
function showGameOver() { return "game over"; }
function showWinState() { return "win"; }
`
	if err := os.WriteFile(filepath.Join(project, "game.js"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Baue game.js mit Pause, Restart, Game over, Win-State, Score und Leben und teste ob es funktioniert."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"game.js": true}, false)
	if len(issues) != 0 {
		t.Fatalf("unexpected completion issues: %v", issues)
	}
}

func TestTaskQualityHintAppliesToGeneralImplementationTasks(t *testing.T) {
	hint := taskQualityHint("Erstelle eine kleine Browser-App mit Button und Tastatursteuerung.")
	for _, want := range []string{"INTERNE QUALITÄTSANFORDERUNGEN", "sprach- und projektunabhängig", "Button", "Dateioperationen", "Bild"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("quality hint missing %q in: %s", want, hint)
		}
	}
	if strings.Contains(hint, "Pac-Man-Klon") {
		t.Fatalf("general app hint must not inject Pac-Man-specific requirements: %s", hint)
	}
}

func TestImplementationIntentCoversFileAndVisualTasks(t *testing.T) {
	for _, task := range []string{
		"Kopiere diese Datei nach assets/output.svg.",
		"Male ein altes Schloss als SVG.",
		"Move config.json into backup/config.json.",
	} {
		if got := classifyTaskIntent(task); got.Kind != "edit" {
			t.Fatalf("classifyTaskIntent(%q).Kind = %q, want edit", task, got.Kind)
		}
	}
}

func TestMentionedProjectFilesIncludesVisualAssets(t *testing.T) {
	got := mentionedProjectFiles("Male ein altes Schloss als assets/castle.svg und exportiere preview.webp.")
	want := []string{"assets/castle.svg", "preview.webp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mentionedProjectFiles() = %#v, want %#v", got, want)
	}
}

func TestSupervisorBlocksUnrequestedGlobalInstalls(t *testing.T) {
	intent := classifyTaskIntent("Baue eine lokale Browser-App und pruefe ob sie funktioniert.")
	allowed, hint := actionAllowedForIntent(intent, AgentAction{Action: "run_command", Command: "npm install -g playwright"})
	if allowed {
		t.Fatal("unrequested global install was allowed")
	}
	if !strings.Contains(hint, "Globale Installationen") {
		t.Fatalf("unexpected hint: %s", hint)
	}
	intent = classifyTaskIntent("Installiere Playwright global.")
	allowed, _ = actionAllowedForIntent(intent, AgentAction{Action: "run_command", Command: "npm install -g playwright"})
	if !allowed {
		t.Fatal("explicitly requested global install was blocked")
	}
}

func TestCompletionGuardBlocksMissingGeneralInteractiveLogic(t *testing.T) {
	project := t.TempDir()
	content := `<main><button id="start">Start</button><p>Fertig</p></main>`
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Erstelle eine Browser-App mit Button und Tastatursteuerung und teste ob sie funktioniert."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"index.html": true}, false)
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"Button-Interaktion", "Tastatursteuerung"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing issue %q in: %s", want, joined)
		}
	}
}

func TestCompletionGuardRequiresConcreteVisualArtifactFile(t *testing.T) {
	project := t.TempDir()
	readme := "# Bild\n\n<svg viewBox=\"0 0 10 10\"><rect width=\"10\" height=\"10\"/></svg>\n"
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Male ein altes Schloss als Bild."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"README.md": true}, false)
	if !strings.Contains(strings.Join(issues, "\n"), "keine konkrete Artefaktdatei") {
		t.Fatalf("README-only visual task was not blocked: %v", issues)
	}
	if err := os.MkdirAll(filepath.Join(project, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	svg := `<svg viewBox="0 0 100 80"><rect width="100" height="80"/><path d="M10 70 L50 20 L90 70 Z"/></svg>`
	if err := os.WriteFile(filepath.Join(project, "assets", "castle.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	issues = completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"assets/castle.svg": true}, false)
	if len(issues) != 0 {
		t.Fatalf("concrete SVG artifact was blocked: %v", issues)
	}
}

func TestCompletionReviewBlocksDocsOnlyImplementation(t *testing.T) {
	project := t.TempDir()
	readme := "# Reparatur\n\nDer Button, Event-Handler und Status wurden beschrieben.\n"
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Repariere die Browser-App mit Button und Status."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"README.md": true}, false)
	if !strings.Contains(strings.Join(issues, "\n"), "Abschluss-Review") {
		t.Fatalf("docs-only implementation was not blocked: %v", issues)
	}
}

func TestCompletionReviewRequiresVerificationForCodeChanges(t *testing.T) {
	project := t.TempDir()
	content := `<main><button id="start">Start</button><p id="status">Bereit</p><script>
let state = "ready";
document.getElementById("start").addEventListener("click", () => {
  state = "running";
  document.getElementById("status").textContent = state;
});
</script></main>`
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Baue eine Browser-App mit Button und Status."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"index.html": true}, true)
	if !strings.Contains(strings.Join(issues, "\n"), "Prüfung nach der letzten Änderung") {
		t.Fatalf("unverified code change was not blocked: %v", issues)
	}
	issues = completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"index.html": true}, false)
	if len(issues) != 0 {
		t.Fatalf("verified code change was blocked: %v", issues)
	}
}

func TestCompletionReviewAllowsDocumentationAndFileOperationTasks(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Nutzung\n\nAktualisiert.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docTask := "Aktualisiere die README-Dokumentation."
	if issues := completionGuardIssues(project, classifyTaskIntent(docTask), docTask, map[string]bool{"README.md": true}, true); len(issues) != 0 {
		t.Fatalf("documentation-only task was blocked: %v", issues)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "README.txt"), []byte("Aktualisiert.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyTask := "Kopiere README.md nach docs/README.txt."
	if issues := completionGuardIssues(project, classifyTaskIntent(copyTask), copyTask, map[string]bool{"docs/README.txt": true}, true); len(issues) != 0 {
		t.Fatalf("pure file operation task was blocked: %v", issues)
	}
}

func TestCompletionRepairDirectivePrioritizesRuntimeLogic(t *testing.T) {
	directive := completionRepairDirective("Baue eine spielbare Browser-App.", []string{"Button fehlt", "Status fehlt"})
	for _, want := range []string{"Laufzeitlogik", "Abschluss-Review", "Event-Handler", "installiere nicht reflexhaft global", "Kernmechanik"} {
		if !strings.Contains(directive, want) {
			t.Fatalf("directive missing %q in: %s", want, directive)
		}
	}
}

func TestCompletionGuardDoesNotTreatNodeJSAsProjectFile(t *testing.T) {
	project := t.TempDir()
	content := "function pauseGame(){}\nfunction restartGame(){}\nlet score = 0;\nlet lives = 3;\n"
	if err := os.WriteFile(filepath.Join(project, "game.js"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Prüfe danach tatsächlich die JavaScript-Syntax von game.js mit Node.js."
	issues := completionGuardIssues(project, classifyTaskIntent("Baue game.js. "+task), "Baue game.js. "+task, map[string]bool{"game.js": true}, false)
	for _, issue := range issues {
		if strings.Contains(issue, "Node.js") {
			t.Fatalf("Node.js was treated as a project file: %v", issues)
		}
	}
}

func TestCompletionGuardEnforcesSingleOutputFile(t *testing.T) {
	project := t.TempDir()
	index := `<canvas id="game"></canvas><button id="restartButton">Restart</button><script>
const MAZE = [
"#################",
"#P....#....#...G#",
"#.###.#.##.#.##.#",
"#.....#....#....#",
"#.###.####.###..#",
"#...............#",
"#.##.###.###.##.#",
"#....#G....#....#",
"###..#.###.#..###",
"#....#.....#....#",
"#.##.###.###.##.#",
"#...............#",
"#.###.####.###..#",
"#G....#....#....#",
"#################",
];
let maze = MAZE.map(row => row.split(""));
let pelletsLeft = 10;
let score = 0;
let gameState = "playing";
function collectPellet(){ pelletsLeft--; score += 10; }
function isWallAtTile(row, col){ return row < 0 || col < 0 || MAZE[row][col] === "#"; }
const ghostStarts = [{x:1,y:1},{x:2,y:2}];
const restartButton = document.getElementById("restartButton");
restartButton.addEventListener("click", restartGame);
document.addEventListener("keydown", event => { switch(event.key.toLowerCase()){ case "w": break; case "arrowup": break; } });
function restartGame(){ maze = MAZE.map(row => row.split("")); gameState = "playing"; }
</script>`
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "game.js"), []byte("console.log('extra');"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Erstelle einen vollständigen Pac-Man-Klon als einzelne Datei index.html."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"index.html": true, "game.js": true}, false)
	if !strings.Contains(strings.Join(issues, "\n"), "einzelne Ausgabedatei") {
		t.Fatalf("extra runtime file was not blocked: %v", issues)
	}
	if err := os.Remove(filepath.Join(project, "game.js")); err != nil {
		t.Fatal(err)
	}
	issues = completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"index.html": true, "game.js": true}, false)
	for _, issue := range issues {
		if strings.Contains(issue, "game.js") {
			t.Fatalf("deleted extra file was still blocked: %v", issues)
		}
	}
}

func TestMentionedProjectFilesSplitsSlashSeparatedFileList(t *testing.T) {
	got := mentionedProjectFiles("Passe index.html/styles.css/README.md an, aber behalte src/agent.go.")
	want := []string{"README.md", "index.html", "src/agent.go", "styles.css"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mentionedProjectFiles() = %#v, want %#v", got, want)
	}
}

func TestCommandLooksLikeNodeSyntaxVerification(t *testing.T) {
	for _, command := range []string{"node -c game.js", "node.exe --check game.js"} {
		if !commandLooksLikeVerification(command) {
			t.Fatalf("%q was not recognized as verification", command)
		}
	}
}

func TestCommandLooksLikeLanguageAgnosticVerification(t *testing.T) {
	for _, command := range []string{
		"go test ./...",
		"python -m py_compile app.py",
		"cargo check",
		"javac Main.java",
		"dotnet test",
		"mvn test",
		"gradle build",
		"php -l index.php",
		"ruby -c app.rb",
		"bash -n script.sh",
		"gcc -fsyntax-only main.c",
	} {
		if !commandLooksLikeVerification(command) {
			t.Fatalf("%q was not recognized as verification", command)
		}
	}
}

func TestReadFileVerificationRequiresExplicitFileCheckTask(t *testing.T) {
	action := AgentAction{Action: "read_file", Path: "index.html"}
	if !actionVerifiesProject(action, "Prüfe danach, dass index.html existiert und die Spiellogik enthalten ist.") {
		t.Fatal("explicit file/content check should allow read_file as verification")
	}
	if actionVerifiesProject(action, "Baue eine Browser-App und teste ob sie funktioniert.") {
		t.Fatal("generic functional test must not be satisfied by read_file")
	}
}

func TestCompletionGuardBlocksIncompletePacManImplementation(t *testing.T) {
	project := t.TempDir()
	content := `const TILE_SIZE = 16;
const MAZE = ['###', '#.#', '###'];
const GHOSTS = [{ x: 1, y: 1 }, { x: 1, y: 1 }, { x: 1, y: 1 }];
let score = 0;
function isValidMove() { return true; }
document.addEventListener('keydown', (event) => {
  if (event.key === 'p') {
    // Pause logic can be added here
  }
});
`
	if err := os.WriteFile(filepath.Join(project, "game.js"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task := "Erstelle einen Pac-Man-Klon mit Labyrinth, Pellets, Geistern, Kollisionen, Score, Pause und teste mit Node.js."
	issues := completionGuardIssues(project, classifyTaskIntent(task), task, map[string]bool{"game.js": true}, false)
	joined := strings.Join(issues, "\n")
	for _, want := range []string{"can be added", "Pellets", "Score durch Spielaktionen"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing issue %q in: %s", want, joined)
		}
	}
}

func TestPacManGuardAcceptsEquivalentMazeArrayNames(t *testing.T) {
	content := `const levelLayout = [
"#################",
"#P....#....#...G#",
"#.###.#.##.#.##.#",
"#.....#....#....#",
"#.###.####.###..#",
"#...............#",
"#.##.###.###.##.#",
"#....#G....#....#",
"###..#.###.#..###",
"#....#.....#....#",
"#.##.###.###.##.#",
"#...............#",
"#.###.####.###..#",
"#G....#....#....#",
"#################",
];
let maze = levelLayout.map(row => row.split(""));
let pelletsRemaining = 10;
let score = 0;
let gameState = "playing";
function collectPellet(){ pelletsRemaining -= 1; score += 10; }
function isWallAtTile(row, col){ return row < 0 || col < 0 || levelLayout[row][col] === "#"; }
const ghostStarts = [{x:1,y:1},{x:2,y:2}];
const restartButton = document.getElementById("restartButton");
restartButton.addEventListener("click", restartGame);
document.addEventListener("keydown", event => { switch(event.key.toLowerCase()){ case "w": break; case "arrowup": break; } });
function restartGame(){ maze = levelLayout.map(row => row.split("")); gameState = "playing"; }`
	issues := pacManImplementationIssues(content)
	if len(issues) != 0 {
		t.Fatalf("equivalent maze implementation was blocked: %v", issues)
	}
}

func TestPacManGuardAcceptsNumericMazeGrid(t *testing.T) {
	content := `const levelGrid = [
[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],
[1,2,0,0,0,0,1,0,0,0,0,0,0,3,1],
[1,0,1,1,1,0,1,0,1,1,1,0,1,0,1],
[1,0,0,0,0,0,0,0,0,0,0,0,0,0,1],
[1,0,1,1,0,1,1,1,1,1,0,1,1,0,1],
[1,0,0,0,0,0,0,0,0,0,0,0,0,0,1],
[1,0,1,0,1,1,0,1,0,1,1,0,1,0,1],
[1,0,0,0,1,3,0,0,0,0,1,0,0,0,1],
[1,1,1,0,1,0,1,1,1,0,1,0,1,1,1],
[1,0,0,0,0,0,0,0,0,0,0,0,0,0,1],
[1,0,1,1,0,1,1,1,1,1,0,1,1,0,1],
[1,0,0,0,0,0,0,0,0,0,0,0,0,0,1],
[1,0,1,1,1,0,1,0,1,1,1,0,1,0,1],
[1,3,0,0,0,0,1,0,0,0,0,0,0,0,1],
[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],
];
let maze = levelGrid.map(row => row.slice());
let pelletsRemaining = 10;
let score = 0;
let gameState = "playing";
function collectPellet(row, col){ pelletsRemaining--; score += 10; maze[row][col] = 4; }
function isWallAtTile(row, col){ return row < 0 || col < 0 || levelGrid[row][col] === 1; }
const ghostStarts = [{x:1,y:1},{x:2,y:2}];
const restartButton = document.getElementById("restartButton");
restartButton.addEventListener("click", restartGame);
document.addEventListener("keydown", event => { switch(event.key.toLowerCase()){ case "w": break; case "arrowup": break; } });
function restartGame(){ maze = levelGrid.map(row => row.slice()); gameState = "playing"; }`
	issues := pacManImplementationIssues(content)
	if len(issues) != 0 {
		t.Fatalf("numeric maze implementation was blocked: %v", issues)
	}
}

func TestPacManGuardAcceptsCommonAlternateStateNames(t *testing.T) {
	content := `const layout = [
"#################",
"#.....#....#....#",
"#.###.#.##.#.##.#",
"#.....#....#....#",
"#.###.####.###..#",
"#...............#",
"#.##.###.###.##.#",
"#....#.....#....#",
"###..#.###.#..###",
"#....#.....#....#",
"#.##.###.###.##.#",
"#...............#",
"#.###.####.###..#",
"#.....#....#....#",
"#################",
];
let board = layout.map(row => row.split(""));
let pacman = { x: 1, y: 1 };
let ghosts = [{ x: 13, y: 1 }, { x: 1, y: 13 }];
let pelletsLeft = 10;
let score = 0;
let isGameOver = false, hasWon = false, paused = false;
function collectPellet(row, col){ pelletsLeft--; score++; board[row][col] = " "; }
function canMove(nextX, nextY){ if (nextX < 0 || nextY < 0 || nextY >= board.length || nextX >= board[0].length) return false; return board[nextY][nextX] !== "#"; }
const resetButton = document.getElementById("reset");
resetButton.addEventListener("click", resetGame);
document.addEventListener("keydown", event => { switch(event.key.toLowerCase()){ case "w": break; case "arrowup": break; } });
function resetGame(){ board = layout.map(row => row.split("")); isGameOver = false; hasWon = false; paused = false; }`
	issues := pacManImplementationIssues(content)
	if len(issues) != 0 {
		t.Fatalf("alternate state names were blocked: %v", issues)
	}
}

func TestManagedStateWriteMustPreserveMarkers(t *testing.T) {
	project := t.TempDir()
	managed := stateBegin + "\nmanaged\n" + stateEnd + "\n\nmanual\n"
	if err := os.WriteFile(filepath.Join(project, "STATE.md"), []byte(managed), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.StateFile = "STATE.md"
	if _, err := previewAction(project, cfg, AgentAction{Action: "write_file", Path: "STATE.md", Content: "manual replacement"}); err == nil {
		t.Fatal("expected managed STATE.md overwrite to be blocked")
	}
	if _, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "write_file", Path: "STATE.md", Content: "manual replacement"}); err == nil {
		t.Fatal("executeAction must also block managed STATE.md overwrite")
	}
	okContent := stateBegin + "\nupdated\n" + stateEnd + "\n\nmanual replacement\n"
	if _, err := previewAction(project, cfg, AgentAction{Action: "write_file", Path: "STATE.md", Content: okContent}); err != nil {
		t.Fatalf("preserving state markers should be allowed: %v", err)
	}
}

func TestNormalizeOllamaBaseURL(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:11434":         "http://127.0.0.1:11434",
		"http://localhost:11434/": "http://localhost:11434",
		"0.0.0.0:11434":           "http://127.0.0.1:11434",
		"[::]:11434":              "http://127.0.0.1:11434",
	}
	for in, want := range cases {
		if got := normalizeOllamaBaseURL(in); got != want {
			t.Fatalf("normalizeOllamaBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentLoopExecutesStructuredActions(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		calls++
		content := `{"action":"list_files","message":"Lese Projektstruktur","path":"","max_depth":2}`
		if calls > 1 {
			content = `{"action":"finish","message":"Projekt analysiert; README.md wurde gefunden; keine Änderungen vorgenommen."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": content},
			"done":    true,
		})
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model"}, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Analysiere das Projekt", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Projekt analysiert") {
					return
				}
			}
			t.Fatalf("agent ended without final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish in time")
}

func TestAgentRejectsEmptyWriteFileAndRetries(t *testing.T) {
	var chatCalls int
	var sawInvalidRetry bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			chatCalls++
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			content := `{"action":"write_file","message":"Erstelle index.html","path":"index.html"}`
			if chatCalls == 2 {
				for _, message := range req.Messages {
					if strings.Contains(message.Content, "Antwort ungültig") && strings.Contains(message.Content, "write_file action requires non-empty content") {
						sawInvalidRetry = true
					}
				}
				content = `{"action":"write_file","message":"Erstelle index.html","path":"index.html","content":"<main>Maze Muncher</main>"}`
			}
			if chatCalls >= 3 {
				content = `{"action":"finish","message":"Datei erstellt und geprüft."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "auto"
	cfg.MaxAgentSteps = 6
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Erstelle eine kleine HTML-Datei", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 4*time.Second)
	if !sawInvalidRetry {
		t.Fatal("model was not told to repair the missing write_file content")
	}
	data, err := os.ReadFile(filepath.Join(project, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "<main>Maze Muncher</main>" {
		t.Fatalf("unexpected file content: %q", data)
	}
}

func TestAgentRepairsRepeatedEmptyWriteFileContent(t *testing.T) {
	var chatCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			chatCalls++
			content := `{"action":"write_file","message":"Erstelle index.html","path":"index.html"}`
			switch chatCalls {
			case 3:
				content = `{"content":"<main>Recovered</main>"}`
			case 4:
				content = `{"action":"finish","message":"Reparierter Dateiinhalt wurde geschrieben."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "auto"
	cfg.MaxAgentSteps = 6
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Erstelle eine kleine HTML-Datei", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 4*time.Second)
	data, err := os.ReadFile(filepath.Join(project, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "<main>Recovered</main>" {
		t.Fatalf("unexpected repaired file content: %q", data)
	}
	if chatCalls != 4 {
		t.Fatalf("chatCalls = %d, want 4", chatCalls)
	}
}

func TestAgentContinuesAfterUnrepairableInvalidAction(t *testing.T) {
	var chatCalls int
	var sawInvalidWarning bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			chatCalls++
			content := `{"action":"write_file","message":"bad","path":"game.js"}`
			switch chatCalls {
			case 3:
				content = `{"content":""}`
			case 4:
				var req OllamaChatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				for _, message := range req.Messages {
					if strings.Contains(message.Content, "Die letzte Modellantwort enthielt keine ausführbare Agentenaktion") {
						sawInvalidWarning = true
					}
				}
				content = `{"action":"write_file","message":"valid","path":"game.js","content":"let score = 0;\\nfunction restartGame() { score = 0; }\\n"}`
			case 5:
				content = `{"action":"finish","message":"Fertig."}`

			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "dangerous"
	cfg.MaxAgentSteps = 8
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Erstelle game.js und teste ob es funktioniert.", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 5*time.Second)
	if !sawInvalidWarning {
		t.Fatal("agent did not continue with an invalid-action recovery hint")
	}
	data, err := os.ReadFile(filepath.Join(project, "game.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "restartGame") {
		t.Fatalf("valid action was not executed: %s", data)
	}
}

func TestAgentBlocksPrematurePlaceholderFinish(t *testing.T) {
	var chatCalls int
	var sawGuard bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			chatCalls++
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			for _, message := range req.Messages {
				if strings.Contains(message.Content, "SYSTEMHINWEIS ZUR ABSCHLUSSPRÜFUNG") {
					sawGuard = true
				}
			}
			content := `{"action":"write_file","message":"Erstelle game.js","path":"game.js","content":"// placeholder\nlet score = 0;\n"}`
			switch chatCalls {
			case 2:
				content = `{"action":"finish","message":"Fertig."}`
			case 3:
				content = `{"action":"write_file","message":"Ersetze Platzhalter durch Spielzustände","path":"game.js","content":"let paused=false;\nlet score=0;\nlet lives=3;\nfunction pauseGame(){paused=true;}\nfunction restartGame(){score=0;lives=3;paused=false;}\nfunction gameOver(){return 'game over';}\nfunction winState(){return 'win';}\n"}`
			case 4:
				content = `{"action":"run_command","message":"Prüfe die erzeugte Datei","command":"echo test passed"}`
			case 5:
				content = `{"action":"finish","message":"game.js wurde umgesetzt und geprüft."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "dangerous"
	cfg.MaxAgentSteps = 8
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	task := "Baue game.js mit Pause, Restart, Score, Leben und teste ob es funktioniert."
	if err := state.StartAgent(task, "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 5*time.Second)
	if !sawGuard {
		t.Fatal("premature finish was not blocked by the completion guard")
	}
	data, err := os.ReadFile(filepath.Join(project, "game.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "placeholder") || !strings.Contains(string(data), "pauseGame") || !strings.Contains(string(data), "restartGame") {
		t.Fatalf("unexpected final game.js: %s", data)
	}
	if chatCalls != 5 {
		t.Fatalf("chatCalls = %d, want 5", chatCalls)
	}
}

func TestAgentEscalatesRepeatedSingleFileCompletionBlock(t *testing.T) {
	var chatCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			chatCalls++
			content := `{"action":"finish","message":"Fertig."}`
			switch chatCalls {
			case 1:
				content = `{"action":"write_file","message":"Erstelle index.html","path":"index.html","content":"<button>Start</button><p>TODO</p>"}`
			case 5:
				content = `{"content":"<main><button id=\"restartButton\">Restart</button><p id=\"status\">Bereit</p><script>let gameState='ready';function restartGame(){gameState='ready';document.getElementById('status').textContent=gameState;}document.getElementById('restartButton').addEventListener('click',restartGame);document.addEventListener('keydown',event=>{if(event.key.toLowerCase()==='r')restartGame();});</script></main>"}`
			case 6:
				content = `{"action":"run_command","message":"Prüfe index.html","command":"echo test passed"}`
			case 7:
				content = `{"action":"finish","message":"index.html wurde repariert und geprüft."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "dangerous"
	cfg.MaxAgentSteps = 10
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Erstelle eine Browser-App als einzelne Datei index.html mit Button, Status, Restart und Tastatur und teste ob sie funktioniert.", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 5*time.Second)
	data, err := os.ReadFile(filepath.Join(project, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "addEventListener") || strings.Contains(string(data), "TODO") {
		t.Fatalf("focused completion repair did not replace the file: %s", data)
	}
	var escalated bool
	state.mu.RLock()
	for _, event := range state.Events {
		if event.Message == "Abschlussreparatur eskaliert" || event.Message == "Completion repair escalated" {
			escalated = true
		}
	}
	state.mu.RUnlock()
	if !escalated {
		t.Fatal("completion guard did not escalate repeated identical blocks")
	}
}

func TestChatAcceptsStructuredActionFromThinking(t *testing.T) {
	var thinkValue any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		thinkValue = req["think"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"role":     "assistant",
				"content":  "",
				"thinking": `interne Verarbeitung\n{"action":"finish","message":"Fertig"}`,
			},
			"done":        true,
			"done_reason": "stop",
		})
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	content, err := client.Chat(context.Background(), "gpt-oss:20b", []OllamaMessage{{Role: "user", Content: "test"}}, actionSchema)
	if err != nil {
		t.Fatal(err)
	}
	if content != `{"action":"finish","message":"Fertig"}` {
		t.Fatalf("unexpected content: %q", content)
	}
	if thinkValue != "low" {
		t.Fatalf("think = %#v, want low", thinkValue)
	}
}

func TestAgentFallsBackFromEmptyGPTOSSResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "gpt-oss:20b", "size": 1, "modified_at": time.Now()},
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/chat":
			var req struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Model == "gpt-oss:20b" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message":     map[string]any{"role": "assistant", "content": "", "thinking": ""},
					"done":        true,
					"done_reason": "length",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{"role": "assistant", "content": `{"action":"finish","message":"Fallback erfolgreich"}`},
				"done":    true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "gpt-oss:20b"}, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Analysiere das Projekt", "gpt-oss:20b", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		model := state.Model
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if model != "qwen2.5-coder:14b" {
				t.Fatalf("model = %q, want fallback", model)
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Fallback erfolgreich") {
					return
				}
			}
			t.Fatalf("agent ended without fallback final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish in time")
}

func TestChooseDefaultModelMigratesFromGPTOSS(t *testing.T) {
	models := []ModelInfo{{Name: "gpt-oss:20b"}, {Name: "qwen2.5-coder:14b"}}
	if got := chooseDefaultModel(models, "gpt-oss:20b"); got != "qwen2.5-coder:14b" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeConfigRepairsAppDataProjectRoot(t *testing.T) {
	profile := t.TempDir()
	projects := filepath.Join(profile, "Projekte")
	if err := os.MkdirAll(filepath.Join(projects, "Geschichten"), 0o755); err != nil {
		t.Fatal(err)
	}
	localAppData := filepath.Join(profile, "AppData", "Local")
	broken := filepath.Join(localAppData, "LocalCode")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALCODE_USER_HOME", profile)
	t.Setenv("USERPROFILE", profile)
	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("APPDATA", filepath.Join(profile, "AppData", "Roaming"))

	cfg := normalizeConfig(Config{RootProjectDir: broken, LastProject: broken, Port: 32145})
	if cfg.RootProjectDir != projects {
		t.Fatalf("root = %q, want %q", cfg.RootProjectDir, projects)
	}
	if cfg.LastProject != "" {
		t.Fatalf("last project should be cleared, got %q", cfg.LastProject)
	}
}

func TestNormalizeConfigKeepsValidCustomProjectRoot(t *testing.T) {
	profile := t.TempDir()
	projects := filepath.Join(profile, "Projekte")
	custom := filepath.Join(profile, "Code")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALCODE_USER_HOME", profile)
	t.Setenv("USERPROFILE", profile)
	t.Setenv("LOCALAPPDATA", filepath.Join(profile, "AppData", "Local"))
	t.Setenv("APPDATA", filepath.Join(profile, "AppData", "Roaming"))

	cfg := normalizeConfig(Config{RootProjectDir: custom, Port: 32145})
	if cfg.RootProjectDir != custom {
		t.Fatalf("root = %q, want %q", cfg.RootProjectDir, custom)
	}
}

func TestProjectsEndpointListsSubdirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Geschichten", "FritzShare"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	state := NewAppState(Config{RootProjectDir: root, Port: 32145}, NewOllamaClient())
	t.Cleanup(state.Close)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/projects", nil)
	rr := httptest.NewRecorder()
	NewServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Geschichten") || !strings.Contains(rr.Body.String(), "FritzShare") {
		t.Fatalf("missing projects: %s", rr.Body.String())
	}
}

func TestProjectPickerControlsAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{"chooseRootBtn", "defaultRootBtn", "/api/browse-root", "/api/reset-root"} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing project picker control %q", required)
		}
	}
	if strings.Contains(html, `id="setRootBtn"`) || strings.Contains(html, `$('#setRootBtn')`) {
		t.Fatal("obsolete free-text root setter is still present")
	}
}

func TestValidateImages(t *testing.T) {
	image := ImageAttachment{Name: "shot.png", MIME: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("png-data"))}
	got, err := validateImages([]ImageAttachment{image})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "shot.png" {
		t.Fatalf("unexpected images: %#v", got)
	}
	if _, err := validateImages([]ImageAttachment{{Name: "bad.txt", MIME: "text/plain", Data: image.Data}}); err == nil {
		t.Fatal("expected unsupported MIME error")
	}
}

func TestFindVisionModelUsesCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
				{"name": "gemma4:e2b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/show":
			var req struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			caps := []string{"completion"}
			if req.Model == "gemma4:e2b" {
				caps = append(caps, "vision")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	model, err := client.FindVisionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if model != "gemma4:e2b" {
		t.Fatalf("model = %q", model)
	}
}

func TestDescribeImagesSendsBase64Images(t *testing.T) {
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req OllamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("messages = %d", len(req.Messages))
		}
		received = req.Messages[0].Images
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "Das Bild zeigt eine Fehlermeldung."},
			"done":    true,
		})
	}))
	defer server.Close()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	data := base64.StdEncoding.EncodeToString([]byte("image"))
	result, err := client.DescribeImages(context.Background(), "vision-model", "Analysiere", []ImageAttachment{{Name: "shot.png", MIME: "image/png", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" || len(received) != 1 || received[0] != data {
		t.Fatalf("result=%q images=%#v", result, received)
	}
}

func TestAgentUsesImageAnalysisBeforeCoding(t *testing.T) {
	var visionCall bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
				{"name": "gemma4:e2b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/show":
			var req struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			caps := []string{"completion"}
			if req.Model == "gemma4:e2b" {
				caps = append(caps, "vision")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
		case "/api/chat":
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if len(req.Messages) > 0 && len(req.Messages[0].Images) > 0 {
				visionCall = true
				_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": "Screenshot: roter Fehlerdialog."}, "done": true})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"finish","message":"Bild und Projekt analysiert"}`}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "qwen2.5-coder:14b"}, client)
	t.Cleanup(state.Close)
	image := ImageAttachment{Name: "shot.png", MIME: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))}
	if err := state.StartAgent("Was zeigt das Bild?", "qwen2.5-coder:14b", []ImageAttachment{image}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if !visionCall {
				t.Fatal("vision call was not made")
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Bild und Projekt") {
					return
				}
			}
			t.Fatalf("missing final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish")
}

func TestComposerSupportsEnterAndGeneralAttachments(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`id="fileInput"`,
		`id="attachBtn"`,
		`id="engineSelect"`,
		`changeEditingEngine(e.target.value)`,
		`syncEngineControls(state.status.editing_engine`,
		`<option value="native">LocalCode`,
		`<option value="claw">Claw Code`,
		`e.key==='Enter'&&!e.shiftKey`,
		`onpaste=`,
		`ondragover=`,
		`ondrop=`,
		`focusPrompt(true)`,
		`const attachments=state.attachments`,
		`Dateien hinzufügen`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing composer feature %q", required)
		}
	}
}

func TestDefaultDesktopEngineIsLocalCodeNative(t *testing.T) {
	cfg := defaultConfig()
	if cfg.EditingEngine != editingEngineNative {
		t.Fatalf("fresh desktop config should default to LocalCode native, got %q", cfg.EditingEngine)
	}
}

func TestStateDocumentIsCreatedAndPreservesManualNotes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "STATE.md"), []byte("## Manuelle Notiz\nNicht löschen.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	cfg.AutoStateUpdate = true
	cfg.StateFile = "STATE.md"
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	state.Project = root
	state.Model = "qwen2.5-coder:14b"
	state.LastTask = "Test"
	state.recordAction("Datei gelesen")
	state.UpdateProjectState("Testlauf")
	data, err := os.ReadFile(filepath.Join(root, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{stateBegin, stateEnd, "Manuelle Notiz", "Testlauf", "Datei gelesen"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestProjectDocsCreatedOnlyWhenMissing(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.CreateProjectDocs = true
	if err := ensureProjectDocs(root, cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectDocs(root, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(data) != "custom" {
		t.Fatal("existing README was overwritten")
	}
}

func TestSettingsEndpointRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	server := NewServer(state)
	payload := cfg
	payload.ApprovalMode = "auto"
	payload.WebSearchProvider = "disabled"
	payload.MCPServers = []MCPServerConfig{{Name: "demo", Enabled: true, Transport: "stdio", Command: "demo"}}
	body, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/settings", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.RLock()
	got := state.Config
	state.mu.RUnlock()
	if got.ApprovalMode != "auto" || findMCPServerIndex(got, "demo") < 0 || findMCPServerIndex(got, "filesystem") < 0 {
		t.Fatalf("unexpected settings: %#v", got)
	}
}

func TestValidatePublicURLBlocksLocalhost(t *testing.T) {
	if _, err := validatePublicURL("http://127.0.0.1:1234/private"); err == nil {
		t.Fatal("expected local URL to be blocked")
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req map[string]any
		_ = json.Unmarshal(scanner.Bytes(), &req)
		id, hasID := req["id"]
		if !hasID {
			continue
		}
		method, _ := req["method"].(string)
		result := map[string]any{}
		if method == "initialize" {
			result = map[string]any{"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
		}
		if method == "tools/list" {
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "description": "test", "inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	os.Exit(0)
}

func TestMCPStdioToolsList(t *testing.T) {
	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{Name: "test", Enabled: true, Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, Env: map[string]string{"GO_WANT_MCP_HELPER": "1"}, TimeoutSec: 10}}
	out, err := mcpCall(context.Background(), cfg, t.TempDir(), "test", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "echo") {
		t.Fatalf("unexpected MCP output: %s", out)
	}
}

func TestSettingsUIIsEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"settingsBtn", "settingsScreen", "settings-nav-btn", "setMCP", "/api/settings", "terminalBtn"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing UI element %q", want)
		}
	}
}

func TestResizablePanelsAndCodexStyleSettingsAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`id="leftSplitter"`, `id="rightSplitter"`, `id="terminalSplitter"`,
		`data-settings="general"`, `data-settings="appearance"`, `data-settings="plugins"`,
		`data-settings="hooks"`, `data-settings="worktrees"`, `data-settings="archived"`,
		`setupSplitters()`, `ui_left_width`, `ui_right_width`, `ui_terminal_height`,
		`terminal_dock`, `terminal.parentElement!==rightPane`, `terminal.parentElement!==chatWrap`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing resizable/settings feature %q", want)
		}
	}
}

func TestRemoteComposerSupportsDragDropAttachments(t *testing.T) {
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`id="fileInput"`,
		`MAX_FILES=20`,
		`MAX_FILE_BYTES=32*1024*1024`,
		`MAX_TOTAL_BYTES=96*1024*1024`,
		`async function addFiles(files)`,
		`composer.ondragover=`,
		`composer.ondrop=`,
		`addFiles(e.dataTransfer.files)`,
		`file_too_large`,
		`total_too_large`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing remote attachment feature %q", required)
		}
	}
}

func TestRemotePairingFormSurvivesMobileKeyboard(t *testing.T) {
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`<form id="pairForm">`,
		`id="pairBtn" type="submit"`,
		`.pair{height:100%;display:grid;place-items:start center;overflow:auto;`,
		`padding:48px 24px 24px`,
		`async function pair(ev){ev?.preventDefault();const btn=$('#pairBtn');`,
		`btn.disabled=true`,
		`finally{btn.disabled=false}`,
		`$('#pairForm').onsubmit=pair`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing mobile pairing keyboard safeguard %q", required)
		}
	}
	if strings.Contains(html, `$('#pairBtn').onclick=pair`) {
		t.Fatal("pairing should be handled by form submit, not only by a button click")
	}
}

func TestRemoteTabsSupportSwipeGestures(t *testing.T) {
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`const viewOrder=['tasks','projects','trash','approve','review','new'];`,
		`function swipeView(step)`,
		`function bindSwipe()`,
		`touchstart`,
		`touchend`,
		`Math.abs(dx)<60`,
		`swipeView(dx<0?1:-1)`,
		`bindSwipe();`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing remote tab swipe feature %q", required)
		}
	}
}

func TestRemoteSendAttachmentAndDefaultEngineAreMobileRobust(t *testing.T) {
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`id="sendBtn" class="round send" type="button"`,
		`$('#sendBtn').onclick=send`,
		`function updateSendState()`,
		`state.sending=true;updateSendState()`,
		`catch(e){window.alert(e.message)}finally{state.sending=false;updateSendState()}`,
		`preferredEngine:localStorage.getItem('localcodeRemoteEngine')||'native'`,
		`async function ensurePreferredEngine()`,
		`await ensurePreferredEngine()`,
		`catch(e){state.engineSynced=false;console.warn(e)}`,
		`localStorage.setItem('localcodeRemoteEngine',engine)`,
		`$('#attachBtn').onclick=()=>$('#fileInput').click()`,
		`id="fileInput" class="hidden" type="file" multiple`,
		`data-i18n-title="upload"`,
		`id="voiceBtn" class="round" type="button" data-i18n-title="voice"`,
		`window.localCodeVoiceResult=text=>appendPromptText(text)`,
		`function startVoiceInput()`,
		`window.LocalCodeAndroid?.startVoiceInput`,
		`window.SpeechRecognition||window.webkitSpeechRecognition`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing mobile send/attachment/default-engine safeguard %q", required)
		}
	}
	if strings.Contains(html, `state.status?.editing_engine||'aider'`) {
		t.Fatal("remote UI must not default to Aider")
	}
	if strings.Contains(html, `accept="image/*`) {
		t.Fatal("remote file picker should allow all file types")
	}
}

func TestApplicationIconsAreBundled(t *testing.T) {
	for _, staticPath := range []string{"static/favicon.svg", "static/index.html", "static/remote.html"} {
		data, err := staticFS.ReadFile(staticPath)
		if err != nil {
			t.Fatalf("%s missing: %v", staticPath, err)
		}
		text := string(data)
		if staticPath == "static/favicon.svg" {
			if !strings.Contains(text, "#06b6d4") || !strings.Contains(text, "LocalCode") {
				t.Fatalf("favicon does not look like the LocalCode icon: %s", staticPath)
			}
			continue
		}
		if !strings.Contains(text, `<link rel="icon" href="/favicon.svg" type="image/svg+xml">`) {
			t.Fatalf("%s does not reference the LocalCode favicon", staticPath)
		}
	}
	if info, err := os.Stat("rsrc_windows_amd64.syso"); err != nil || info.Size() == 0 {
		t.Fatalf("Windows icon resource missing or empty: info=%v err=%v", info, err)
	}
}

func TestValidateGeneralAttachments(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("package main\n"))
	items, err := validateAttachments([]Attachment{{Name: "main.go", MIME: "text/plain", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "main.go" || items[0].Size == 0 {
		t.Fatalf("unexpected attachments: %#v", items)
	}
	if _, err := validateAttachments([]Attachment{{Name: "empty.txt", MIME: "text/plain", Data: ""}}); err == nil {
		t.Fatal("expected empty attachment error")
	}
}

func TestExtractTextAndZipAttachments(t *testing.T) {
	text, kind := extractAttachmentText(context.Background(), "README.md", "text/markdown", []byte("# Hallo\nInhalt"), "")
	if kind != "Text" || !strings.Contains(text, "Hallo") {
		t.Fatalf("text=%q kind=%q", text, kind)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("package main"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	listing, kind := extractAttachmentText(context.Background(), "project.zip", "application/zip", buf.Bytes(), "")
	if kind != "Archivinhalt" || !strings.Contains(listing, "src/main.go") {
		t.Fatalf("listing=%q kind=%q", listing, kind)
	}
}

func TestCodexLikeLayoutHasNoPlaceholderNavigation(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{`id="newChatBtn"`, `id="gitBtn"`, `id="terminalBtn"`, `id="settingsBtn"`, `id="projectTree"`, `data-tab="outputs"`, `data-tab="sources"`} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing functional UI element %q", required)
		}
	}
	for _, forbidden := range []string{">Pull Requests<", ">Websites<", ">Geplant<"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("placeholder navigation found: %q", forbidden)
		}
	}
}

func TestAgentContinuesAfterAskUserAnswer(t *testing.T) {
	var calls int
	var sawAnswer bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			var req struct {
				Messages []OllamaMessage `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			content := `{"action":"ask_user","message":"Soll Git initialisiert werden?"}`
			if calls > 1 {
				for _, m := range req.Messages {
					if strings.Contains(m.Content, "Antwort: ja") && strings.Contains(m.Content, "Soll Git initialisiert werden?") {
						sawAnswer = true
					}
				}
				content = `{"action":"finish","message":"Git-Entscheidung übernommen; Aufgabe fortgesetzt."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model", MaxAgentSteps: 10}, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Git im Projekt einrichten", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		continuation := state.Continuation
		state.mu.RUnlock()
		if !running && continuation != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	state.mu.RLock()
	continuation := state.Continuation
	state.mu.RUnlock()
	if continuation == nil {
		t.Fatal("expected pending agent continuation")
	}

	if err := state.StartAgent("ja", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if !sawAnswer {
				t.Fatal("continuation request did not include the user's answer and original question")
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Aufgabe fortgesetzt") {
					return
				}
			}
			t.Fatalf("continuation ended without final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("continuation did not finish in time")
}

func TestReadOnlyToolResultIsVisibleAsEvent(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("sichtbarer Inhalt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, GitEnabled: true}, NewOllamaClient())
	t.Cleanup(state.Close)
	result, done := state.handleAgentAction(context.Background(), project, AgentAction{Action: "read_file", Path: "README.md", Message: "Lese README.md"})
	if done {
		t.Fatal("read_file unexpectedly finished agent")
	}
	if !strings.Contains(result, "sichtbarer Inhalt") {
		t.Fatalf("unexpected result: %q", result)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	for _, event := range events {
		if event.Type == "tool_result" && event.Action == "read_file" && strings.Contains(event.Detail, "sichtbarer Inhalt") {
			return
		}
	}
	t.Fatalf("tool result event missing: %#v", events)
}

func TestAskUserProducesOnlyOneVisibleQuestion(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"ask_user","message":"Soll Git initialisiert werden?"}`}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model", MaxAgentSteps: 4}, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Git im Projekt einrichten", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			questions := 0
			for _, event := range events {
				if event.Type == "question" && event.Message == "Soll Git initialisiert werden?" {
					questions++
				}
				if event.Type == "agent_step" && event.Action == "ask_user" {
					t.Fatalf("ask_user leaked as duplicate agent_step: %#v", events)
				}
			}
			if questions != 1 {
				t.Fatalf("expected one question event, got %d: %#v", questions, events)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not reach question state")
}

func TestSettingsEndpointAcceptsForwardCompatibleFieldsWhileAgentRuns(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	state.Running = true
	server := NewServer(state)
	body := `{"approval_mode":"balanced","model_timeout_seconds":240,"future_ui_option":"ignored"}`
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/settings", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.RLock()
	got := state.Config
	state.mu.RUnlock()
	if got.ApprovalMode != "balanced" || got.ModelTimeout != 240 || !got.NetworkEnabled || !got.GitEnabled {
		t.Fatalf("settings were not merged correctly: %#v", got)
	}
}

func TestForceStopAlwaysReleasesUIState(t *testing.T) {
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	ctx, cancel := context.WithCancel(context.Background())
	state.Running = true
	state.Cancel = cancel
	state.RunID = "active-run"
	state.RunPhase = "model"
	pending := &PendingAction{ID: "pending", Result: make(chan ApprovalDecision, 1)}
	state.Pending = pending
	if !state.ForceStopAgent() {
		t.Fatal("expected running agent to be force-stopped")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("agent context was not cancelled")
	}
	state.mu.RLock()
	running, phase, currentPending := state.Running, state.RunPhase, state.Pending
	state.mu.RUnlock()
	if running || phase != "idle" || currentPending != nil {
		t.Fatalf("UI state not released: running=%v phase=%q pending=%#v", running, phase, currentPending)
	}
}

func TestControlAndOutputUIAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`id="runControl"`, `id="headerStopBtn"`, `cancelRun()`, `/api/force-stop`,
		`tool_result`, `action_done`, `tool_error`, `output-list`, `model_timeout_seconds`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing control/output UI feature %q", want)
		}
	}
}

func TestBackgroundWindowsCommandsUseNoWindowFlag(t *testing.T) {
	data, err := os.ReadFile("platform_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{"CREATE_NO_WINDOW", "hideCommandWindow(cmd)", "runProjectCommand"} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing hidden-window implementation %q", want)
		}
	}
}

func TestModelCallTimeoutReturnsControl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "slow-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer server.CloseClientConnections()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "slow-model"
	cfg.ModelTimeout = 1
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Analysiere das Projekt", "slow-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			for _, ev := range events {
				if ev.Type == "error" && strings.Contains(ev.Message, "Zeitüberschreitung") {
					return
				}
			}
			t.Fatalf("agent stopped without timeout event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not return control after model timeout")
}

func TestStopAgentCancelsBlockedModelCall(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "blocked-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer server.CloseClientConnections()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "blocked-model"
	cfg.ModelTimeout = 60
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Analysiere das Projekt", "blocked-model", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(8 * time.Second):
		t.Fatal("model call did not start")
	}
	if !state.StopAgent() {
		t.Fatal("StopAgent did not report an active run")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent remained running after cancellation")
}

func TestLegacyProductDataMigrationCopiesUserState(t *testing.T) {
	base := t.TempDir()
	configBase := filepath.Join(base, "config")
	cacheBase := filepath.Join(base, "cache")
	t.Setenv("LOCALCODE_CONFIG_HOME", configBase)
	t.Setenv("LOCALCODE_CACHE_HOME", cacheBase)
	t.Setenv("XDG_CONFIG_HOME", configBase)
	t.Setenv("XDG_CACHE_HOME", cacheBase)

	legacyDir := filepath.Join(configBase, legacyProductDirName)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configData := []byte(`{"port":32145,"root_project_dir":"C:\\\\Users\\\\frede\\\\Projekte"}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), configData, 0o600); err != nil {
		t.Fatal(err)
	}
	threadsData := []byte(`{"version":1,"threads":[]}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "threads.json"), threadsData, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyBackups := filepath.Join(cacheBase, legacyProductDirName, "backups", "demo")
	if err := os.MkdirAll(legacyBackups, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyBackups, "file.txt"), []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacyProductData()

	for _, item := range []struct {
		path string
		want string
	}{
		{filepath.Join(configBase, productDirName, "config.json"), string(configData)},
		{filepath.Join(configBase, productDirName, "threads.json"), string(threadsData)},
		{filepath.Join(cacheBase, productDirName, "backups", "demo", "file.txt"), "backup"},
	} {
		data, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatalf("read migrated %s: %v", item.path, err)
		}
		if string(data) != item.want {
			t.Fatalf("migrated %s = %q, want %q", item.path, data, item.want)
		}
	}
}

func TestStateDocumentMigratesLegacyManagedMarkers(t *testing.T) {
	root := t.TempDir()
	old := "# Handbuch\n\n" + legacyStateBegin + "\nalt\n" + legacyStateEnd + "\n\n## Manuell\nBehalten.\n"
	if err := os.WriteFile(filepath.Join(root, "STATE.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.StateFile = "STATE.md"
	if err := updateStateDocument(root, cfg, false, "qwen2.5-coder:14b", "Umbenennung", "fertig", []string{"Migration"}, "Markertest"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, stateBegin) || !strings.Contains(text, stateEnd) {
		t.Fatalf("new markers missing: %s", text)
	}
	if strings.Contains(text, legacyStateBegin) || strings.Contains(text, legacyStateEnd) {
		t.Fatalf("legacy markers still present: %s", text)
	}
	for _, want := range []string{"# Handbuch", "## Manuell", "Behalten.", "Markertest"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestLocalCodeBrandAndLicenseAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"<title>LocalCode</title>", ">LocalCode<", "Apache-2.0"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing brand or license marker %q", want)
		}
	}
	legacyVisibleName := "Local" + "Codex"
	if strings.Contains(html, legacyVisibleName) {
		t.Fatalf("legacy product name remains visible in UI")
	}
	license, err := os.ReadFile(filepath.Join("..", "LICENSE"))
	if err != nil {
		t.Fatalf("LICENSE missing: %v", err)
	}
	if !strings.Contains(string(license), "Apache License") || !strings.Contains(string(license), "Version 2.0") {
		t.Fatal("LICENSE is not the complete Apache License 2.0 text")
	}
	if _, err := os.Stat(filepath.Join("..", "NOTICE")); err != nil {
		t.Fatalf("NOTICE missing: %v", err)
	}
}

func TestPingReportsLocalCodeAndLicense(t *testing.T) {
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/ping", nil)
	rr := httptest.NewRecorder()
	NewServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"app":"LocalCode"`, `"license":"Apache-2.0"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
}

func TestAnalysisTaskDoesNotRequireGitOrMutateProject(t *testing.T) {
	var modelCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			modelCalls++
			content := `{"action":"ask_user","message":"Es scheint, dass kein Git-Repository initialisiert wurde. Möchten Sie ein neues Git-Repository erstellen?"}`
			if modelCalls == 2 {
				content = `{"action":"replace_text","message":"Ändere einen Platzhalter","path":"README.md","old_text":"demo","new_text":"changed"}`
			}
			if modelCalls >= 3 {
				content = `{"action":"finish","message":"Analyse abgeschlossen. Git war dafür nicht erforderlich; keine Dateien wurden geändert."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	readme := filepath.Join(project, "README.md")
	if err := os.WriteFile(readme, []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.MaxAgentSteps = 10
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("analysiere das projekt", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	waitForAgentStop(t, state, 4*time.Second)
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "demo\n" {
		t.Fatalf("analysis mutated README: %q", data)
	}
	if _, err := os.Stat(filepath.Join(project, ".git")); !os.IsNotExist(err) {
		t.Fatalf("analysis created .git unexpectedly: %v", err)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	for _, event := range events {
		if event.Type == "question" {
			t.Fatalf("analysis leaked unnecessary question: %#v", event)
		}
	}
	if !eventContains(events, "final", "Projektanalyse kontrolliert abgeschlossen") {
		t.Fatalf("analysis did not reach final report: %#v", events)
	}
}

func TestAffirmativeGitQuestionInitializesRepositoryAndContinues(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			content := `{"action":"ask_user","message":"Soll Git in diesem Projekt initialisiert werden?"}`
			if calls > 1 {
				content = `{"action":"finish","message":"Git wurde initialisiert und verifiziert."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.MaxAgentSteps = 10
	cfg.CreateProjectDocs = false
	cfg.ApprovalMode = "balanced"
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Richte die Git-Versionierung ein", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 4*time.Second)
	state.mu.RLock()
	continuation := state.Continuation
	state.mu.RUnlock()
	if continuation == nil || continuation.SuggestedAction == nil {
		t.Fatalf("expected executable Git continuation: %#v", continuation)
	}
	if err := state.StartAgent("ja", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 6*time.Second)
	if !isGitRepository(project, cfg) {
		t.Fatal("confirmed Git initialization did not create repository")
	}
	if _, err := os.Stat(filepath.Join(project, ".gitignore")); err != nil {
		t.Fatalf("Git initialization did not create .gitignore: %v", err)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	questions := 0
	for _, event := range events {
		if event.Type == "question" {
			questions++
		}
	}
	if questions != 1 {
		t.Fatalf("Git question repeated instead of executing confirmation: %d %#v", questions, events)
	}
	if !eventContains(events, "final", "initialisiert") {
		t.Fatalf("Git continuation did not finish: %#v", events)
	}
}

func TestAgentCompactsLargeContextAndContinues(t *testing.T) {
	var actionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			isCompaction := false
			for _, message := range req.Messages {
				if strings.Contains(message.Content, "Verdichte den folgenden Verlauf") {
					isCompaction = true
					break
				}
			}
			content := ""
			if isCompaction {
				content = `{"summary":"Große Datei wurde gelesen.","original_task":"prüfe die große datei","decisions":[],"project_facts":["big.txt ist groß"],"files_read":["big.txt"],"files_changed":[],"commands":[],"failures":[],"open_items":[],"next_recommended_action":"Bericht abschließen"}`
			} else {
				actionCalls++
				if actionCalls == 1 {
					content = `{"action":"read_file","message":"Lese große Datei","path":"big.txt"}`
				} else {
					content = `{"action":"finish","message":"Große Datei geprüft; Kontext wurde kontrolliert weitergeführt."}`
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "big.txt"), []byte(strings.Repeat("large-context-line\n", 9000)), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.ContextLength = 4096
	cfg.ContextCompactionEnabled = true
	cfg.ContextCompactionThresholdPercent = 45
	cfg.ContextCompactionKeepRecent = 6
	cfg.MaxAgentSteps = 8
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("prüfe die große datei", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 6*time.Second)
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	if !eventContains(events, "context_compacted", "Kontext komprimiert") {
		t.Fatalf("large context was not compacted: %#v", events)
	}
	if !eventContains(events, "final", "kontrolliert weitergeführt") {
		t.Fatalf("agent did not continue after compaction: %#v", events)
	}
}

func waitForAgentStop(t *testing.T, state *AppState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not stop before timeout")
}

func eventContains(events []UIEvent, eventType, fragment string) bool {
	for _, event := range events {
		if event.Type == eventType && strings.Contains(event.Message+"\n"+event.Detail, fragment) {
			return true
		}
	}
	return false
}
