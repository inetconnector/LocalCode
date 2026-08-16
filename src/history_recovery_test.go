// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadThreadsFallsBackToLastKnownGoodBackup(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", "")

	project := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	original := map[string]*ChatThread{
		"thread-1": {
			ID:        "thread-1",
			Project:   project,
			Title:     "Recovery test",
			Model:     "test-model",
			CreatedAt: now,
			UpdatedAt: now,
			Events:    []UIEvent{{Type: "user", Message: "hello", Timestamp: now}},
		},
	}
	if err := saveThreads(original); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// A second successful save creates the last-known-good .bak snapshot.
	original["thread-1"].Title = "Newest primary"
	if err := saveThreads(original); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if _, err := os.Stat(threadsPath() + ".bak"); err != nil {
		t.Fatalf("expected persistent backup: %v", err)
	}

	if err := os.WriteFile(threadsPath(), []byte(`{"version":1,"threads":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered := loadThreads()
	thread := recovered["thread-1"]
	if thread == nil {
		t.Fatalf("backup thread not recovered; got %#v", recovered)
	}
	if thread.Title != "Recovery test" {
		t.Fatalf("recovered title = %q, want last-known-good backup title", thread.Title)
	}
}

func TestLoadThreadsDoesNotInventDataWhenPrimaryAndBackupAreInvalid(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(filepath.Dir(threadsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(threadsPath(), []byte(`{bad primary`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(threadsPath()+".bak", []byte(`{bad backup`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadThreads(); len(got) != 0 {
		t.Fatalf("invalid primary+backup should yield empty history, got %#v", got)
	}
}
