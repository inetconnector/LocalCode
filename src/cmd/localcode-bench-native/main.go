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

const defaultNativeBenchmarkOllamaHost = "http://127.0.0.1:11434"

type nativeBenchmarkConfig struct {
	NativePath string
	Worktree   string
	Task       string
	Model      string
	OllamaHost string
}

func loadNativeBenchmarkConfig() nativeBenchmarkConfig {
	host := strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OLLAMA_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	return nativeBenchmarkConfig{
		NativePath: strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_NATIVE")),
		Worktree:   strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_WORKTREE")),
		Task:       strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_TASK")),
		Model:      strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_MODEL")),
		OllamaHost: normalizeNativeBenchmarkOllamaHost(host),
	}
}

func normalizeNativeBenchmarkOllamaHost(raw string) string {
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	if host == "" {
		host = defaultNativeBenchmarkOllamaHost
	}
	if strings.HasSuffix(strings.ToLower(host), "/v1") {
		host = strings.TrimRight(host[:len(host)-3], "/")
	}
	return host
}

func nativeBenchmarkArgs(cfg nativeBenchmarkConfig) []string {
	return []string{
		"--batch",
		"--worktree", cfg.Worktree,
		"--model", cfg.Model,
		"--task", cfg.Task,
	}
}

func removeNativeBenchmarkEnvironmentKeys(env []string, keys ...string) []string {
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

func nativeBenchmarkEnvironment(base []string, cfg nativeBenchmarkConfig) []string {
	env := removeNativeBenchmarkEnvironmentKeys(base,
		"OLLAMA_HOST",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"XAI_BASE_URL", "XAI_API_KEY",
		"DASHSCOPE_BASE_URL", "DASHSCOPE_API_KEY",
	)
	host := normalizeNativeBenchmarkOllamaHost(cfg.OllamaHost)
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

func validateNativeBenchmarkConfig(cfg nativeBenchmarkConfig) (nativeBenchmarkConfig, error) {
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
		return cfg, fmt.Errorf("Native benchmark adapter must run inside LOCALCODE_BENCH_WORKTREE: cwd=%q worktree=%q", cwd, worktree)
	}
	if cfg.Task == "" {
		return cfg, errors.New("LOCALCODE_BENCH_TASK is required")
	}
	if cfg.Model == "" {
		return cfg, errors.New("LOCALCODE_BENCH_MODEL is required for a fair cross-engine run")
	}

	if cfg.NativePath != "" {
		if !filepath.IsAbs(cfg.NativePath) {
			return cfg, errors.New("LOCALCODE_BENCH_NATIVE must be an absolute path")
		}
		nativePath, err := filepath.Abs(cfg.NativePath)
		if err != nil {
			return cfg, fmt.Errorf("resolve Native executable: %w", err)
		}
		info, err := os.Stat(nativePath)
		if err != nil {
			return cfg, fmt.Errorf("stat Native executable: %w", err)
		}
		if info.IsDir() {
			return cfg, errors.New("LOCALCODE_BENCH_NATIVE points to a directory")
		}
		cfg.NativePath = nativePath
	}

	cfg.Worktree = worktree
	cfg.OllamaHost = normalizeNativeBenchmarkOllamaHost(cfg.OllamaHost)
	return cfg, nil
}

func runNativeBenchmark(ctx context.Context, cfg nativeBenchmarkConfig) error {
	validated, err := validateNativeBenchmarkConfig(cfg)
	if err != nil {
		return err
	}
	executable := validated.NativePath
	if executable == "" {
		executable = os.Args[0]
	}
	cmd := exec.CommandContext(ctx, executable, nativeBenchmarkArgs(validated)...)
	cmd.Dir = validated.Worktree
	cmd.Env = nativeBenchmarkEnvironment(os.Environ(), validated)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runNativeBenchmark(ctx, loadNativeBenchmarkConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "native benchmark adapter:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(2)
	}
}
