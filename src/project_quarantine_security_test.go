// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func quarantineFixture(t *testing.T, root, name string) QuarantinedProject {
	t.Helper()
	project := filepath.Join(root, name)
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
	return entry
}

func rewriteQuarantineMetadata(t *testing.T, root string, entry QuarantinedProject) {
	t.Helper()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := quarantineMetadataPath(filepath.Join(projectQuarantineRoot(root), entry.ID))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadQuarantinedProjectRejectsTamperedOriginalPath(t *testing.T) {
	root := t.TempDir()
	entry := quarantineFixture(t, root, "Tampered")
	entry.OriginalPath = filepath.Join(root, "nested", "Tampered")
	rewriteQuarantineMetadata(t, root, entry)

	if _, _, err := loadQuarantinedProject(root, entry.ID); err == nil || !strings.Contains(err.Error(), "invalid original project path") {
		t.Fatalf("tampered original path must fail closed, got %v", err)
	}
}

func TestLoadQuarantinedProjectRejectsNonDirectoryPayload(t *testing.T) {
	root := t.TempDir()
	entry := quarantineFixture(t, root, "Payload")
	entryDir := filepath.Join(projectQuarantineRoot(root), entry.ID)
	payload := quarantinePayloadPath(entryDir)
	if err := os.RemoveAll(payload); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := loadQuarantinedProject(root, entry.ID); err == nil || !strings.Contains(err.Error(), "payload is not a real directory") {
		t.Fatalf("non-directory quarantine payload must fail closed, got %v", err)
	}
}

func TestListQuarantinedProjectsSkipsNoiseAndCorruptEntries(t *testing.T) {
	root := t.TempDir()
	entry := quarantineFixture(t, root, "Corrupt")
	quarantineRoot := projectQuarantineRoot(root)

	if err := os.Mkdir(filepath.Join(quarantineRoot, "..invalid"), 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := quarantineMetadataPath(filepath.Join(quarantineRoot, entry.ID))
	if err := os.WriteFile(metadata, []byte("{broken-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("corrupt or invalid quarantine entries must not be surfaced: %#v", entries)
	}
}
