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
