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

const defaultOpenCodeBenchmarkOllamaHost = "http://127.0.0.1:11434"

type openCodeBenchmarkConfig struct {
	OpenCodePath string
	Worktree     string
	Task         string
	Model        string
	OllamaHost   string
}

func loadOpenCodeBenchmarkConfig() openCodeBenchmarkConfig {
	host := strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OLLAMA_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	return openCodeBenchmarkConfig{
		OpenCodePath: strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OPENCODE")),
		Worktree:     strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_WORKTREE")),
		Task:         strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_TASK")),
		Model:        strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_MODEL")),
		OllamaHost:   normalizeOpenCodeBenchmarkOllamaHost(host),
	}
}

func normalizeOpenCodeBenchmarkOllamaHost(raw string) string {
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	if host == "" {
		host = defaultOpenCodeBenchmarkOllamaHost
	}
	if strings.HasSuffix(strings.ToLower(host), "/v1") {
		host = strings.TrimRight(host[:len(host)-3], "/")
	}
	return host
}

func openCodeBenchmarkArgs(cfg openCodeBenchmarkConfig) []string {
	return []string{
		"run",
		"--model", cfg.Model,
		"--prompt", cfg.Task,
		"--auto-approve",
		"--format", "json",
	}
}

func removeOpenCodeBenchmarkEnvironmentKeys(env []string, keys ...string) []string {
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

func openCodeBenchmarkEnvironment(base []string, cfg openCodeBenchmarkConfig) []string {
	env := removeOpenCodeBenchmarkEnvironmentKeys(base,
		"OLLAMA_HOST",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"XAI_BASE_URL", "XAI_API_KEY",
		"DASHSCOPE_BASE_URL", "DASHSCOPE_API_KEY",
	)
	host := normalizeOpenCodeBenchmarkOllamaHost(cfg.OllamaHost)
	return append(env,
		"OLLAMA_HOST="+host,
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

func validateOpenCodeBenchmarkConfig(cfg openCodeBenchmarkConfig) (openCodeBenchmarkConfig, error) {
	if cfg.OpenCodePath == "" {
		return cfg, errors.New("LOCALCODE_BENCH_OPENCODE must point to the exact OpenCode executable under test")
	}
	if !filepath.IsAbs(cfg.OpenCodePath) {
		return cfg, errors.New("LOCALCODE_BENCH_OPENCODE must be an absolute path")
	}
	openCodePath, err := filepath.Abs(cfg.OpenCodePath)
	if err != nil {
		return cfg, fmt.Errorf("resolve OpenCode executable: %w", err)
	}
	info, err := os.Stat(openCodePath)
	if err != nil {
		return cfg, fmt.Errorf("stat OpenCode executable: %w", err)
	}
	if info.IsDir() {
		return cfg, errors.New("LOCALCODE_BENCH_OPENCODE points to a directory")
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
		return cfg, fmt.Errorf("OpenCode benchmark adapter must run inside LOCALCODE_BENCH_WORKTREE: cwd=%q worktree=%q", cwd, worktree)
	}
	if cfg.Task == "" {
		return cfg, errors.New("LOCALCODE_BENCH_TASK is required")
	}
	if cfg.Model == "" {
		return cfg, errors.New("LOCALCODE_BENCH_MODEL is required for a fair cross-engine run")
	}

	cfg.OpenCodePath = openCodePath
	cfg.Worktree = worktree
	cfg.OllamaHost = normalizeOpenCodeBenchmarkOllamaHost(cfg.OllamaHost)
	return cfg, nil
}

func runOpenCodeBenchmark(ctx context.Context, cfg openCodeBenchmarkConfig) error {
	validated, err := validateOpenCodeBenchmarkConfig(cfg)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, validated.OpenCodePath, openCodeBenchmarkArgs(validated)...)
	cmd.Dir = validated.Worktree
	cmd.Env = openCodeBenchmarkEnvironment(os.Environ(), validated)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runOpenCodeBenchmark(ctx, loadOpenCodeBenchmarkConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "opencode benchmark adapter:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(2)
	}
}
