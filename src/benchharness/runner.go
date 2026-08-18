// SPDX-License-Identifier: Apache-2.0

package benchharness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const outputLimit = 24000

type AdapterMetrics struct {
	AgentTurns        int `json:"agent_turns,omitempty"`
	ToolCalls         int `json:"tool_calls,omitempty"`
	InputTokens       int `json:"input_tokens,omitempty"`
	OutputTokens      int `json:"output_tokens,omitempty"`
	FailedPatches     int `json:"failed_patches,omitempty"`
	Retries           int `json:"retries,omitempty"`
	Compactions       int `json:"compactions,omitempty"`
	HumanIntervention int `json:"human_intervention,omitempty"`
}

type CommandResult struct {
	Name       string        `json:"name"`
	Kind       string        `json:"kind,omitempty"`
	Required   bool          `json:"required,omitempty"`
	Command    []string      `json:"command"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration"`
	TimedOut   bool          `json:"timed_out,omitempty"`
	Output     string        `json:"output,omitempty"`
	Successful bool          `json:"successful"`
}

type FileChange struct {
	Path      string `json:"path"`
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
	Untracked bool   `json:"untracked,omitempty"`
}

type Result struct {
	ManifestName       string          `json:"manifest_name"`
	Engine             string          `json:"engine"`
	Model              string          `json:"model"`
	BaseCommit         string          `json:"base_commit"`
	Worktree           string          `json:"worktree,omitempty"`
	StartedAt          time.Time       `json:"started_at"`
	Duration           time.Duration   `json:"duration"`
	EngineResult       CommandResult   `json:"engine_result"`
	SetupResults       []CommandResult `json:"setup_results,omitempty"`
	Checks             []CommandResult `json:"checks"`
	Changes            []FileChange    `json:"changes,omitempty"`
	ChangedFiles       int             `json:"changed_files"`
	ChangedLines       int             `json:"changed_lines"`
	UnnecessaryLines   int             `json:"unnecessary_changed_lines"`
	RequiredChecksPass bool            `json:"required_checks_pass"`
	Success            bool            `json:"success"`
	Adapter            AdapterMetrics  `json:"adapter_metrics,omitempty"`
	Error              string          `json:"error,omitempty"`
}

type Runner struct {
	TempRoot string
	Now      func() time.Time
}

