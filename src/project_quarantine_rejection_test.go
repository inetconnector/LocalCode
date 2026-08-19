// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestQuarantineProjectRejectsMissingAndEmptyProjects(t *testing.T) {
	root := t.TempDir()
	if _, err := quarantineProject(root, filepath.Join(root, "Missing")); err == nil {
		t.Fatal("missing project must not be quarantined")
	}

	empty := filepath.Join(root, "Empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := quarantineProject(root, empty); err == nil || !strings.Contains(err.Error(), "delete_empty") {
		t.Fatalf("empty project must use delete_empty, got %v", err)
	}
}

func TestQuarantineProjectRejectsProjectSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows CI may not grant symlink creation privileges; symlink behavior is covered by dedicated Windows-safe metadata tests")
	}
	root := t.TempDir()
	target := filepath.Join(root, "Real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "data.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "Linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := quarantineProject(root, link); err == nil || !strings.Contains(err.Error(), "real project directory") {
		t.Fatalf("project symlink must fail closed, got %v", err)
	}
}

func TestQuarantineProjectRejectsNonDirectoryQuarantineRoot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "data.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := projectQuarantineRoot(root)
	if err := os.WriteFile(quarantine, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := quarantineProject(root, project); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("non-directory quarantine root must fail closed, got %v", err)
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("project must remain untouched after quarantine-root rejection: %v", err)
	}
}
