// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"localcode/benchharness"
)

func main() {
	manifestPath := flag.String("manifest", "", "Path to benchmark manifest JSON")
	outputPath := flag.String("out", "", "Optional result JSON path")
	keep := flag.Bool("keep-worktree", false, "Keep isolated benchmark worktree after the run")
	flag.Parse()
	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "usage: localcode-bench -manifest benchmark.json [-out result.json] [-keep-worktree]")
		os.Exit(2)
	}
	manifest, err := benchharness.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "manifest:", err)
		os.Exit(2)
	}
	if *keep {
		manifest.KeepWorktree = true
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := (benchharness.Runner{}).Run(ctx, manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(2)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "result:", err)
		os.Exit(2)
	}
	data = append(data, '\n')
	if *outputPath != "" {
		path := filepath.Clean(*outputPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "output:", err)
			os.Exit(2)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "output:", err)
			os.Exit(2)
		}
	}
	_, _ = os.Stdout.Write(data)
	if !result.Success {
		os.Exit(1)
	}
}
