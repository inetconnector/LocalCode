// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegratorCleanMerge(t *testing.T) {
	repoDir := createTestGitRepo(t)
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create worktree
	wt, err := CreateAgentWorktree(ctx, repoDir, "mission-1", "task-builder-1", "", cfg)
	if err != nil {
		t.Fatalf("CreateAgentWorktree failed: %v", err)
	}
	defer func() { _ = CleanupAgentWorktree(ctx, wt, true, cfg) }()

	// 2. Commit a new file in the worktree branch
	featureFile := filepath.Join(wt.WorktreePath, "feature.go")
	if err := os.WriteFile(featureFile, []byte("package main\n\nfunc Feature() bool { return true }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	addCmd := exec.Command("git", "-C", wt.WorktreePath, "add", "feature.go")
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add in worktree failed: %v\n%s", err, out)
	}

	commitCmd := exec.Command("git", "-C", wt.WorktreePath, "commit", "-m", "Add feature.go")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit in worktree failed: %v\n%s", err, out)
	}

	// 3. Integrate worktree into main branch
	res, err := IntegrateBuilderWorktree(ctx, wt, "", cfg)
	if err != nil {
		t.Fatalf("IntegrateBuilderWorktree failed: %v", err)
	}
	if res.Status != IntegrationSuccess {
		t.Fatalf("integration status=%q want success", res.Status)
	}
	if res.MergedCommit == "" {
		t.Fatal("merged commit SHA is empty")
	}

	// 4. Verify feature.go is now present on the main repo
	mainFeature := filepath.Join(repoDir, "feature.go")
	if _, err := os.Stat(mainFeature); err != nil {
		t.Fatalf("feature.go not found in main repository after integration: %v", err)
	}
}

