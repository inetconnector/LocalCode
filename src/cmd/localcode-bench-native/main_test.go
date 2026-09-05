// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateNativeBenchmarkConfig(t *testing.T) {
	temp := t.TempDir()
	exePath := filepath.Join(temp, "mock-native.cmd")
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

	cfg := nativeBenchmarkConfig{
		NativePath: exePath,
		Worktree:   temp,
		Task:       "implement string calculator",
		Model:      "qwen2.5-coder:14b",
	}

	validated, err := validateNativeBenchmarkConfig(cfg)
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if validated.OllamaHost != defaultNativeBenchmarkOllamaHost {
		t.Errorf("expected default ollama host %q, got %q", defaultNativeBenchmarkOllamaHost, validated.OllamaHost)
	}

	args := nativeBenchmarkArgs(validated)
	if len(args) < 2 || args[0] != "--batch" {
		t.Errorf("unexpected args: %#v", args)
	}

	env := nativeBenchmarkEnvironment([]string{"OPENAI_API_KEY=secret", "PATH=/bin"}, validated)
	for _, e := range env {
		if strings.HasPrefix(e, "OPENAI_API_KEY=") {
			t.Errorf("ambient OPENAI_API_KEY was not stripped from environment")
		}
	}
}

func TestValidateNativeBenchmarkConfigErrors(t *testing.T) {
	temp := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origCwd) }()
	if err := os.Chdir(temp); err != nil {
		t.Fatal(err)
	}

	exePath := filepath.Join(temp, "mock-native.cmd")
	if err := os.WriteFile(exePath, []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing Worktree
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{}); err == nil {
		t.Error("expected error for missing Worktree")
	}

	// 2. Relative Worktree
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: "sub"}); err == nil {
		t.Error("expected error for relative Worktree")
	}

	// 3. Nonexistent Worktree
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: filepath.Join(temp, "missing")}); err == nil {
		t.Error("expected error for nonexistent Worktree")
	}

	// 4. Worktree is a file
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: exePath}); err == nil {
		t.Error("expected error when Worktree is a file")
	}

	// 5. Cwd mismatch
	otherDir := t.TempDir()
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: otherDir}); err == nil {
		t.Error("expected error when cwd does not match worktree")
	}

	// 6. Missing Task
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: temp}); err == nil {
		t.Error("expected error for missing Task")
	}

	// 7. Missing Model
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: temp, Task: "test task"}); err == nil {
		t.Error("expected error for missing Model")
	}

	// 8. Relative NativePath
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: temp, Task: "task", Model: "m", NativePath: "localcode.exe"}); err == nil {
		t.Error("expected error for relative NativePath")
	}

	// 9. Nonexistent NativePath
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: temp, Task: "task", Model: "m", NativePath: filepath.Join(temp, "missing.exe")}); err == nil {
		t.Error("expected error for nonexistent NativePath")
	}

	// 10. NativePath is a directory
	if _, err := validateNativeBenchmarkConfig(nativeBenchmarkConfig{Worktree: temp, Task: "task", Model: "m", NativePath: temp}); err == nil {
		t.Error("expected error when NativePath is a directory")
	}
}

func TestNormalizeNativeBenchmarkOllamaHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", defaultNativeBenchmarkOllamaHost},
		{"   ", defaultNativeBenchmarkOllamaHost},
		{"http://localhost:11434/", "http://localhost:11434"},
		{"http://localhost:11434/v1", "http://localhost:11434"},
		{"http://localhost:11434/v1/", "http://localhost:11434"},
		{"http://custom-host:8080", "http://custom-host:8080"},
	}
	for _, tc := range tests {
		got := normalizeNativeBenchmarkOllamaHost(tc.input)
		if got != tc.want {
			t.Errorf("normalizeNativeBenchmarkOllamaHost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestLoadNativeBenchmarkConfig(t *testing.T) {
	t.Setenv("LOCALCODE_BENCH_NATIVE", "C:\\bin\\localcode.exe")
	t.Setenv("LOCALCODE_BENCH_WORKTREE", "C:\\repo")
	t.Setenv("LOCALCODE_BENCH_TASK", "do something")
	t.Setenv("LOCALCODE_BENCH_MODEL", "qwen2.5-coder")
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "http://127.0.0.1:11434")

	cfg := loadNativeBenchmarkConfig()
	if cfg.NativePath != "C:\\bin\\localcode.exe" || cfg.Worktree != "C:\\repo" || cfg.Task != "do something" || cfg.Model != "qwen2.5-coder" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	// Fallback to OLLAMA_HOST
	t.Setenv("LOCALCODE_BENCH_OLLAMA_HOST", "")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	cfg = loadNativeBenchmarkConfig()
	if cfg.OllamaHost != "http://ollama-host:11434" {
		t.Fatalf("expected OLLAMA_HOST fallback, got %q", cfg.OllamaHost)
	}
}

func TestRunNativeBenchmarkSmoke(t *testing.T) {
	temp := t.TempDir()
	scriptContent := "@echo off\r\necho NATIVE_MOCK_SUCCESS\r\nexit /b 0\r\n"
	exePath := filepath.Join(temp, "mock-native.cmd")
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

	cfg := nativeBenchmarkConfig{
		NativePath: exePath,
		Worktree:   temp,
		Task:       "test task",
		Model:      "qwen2.5-coder:14b",
	}

	if err := runNativeBenchmark(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected benchmark run error: %v", err)
	}
}
