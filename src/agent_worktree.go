// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	errWorktreeNotGitRepo        = errors.New("main project is not a git repository")
	errWorktreePathEscape        = errors.New("worktree path escapes allowed worktree directory")
	errWorktreeConcurrentActive  = errors.New("a builder worktree is already active for this task")
	errWorktreeInvalidBaseCommit = errors.New("invalid or empty base commit for worktree")
	errWorktreeNotFound          = errors.New("worktree workspace not found")
)

var gitBranchInvalidCharPattern = regexp.MustCompile(`[^a-zA-Z0-9_\-\./]`)

type AgentWorktreeWorkspace struct {
	ID           string    `json:"id"`
	MissionID    string    `json:"mission_id"`
	TaskID       string    `json:"task_id"`
	MainProject  string    `json:"main_project"`
	WorktreePath string    `json:"worktree_path"`
	BranchName   string    `json:"branch_name"`
	BaseCommit   string    `json:"base_commit"`
	CreatedAt    time.Time `json:"created_at"`
	Active       bool      `json:"active"`
}

type worktreeRegistry struct {
	mu        sync.Mutex
	worktrees map[string]*AgentWorktreeWorkspace
}

var defaultWorktreeRegistry = &worktreeRegistry{
	worktrees: make(map[string]*AgentWorktreeWorkspace),
}

func sanitizeBranchName(raw string) string {
	raw = strings.TrimSpace(raw)
	clean := gitBranchInvalidCharPattern.ReplaceAllString(raw, "-")
	clean = strings.Trim(clean, "-/.")
	for strings.Contains(clean, "..") {
		clean = strings.ReplaceAll(clean, "..", ".")
	}
	for strings.Contains(clean, "//") {
		clean = strings.ReplaceAll(clean, "//", "/")
	}
	if clean == "" {
		clean = "builder-task"
	}
	return clean
}

func managedWorktreesRootDir() string {
	return filepath.Join(appDataDir(), "worktrees")
}

func validateWorktreePathContainment(targetPath string) error {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("%w: %v", errWorktreePathEscape, err)
	}
	absRoot, err := filepath.Abs(managedWorktreesRootDir())
	if err != nil {
		return fmt.Errorf("%w: %v", errWorktreePathEscape, err)
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s is outside %s", errWorktreePathEscape, absTarget, absRoot)
	}
	return nil
}

func (r *worktreeRegistry) register(wt *AgentWorktreeWorkspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%s::%s", wt.MainProject, wt.TaskID)
	if existing, exists := r.worktrees[key]; exists && existing.Active {
		return fmt.Errorf("%w: task %s", errWorktreeConcurrentActive, wt.TaskID)
	}
	wt.Active = true
	r.worktrees[key] = wt
	return nil
}

func (r *worktreeRegistry) unregister(wt *AgentWorktreeWorkspace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%s::%s", wt.MainProject, wt.TaskID)
	delete(r.worktrees, key)
	wt.Active = false
}

func (r *worktreeRegistry) get(mainProject, taskID string) (*AgentWorktreeWorkspace, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%s::%s", mainProject, taskID)
	wt, ok := r.worktrees[key]
	return wt, ok
}

func extractGitStdout(raw string) string {
	val := strings.TrimSpace(raw)
	if idx := strings.Index(val, "STDOUT:\r\n"); idx >= 0 {
		val = val[idx+len("STDOUT:\r\n"):]
	} else if idx := strings.Index(val, "STDOUT:\n"); idx >= 0 {
		val = val[idx+len("STDOUT:\n"):]
	} else {
		return ""
	}
	for _, marker := range []string{"\r\nSTDERR:", "\nSTDERR:", "\r\nFEHLERDETAIL:", "\nFEHLERDETAIL:"} {
		if idx := strings.Index(val, marker); idx >= 0 {
			val = val[:idx]
			break
		}
	}
	return strings.TrimSpace(val)
}

