// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxRepairCyclesPerTask = 3
)

var (
	errIntegratorWorkspaceNil    = errors.New("integrator workspace is nil")
	errIntegratorMergeConflict   = errors.New("integration failed due to merge conflicts; aborted cleanly")
	errMaxRepairAttemptsExceeded = errors.New("maximum repair cycles exceeded (limit: 3)")
	errRepairStagnationDetected  = errors.New("repair stagnation detected: identical failure repeated across cycles")
)

type StagnationTracker struct {
	mu           sync.Mutex
	fingerprints map[string][]string
}

var defaultStagnationTracker = &StagnationTracker{
	fingerprints: make(map[string][]string),
}

func (t *StagnationTracker) RecordAttempt(taskID string, failingEvidence string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	hasher := sha256.New()
	hasher.Write([]byte(strings.TrimSpace(failingEvidence)))
	fp := hex.EncodeToString(hasher.Sum(nil))

	history := t.fingerprints[taskID]
	attemptCount := len(history) + 1

	if attemptCount > maxRepairCyclesPerTask {
		return attemptCount, fmt.Errorf("%w for task %q (attempt %d/%d)", errMaxRepairAttemptsExceeded, taskID, attemptCount, maxRepairCyclesPerTask)
	}

	// Check if the exact same failure fingerprint occurred in the immediate previous cycle
	if len(history) > 0 && history[len(history)-1] == fp {
		return attemptCount, fmt.Errorf("%w for task %q (failure fingerprint: %s)", errRepairStagnationDetected, taskID, fp[:12])
	}

	t.fingerprints[taskID] = append(history, fp)
	return attemptCount, nil
}

func (t *StagnationTracker) Reset(taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fingerprints, taskID)
}

func IntegrateBuilderWorktree(ctx context.Context, wt *AgentWorktreeWorkspace, targetBranch string, cfg Config) (*IntegrationResult, error) {
	if wt == nil || strings.TrimSpace(wt.MainProject) == "" || strings.TrimSpace(wt.BranchName) == "" {
		return nil, errIntegratorWorkspaceNil
	}

	project := filepath.Clean(wt.MainProject)

	// 1. Determine target branch
	if strings.TrimSpace(targetBranch) == "" {
		targetBranch = gitBranchName(project, cfg)
	}
	targetBranch = strings.TrimSpace(targetBranch)
	if targetBranch == "" || targetBranch == "(kein Git-Branch)" {
		targetBranch = "master"
	}

	// 2. Ensure target working tree is clean before merge
	statusOut, err := runGit(ctx, project, []string{"status", "--porcelain"}, cfg)
	if err != nil {
		return nil, fmt.Errorf("pre-integration git status failed: %w", err)
	}
	cleanStatus := extractGitStdout(statusOut)
	if strings.TrimSpace(cleanStatus) != "" {
		return &IntegrationResult{
			Status:       IntegrationFailed,
			TargetBranch: targetBranch,
			Detail:       fmt.Sprintf("Main repository working tree is not clean (%s); cannot integrate.", cleanStatus),
		}, fmt.Errorf("main repository working tree is dirty: %s", cleanStatus)
	}

	// 3. Inspect diff and changed files between base and worktree branch
	diffStatOut, err := runGit(ctx, project, []string{"diff", "--stat", "--name-only", wt.BaseCommit + ".." + wt.BranchName}, cfg)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	cleanStat := extractGitStdout(diffStatOut)
	var changedFiles []string
	for _, f := range strings.Split(cleanStat, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			changedFiles = append(changedFiles, filepath.ToSlash(f))
		}
	}

	// 4. Perform non-fast-forward merge
	commitMsg := fmt.Sprintf("Integrate %s: %s", wt.TaskID, wt.BranchName)
	mergeOut, err := runGit(ctx, project, []string{"merge", "--no-ff", "-m", commitMsg, wt.BranchName}, cfg)
	if err != nil {
		// Conflict or merge error -> abort merge immediately to preserve repo state
		_, _ = runGit(ctx, project, []string{"merge", "--abort"}, cfg)
		return &IntegrationResult{
			Status:       IntegrationConflict,
			TargetBranch: targetBranch,
			Detail:       fmt.Sprintf("Merge conflict integrating %s: %s", wt.BranchName, extractGitStdout(mergeOut)),
		}, errIntegratorMergeConflict
	}

	// 5. Retrieve new merged commit SHA
	headOut, err := runGit(ctx, project, []string{"rev-parse", "--verify", "HEAD"}, cfg)
	mergedCommit := extractGitStdout(headOut)

	return &IntegrationResult{
		Status:       IntegrationSuccess,
		TargetBranch: targetBranch,
		MergedCommit: mergedCommit,
		ChangedFiles: changedFiles,
		Detail:       fmt.Sprintf("Successfully integrated %s into %s (%d files changed).", wt.BranchName, targetBranch, len(changedFiles)),
	}, nil
}

func EvaluateTestResults(criteria []string, tests []TestResult, buildErr error) (EvaluationDecision, *RepairProposal) {
	if buildErr != nil {
		return DecisionRepair, &RepairProposal{
			Summary:         "Build or compilation verification failed",
			Recommendations: []string{buildErr.Error()},
		}
	}

	var failingTests []string
	var failingDetails []string
	for _, t := range tests {
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "fail" || status == "failed" || status == "error" {
			failingTests = append(failingTests, t.Name)
			if t.Detail != "" {
				failingDetails = append(failingDetails, fmt.Sprintf("%s: %s", t.Name, t.Detail))
			}
		}
	}

	if len(failingTests) == 0 {
		return DecisionPass, nil
	}

	proposal := &RepairProposal{
		Summary:         fmt.Sprintf("%d test(s) failed during evaluation", len(failingTests)),
		FailingTests:    failingTests,
		Recommendations: failingDetails,
	}
	return DecisionRepair, proposal
}

func SanitizeReviewerEvidence(objective string, criteria []string, changedFiles []string, diff string, testResults []TestResult) string {
	var b strings.Builder
	b.WriteString("# Objective\n")
	b.WriteString(strings.TrimSpace(objective) + "\n\n")

	if len(criteria) > 0 {
		b.WriteString("# Acceptance Criteria\n")
		for _, c := range criteria {
			b.WriteString(fmt.Sprintf("- %s\n", strings.TrimSpace(c)))
		}
		b.WriteString("\n")
	}

	if len(changedFiles) > 0 {
		b.WriteString("# Changed Files\n")
		for _, f := range changedFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	if len(testResults) > 0 {
		b.WriteString("# Test Evidence\n")
		for _, t := range testResults {
			b.WriteString(fmt.Sprintf("- [%s] %s", strings.ToUpper(t.Status), t.Name))
			if t.Detail != "" {
				b.WriteString(fmt.Sprintf(": %s", t.Detail))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(diff) != "" {
		b.WriteString("# Diff\n```diff\n")
		if len(diff) > 12000 {
			diff = diff[:12000] + "\n… [diff truncated]"
		}
		b.WriteString(diff)
		b.WriteString("\n```\n")
	}

	return strings.TrimSpace(b.String())
}
