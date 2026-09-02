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
