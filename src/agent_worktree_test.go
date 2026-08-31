// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInit := exec.Command("git", "init")
	gitInit.Dir = dir
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\noutput: %s", err, out)
	}

	// Configure local test user
	_ = exec.Command("git", "-C", dir, "config", "user.name", "LocalCode Test").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.email", "test@localcode.local").Run()
	_ = exec.Command("git", "-C", dir, "config", "commit.gpgsign", "false").Run()

	// Create initial file & commit
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\nInitial content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gitAdd := exec.Command("git", "-C", dir, "add", "README.md")
	if out, err := gitAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\noutput: %s", err, out)
	}

	gitCommit := exec.Command("git", "-C", dir, "commit", "-m", "Initial commit")
	if out, err := gitCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\noutput: %s", err, out)
	}

	return dir
}

func TestAgentWorktreeLifecycle(t *testing.T) {
	repoDir := createTestGitRepo(t)
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create worktree
	wt, err := CreateAgentWorktree(ctx, repoDir, "mission-1", "task-builder-1", "", cfg)
	if err != nil {
		t.Fatalf("CreateAgentWorktree failed: %v", err)
	}
	if wt == nil || wt.WorktreePath == "" || wt.BranchName == "" {
		t.Fatalf("invalid worktree workspace: %#v", wt)
	}
	if !wt.Active {
		t.Fatal("expected worktree to be active")
	}

	// Verify file exists in worktree
	wtReadme := filepath.Join(wt.WorktreePath, "README.md")
	data, err := os.ReadFile(wtReadme)
	if err != nil {
		t.Fatalf("failed to read worktree file: %v", err)
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	if normalized != "# Test Repo\nInitial content\n" {
		t.Fatalf("unexpected content in worktree: %s", data)
	}

	// 2. Modify file in worktree
	newContent := "# Test Repo\nModified by Builder Agent\n"
	if err := os.WriteFile(wtReadme, []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Add new file in worktree
	newFile := filepath.Join(wt.WorktreePath, "feature.go")
	if err := os.WriteFile(newFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Collect result
	res, err := CollectAgentWorktreeResult(ctx, wt, cfg)
	if err != nil {
		t.Fatalf("CollectAgentWorktreeResult failed: %v", err)
	}
	if len(res.ChangedFiles) != 2 {
		t.Fatalf("changed files count=%d want 2: %#v", len(res.ChangedFiles), res.ChangedFiles)
	}

	// 4. Safe cleanup
	if err := CleanupAgentWorktree(ctx, wt, true, cfg); err != nil {
		t.Fatalf("CleanupAgentWorktree failed: %v", err)
	}

	// Verify main repo README is untouched
	mainReadme := filepath.Join(repoDir, "README.md")
	mainData, err := os.ReadFile(mainReadme)
	if err != nil {
		t.Fatal(err)
	}
	mainNormalized := strings.ReplaceAll(string(mainData), "\r\n", "\n")
	if mainNormalized != "# Test Repo\nInitial content\n" {
		t.Fatalf("main repo README was mutated: %s", mainData)
	}
}

func TestAgentWorktreePathEscape(t *testing.T) {
	outsidePath := filepath.Join(os.TempDir(), "outside-worktrees", "escape")
	if err := validateWorktreePathContainment(outsidePath); !errors.Is(err, errWorktreePathEscape) {
		t.Fatalf("expected errWorktreePathEscape for %q, got: %v", outsidePath, err)
	}

	insidePath := filepath.Join(managedWorktreesRootDir(), "mission-1", "task-1")
	if err := validateWorktreePathContainment(insidePath); err != nil {
		t.Fatalf("unexpected error for inside path %q: %v", insidePath, err)
	}
}

func TestAgentWorktreeConcurrencyAndLease(t *testing.T) {
	repoDir := createTestGitRepo(t)
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wt1, err := CreateAgentWorktree(ctx, repoDir, "mission-1", "task-same", "", cfg)
	if err != nil {
		t.Fatalf("wt1 creation failed: %v", err)
	}
	defer func() { _ = CleanupAgentWorktree(ctx, wt1, true, cfg) }()

	// Second attempt for the same task while first is active must fail
	_, err = CreateAgentWorktree(ctx, repoDir, "mission-1", "task-same", "", cfg)
	if !errors.Is(err, errWorktreeConcurrentActive) {
		t.Fatalf("expected errWorktreeConcurrentActive, got: %v", err)
	}
}

func TestAgentWorktreeInvalidBaseCommit(t *testing.T) {
	repoDir := createTestGitRepo(t)
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := CreateAgentWorktree(ctx, repoDir, "mission-1", "task-1", "deadbeef00000000000000000000000000000000", cfg)
	if !errors.Is(err, errWorktreeInvalidBaseCommit) {
		t.Fatalf("expected errWorktreeInvalidBaseCommit, got: %v", err)
	}
}

func TestAgentWorktreeNonGitRepo(t *testing.T) {
	nonGitDir := t.TempDir()
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := CreateAgentWorktree(ctx, nonGitDir, "mission-1", "task-1", "", cfg)
	if !errors.Is(err, errWorktreeNotGitRepo) {
		t.Fatalf("expected errWorktreeNotGitRepo, got: %v", err)
	}
}

func TestAgentWorktreeSanitizeBranchName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"feature branch", "feature-branch"},
		{"feat/builder:task@1", "feat/builder-task-1"},
		{"..//invalid..name//..", "invalid.name"},
		{"", "builder-task"},
	}

	for _, tc := range cases {
		got := sanitizeBranchName(tc.input)
		if got != tc.want {
			t.Fatalf("sanitizeBranchName(%q)=%q want %q", tc.input, got, tc.want)
		}
	}
}
