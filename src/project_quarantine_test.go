// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectQuarantineMovesListsAndRestoresProject(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Demo")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Name != "Demo" || entry.Files != 1 || entry.Bytes != 5 || entry.ID == "" {
		t.Fatalf("unexpected quarantine metadata: %#v", entry)
	}
	if _, err := os.Stat(project); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original project still exists after quarantine: %v", err)
	}
	payload := filepath.Join(projectQuarantineRoot(root), entry.ID, "project", "main.txt")
	if data, err := os.ReadFile(payload); err != nil || string(data) != "hello" {
		t.Fatalf("quarantine payload missing/corrupt: %q err=%v", data, err)
	}

	listed, err := listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != entry.ID {
		t.Fatalf("unexpected quarantine list: %#v", listed)
	}

	restored, err := restoreQuarantinedProject(root, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.OriginalPath != project {
		t.Fatalf("restored wrong project: %#v", restored)
	}
	if data, err := os.ReadFile(filepath.Join(project, "main.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("restored project missing/corrupt: %q err=%v", data, err)
	}
	listed, err = listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("restored project remained in quarantine list: %#v", listed)
	}
}

func TestRestoreQuarantinedProjectRefusesOccupiedDestination(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Occupied")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := restoreQuarantinedProject(root, entry.ID); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("occupied restore must fail safely, got %v", err)
	}
	if _, _, err := loadQuarantinedProject(root, entry.ID); err != nil {
		t.Fatalf("failed restore must keep quarantine entry recoverable: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(project, "new.txt")); err != nil || string(data) != "new" {
		t.Fatalf("occupied destination was modified: %q err=%v", data, err)
	}
}

func TestPurgeQuarantinedProjectRequiresExactConfirmation(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Danger")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "data.txt"), []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"", "Danger", "purge Danger", "PURGE danger"} {
		if _, err := purgeQuarantinedProject(root, entry.ID, bad); err == nil {
			t.Fatalf("purge accepted unsafe confirmation %q", bad)
		}
		if _, _, err := loadQuarantinedProject(root, entry.ID); err != nil {
			t.Fatalf("failed purge removed recoverable entry: %v", err)
		}
	}
	if _, err := purgeQuarantinedProject(root, entry.ID, "PURGE Danger"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(projectQuarantineRoot(root), entry.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed purge left quarantine entry: %v", err)
	}
}

func TestQuarantineRootSymlinkIsRejected(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	link := projectQuarantineRoot(root)
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink creation unavailable on this Windows runner: %v", err)
	}
	if _, err := ensureProjectQuarantineRoot(root); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("symlink quarantine root must be rejected, got %v", err)
	}
}

func TestPurgeDoesNotFollowSymlinkInsideProject(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Linked")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "local.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	externalFile := filepath.Join(external, "keep.txt")
	if err := os.WriteFile(externalFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(project, "external-link")); err != nil {
		t.Skipf("symlink creation unavailable on this Windows runner: %v", err)
	}

	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Symlinks != 1 {
		t.Fatalf("delete preview lost symlink count: %#v", entry)
	}
	if _, err := purgeQuarantinedProject(root, entry.ID, "PURGE Linked"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(externalFile); err != nil || string(data) != "keep" {
		t.Fatalf("purge followed project symlink into external target: %q err=%v", data, err)
	}
}

func TestListQuarantinedProjectsWithoutStoreIsEmpty(t *testing.T) {
	entries, err := listQuarantinedProjects(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected quarantine entries: %#v", entries)
	}
}

func TestQuarantineProjectRejectsEmptyAndNonDirectoryTargets(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "Empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := quarantineProject(root, empty); err == nil || !strings.Contains(err.Error(), "empty project") {
		t.Fatalf("empty project must use delete_empty, got %v", err)
	}

	file := filepath.Join(root, "NotADirectory")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := quarantineProject(root, file); err == nil || !strings.Contains(err.Error(), "real project directory") {
		t.Fatalf("non-directory quarantine target must fail closed, got %v", err)
	}
}

func TestEnsureProjectQuarantineRootRejectsRegularFile(t *testing.T) {
	root := t.TempDir()
	path := projectQuarantineRoot(root)
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureProjectQuarantineRoot(root); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("regular-file quarantine root must be rejected, got %v", err)
	}
}

func TestLoadQuarantinedProjectRejectsInvalidIDAndIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	if _, _, err := loadQuarantinedProject(root, "../escape"); err == nil || !strings.Contains(err.Error(), "invalid quarantine id") {
		t.Fatalf("invalid quarantine id must fail before filesystem access, got %v", err)
	}

	project := filepath.Join(root, "Mismatch")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "data.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	entryDir := filepath.Join(projectQuarantineRoot(root), entry.ID)
	metadata := quarantineMetadataPath(entryDir)
	data, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := strings.Replace(string(data), `"id": "`+entry.ID+`"`, `"id": "different"`, 1)
	if corrupt == string(data) {
		t.Fatal("test could not alter quarantine metadata id")
	}
	if err := os.WriteFile(metadata, []byte(corrupt), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadQuarantinedProject(root, entry.ID); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("metadata identity mismatch must fail closed, got %v", err)
	}
}

func TestLoadQuarantinedProjectRejectsMalformedMetadata(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Malformed")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "data.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	metadata := quarantineMetadataPath(filepath.Join(projectQuarantineRoot(root), entry.ID))
	if err := os.WriteFile(metadata, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadQuarantinedProject(root, entry.ID); err == nil {
		t.Fatal("malformed quarantine metadata must be rejected")
	}
}
