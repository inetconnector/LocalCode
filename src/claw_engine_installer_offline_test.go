// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func isolateClawManagedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", home)
	return home
}

func TestClawManagedPathsStayInsideLocalCodeConfigHome(t *testing.T) {
	home := isolateClawManagedHome(t)
	for _, path := range []string{clawToolRoot(), clawManagedSourceRoot(), clawManagedBinary()} {
		clean := filepath.Clean(path)
		if !strings.HasPrefix(strings.ToLower(clean), strings.ToLower(filepath.Clean(home))+string(filepath.Separator)) {
			t.Fatalf("managed Claw path escaped LocalCode config home: %q outside %q", clean, home)
		}
	}
}

func TestWriteFileAtomicCreatesAndReplacesManagedBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin", "claw-test.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("first"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("second"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("atomic managed write left unexpected content %q", data)
	}
}

func TestEnsureClawBuildDependencyUsesExistingGitAndRejectsUnknownTool(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required by the LocalCode repository but was not found in this test environment")
	}
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	updated, path, err := ensureClawBuildDependency(context.Background(), t.TempDir(), "git", cfg)
	if err != nil {
		t.Fatalf("existing git should be reused without download: %v", err)
	}
	if strings.TrimSpace(path) == "" || updated.RootProjectDir == "" {
		t.Fatalf("existing git dependency result incomplete: path=%q cfg=%#v", path, updated)
	}
	if _, _, err := ensureClawBuildDependency(context.Background(), t.TempDir(), "definitely-missing-claw-tool", cfg); err == nil {
		t.Fatal("unknown Claw build dependency must fail closed")
	}
}

func TestPreparePinnedClawSourceRejectsNonDirectoryManagedPath(t *testing.T) {
	isolateClawManagedHome(t)
	root := clawManagedSourceRoot()
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("not a repository"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePinnedClawSource(context.Background(), "git", defaultConfig()); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("non-directory managed source must fail closed, got %v", err)
	}
}

func TestPreparePinnedClawSourceRejectsUnexpectedGitOriginWithoutNetwork(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	isolateClawManagedHome(t)
	root := clawManagedSourceRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(gitPath, "-C", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if output, err := exec.Command(gitPath, "-C", root, "remote", "add", "origin", "https://example.invalid/not-claw.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, output)
	}
	_, detail, err := preparePinnedClawSource(context.Background(), gitPath, defaultConfig())
	if err == nil || !strings.Contains(err.Error(), "unexpected origin") {
		t.Fatalf("unexpected origin must be rejected before fetch, err=%v detail=%q", err, detail)
	}
}

func TestInstallClawCodeHonorsDisabledDownloads(t *testing.T) {
	isolateClawManagedHome(t)
	isolateClawExecutableSearch(t)
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	_, _, detail, err := installClawCode(context.Background(), t.TempDir(), cfg)
	if err == nil || !strings.Contains(err.Error(), "downloads") {
		t.Fatalf("disabled setup downloads must block Claw installation, err=%v detail=%q", err, detail)
	}
}