func TestIntegratorConflictHandling(t *testing.T) {
	repoDir := createTestGitRepo(t)
	cfg := Config{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create worktree
	wt, err := CreateAgentWorktree(ctx, repoDir, "mission-1", "task-builder-conflict", "", cfg)
	if err != nil {
		t.Fatalf("CreateAgentWorktree failed: %v", err)
	}
	defer func() { _ = CleanupAgentWorktree(ctx, wt, true, cfg) }()

	// 2. Modify README in worktree and commit
	wtReadme := filepath.Join(wt.WorktreePath, "README.md")
	if err := os.WriteFile(wtReadme, []byte("# Worktree Version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command("git", "-C", wt.WorktreePath, "commit", "-am", "Worktree change").Run()

	// 3. Modify README in main repo and commit to cause conflict
	mainReadme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(mainReadme, []byte("# Main Repo Version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command("git", "-C", repoDir, "commit", "-am", "Main branch change").Run()

	// 4. Integrate worktree: must detect conflict and abort cleanly
	res, err := IntegrateBuilderWorktree(ctx, wt, "", cfg)
	if !errors.Is(err, errIntegratorMergeConflict) {
		t.Fatalf("expected errIntegratorMergeConflict, got: %v", err)
	}
	if res.Status != IntegrationConflict {
		t.Fatalf("status=%q want conflict", res.Status)
	}

	// 5. Ensure main repo is not left in conflicted / unmerged state
	statusOut, _ := exec.Command("git", "-C", repoDir, "status", "--porcelain").CombinedOutput()
	if len(statusOut) > 0 {
		t.Fatalf("main repo working tree dirty after aborted conflict: %s", statusOut)
	}
}

func TestTestAgentObjectiveEvaluation(t *testing.T) {
	// Case 1: Pass
	passTests := []TestResult{
		{Name: "TestAuth", Status: "PASS"},
		{Name: "TestPayment", Status: "passed"},
	}
	decision, proposal := EvaluateTestResults([]string{"auth works", "payment works"}, passTests, nil)
	if decision != DecisionPass || proposal != nil {
		t.Fatalf("expected DecisionPass with nil proposal, got %v / %#v", decision, proposal)
	}

	// Case 2: Fail / Repair
	failTests := []TestResult{
		{Name: "TestAuth", Status: "PASS"},
		{Name: "TestPayment", Status: "FAIL", Detail: "timeout waiting for gateway"},
	}
	decision, proposal = EvaluateTestResults([]string{"auth works", "payment works"}, failTests, nil)
	if decision != DecisionRepair || proposal == nil {
		t.Fatalf("expected DecisionRepair with proposal, got %v / %#v", decision, proposal)
	}
	if len(proposal.FailingTests) != 1 || proposal.FailingTests[0] != "TestPayment" {
		t.Fatalf("unexpected failing tests in proposal: %#v", proposal)
	}

	// Case 3: Build Error
	decision, proposal = EvaluateTestResults(nil, nil, errors.New("syntax error in main.go:12"))
	if decision != DecisionRepair || proposal == nil || proposal.Summary != "Build or compilation verification failed" {
		t.Fatalf("unexpected build error proposal: %#v", proposal)
	}
}

func TestReviewerEvidenceSanitization(t *testing.T) {
	evidence := SanitizeReviewerEvidence(
		"Implement user authentication",
		[]string{"Must use bcrypt", "Token expiry 1 hour"},
		[]string{"auth/login.go", "auth/token.go"},
		"- old code\n+ new code\n",
		[]TestResult{{Name: "TestTokenExpiry", Status: "PASS"}},
	)

	if !errorsContains(evidence, "# Objective") || !errorsContains(evidence, "Implement user authentication") {
		t.Fatal("Objective missing")
	}
	if !errorsContains(evidence, "# Acceptance Criteria") || !errorsContains(evidence, "Must use bcrypt") {
		t.Fatal("Acceptance criteria missing")
	}
	if !errorsContains(evidence, "# Changed Files") || !errorsContains(evidence, "auth/login.go") {
		t.Fatal("Changed files missing")
	}
	if !errorsContains(evidence, "# Test Evidence") || !errorsContains(evidence, "[PASS] TestTokenExpiry") {
		t.Fatal("Test evidence missing")
	}
	if !errorsContains(evidence, "# Diff") || !errorsContains(evidence, "+ new code") {
		t.Fatal("Diff missing")
	}
}

func errorsContains(s, sub string) bool {
	return filepath.Clean(s) != "" && (len(s) >= len(sub)) && (s == sub || len(sub) == 0 || (len(s) > 0 && len(sub) > 0 && (s[:len(sub)] == sub || containsSubstr(s, sub))))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRepairStagnationDetection(t *testing.T) {
	tracker := &StagnationTracker{
		fingerprints: make(map[string][]string),
	}

	taskID := "task-repair-1"

	// 1. First failure -> recorded (attempt 1)
	attempt, err := tracker.RecordAttempt(taskID, "TestLogin failed with timeout")
	if err != nil || attempt != 1 {
		t.Fatalf("attempt 1 error: %v, attempt=%d", err, attempt)
	}

	// 2. Identical failure -> stagnation detected
	_, err = tracker.RecordAttempt(taskID, "TestLogin failed with timeout")
	if !errors.Is(err, errRepairStagnationDetected) {
		t.Fatalf("expected errRepairStagnationDetected, got: %v", err)
	}

	// 3. Different failure -> succeeds (attempt 2)
	attempt, err = tracker.RecordAttempt(taskID, "TestLogin failed with 401 Unauthorized")
	if err != nil || attempt != 2 {
		t.Fatalf("attempt 2 error: %v, attempt=%d", err, attempt)
	}

	// 4. Third failure (different) -> succeeds (attempt 3)
	attempt, err = tracker.RecordAttempt(taskID, "TestToken failed with 500 Internal Error")
	if err != nil || attempt != 3 {
		t.Fatalf("attempt 3 error: %v, attempt=%d", err, attempt)
	}

	// 5. Fourth failure -> exceeds max attempts
	_, err = tracker.RecordAttempt(taskID, "TestToken failed with another error")
	if !errors.Is(err, errMaxRepairAttemptsExceeded) {
		t.Fatalf("expected errMaxRepairAttemptsExceeded, got: %v", err)
	}
}
