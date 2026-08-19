// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestRunClawBenchmarkLaunchesValidatedExecutable(t *testing.T) {
	worktree := t.TempDir()
	t.Chdir(worktree)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := clawBenchmarkConfig{
		ClawPath: executable,
		Worktree: worktree,
		Task:     "exercise the validated process launch path",
		Model:    "qwen2.5-coder:14b",
	}
	err = runClawBenchmark(t.Context(), cfg)
	if err == nil {
		t.Fatal("test executable unexpectedly accepted Claw CLI arguments")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("validated benchmark did not reach process execution: %T: %v", err, err)
	}
}
