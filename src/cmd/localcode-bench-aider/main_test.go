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
