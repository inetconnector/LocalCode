// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOpenCodeBenchmarkConfig(t *testing.T) {
	temp := t.TempDir()
	exePath := filepath.Join(temp, "mock-opencode.cmd")
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

	cfg := openCodeBenchmarkConfig{
		OpenCodePath: exePath,
		Worktree:     temp,
		Task:         "refactor word counter",
		Model:        "qwen2.5-coder:14b",
	}

	validated, err := validateOpenCodeBenchmarkConfig(cfg)
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if validated.OllamaHost != defaultOpenCodeBenchmarkOllamaHost {
		t.Errorf("expected default ollama host %q, got %q", defaultOpenCodeBenchmarkOllamaHost, validated.OllamaHost)
	}

	args := openCodeBenchmarkArgs(validated)
	if len(args) < 2 || args[0] != "run" {
		t.Errorf("unexpected args: %#v", args)
	}

	env := openCodeBenchmarkEnvironment([]string{"ANTHROPIC_API_KEY=secret", "PATH=/bin"}, validated)
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			t.Errorf("ambient ANTHROPIC_API_KEY was not stripped from environment")
		}
	}
}

func TestValidateOpenCodeBenchmarkConfigErrors(t *testing.T) {
	temp := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCwd) }()
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}

	exePath := filepath.Join(temp, "mock-opencode.cmd")
	if err := os.WriteFile(exePath, []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing OpenCodePath
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{}); err == nil {
		t.Error("expected error for missing OpenCodePath")
	}

	// 2. Relative OpenCodePath
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: "opencode"}); err == nil {
		t.Error("expected error for relative OpenCodePath")
	}

	// 3. Nonexistent OpenCodePath
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: filepath.Join(temp, "missing.exe")}); err == nil {
		t.Error("expected error for nonexistent OpenCodePath")
	}

	// 4. OpenCodePath is a directory
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: temp}); err == nil {
		t.Error("expected error when OpenCodePath is a directory")
	}

	// 5. Missing Worktree
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath}); err == nil {
		t.Error("expected error for missing Worktree")
	}

	// 6. Relative Worktree
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: "sub"}); err == nil {
		t.Error("expected error for relative Worktree")
	}

	// 7. Nonexistent Worktree
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: filepath.Join(temp, "missing")}); err == nil {
		t.Error("expected error for nonexistent Worktree")
	}

	// 8. Worktree is a file
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: exePath}); err == nil {
		t.Error("expected error when Worktree is a file")
	}

	// 9. Cwd mismatch
	otherDir := t.TempDir()
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: otherDir}); err == nil {
		t.Error("expected error when cwd does not match worktree")
	}

	// 10. Missing Task
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: temp}); err == nil {
		t.Error("expected error for missing Task")
	}

	// 11. Missing Model
	if _, err := validateOpenCodeBenchmarkConfig(openCodeBenchmarkConfig{OpenCodePath: exePath, Worktree: temp, Task: "test task"}); err == nil {
		t.Error("expected error for missing Model")
	}
}

func TestNormalizeOpenCodeBenchmarkOllamaHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", defaultOpenCodeBenchmarkOllamaHost},
		{"   ", defaultOpenCodeBenchmarkOllamaHost},
		{"http://localhost:11434/", "http://localhost:11434"},
		{"http://localhost:11434/v1", "http://localhost:11434"},
		{"http://localhost:11434/v1/", "http://localhost:11434"},
		{"http://custom-host:8080", "http://custom-host:8080"},
	}
	for _, tc := range tests {
		got := normalizeOpenCodeBenchmarkOllamaHost(tc.input)
		if got != tc.want {
			t.Errorf("normalizeOpenCodeBenchmarkOllamaHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestLoadOpenCodeBenchmarkConfig(t *testing.T) {
	t.Setenv("LOCALCODE_BENCH_OPENCODE", "C:\\bin\\opencode.exe")
	t.Setenv("LOCALCODE_BENCH_WORKTREE", "C:\\repo")
	t.Setenv("LOCALCODE_BENCH_TASK", "do something")
	t.Setenv("LOCALCODE_BENCH_MODEL", "qwen2.5-coder")
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "http://127.0.0.1:11434")

	cfg := loadOpenCodeBenchmarkConfig()
	if cfg.OpenCodePath != "C:\\bin\\opencode.exe" || cfg.Worktree != "C:\\repo" || cfg.Task != "do something" || cfg.Model != "qwen2.5-coder" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	// Fallback to OLLAMA_HOST
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	cfg = loadOpenCodeBenchmarkConfig()
	if cfg.OllamaHost != "http://ollama-host:11434" {
		t.Fatalf("expected OLLAMA_HOST fallback, got %q", cfg.OllamaHost)
	}
}

func TestRunOpenCodeBenchmarkSmoke(t *testing.T) {
	temp := t.TempDir()
	scriptContent := "@echo off\r\necho OPENCODE_MOCK_SUCCESS\r\nexit /b 0\r\n"
	exePath := filepath.Join(temp, "mock-opencode.cmd")
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

	cfg := openCodeBenchmarkConfig{
		OpenCodePath: exePath,
		Worktree:     temp,
		Task:         "test task",
		Model:        "qwen2.5-coder:14b",
	}

	if err := runOpenCodeBenchmark(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected benchmark run error: %v", err)
	}
}
