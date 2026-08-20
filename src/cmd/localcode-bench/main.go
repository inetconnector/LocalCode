// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"localcode/benchharness"
)

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("localcode-bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "Path to benchmark manifest JSON")
	outputPath := flags.String("out", "", "Optional result JSON path")
	keep := flags.Bool("keep-worktree", false, "Keep isolated benchmark worktree after the run")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" {
		fmt.Fprintln(stderr, "usage: localcode-bench -manifest benchmark.json [-out result.json] [-keep-worktree]")
		return 2
	}
	manifest, err := benchharness.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, "manifest:", err)
		return 2
	}
	if *keep {
		manifest.KeepWorktree = true
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := (benchharness.Runner{}).Run(ctx, manifest)
	if err != nil {
		fmt.Fprintln(stderr, "benchmark:", err)
		return 2
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "result:", err)
		return 2
	}
	data = append(data, '\n')
	if *outputPath != "" {
		path := filepath.Clean(*outputPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(stderr, "output:", err)
			return 2
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintln(stderr, "output:", err)
			return 2
		}
	}
	_, _ = stdout.Write(data)
	if !result.Success {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
