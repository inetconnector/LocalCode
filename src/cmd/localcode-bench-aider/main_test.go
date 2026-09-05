// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAiderBenchmarkConfig(t *testing.T) {
	temp := t.TempDir()
	exePath := filepath.Join(temp, "mock-aider.cmd")
	if err := os.WriteFile(exePath, []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCwd) }()
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}

	cfg := aiderBenchmarkConfig{
		AiderPath: exePath,
		Worktree:  temp,
		Task:      "refactor parser",
		Model:     "qwen2.5-coder:14b",
	}

	validated, err := validateAiderBenchmarkConfig(cfg)
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if validated.OllamaHost != defaultAiderBenchmarkOllamaHost {
		t.Errorf("expected default ollama host %q, got %q", defaultAiderBenchmarkOllamaHost, validated.OllamaHost)
	}

	args := aiderBenchmarkArgs(validated)
	if len(args) == 0 || args[0] != "--yes" {
		t.Errorf("unexpected args: %#v", args)
	}

	env := aiderBenchmarkEnvironment([]string{"OPENAI_API_KEY=secret", "PATH=/bin"}, validated)
	for _, e := range env {
		if strings.HasPrefix(e, "OPENAI_API_KEY=") {
			t.Errorf("ambient OPENAI_API_KEY was not stripped from environment")
		}
	}
}

func TestValidateAiderBenchmarkConfigErrors(t *testing.T) {
	temp := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCwd) }()
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}

	exePath := filepath.Join(temp, "mock-aider.cmd")
	if err := os.WriteFile(exePath, []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing AiderPath
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{}); err == nil {
		t.Error("expected error for missing AiderPath")
	}

	// 2. Relative AiderPath
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: "aider"}); err == nil {
		t.Error("expected error for relative AiderPath")
	}

	// 3. Nonexistent AiderPath
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: filepath.Join(temp, "missing.exe")}); err == nil {
		t.Error("expected error for nonexistent AiderPath")
	}

	// 4. AiderPath is a directory
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: temp}); err == nil {
		t.Error("expected error when AiderPath is a directory")
	}

	// 5. Missing Worktree
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath}); err == nil {
		t.Error("expected error for missing Worktree")
	}

	// 6. Relative Worktree
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: "sub"}); err == nil {
		t.Error("expected error for relative Worktree")
	}

	// 7. Nonexistent Worktree
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: filepath.Join(temp, "nonexistent")}); err == nil {
		t.Error("expected error for nonexistent Worktree")
	}

	// 8. Worktree is a file, not a directory
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: exePath}); err == nil {
		t.Error("expected error when Worktree is a file")
	}

	// 9. Cwd mismatch
	otherDir := t.TempDir()
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: otherDir}); err == nil {
		t.Error("expected error when cwd does not match worktree")
	}

	// 10. Missing Task
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: temp}); err == nil {
		t.Error("expected error for missing Task")
	}

	// 11. Missing Model
	if _, err := validateAiderBenchmarkConfig(aiderBenchmarkConfig{AiderPath: exePath, Worktree: temp, Task: "some task"}); err == nil {
		t.Error("expected error for missing Model")
	}
}

func TestNormalizeAiderBenchmarkOllamaHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", defaultAiderBenchmarkOllamaHost},
		{"   ", defaultAiderBenchmarkOllamaHost},
		{"http://localhost:11434/", "http://localhost:11434"},
		{"http://localhost:11434/v1", "http://localhost:11434"},
		{"http://localhost:11434/v1/", "http://localhost:11434"},
		{"http://custom-host:8080", "http://custom-host:8080"},
	}
	for _, tc := range tests {
		got := normalizeAiderBenchmarkOllamaHost(tc.input)
		if got != tc.want {
			t.Errorf("normalizeAiderBenchmarkOllamaHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAiderBenchmarkModel(t *testing.T) {
	if got := aiderBenchmarkModel("ollama/llama3"); got != "ollama/llama3" {
		t.Errorf("got %q, want ollama/llama3", got)
	}
	if got := aiderBenchmarkModel("ollama_chat/llama3"); got != "ollama_chat/llama3" {
		t.Errorf("got %q, want ollama_chat/llama3", got)
	}
	if got := aiderBenchmarkModel("llama3"); got != "ollama_chat/llama3" {
		t.Errorf("got %q, want ollama_chat/llama3", got)
	}
}

func TestLoadAiderBenchmarkConfig(t *testing.T) {
	t.Setenv("LOCALCODE_BENCH_AIDER", "C:\\bin\\aider.exe")
	t.Setenv("LOCALCODE_BENCH_WORKTREE", "C:\\repo")
	t.Setenv("LOCALCODE_BENCH_TASK", "do something")
	t.Setenv("LOCALCODE_BENCH_MODEL", "qwen2.5-coder")
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "http://127.0.0.1:11434")

	cfg := loadAiderBenchmarkConfig()
	if cfg.AiderPath != "C:\\bin\\aider.exe" || cfg.Worktree != "C:\\repo" || cfg.Task != "do something" || cfg.Model != "qwen2.5-coder" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	// Fallback to OLLAMA_API_BASE
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "")
	t.Setenv("OLLAMA_API_BASE", "http://api-base:11434")
	cfg = loadAiderBenchmarkConfig()
	if cfg.OllamaHost != "http://api-base:11434" {
		t.Fatalf("expected OLLAMA_API_BASE fallback, got %q", cfg.OllamaHost)
	}

	// Fallback to OLLAMA_HOST
	t.Setenv("OLLAMA_API_BASE", "")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	cfg = loadAiderBenchmarkConfig()
	if cfg.OllamaHost != "http://ollama-host:11434" {
		t.Fatalf("expected OLLAMA_HOST fallback, got %q", cfg.OllamaHost)
	}
}

func TestRunAiderBenchmarkSmoke(t *testing.T) {
	temp := t.TempDir()
	scriptContent := "@echo off\r\necho AIDER_MOCK_SUCCESS\r\nexit /b 0\r\n"
	exePath := filepath.Join(temp, "mock-aider.cmd")
	if err := os.WriteFile(exePath, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCwd) }()
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}

	cfg := aiderBenchmarkConfig{
		AiderPath: exePath,
		Worktree:  temp,
		Task:      "test task",
		Model:     "qwen2.5-coder:14b",
	}

	if err := runAiderBenchmark(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected benchmark run error: %v", err)
	}
}