func CreateAgentWorktree(ctx context.Context, project, missionID, taskID, baseCommit string, cfg Config) (*AgentWorktreeWorkspace, error) {
	project = filepath.Clean(project)
	missionID = strings.TrimSpace(missionID)
	taskID = strings.TrimSpace(taskID)
	if missionID == "" {
		missionID = "mission"
	}
	if taskID == "" {
		taskID = "task"
	}

	// 1. Verify main project is a valid Git repository
	headOut, err := runGit(ctx, project, []string{"rev-parse", "--verify", "HEAD"}, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errWorktreeNotGitRepo, err)
	}
	headCommit := extractGitStdout(headOut)
	if headCommit == "" {
		return nil, fmt.Errorf("%w: HEAD output was empty", errWorktreeNotGitRepo)
	}

	if strings.TrimSpace(baseCommit) == "" {
		baseCommit = headCommit
	} else {
		// Verify specified base commit exists
		baseOut, err := runGit(ctx, project, []string{"rev-parse", "--verify", baseCommit + "^{commit}"}, cfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errWorktreeInvalidBaseCommit, err)
		}
		baseCommit = extractGitStdout(baseOut)
		if baseCommit == "" {
			return nil, fmt.Errorf("%w: base commit output was empty", errWorktreeInvalidBaseCommit)
		}
	}

	// 2. Prepare target directory under managed worktree root
	cleanMission := sanitizeBranchName(missionID)
	cleanTask := sanitizeBranchName(taskID)
	uniqueSuffix := newID()[:8]
	worktreeDirName := fmt.Sprintf("%s-%s-%s", cleanMission, cleanTask, uniqueSuffix)
	targetPath := filepath.Join(managedWorktreesRootDir(), cleanMission, worktreeDirName)

	if err := validateWorktreePathContainment(targetPath); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create parent worktree dir: %w", err)
	}

	// 3. Formulate branch name
	branchName := fmt.Sprintf("localcode-builder/%s/%s-%s", cleanMission, cleanTask, uniqueSuffix)

	workspace := &AgentWorktreeWorkspace{
		ID:           newID(),
		MissionID:    missionID,
		TaskID:       taskID,
		MainProject:  project,
		WorktreePath: targetPath,
		BranchName:   branchName,
		BaseCommit:   baseCommit,
		CreatedAt:    time.Now(),
	}

	// 4. Check registry concurrency lease
	if err := defaultWorktreeRegistry.register(workspace); err != nil {
		return nil, err
	}

	// 5. Run git worktree add
	out, err := runGit(ctx, project, []string{"worktree", "add", "-b", branchName, targetPath, baseCommit}, cfg)
	if err != nil {
		defaultWorktreeRegistry.unregister(workspace)
		_ = os.RemoveAll(targetPath)
		return nil, fmt.Errorf("git worktree add failed (%s): %w", strings.TrimSpace(out), err)
	}

	return workspace, nil
}

func CollectAgentWorktreeResult(ctx context.Context, wt *AgentWorktreeWorkspace, cfg Config) (*AgentResult, error) {
	if wt == nil || !wt.Active {
		return nil, errWorktreeNotFound
	}

	// 1. Inspect status
	statusOut, err := runGit(ctx, wt.WorktreePath, []string{"status", "--porcelain"}, cfg)
	if err != nil {
		return nil, fmt.Errorf("git status failed in worktree: %w", err)
	}
	cleanStatus := extractGitStdout(statusOut)

	var changedFiles []string
	for _, line := range strings.Split(cleanStatus, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 3 {
			filePath := strings.TrimSpace(line[3:])
			if filePath != "" {
				changedFiles = append(changedFiles, filepath.ToSlash(filePath))
			}
		}
	}

	// 2. Inspect diff against base commit
	diffOut, _ := runGit(ctx, wt.WorktreePath, []string{"diff", wt.BaseCommit}, cfg)
	cleanDiff := extractGitStdout(diffOut)
	if len(cleanDiff) > 16000 {
		cleanDiff = cleanDiff[:16000] + "\n… [diff truncated]"
	}

	summary := fmt.Sprintf("Builder worktree task completed on branch %s with %d changed files.", wt.BranchName, len(changedFiles))
	if len(changedFiles) == 0 {
		summary = fmt.Sprintf("Builder worktree task produced no file changes on branch %s.", wt.BranchName)
	}

	result := &AgentResult{
		Status:       AgentResultCompleted,
		Summary:      summary,
		ChangedFiles: changedFiles,
	}
	return result, nil
}

func CleanupAgentWorktree(ctx context.Context, wt *AgentWorktreeWorkspace, force bool, cfg Config) error {
	if wt == nil {
		return nil
	}
	defer defaultWorktreeRegistry.unregister(wt)

	args := []string{"worktree", "remove", wt.WorktreePath}
	if force {
		args = []string{"worktree", "remove", "--force", wt.WorktreePath}
	}

	_, err := runGit(ctx, wt.MainProject, args, cfg)
	// Even if git worktree remove returned an error, run prune and clean up directory
	_, _ = runGit(ctx, wt.MainProject, []string{"worktree", "prune"}, cfg)
	_ = os.RemoveAll(wt.WorktreePath)

	return err
}
