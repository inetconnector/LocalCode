// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectHasGitMetadata(t *testing.T) {
	t.Run("plain directory", func(t *testing.T) {
		if projectHasGitMetadata(t.TempDir()) {
			t.Fatal("plain directory must not be treated as Git repository")
		}
	})

	t.Run("git directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !projectHasGitMetadata(root) {
			t.Fatal(".git directory must be detected")
		}
	})

	t.Run("nested project", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "src", "feature")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if !projectHasGitMetadata(nested) {
			t.Fatal("repository subdirectory must inherit Git metadata")
		}
	})

	t.Run("worktree git file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../metadata\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !projectHasGitMetadata(root) {
			t.Fatal(".git worktree file must be detected")
		}
	})
}

func TestStateDocumentSkipsGitForPlainDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.StateFile = "STATE.md"
	cfg.Language = "en"
	if err := updateStateDocument(root, cfg, false, "test-model", "task", "done", nil, "test"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "`(no Git branch)`") || !strings.Contains(text, "Not a Git repository.") {
		t.Fatalf("plain project should use deterministic Git-free state:\n%s", text)
	}
	if strings.Contains(text, "git status failed") || strings.Contains(text, "Git wurde nach vollständiger Werkzeugerkennung") {
		t.Fatalf("state output shows evidence that Git discovery/process path was used:\n%s", text)
	}
}
