// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClawBenchmarkArgsUseSafeWorkspaceMode(t *testing.T) {
	cfg := clawBenchmarkConfig{Model: "qwen2.5-coder:14b", Task: "fix the test"}
	args := clawBenchmarkArgs(cfg)
	got := strings.Join(args, " ")
	for _, required := range []string{
		"--output-format json",
		"--permission-mode workspace-write",
		"--model qwen2.5-coder:14b",
		"prompt fix the test",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("benchmark args %q missing %q", got, required)
		}
	}
	if strings.Contains(got, "danger-full-access") || strings.Contains(got, "skip-permissions") {
		t.Fatalf("benchmark args requested unsafe permission mode: %q", got)
	}
}

func TestClawBenchmarkEnvironmentForcesLocalOllamaAndStripsCloudCredentials(t *testing.T) {
	base := []string{
		"PATH=C:\\Tools",
		"OLLAMA_HOST=https://wrong.example/v1",
		"OPENAI_API_KEY=openai-secret",
		"OPENAI_BASE_URL=https://api.openai.example/v1",
		"ANTHROPIC_API_KEY=anthropic-secret",
		"ANTHROPIC_AUTH_TOKEN=anthropic-token",
		"ANTHROPIC_BASE_URL=https://anthropic.example",
		"XAI_API_KEY=xai-secret",
		"DASHSCOPE_API_KEY=dashscope-secret",
		"CLAUDE_CODE_PROVIDER=cloud",
		"CLAW_MODEL=cloud-model",
		"CLAW_PERMISSION_MODE=danger-full-access",
		"CLAW_OUTPUT_FORMAT=text",
	}
	cfg := clawBenchmarkConfig{OllamaHost: "http://127.0.0.1:11434/v1/"}
	env := clawBenchmarkEnvironment(base, cfg)
	joined := strings.Join(env, "\n")
	for _, blocked := range []string{
		"OPENAI_API_KEY=", "OPENAI_BASE_URL=", "ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN=",
		"ANTHROPIC_BASE_URL=", "XAI_API_KEY=", "DASHSCOPE_API_KEY=", "CLAUDE_CODE_PROVIDER=",
		"CLAW_MODEL=", "CLAW_PERMISSION_MODE=",
	} {
		if strings.Contains(joined, blocked) {
			t.Fatalf("benchmark environment retained blocked provider setting %q:\n%s", blocked, joined)
		}
	}
	if strings.Count(joined, "OLLAMA_HOST=") != 1 || !strings.Contains(joined, "OLLAMA_HOST=http://127.0.0.1:11434") {
		t.Fatalf("benchmark environment did not force normalized Ollama host:\n%s", joined)
	}
	if strings.Count(joined, "CLAW_OUTPUT_FORMAT=") != 1 || !strings.Contains(joined, "CLAW_OUTPUT_FORMAT=json") {
		t.Fatalf("benchmark environment did not force JSON output:\n%s", joined)
	}
	if !strings.Contains(joined, "PATH=C:\\Tools") {
		t.Fatalf("unrelated environment entry was removed:\n%s", joined)
	}
}

func TestNormalizeClawBenchmarkOllamaHost(t *testing.T) {
	cases := map[string]string{
		"":                                 defaultClawBenchmarkOllamaHost,
		"http://127.0.0.1:11434/":          "http://127.0.0.1:11434",
		"http://127.0.0.1:11434/v1":        "http://127.0.0.1:11434",
		"http://127.0.0.1:11434/v1/":       "http://127.0.0.1:11434",
		"http://localhost:11434/custom/v1": "http://localhost:11434/custom",
	}
	for input, want := range cases {
		if got := normalizeClawBenchmarkOllamaHost(input); got != want {
			t.Fatalf("normalizeClawBenchmarkOllamaHost(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestValidateClawBenchmarkConfigRequiresExplicitBinary(t *testing.T) {
	_, err := validateClawBenchmarkConfig(clawBenchmarkConfig{})
	if err == nil || !strings.Contains(err.Error(), "LOCALCODE_BENCH_CLAW") {
		t.Fatalf("missing explicit Claw path must fail, got %v", err)
	}
}

func TestValidateClawBenchmarkConfigAcceptsHarnessWorktree(t *testing.T) {
	worktree := t.TempDir()
	t.Chdir(worktree)
	claw := filepath.Join(worktree, "claw-test-binary")
	if err := os.WriteFile(claw, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := validateClawBenchmarkConfig(clawBenchmarkConfig{
		ClawPath: claw,
		Worktree: worktree,
		Task:     "change the fixture",
		Model:    "qwen2.5-coder:14b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClawPath != filepath.Clean(claw) || cfg.Worktree != filepath.Clean(worktree) {
		t.Fatalf("validated paths = claw %q worktree %q", cfg.ClawPath, cfg.Worktree)
	}
	if cfg.OllamaHost != defaultClawBenchmarkOllamaHost {
		t.Fatalf("default Ollama host = %q", cfg.OllamaHost)
	}
}

func TestValidateClawBenchmarkConfigRejectsDifferentWorkingDirectory(t *testing.T) {
	worktree := t.TempDir()
	other := t.TempDir()
	t.Chdir(other)
	claw := filepath.Join(other, "claw-test-binary")
	if err := os.WriteFile(claw, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := validateClawBenchmarkConfig(clawBenchmarkConfig{
		ClawPath: claw,
		Worktree: worktree,
		Task:     "change the fixture",
		Model:    "qwen2.5-coder:14b",
	})
	if err == nil || !strings.Contains(err.Error(), "must run inside LOCALCODE_BENCH_WORKTREE") {
		t.Fatalf("mismatched cwd/worktree must fail, got %v", err)
	}
}
