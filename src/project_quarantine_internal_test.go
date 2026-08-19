// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAtomicWriteQuarantineMetadataCreatesAndReplaces(t *testing.T) {
	root := t.TempDir()
	entryDir := filepath.Join(root, "entry")
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		t.Fatal(err)
	}
	first := QuarantinedProject{ID: "one", Name: "Demo", OriginalPath: filepath.Join(root, "Demo"), QuarantinedAt: time.Unix(1, 0)}
	if err := atomicWriteQuarantineMetadata(entryDir, first); err != nil {
		t.Fatal(err)
	}
	path := quarantineMetadataPath(entryDir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"id": "one"`) || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("unexpected first metadata: %s", data)
	}
	second := QuarantinedProject{ID: "two", Name: "Demo2", OriginalPath: filepath.Join(root, "Demo2"), QuarantinedAt: time.Unix(2, 0)}
	if err := atomicWriteQuarantineMetadata(entryDir, second); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"id": "one"`) || !strings.Contains(string(data), `"id": "two"`) {
		t.Fatalf("metadata replacement was not atomic/current: %s", data)
	}
	matches, err := filepath.Glob(filepath.Join(entryDir, ".metadata-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary metadata files leaked: %v", matches)
	}
}

func TestAtomicWriteQuarantineMetadataFailsWhenEntryMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := atomicWriteQuarantineMetadata(missing, QuarantinedProject{ID: "x"})
	if err == nil {
		t.Fatal("metadata write must fail when entry directory does not exist")
	}
}

func TestRollbackQuarantinePayloadCoversNoopConflictAndRestore(t *testing.T) {
	t.Run("missing payload is already rolled back", func(t *testing.T) {
		root := t.TempDir()
		if err := rollbackQuarantinePayload(filepath.Join(root, "project"), filepath.Join(root, "payload")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("occupied source fails closed", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "project")
		payload := filepath.Join(root, "payload")
		if err := os.Mkdir(source, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(payload, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := rollbackQuarantinePayload(source, payload); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("occupied rollback destination must fail closed, got %v", err)
		}
		if _, err := os.Stat(payload); err != nil {
			t.Fatalf("payload must remain after refused rollback: %v", err)
		}
	})

	t.Run("payload is restored by rename", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "project")
		payload := filepath.Join(root, "payload")
		if err := os.Mkdir(payload, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(payload, "keep.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := rollbackQuarantinePayload(source, payload); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(source, "keep.txt")); err != nil {
			t.Fatalf("rollback did not restore payload: %v", err)
		}
		if _, err := os.Stat(payload); !os.IsNotExist(err) {
			t.Fatalf("payload source should be gone after rollback, got %v", err)
		}
	})
}

func TestRemoveAllNoFollowHandlesFileDirectoryAndMissing(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "one.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeAllNoFollow(file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, got %v", err)
	}

	dir := filepath.Join(root, "dir")
	if err := os.MkdirAll(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeAllNoFollow(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory should be removed, got %v", err)
	}
	if err := removeAllNoFollow(filepath.Join(root, "missing")); err != nil {
		t.Fatalf("missing path should be an idempotent remove: %v", err)
	}
}

func TestListQuarantinedProjectsCreatesEmptyStore(t *testing.T) {
	root := t.TempDir()
	entries, err := listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("new quarantine store should be empty: %#v", entries)
	}
	info, err := os.Stat(projectQuarantineRoot(root))
	if err != nil || !info.IsDir() {
		t.Fatalf("quarantine root should be created as a directory: info=%v err=%v", info, err)
	}
}
