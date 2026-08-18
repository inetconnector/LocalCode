// SPDX-License-Identifier: Apache-2.0

package benchharness

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunnerUsesDetachedBaseAndMeasuresUnnecessaryChanges(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_HELPER") == "1" {
		return
	}
	repo := initBenchmarkGitRepo(t)
	manifest := Manifest{
		Version:       ManifestVersion,
		Name:          "synthetic-multifile",
		Repository:    repo,
		BaseRef:       "HEAD",
		Task:          "create the requested implementation",
		Engine:        "synthetic",
		Model:         "same-model",
		EngineCommand: []string{os.Args[0], "-test.run=TestBenchmarkHelperProcess", "--", "engine"},
		Checks: []Check{{
			Name:     "hidden-contract",
			Kind:     "hidden",
			Command:  []string{os.Args[0], "-test.run=TestBenchmarkHelperProcess", "--", "check"},
			Required: true,
		}},
		AllowedPaths: []string{"desired.txt"},
		Environment:  map[string]string{"LOCALCODE_BENCH_HELPER": "1"},
	}
	result, err := (Runner{TempRoot: t.TempDir()}).Run(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || !result.EngineResult.Successful || !result.RequiredChecksPass {
		data, _ := json.MarshalIndent(result, "", "  ")
		t.Fatalf("benchmark should pass:\n%s", data)
	}
	if result.BaseCommit == "" {
		t.Fatal("base commit not captured")
	}
	if result.ChangedFiles != 2 {
		t.Fatalf("changed files=%d want 2: %#v", result.ChangedFiles, result.Changes)
	}
	if result.UnnecessaryLines == 0 {
		t.Fatalf("unnecessary change metric was not detected: %#v", result.Changes)
	}
	if result.Worktree != "" {
		t.Fatalf("temporary worktree path leaked after cleanup: %q", result.Worktree)
	}
	if _, err := os.Stat(filepath.Join(repo, "desired.txt")); !os.IsNotExist(err) {
		t.Fatalf("benchmark mutated source repository; stat err=%v", err)
	}
}

func TestBenchmarkHelperProcess(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "engine":
		if os.Getenv("LOCALCODE_BENCH_TASK") == "" || os.Getenv("LOCALCODE_BENCH_MODEL") != "same-model" {
			os.Exit(3)
		}
		if err := os.WriteFile("desired.txt", []byte("implemented\n"), 0o644); err != nil {
			os.Exit(4)
		}
		if err := os.WriteFile("unrelated.txt", []byte("unnecessary\n"), 0o644); err != nil {
			os.Exit(5)
		}
		os.Exit(0)
	case "check":
		data, err := os.ReadFile("desired.txt")
		if err != nil || string(data) != "implemented\n" {
			os.Exit(6)
		}
		os.Exit(0)
	default:
		os.Exit(7)
	}
}

func TestManifestRejectsUnsafePathsAndUnknownChecks(t *testing.T) {
	base := Manifest{Version: ManifestVersion, Name: "x", Repository: t.TempDir(), BaseRef: "HEAD", Task: "x", Engine: "native", Model: "m", EngineCommand: []string{"engine"}}
	unsafe := base
	unsafe.AllowedPaths = []string{"../outside"}
	if err := unsafe.Validate(); err == nil {
		t.Fatal("unsafe allowed path accepted")
	}
	badKind := base
	badKind.Checks = []Check{{Name: "x", Kind: "magic", Command: []string{"x"}, Required: true}}
	if err := badKind.Validate(); err == nil {
		t.Fatal("unknown check kind accepted")
	}
}

