// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicCreatesAndReplacesManagedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "claw.bin")
	if err := writeFileAtomic(path, []byte("first"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" {
		t.Fatalf("initial atomic content = %q", data)
	}
	if err := writeFileAtomic(path, []byte("second"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("replacement atomic content = %q", data)
	}
}

func TestClawManagedPathsStayInsideLocalCodeToolRoot(t *testing.T) {
	root := filepath.Clean(clawToolRoot())
	source := filepath.Clean(clawManagedSourceRoot())
	binary := filepath.Clean(clawManagedBinary())
	for label, path := range map[string]string{"source": source, "binary": binary} {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("%s relative path: %v", label, err)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			t.Fatalf("%s escaped managed Claw root: root=%q path=%q rel=%q", label, root, path, rel)
		}
	}
}

func TestEnsureClawBuildDependencyHonorsVerifiedToolOverride(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ToolOverrides == nil {
		cfg.ToolOverrides = map[string]string{}
	}
	cfg.ToolOverrides["cargo"] = os.Args[0]
	updated, path, err := ensureClawBuildDependency(context.Background(), t.TempDir(), "cargo", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" || filepath.Clean(path) != filepath.Clean(os.Args[0]) {
		t.Fatalf("resolved cargo path = %q; want %q", path, os.Args[0])
	}
	if updated.ToolOverrides["cargo"] != os.Args[0] {
		t.Fatalf("tool override changed unexpectedly: %#v", updated.ToolOverrides)
	}
}

func TestEnsureClawBuildDependencyFailsClosedForUnknownTool(t *testing.T) {
	cfg := defaultConfig()
	_, _, err := ensureClawBuildDependency(context.Background(), t.TempDir(), "localcode-claw-definitely-missing-tool", cfg)
	if err == nil || !strings.Contains(err.Error(), "required to build Claw Code") {
		t.Fatalf("expected explicit missing dependency error, got %v", err)
	}
}

func TestInstallClawCodeRespectsSetupDownloadPolicy(t *testing.T) {
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	status, returned, output, err := installClawCode(context.Background(), t.TempDir(), cfg)
	if err == nil || !strings.Contains(err.Error(), "downloads for automatic setup are disabled") {
		t.Fatalf("expected setup-download policy error, got %v", err)
	}
	if output != "" {
		t.Fatalf("disabled installer unexpectedly produced output: %q", output)
	}
	if returned.SetupDownloadsEnabled {
		t.Fatal("installer changed disabled setup-download policy")
	}
	if status.Engine != editingEngineClaw {
		t.Fatalf("status engine = %q; want %q", status.Engine, editingEngineClaw)
	}
}