func (r Runner) Run(ctx context.Context, manifest Manifest) (result Result, runErr error) {
	if err := manifest.Validate(); err != nil {
		return Result{}, err
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	result.ManifestName = manifest.Name
	result.Engine = manifest.Engine
	result.Model = manifest.Model
	result.StartedAt = r.Now()
	started := time.Now()
	defer func() { result.Duration = time.Since(started) }()

	baseCommit, err := gitOutput(ctx, manifest.Repository, "rev-parse", "--verify", manifest.BaseRef+"^{commit}")
	if err != nil {
		return result, fmt.Errorf("resolve base ref: %w", err)
	}
	result.BaseCommit = strings.TrimSpace(baseCommit)

	tempRoot := r.TempRoot
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	root, err := os.MkdirTemp(tempRoot, "localcode-bench-*")
	if err != nil {
		return result, err
	}
	worktree := filepath.Join(root, "worktree")
	if _, err := gitOutput(ctx, manifest.Repository, "worktree", "add", "--detach", worktree, result.BaseCommit); err != nil {
		_ = os.RemoveAll(root)
		return result, fmt.Errorf("create benchmark worktree: %w", err)
	}
	result.Worktree = worktree
	defer func() {
		if manifest.KeepWorktree {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = gitOutput(cleanupCtx, manifest.Repository, "worktree", "remove", "--force", worktree)
		_ = os.RemoveAll(root)
		result.Worktree = ""
	}()

	variables := map[string]string{
		"WORKTREE": worktree,
		"TASK":     manifest.Task,
		"MODEL":    manifest.Model,
		"ENGINE":   manifest.Engine,
		"BASE":     result.BaseCommit,
	}
	env := benchmarkEnvironment(manifest, variables)

	for index, command := range manifest.SetupCommands {
		name := command.Name
		if name == "" {
			name = fmt.Sprintf("setup-%d", index+1)
		}
		res := runCommand(ctx, worktree, name, "setup", true, command.Args, command.Timeout, env, variables, manifest.TimeoutSeconds)
		result.SetupResults = append(result.SetupResults, res)
		if !res.Successful {
			result.Error = "setup failed: " + name
			return result, nil
		}
	}

	result.EngineResult = runCommand(ctx, worktree, manifest.Engine, "engine", true, manifest.EngineCommand, manifest.TimeoutSeconds, env, variables, manifest.TimeoutSeconds)

	for _, check := range manifest.Checks {
		result.Checks = append(result.Checks, runCommand(ctx, worktree, check.Name, strings.ToLower(check.Kind), check.Required, check.Command, check.Timeout, env, variables, manifest.TimeoutSeconds))
	}

	changes, diffErr := collectChanges(ctx, worktree)
	if manifest.MetricsFile != "" {
		changes = excludeBenchmarkChange(changes, manifest.MetricsFile)
	}
	if diffErr != nil {
		result.Error = "collect git diff: " + diffErr.Error()
	} else {
		result.Changes = changes
		result.ChangedFiles = len(changes)
		for _, change := range changes {
			lines := change.Added + change.Deleted
			result.ChangedLines += lines
			if !pathAllowed(change.Path, manifest.AllowedPaths) {
				result.UnnecessaryLines += lines
			}
		}
	}

	if manifest.MetricsFile != "" {
		metricsPath := filepath.Join(worktree, filepath.Clean(manifest.MetricsFile))
		if metrics, err := readAdapterMetrics(metricsPath); err == nil {
			result.Adapter = metrics
		} else if !errors.Is(err, os.ErrNotExist) && result.Error == "" {
			result.Error = "adapter metrics: " + err.Error()
		}
	}

	result.RequiredChecksPass = true
	for _, check := range result.Checks {
		if check.Required && !check.Successful {
			result.RequiredChecksPass = false
		}
	}
	result.Success = result.EngineResult.Successful && result.RequiredChecksPass && result.Error == ""
	return result, nil
}

func benchmarkEnvironment(manifest Manifest, variables map[string]string) []string {
	env := append([]string(nil), os.Environ()...)
	for key, value := range manifest.Environment {
		env = append(env, key+"="+expandValue(value, variables))
	}
	env = append(env,
		"LOCALCODE_BENCH_TASK="+manifest.Task,
		"LOCALCODE_BENCH_MODEL="+manifest.Model,
		"LOCALCODE_BENCH_ENGINE="+manifest.Engine,
		"LOCALCODE_BENCH_WORKTREE="+variables["WORKTREE"],
		"LOCALCODE_BENCH_BASE="+variables["BASE"],
	)
	return env
}

func runCommand(parent context.Context, dir, name, kind string, required bool, command []string, timeoutSeconds int, env []string, variables map[string]string, defaultTimeout int) CommandResult {
	res := CommandResult{Name: name, Kind: kind, Required: required, Command: expandArgs(command, variables), ExitCode: -1}
	if len(res.Command) == 0 {
		res.Output = "empty command"
		return res
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeout
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 900
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.Command(res.Command[0], res.Command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	prepareBenchmarkCommand(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	started := time.Now()
	err := cmd.Start()
	if err == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-ctx.Done():
			_ = killBenchmarkCommandTree(cmd)
			err = <-done
		}
	}
	res.Duration = time.Since(started)
	res.Output = truncate(output.String(), outputLimit)
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		return res
	}
	if ctx.Err() != nil {
		res.ExitCode = -1
		res.Output = truncate(strings.TrimSpace(res.Output+"\n"+ctx.Err().Error()), outputLimit)
		return res
	}
	if err == nil {
		res.ExitCode = 0
		res.Successful = true
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	} else {
		res.Output = truncate(strings.TrimSpace(res.Output+"\n"+err.Error()), outputLimit)
	}
	return res
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return string(data), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func collectChanges(ctx context.Context, worktree string) ([]FileChange, error) {
	tracked, err := gitOutput(ctx, worktree, "diff", "--numstat", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	changes := map[string]FileChange{}
	scanner := bufio.NewScanner(strings.NewReader(tracked))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		path := filepath.ToSlash(fields[len(fields)-1])
		changes[path] = FileChange{Path: path, Added: added, Deleted: deleted}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	untracked, err := gitOutput(ctx, worktree, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, raw := range strings.Split(untracked, "\x00") {
		if raw == "" {
			continue
		}
		path := filepath.ToSlash(raw)
		lineCount := countTextLines(filepath.Join(worktree, filepath.FromSlash(path)))
		changes[path] = FileChange{Path: path, Added: lineCount, Untracked: true}
	}
	out := make([]FileChange, 0, len(changes))
	for _, change := range changes {
		out = append(out, change)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func countTextLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	count := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		count++
	}
	return count
}

func pathAllowed(path string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	path = filepath.ToSlash(filepath.Clean(path))
	for _, prefix := range allowed {
		prefix = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(prefix)), "/")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func excludeBenchmarkChange(changes []FileChange, ignored string) []FileChange {
	ignored = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ignored)))
	if ignored == "" || ignored == "." {
		return changes
	}
	out := changes[:0]
	for _, change := range changes {
		if filepath.ToSlash(filepath.Clean(change.Path)) == ignored {
			continue
		}
		out = append(out, change)
	}
	return out
}

func readAdapterMetrics(path string) (AdapterMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AdapterMetrics{}, err
	}
	var metrics AdapterMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return AdapterMetrics{}, err
	}
	return metrics, nil
}

func expandArgs(args []string, variables map[string]string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = expandValue(arg, variables)
	}
	return out
}

func expandValue(value string, variables map[string]string) string {
	for key, replacement := range variables {
		value = strings.ReplaceAll(value, "${"+key+"}", replacement)
	}
	return value
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...[truncated]"
}
