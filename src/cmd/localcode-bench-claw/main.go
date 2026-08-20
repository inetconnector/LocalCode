// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultClawBenchmarkOllamaHost = "http://127.0.0.1:11434"

type clawBenchmarkConfig struct {
	ClawPath   string
	Worktree   string
	Task       string
	Model      string
	OllamaHost string
}

func loadClawBenchmarkConfig() clawBenchmarkConfig {
	host := strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OLLAMA_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	return clawBenchmarkConfig{
		ClawPath:   strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_CLAW")),
		Worktree:   strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_WORKTREE")),
		Task:       strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_TASK")),
		Model:      strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_MODEL")),
		OllamaHost: normalizeClawBenchmarkOllamaHost(host),
	}
}

func normalizeClawBenchmarkOllamaHost(raw string) string {
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	if host == "" {
		host = defaultClawBenchmarkOllamaHost
	}
	if strings.HasSuffix(strings.ToLower(host), "/v1") {
		host = strings.TrimRight(host[:len(host)-3], "/")
	}
	return host
}

func clawBenchmarkArgs(cfg clawBenchmarkConfig) []string {
	return []string{
		"--output-format", "json",
		"--permission-mode", "workspace-write",
		"--model", cfg.Model,
		"prompt", cfg.Task,
	}
}

func removeClawBenchmarkEnvironmentKeys(env []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[strings.ToUpper(strings.TrimSpace(key))] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, item := range env {
		key := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if _, found := blocked[strings.ToUpper(strings.TrimSpace(key))]; found {
			continue
		}
		out = append(out, item)
	}
	return out
}

func clawBenchmarkEnvironment(base []string, cfg clawBenchmarkConfig) []string {
	env := removeClawBenchmarkEnvironmentKeys(base,
		"OLLAMA_HOST",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"XAI_BASE_URL", "XAI_API_KEY",
		"DASHSCOPE_BASE_URL", "DASHSCOPE_API_KEY",
		"CLAUDE_CODE_PROVIDER",
		"CLAW_MODEL", "CLAW_PERMISSION_MODE", "CLAW_OUTPUT_FORMAT",
	)
	return append(env,
		"OLLAMA_HOST="+normalizeClawBenchmarkOllamaHost(cfg.OllamaHost),
		"CLAW_OUTPUT_FORMAT=json",
	)
}

func sameBenchmarkPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func validateClawBenchmarkConfig(cfg clawBenchmarkConfig) (clawBenchmarkConfig, error) {
	if cfg.ClawPath == "" {
		return cfg, errors.New("LOCALCODE_BENCH_CLAW must point to the exact Claw executable under test")
	}
	if !filepath.IsAbs(cfg.ClawPath) {
		return cfg, errors.New("LOCALCODE_BENCH_CLAW must be an absolute path")
	}
	clawPath, err := filepath.Abs(cfg.ClawPath)
	if err != nil {
		return cfg, fmt.Errorf("resolve Claw executable: %w", err)
	}
	info, err := os.Stat(clawPath)
	if err != nil {
		return cfg, fmt.Errorf("stat Claw executable: %w", err)
	}
	if info.IsDir() {
		return cfg, errors.New("LOCALCODE_BENCH_CLAW points to a directory")
	}

	if cfg.Worktree == "" {
		return cfg, errors.New("LOCALCODE_BENCH_WORKTREE is required")
	}
	if !filepath.IsAbs(cfg.Worktree) {
		return cfg, errors.New("LOCALCODE_BENCH_WORKTREE must be an absolute path")
	}
	worktree, err := filepath.Abs(cfg.Worktree)
	if err != nil {
		return cfg, fmt.Errorf("resolve benchmark worktree: %w", err)
	}
	worktreeInfo, err := os.Stat(worktree)
	if err != nil {
		return cfg, fmt.Errorf("stat benchmark worktree: %w", err)
	}
	if !worktreeInfo.IsDir() {
		return cfg, errors.New("LOCALCODE_BENCH_WORKTREE is not a directory")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return cfg, fmt.Errorf("resolve current working directory: %w", err)
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return cfg, fmt.Errorf("resolve current working directory: %w", err)
	}
	if !sameBenchmarkPath(cwd, worktree) {
		return cfg, fmt.Errorf("Claw benchmark adapter must run inside LOCALCODE_BENCH_WORKTREE: cwd=%q worktree=%q", cwd, worktree)
	}
	if cfg.Task == "" {
		return cfg, errors.New("LOCALCODE_BENCH_TASK is required")
	}
	if cfg.Model == "" {
		return cfg, errors.New("LOCALCODE_BENCH_MODEL is required for a fair cross-engine run")
	}

	cfg.ClawPath = clawPath
	cfg.Worktree = worktree
	cfg.OllamaHost = normalizeClawBenchmarkOllamaHost(cfg.OllamaHost)
	return cfg, nil
}

func runClawBenchmark(ctx context.Context, cfg clawBenchmarkConfig) error {
	validated, err := validateClawBenchmarkConfig(cfg)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, validated.ClawPath, clawBenchmarkArgs(validated)...)
	cmd.Dir = validated.Worktree
	cmd.Env = clawBenchmarkEnvironment(os.Environ(), validated)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runClawBenchmark(ctx, loadClawBenchmarkConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "claw benchmark adapter:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(2)
	}
}
