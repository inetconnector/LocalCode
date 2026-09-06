// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"localcode/benchharness"
)

func TestRunRequiresManifest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: localcode-bench") {
		t.Fatalf("usage missing from stderr: %q", stderr.String())
	}
}

func TestRunRejectsInvalidFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-invalid-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
}

func TestRunRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-manifest", path}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "manifest:") {
		t.Fatalf("manifest error missing from stderr: %q", stderr.String())
	}
}

func TestRunExecutesBenchmarkAndWritesResult(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_CLI_HELPER") == "1" {
		return
	}
	repo := initCLIBenchmarkRepo(t)
	manifest := benchharness.Manifest{
		Version:       benchharness.ManifestVersion,
		Name:          "cli-smoke",
		Repository:    repo,
		BaseRef:       "HEAD",
		Task:          "perform benchmark smoke test",
		Engine:        "synthetic",
		Model:         "same-model",
		EngineCommand: []string{os.Args[0], "-test.run=TestBenchmarkCLIHelperProcess", "--", "engine"},
		Environment:   map[string]string{"LOCALCODE_BENCH_CLI_HELPER": "1"},
	}
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "nested", "result.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-manifest", manifestPath, "-out", outputPath, "-keep-worktree"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code=%d want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"success": true`) {
		t.Fatalf("successful result missing from stdout: %s", stdout.String())
	}
	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, stdout.Bytes()) {
		t.Fatalf("output file differs from stdout\nfile=%s\nstdout=%s", written, stdout.String())
	}
}

func TestRunBenchmarkFailsReturnsOne(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_CLI_HELPER") == "1" {
		return
	}
	repo := initCLIBenchmarkRepo(t)
	// Engine that fails by exiting 1
	manifest := benchharness.Manifest{
		Version:       benchharness.ManifestVersion,
		Name:          "cli-failure",
		Repository:    repo,
		BaseRef:       "HEAD",
		Task:          "perform benchmark failure test",
		Engine:        "synthetic",
		Model:         "same-model",
		EngineCommand: []string{os.Args[0], "-test.run=TestBenchmarkCLIHelperProcessFail", "--", "engine"},
		Environment:   map[string]string{"LOCALCODE_BENCH_CLI_HELPER_FAIL": "1"},
	}
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-manifest", manifestPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code=%d want 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestBenchmarkCLIHelperProcess(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_CLI_HELPER") != "1" {
		return
	}
	os.Exit(0)
}

func TestBenchmarkCLIHelperProcessFail(t *testing.T) {
	if os.Getenv("LOCALCODE_BENCH_CLI_HELPER_FAIL") != "1" {
		return
	}
	os.Exit(1)
}

func initCLIBenchmarkRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitCLI(t, repo, "init")
	runGitCLI(t, repo, "config", "user.email", "bench@example.invalid")
	runGitCLI(t, repo, "config", "user.name", "Benchmark CLI Test")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCLI(t, repo, "add", "base.txt")
	runGitCLI(t, repo, "commit", "-m", "base")
	return repo
}

func runGitCLI(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", command...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
