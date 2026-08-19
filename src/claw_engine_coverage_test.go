// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
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

func isolateClawExecutableSearch(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("APPDATA", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("PATH", "")
}

func TestSelectedEngineModelRoutesClawToLocalOllamaName(t *testing.T) {
	cfg := defaultConfig()
	cfg.EditingEngine = editingEngineClaw
	cfg.OllamaDefaultModel = "qwen2.5-coder:7b"
	if got := selectedEngineModel(cfg, "qwen2.5-coder:14b"); got != "qwen2.5-coder:14b" {
		t.Fatalf("selected Claw model = %q", got)
	}
	if got := selectedEngineModel(cfg, ""); got != "qwen2.5-coder:7b" {
		t.Fatalf("default Claw model = %q", got)
	}
}

func TestInstallCodingEngineDispatchesClawWithoutBypassingDownloadPolicy(t *testing.T) {
	isolateClawExecutableSearch(t)
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	status, returned, output, err := installCodingEngine(context.Background(), t.TempDir(), editingEngineClaw, cfg)
	if err == nil || !strings.Contains(err.Error(), "downloads for automatic setup are disabled") {
		t.Fatalf("expected Claw installer policy error, got %v", err)
	}
	if status.Engine != editingEngineClaw || returned.SetupDownloadsEnabled || output != "" {
		t.Fatalf("unexpected Claw install dispatch result: status=%#v downloads=%v output=%q", status, returned.SetupDownloadsEnabled, output)
	}
}

func TestRunCodingEngineReturnsTypedNotInstalledForClaw(t *testing.T) {
	isolateClawExecutableSearch(t)
	cfg := defaultConfig()
	cfg.EditingEngine = editingEngineClaw
	project := t.TempDir()
	_, err := runCodingEngine(context.Background(), project, "inspect", "qwen2.5-coder:14b", "", "repo-map", cfg)
	var missing *CodingEngineNotInstalledError
	if !errors.As(err, &missing) {
		t.Fatalf("expected CodingEngineNotInstalledError, got %T: %v", err, err)
	}
	if missing.Status.Engine != editingEngineClaw {
		t.Fatalf("missing-engine status = %#v", missing.Status)
	}
}
