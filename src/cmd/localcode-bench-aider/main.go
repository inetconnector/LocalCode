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

const defaultAiderBenchmarkOllamaHost = "http://127.0.0.1:11434"

type aiderBenchmarkConfig struct {
	AiderPath  string
	Worktree   string
	Task       string
	Model      string
	OllamaHost string
}

func loadAiderBenchmarkConfig() aiderBenchmarkConfig {
	host := strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OLLAMA_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("OLLAMA_API_BASE"))
	}
	if host == "" {
		host = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	return aiderBenchmarkConfig{
		AiderPath:  strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_AIDER")),
		Worktree:   strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_WORKTREE")),
		Task:       strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_TASK")),
		Model:      strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_MODEL")),
		OllamaHost: normalizeAiderBenchmarkOllamaHost(host),
	}
}

func normalizeAiderBenchmarkOllamaHost(raw string) string {
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	if host == "" {
		host = defaultAiderBenchmarkOllamaHost
	}
	if strings.HasSuffix(strings.ToLower(host), "/v1") {
		host = strings.TrimRight(host[:len(host)-3], "/")
	}
	return host
}

func aiderBenchmarkModel(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(strings.ToLower(model), "ollama/") || strings.HasPrefix(strings.ToLower(model), "ollama_chat/") {
		return model
	}
	return "ollama_chat/" + model
}

func aiderBenchmarkArgs(cfg aiderBenchmarkConfig) []string {
	return []string{
		"--yes",
		"--model", aiderBenchmarkModel(cfg.Model),
		"--message", cfg.Task,
		"--no-auto-commits",
		"--no-notifications",
		"--no-fancy-input",
		"--no-suggest-shell-commands",
		"--no-browser",
	}
}

func removeAiderBenchmarkEnvironmentKeys(env []string, keys ...string) []string {
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

func aiderBenchmarkEnvironment(base []string, cfg aiderBenchmarkConfig) []string {
	env := removeAiderBenchmarkEnvironmentKeys(base,
		"OLLAMA_API_BASE", "OLLAMA_HOST",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"XAI_BASE_URL", "XAI_API_KEY",
		"DASHSCOPE_BASE_URL", "DASHSCOPE_API_KEY",
	)
	host := normalizeAiderBenchmarkOllamaHost(cfg.OllamaHost)
	return append(env,
		"OLLAMA_API_BASE="+host,
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

func validateAiderBenchmarkConfig(cfg aiderBenchmarkConfig) (aiderBenchmarkConfig, error) {
	if cfg.AiderPath == "" {
		return cfg, errors.New("LOCALCODE_BENCH_AIDER must point to the exact Aider executable under test")
	}
	if !filepath.IsAbs(cfg.AiderPath) {
		return cfg, errors.New("LOCALCODE_BENCH_AIDER must be an absolute path")
	}
	aiderPath, err := filepath.Abs(cfg.AiderPath)
	if err != nil {
		return cfg, fmt.Errorf("resolve Aider executable: %w", err)
	}
	info, err := os.Stat(aiderPath)
	if err != nil {
		return cfg, fmt.Errorf("stat Aider executable: %w", err)
	}
	if info.IsDir() {
		return cfg, errors.New("LOCALCODE_BENCH_AIDER points to a directory")
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
		return cfg, fmt.Errorf("Aider benchmark adapter must run inside LOCALCODE_BENCH_WORKTREE: cwd=%q worktree=%q", cwd, worktree)
	}
	if cfg.Task == "" {
		return cfg, errors.New("LOCALCODE_BENCH_TASK is required")
	}
	if cfg.Model == "" {
		return cfg, errors.New("LOCALCODE_BENCH_MODEL is required for a fair cross-engine run")
	}

	cfg.AiderPath = aiderPath
	cfg.Worktree = worktree
	cfg.OllamaHost = normalizeAiderBenchmarkOllamaHost(cfg.OllamaHost)
	return cfg, nil
}

func runAiderBenchmark(ctx context.Context, cfg aiderBenchmarkConfig) error {
	validated, err := validateAiderBenchmarkConfig(cfg)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, validated.AiderPath, aiderBenchmarkArgs(validated)...)
	cmd.Dir = validated.Worktree
	cmd.Env = aiderBenchmarkEnvironment(os.Environ(), validated)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runAiderBenchmark(ctx, loadAiderBenchmarkConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "aider benchmark adapter:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(2)
	}
}