func TestLoadManifestResolvesRelativeRepositoryAndValidates(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "fixture")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "benchmark.json")
	data := `{"version":1,"name":"load","repository":"fixture","base_ref":"HEAD","task":"task","engine":"native","model":"model","engine_command":["engine"],"checks":[]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Repository != filepath.Clean(repo) {
		t.Fatalf("repository=%q want %q", manifest.Repository, filepath.Clean(repo))
	}
}

func TestManifestValidationRequiredFields(t *testing.T) {
	valid := Manifest{Version: ManifestVersion, Name: "x", Repository: t.TempDir(), BaseRef: "HEAD", Task: "task", Engine: "native", Model: "model", EngineCommand: []string{"engine"}}
	cases := []Manifest{
		{Version: 99, Name: "x", Repository: valid.Repository, BaseRef: "HEAD", Task: "task", Engine: "native", Model: "model", EngineCommand: []string{"engine"}},
		{Version: ManifestVersion, Repository: valid.Repository, BaseRef: "HEAD", Task: "task", Engine: "native", Model: "model", EngineCommand: []string{"engine"}},
		{Version: ManifestVersion, Name: "x", Repository: valid.Repository, BaseRef: "HEAD", Task: "task", Engine: "native", Model: "model"},
	}
	for i, candidate := range cases {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("case %d unexpectedly valid", i)
		}
	}
	valid.MetricsFile = "../metrics.json"
	if err := valid.Validate(); err == nil {
		t.Fatal("unsafe metrics file accepted")
	}
}

func TestReadAdapterMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := os.WriteFile(path, []byte(`{"agent_turns":4,"tool_calls":9,"input_tokens":1200,"output_tokens":300,"failed_patches":1,"retries":2,"compactions":1,"human_intervention":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, err := readAdapterMetrics(path)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.AgentTurns != 4 || metrics.ToolCalls != 9 || metrics.Retries != 2 || metrics.InputTokens != 1200 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestBenchmarkHelpersCoverFilteringAndTextMetrics(t *testing.T) {
	changes := []FileChange{{Path: "result.json", Added: 3}, {Path: "src/main.go", Added: 2, Deleted: 1}}
	filtered := excludeBenchmarkChange(changes, "result.json")
	if len(filtered) != 1 || filtered[0].Path != "src/main.go" {
		t.Fatalf("unexpected filtered changes: %#v", filtered)
	}
	if !pathAllowed("src/main.go", []string{"src"}) || pathAllowed("docs/readme.md", []string{"src"}) {
		t.Fatal("path allowlist behavior incorrect")
	}
	path := filepath.Join(t.TempDir(), "lines.txt")
	if err := os.WriteFile(path, []byte("one\ntwo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countTextLines(path); got != 2 {
		t.Fatalf("countTextLines=%d want 2", got)
	}
	if got := truncate("abcdef", 3); !strings.Contains(got, "abc") || !strings.Contains(got, "truncated") {
		t.Fatalf("truncate=%q", got)
	}
}

func TestRunCommandRejectsEmptyCommand(t *testing.T) {
	result := runCommand(context.Background(), t.TempDir(), "empty", "engine", true, nil, 1, os.Environ(), nil, 1)
	if result.Successful || result.ExitCode != -1 || result.Output != "empty command" {
		t.Fatalf("unexpected empty command result: %#v", result)
	}
}

func initBenchmarkGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.email", "bench@example.invalid")
	runGitTest(t, repo, "config", "user.name", "Benchmark Test")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "base.txt")
	runGitTest(t, repo, "commit", "-m", "base")
	return repo
}

func runGitTest(t *testing.T, repo string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(ctx, "git", command...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestExpandArgsKeepsArgumentsSeparated(t *testing.T) {
	args := expandArgs([]string{"tool", "--task", "${TASK}", "${WORKTREE}"}, map[string]string{"TASK": "value with spaces & shell chars", "WORKTREE": `C:\repo path`})
	if len(args) != 4 || args[2] != "value with spaces & shell chars" {
		t.Fatalf("arguments were not preserved: %#v", args)
	}
	if runtime.GOOS == "windows" && !strings.Contains(args[3], `C:\repo`) {
		t.Fatalf("worktree substitution unexpected: %#v", args)
	}
}
