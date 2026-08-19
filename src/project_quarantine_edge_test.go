// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQuarantineRejectsEmptyProjectAndInvalidIDs(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "Empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := quarantineProject(root, empty); err == nil || !strings.Contains(err.Error(), "delete_empty") {
		t.Fatalf("empty project must use delete_empty, got %v", err)
	}
	for _, id := range []string{"", "..", "../escape", `a\\b`, "a/b", "C:escape"} {
		if validQuarantineID(id) {
			t.Fatalf("unsafe quarantine id accepted: %q", id)
		}
		if _, _, err := loadQuarantinedProject(root, id); err == nil || !strings.Contains(err.Error(), "invalid quarantine id") {
			t.Fatalf("load accepted invalid id %q: %v", id, err)
		}
	}
	if !validQuarantineID("123-safe_id") {
		t.Fatal("safe quarantine id was rejected")
	}
}

func TestQuarantineRootMustBeDirectory(t *testing.T) {
	root := t.TempDir()
	path := projectQuarantineRoot(root)
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureProjectQuarantineRoot(root); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("file quarantine root must be rejected, got %v", err)
	}
}

func writeRawQuarantineEntry(t *testing.T, root, id string, metadata any, withPayload bool) string {
	t.Helper()
	store, err := ensureProjectQuarantineRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	entryDir := filepath.Join(store, id)
	if err := os.Mkdir(entryDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(quarantineMetadataPath(entryDir), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if withPayload {
		if err := os.Mkdir(quarantinePayloadPath(entryDir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return entryDir
}

func TestLoadQuarantinedProjectRejectsCorruptMetadata(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		root := t.TempDir()
		store, err := ensureProjectQuarantineRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		id := "bad-json"
		entryDir := filepath.Join(store, id)
		if err := os.Mkdir(entryDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(quarantineMetadataPath(entryDir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadQuarantinedProject(root, id); err == nil {
			t.Fatal("invalid metadata JSON must be rejected")
		}
	})

	t.Run("identity mismatch", func(t *testing.T) {
		root := t.TempDir()
		id := "entry-1"
		writeRawQuarantineEntry(t, root, id, QuarantinedProject{ID: "other", Name: "Demo", OriginalPath: filepath.Join(root, "Demo")}, true)
		if _, _, err := loadQuarantinedProject(root, id); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("identity mismatch must be rejected, got %v", err)
		}
	})

	t.Run("invalid original path", func(t *testing.T) {
		root := t.TempDir()
		id := "entry-2"
		writeRawQuarantineEntry(t, root, id, QuarantinedProject{ID: id, Name: "Elsewhere", OriginalPath: filepath.Join(t.TempDir(), "Elsewhere")}, true)
		if _, _, err := loadQuarantinedProject(root, id); err == nil || !strings.Contains(err.Error(), "invalid original project path") {
			t.Fatalf("escaped original path must be rejected, got %v", err)
		}
	})

	t.Run("payload is file", func(t *testing.T) {
		root := t.TempDir()
		id := "entry-4"
		entryDir := writeRawQuarantineEntry(t, root, id, QuarantinedProject{ID: id, Name: "Demo", OriginalPath: filepath.Join(root, "Demo")}, false)
		if err := os.WriteFile(quarantinePayloadPath(entryDir), []byte("bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadQuarantinedProject(root, id); err == nil || !strings.Contains(err.Error(), "real directory") {
			t.Fatalf("non-directory payload must be rejected, got %v", err)
		}
	})
}

func TestListQuarantinedProjectsSkipsGarbageAndSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	store, err := ensureProjectQuarantineRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "junk-file"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store, "..bad"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeRawQuarantineEntry(t, root, "broken", QuarantinedProject{ID: "wrong", Name: "Broken", OriginalPath: filepath.Join(root, "Broken")}, true)

	olderDir := writeRawQuarantineEntry(t, root, "older", QuarantinedProject{ID: "older", Name: "Older", OriginalPath: filepath.Join(root, "Older"), QuarantinedAt: time.Unix(10, 0)}, true)
	newerDir := writeRawQuarantineEntry(t, root, "newer", QuarantinedProject{ID: "newer", Name: "Newer", OriginalPath: filepath.Join(root, "Newer"), QuarantinedAt: time.Unix(20, 0)}, true)
	_ = olderDir
	_ = newerDir

	entries, err := listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "newer" || entries[1].ID != "older" {
		t.Fatalf("unexpected filtered/sorted entries: %#v", entries)
	}
}

func TestRestoreAndPurgeMissingEntryFailClosed(t *testing.T) {
	root := t.TempDir()
	for name, call := range map[string]func() error{
		"restore": func() error { _, err := restoreQuarantinedProject(root, "missing"); return err },
		"purge":   func() error { _, err := purgeQuarantinedProject(root, "missing", "PURGE Missing"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing quarantine entry should fail closed with not-exist, got %v", err)
			}
		})
	}
}
