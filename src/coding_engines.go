// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	editingEngineAider    = "aider"
	editingEngineClaude   = "claude"
	editingEngineOpenCode = "opencode"
	editingEngineNative   = "native"
)

type CodingEngineStatus struct {
	Engine           string `json:"engine"`
	DisplayName      string `json:"display_name"`
	Enabled          bool   `json:"enabled"`
	Installed        bool   `json:"installed"`
	Authenticated    bool   `json:"authenticated"`
	Authentication   string `json:"authentication,omitempty"`
	Version          string `json:"version,omitempty"`
	ExpectedVersion  string `json:"expected_version,omitempty"`
	Executable       string `json:"executable,omitempty"`
	InstallationRoot string `json:"installation_root,omitempty"`
	Error            string `json:"error,omitempty"`
}

type CodingEngineNotInstalledError struct {
	Status CodingEngineStatus
}

func (e *CodingEngineNotInstalledError) Error() string {
	if strings.TrimSpace(e.Status.Error) != "" {
		return e.Status.DisplayName + " ist nicht einsatzbereit: " + e.Status.Error
	}
	return e.Status.DisplayName + " ist nicht installiert"
}

type CodingEngineRunResult struct {
	Engine       string
	Output       string
	ChangedFiles []string
	BackupDir    string
	Executable   string
	Arguments    []string
	Duration     time.Duration
	ExitCode     int
}

func normalizeEditingEngine(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case editingEngineAider, editingEngineClaude, editingEngineOpenCode, editingEngineNative:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return editingEngineAider
	}
}

func isCodingEngineAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "engine_edit", "engine_repo_map", "engine_lint", "engine_test", "engine_undo", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "aider_undo":
		return true
	default:
		return false
	}
}

func codingEngineDisplayName(engine string) string {
	switch normalizeEditingEngine(engine) {
	case editingEngineClaude:
		return "Claude Code"
	case editingEngineOpenCode:
		return "OpenCode"
	case editingEngineNative:
		return "LocalCode nativ"
	default:
		return "Aider"
	}
}

func codingEngineEnabled(cfg Config, engine string) bool {
	switch normalizeEditingEngine(engine) {
	case editingEngineClaude:
		return cfg.ClaudeCodeEnabled
	case editingEngineOpenCode:
		return cfg.OpenCodeEnabled
	case editingEngineNative:
		return true
	default:
		return cfg.AiderEnabled
	}
}

func codingEngineAutoInstall(cfg Config, engine string) bool {
	switch normalizeEditingEngine(engine) {
	case editingEngineClaude:
		return cfg.ClaudeCodeAutoInstall
	case editingEngineOpenCode:
		return cfg.OpenCodeAutoInstall
	case editingEngineNative:
		return false
	default:
		return cfg.AiderAutoInstall
	}
}

func executableCandidate(path string) string {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		abs, absErr := filepath.Abs(path)
		if absErr == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(path)
	}
	return ""
}

func firstExecutable(candidates ...string) string {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(candidate))
		if seen[key] {
			continue
		}
		seen[key] = true
		if found := executableCandidate(candidate); found != "" {
			return found
		}
	}
	return ""
}

func claudeToolRoot() string {
	return filepath.Join(appDataDir(), "tools", "claude-code")
}

func openCodeToolRoot() string {
	return filepath.Join(appDataDir(), "tools", "opencode")
}

func findClaudeCodeExecutable(cfg Config) string {
	if configured := strings.TrimSpace(cfg.ClaudeCodeExecutable); configured != "" {
		return executableCandidate(configured)
	}
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	pathCandidate, _ := exec.LookPath(name)
	profile := userProfileDir()
	localApp := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")
	return firstExecutable(
		pathCandidate,
		filepath.Join(profile, ".local", "bin", name),
		filepath.Join(claudeToolRoot(), name),
		filepath.Join(localApp, "Programs", "Claude Code", name),
		filepath.Join(appData, "npm", "claude.cmd"),
	)
}

func openCodeExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"opencode.exe", "opencode.cmd", "opencode.bat"}
	}
	return []string{"opencode"}
}

func findOpenCodeExecutable(cfg Config) string {
	if configured := strings.TrimSpace(cfg.OpenCodeExecutable); configured != "" {
		return executableCandidate(configured)
	}
	var candidates []string
	for _, name := range openCodeExecutableNames() {
		if found, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, found)
		}
		candidates = append(candidates,
			filepath.Join(openCodeToolRoot(), name),
			filepath.Join(openCodeToolRoot(), "bin", name),
			filepath.Join(os.Getenv("APPDATA"), "npm", name),
		)
	}
	return firstExecutable(candidates...)
}

func engineExecutableVersion(ctx context.Context, executable string, cfg Config) (string, error) {
	if executable == "" {
		return "", errors.New("executable not found")
	}
	output, code, err := runCapturedCommand(ctx, executable, []string{"--version"}, commandEnvironment(cfg), "")
	if err != nil {
		return strings.TrimSpace(output), fmt.Errorf("version command failed with exit code %d: %w", code, err)
	}
	return strings.TrimSpace(strings.TrimPrefix(output, "STDOUT:\n")), nil
}

func claudeAuthenticationStatus(ctx context.Context, executable string, cfg Config) (bool, string) {
	if executable == "" {
		return false, ""
	}
	output, code, err := runCapturedCommand(ctx, executable, []string{"auth", "status", "--text"}, commandEnvironment(cfg), "")
	text := strings.TrimSpace(output)
	if err == nil && code == 0 {
		return true, text
	}
	return false, text
}

func openCodeAuthenticationStatus(ctx context.Context, executable string, cfg Config) (bool, string) {
	if executable == "" {
		return false, ""
	}
	output, code, err := runCapturedCommand(ctx, executable, []string{"auth", "list"}, commandEnvironment(cfg), "")
	text := strings.TrimSpace(output)
	if err == nil && code == 0 {
		lower := strings.ToLower(text)
		return text != "" && !strings.Contains(lower, "no authenticated") && !strings.Contains(lower, "not authenticated"), text
	}
	return false, text
}

func selectedCodingEngineStatus(ctx context.Context, cfg Config) CodingEngineStatus {
	return codingEngineStatus(ctx, cfg, cfg.EditingEngine)
}

func codingEngineStatus(ctx context.Context, cfg Config, engine string) CodingEngineStatus {
	engine = normalizeEditingEngine(engine)
	status := CodingEngineStatus{
		Engine:      engine,
		DisplayName: codingEngineDisplayName(engine),
		Enabled:     codingEngineEnabled(cfg, engine),
	}
	if engine == editingEngineNative {
		status.Installed = true
		status.Authenticated = true
		status.Version = version
		status.Executable = "embedded"
		return status
	}
	if !status.Enabled {
		status.Error = status.DisplayName + " is disabled"
		return status
	}
	if engine == editingEngineAider {
		aider := aiderStatus(ctx, cfg)
		status.Installed = aider.Installed
		status.Authenticated = aider.Installed
		status.Version = aider.Version
		status.ExpectedVersion = aider.ExpectedVersion
		status.Executable = aider.Executable
		status.InstallationRoot = aider.InstallationRoot
		status.Error = aider.Error
		return status
	}
	if engine == editingEngineClaude {
		status.Executable = findClaudeCodeExecutable(cfg)
		status.InstallationRoot = claudeToolRoot()
		status.ExpectedVersion = strings.TrimSpace(cfg.ClaudeCodeChannel)
	} else {
		status.Executable = findOpenCodeExecutable(cfg)
		status.InstallationRoot = openCodeToolRoot()
		status.ExpectedVersion = strings.TrimSpace(cfg.OpenCodeVersion)
	}
	if status.Executable == "" {
		status.Error = status.DisplayName + " executable not found"
		return status
	}
	versionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	foundVersion, err := engineExecutableVersion(versionCtx, status.Executable, cfg)
	cancel()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = foundVersion
	status.Installed = true
	authCtx, authCancel := context.WithTimeout(ctx, 20*time.Second)
	if engine == editingEngineClaude {
		status.Authenticated, status.Authentication = claudeAuthenticationStatus(authCtx, status.Executable, cfg)
	} else {
		status.Authenticated, status.Authentication = openCodeAuthenticationStatus(authCtx, status.Executable, cfg)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.OpenCodeModel)), "ollama/") || strings.TrimSpace(cfg.OpenCodeModel) == "" {
			// Local Ollama providers do not require provider credentials.
			status.Authenticated = true
		}
	}
	authCancel()
	if !status.Authenticated {
		status.Error = status.DisplayName + " is installed but not authenticated"
	}
	return status
}

func claudeInstallPowerShell(channel string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "stable"
	}
	return "$ErrorActionPreference='Stop'; & ([scriptblock]::Create((irm https://claude.ai/install.ps1))) " + quotePowerShellLiteral(channel)
}

func installClaudeCode(ctx context.Context, cfg Config) (CodingEngineStatus, string, error) {
	if !cfg.SetupDownloadsEnabled {
		return codingEngineStatus(ctx, cfg, editingEngineClaude), "", errors.New("downloads for automatic setup are disabled")
	}
	if runtime.GOOS != "windows" {
		return codingEngineStatus(ctx, cfg, editingEngineClaude), "", errors.New("automatic Claude Code installation is currently supported on Windows only")
	}
	channel := strings.TrimSpace(cfg.ClaudeCodeChannel)
	if channel == "" {
		channel = "stable"
	}
	// Anthropic's native Windows installer is user-local and accepts the
	// release channel (stable/latest) or an exact version as its argument.
	script := claudeInstallPowerShell(channel)
	output, code, err := runCapturedCommand(ctx, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}, commandEnvironment(cfg), appDataDir())
	if err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaude), output, fmt.Errorf("Claude Code installer failed with exit code %d: %w", code, err)
	}
	status := codingEngineStatus(ctx, cfg, editingEngineClaude)
	if !status.Installed {
		return status, output, errors.New("Claude Code installation could not be verified")
	}
	return status, output, nil
}

func installOpenCode(ctx context.Context, project string, cfg Config) (CodingEngineStatus, Config, string, error) {
	if !cfg.SetupDownloadsEnabled {
		return codingEngineStatus(ctx, cfg, editingEngineOpenCode), cfg, "", errors.New("downloads for automatic setup are disabled")
	}
	npm := discoverTool(project, "npm", cfg, true)
	var details []string
	if !npm.Available {
		updated, output, err := installKnownTool(ctx, project, "node", cfg)
		if strings.TrimSpace(output) != "" {
			details = append(details, output)
		}
		if err != nil {
			return codingEngineStatus(ctx, cfg, editingEngineOpenCode), cfg, strings.Join(details, "\n\n"), fmt.Errorf("Node.js/npm installation failed: %w", err)
		}
		cfg = updated
		npm = discoverTool(project, "npm", cfg, true)
	}
	if !npm.Available {
		return codingEngineStatus(ctx, cfg, editingEngineOpenCode), cfg, strings.Join(details, "\n\n"), errors.New("npm is not available after installation")
	}
	if err := os.MkdirAll(openCodeToolRoot(), 0o755); err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineOpenCode), cfg, strings.Join(details, "\n\n"), err
	}
	wanted := strings.TrimSpace(cfg.OpenCodeVersion)
	if wanted == "" {
		wanted = "latest"
	}
	pkg := "opencode-ai@" + wanted
	args := []string{"install", "--global", "--prefix", openCodeToolRoot(), "--no-audit", "--no-fund", pkg}
	env := append(commandEnvironment(cfg), "npm_config_cache="+filepath.Join(openCodeToolRoot(), "npm-cache"))
	output, code, err := runCapturedCommand(ctx, npm.Path, args, env, openCodeToolRoot())
	if strings.TrimSpace(output) != "" {
		details = append(details, output)
	}
	if err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineOpenCode), cfg, strings.Join(details, "\n\n"), fmt.Errorf("OpenCode npm installation failed with exit code %d: %w", code, err)
	}
	cfg.OpenCodeExecutable = findOpenCodeExecutable(cfg)
	status := codingEngineStatus(ctx, cfg, editingEngineOpenCode)
	if !status.Installed {
		return status, cfg, strings.Join(details, "\n\n"), errors.New("OpenCode installation could not be verified")
	}
	return status, cfg, strings.Join(details, "\n\n"), nil
}

func installCodingEngine(ctx context.Context, project, engine string, cfg Config) (CodingEngineStatus, Config, string, error) {
	engine = normalizeEditingEngine(engine)
	switch engine {
	case editingEngineAider:
		status, output, err := installAider(ctx, cfg)
		generic := codingEngineStatus(ctx, cfg, engine)
		if status.Executable != "" {
			generic.Executable = status.Executable
			generic.Version = status.Version
			generic.Installed = status.Installed
			generic.Authenticated = status.Installed
		}
		if err == nil {
			cfg.AiderExecutable = status.Executable
			cfg.AiderVersion = status.ExpectedVersion
		}
		return generic, normalizeConfig(cfg), output, err
	case editingEngineClaude:
		status, output, err := installClaudeCode(ctx, cfg)
		if err == nil {
			cfg.ClaudeCodeExecutable = status.Executable
		}
		return status, normalizeConfig(cfg), output, err
	case editingEngineOpenCode:
		return installOpenCode(ctx, project, cfg)
	case editingEngineNative:
		return codingEngineStatus(ctx, cfg, engine), cfg, "embedded engine", nil
	default:
		return CodingEngineStatus{}, cfg, "", errors.New("unsupported editing engine")
	}
}

func selectedEngineModel(cfg Config, fallback string) string {
	switch normalizeEditingEngine(cfg.EditingEngine) {
	case editingEngineClaude:
		if strings.TrimSpace(cfg.ClaudeCodeModel) != "" {
			return strings.TrimSpace(cfg.ClaudeCodeModel)
		}
		return "sonnet"
	case editingEngineOpenCode:
		if strings.TrimSpace(cfg.OpenCodeModel) != "" {
			return strings.TrimSpace(cfg.OpenCodeModel)
		}
		fallback = normalizeConfiguredOllamaModel(fallback)
		if fallback == "" {
			fallback = cfg.OllamaDefaultModel
		}
		if fallback != "" {
			return "ollama/" + fallback
		}
		return ""
	default:
		if strings.TrimSpace(cfg.AiderMainModel) != "" {
			return strings.TrimSpace(cfg.AiderMainModel)
		}
		return fallback
	}
}

func buildClaudeCodeArgs(task, model, mode, threadID string, cfg Config) []string {
	permission := strings.TrimSpace(cfg.ClaudeCodePermissionMode)
	if permission == "" {
		permission = "acceptEdits"
	}
	if mode == "repo-map" {
		permission = "plan"
	}
	maxTurns := cfg.ClaudeCodeMaxTurns
	if maxTurns < 1 {
		maxTurns = 24
	}
	prompt := task
	args := []string{"-p", "--output-format", "text", "--permission-mode", permission, "--max-turns", strconv.Itoa(maxTurns), "--no-chrome", "--append-system-prompt", "Work only inside the current project. Do not create commits or push unless explicitly requested. Summarize changed files and verification results."}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	if strings.TrimSpace(threadID) != "" {
		args = append(args, "--name", "localcode-"+safeThreadFileName(threadID))
	}
	args = append(args, prompt)
	return args
}

func buildOpenCodeArgs(task, model, mode string, cfg Config) []string {
	agent := strings.TrimSpace(cfg.OpenCodeAgent)
	if agent == "" {
		if mode == "repo-map" {
			agent = "plan"
		} else {
			agent = "build"
		}
	}
	args := []string{"run", "--format", "default", "--agent", agent}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	if cfg.OpenCodeAutoApprove && mode != "repo-map" {
		args = append(args, "--auto")
	}
	args = append(args, task)
	return args
}

func engineTaskForMode(project, task, mode string, cfg Config) string {
	lintCommand, testCommand := inferAiderQualityCommands(project)
	if strings.TrimSpace(cfg.AiderLintCommand) != "" {
		lintCommand = strings.TrimSpace(cfg.AiderLintCommand)
	}
	if strings.TrimSpace(cfg.AiderTestCommand) != "" {
		testCommand = strings.TrimSpace(cfg.AiderTestCommand)
	}
	switch mode {
	case "repo-map":
		return "Analyze this repository without modifying files. Produce a concise repository map covering architecture, entry points, major packages, build system, tests, risks, and the files most relevant to future edits."
	case "lint":
		if lintCommand == "" {
			return "Detect the appropriate linters for this project, run them, fix all actionable issues, and rerun them until clean. Do not commit or push."
		}
		return "Run this lint command, fix all actionable issues, and rerun it until clean: " + lintCommand + ". Do not commit or push."
	case "test":
		if testCommand == "" {
			return "Detect and run the appropriate test and build commands for this project, fix failures, and rerun until clean. Do not commit or push."
		}
		return "Run this test/build command, fix failures, and rerun it until clean: " + testCommand + ". Do not commit or push."
	default:
		return task
	}
}

func ollamaOpenAIBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	if !strings.HasSuffix(strings.ToLower(base), "/v1") {
		base += "/v1"
	}
	return base
}

func openCodeCommandEnvironment(cfg Config, model string) []string {
	env := append([]string{}, commandEnvironment(cfg)...)
	env = append(env, "OPENCODE_DISABLE_AUTOUPDATE=1", "OLLAMA_HOST="+strings.TrimRight(cfg.OllamaURL, "/"))
	model = strings.TrimSpace(model)
	if !strings.HasPrefix(strings.ToLower(model), "ollama/") {
		return env
	}
	modelID := strings.TrimSpace(model[len("ollama/"):])
	if modelID == "" {
		return env
	}
	// OpenCode requires an explicit custom-provider entry for a local Ollama
	// endpoint. OPENCODE_CONFIG_CONTENT is an official per-process config
	// source, so this does not overwrite the user's global/project config.
	content := map[string]any{
		"provider": map[string]any{
			"ollama": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Ollama (local)",
				"options": map[string]any{
					"baseURL": ollamaOpenAIBaseURL(cfg.OllamaURL),
				},
				"models": map[string]any{
					modelID: map[string]any{"name": modelID},
				},
			},
		},
	}
	if data, err := json.Marshal(content); err == nil {
		env = append(env, "OPENCODE_CONFIG_CONTENT="+string(data))
	}
	return env
}

func runCodingEngine(ctx context.Context, project, task, model, threadID, mode string, cfg Config) (CodingEngineRunResult, error) {
	engine := normalizeEditingEngine(cfg.EditingEngine)
	if engine == editingEngineAider {
		if mode == "edit" {
			result, err := runAider(ctx, project, task, model, threadID, cfg)
			return CodingEngineRunResult{Engine: engine, Output: result.Output, ChangedFiles: result.ChangedFiles, BackupDir: result.BackupDir, Executable: result.Executable, Arguments: result.Arguments, Duration: result.Duration, ExitCode: result.ExitCode}, err
		}
		output, err := runAiderUtility(ctx, project, mode, model, threadID, cfg)
		return CodingEngineRunResult{Engine: engine, Output: output, Executable: findAiderExecutable(cfg)}, err
	}
	if engine == editingEngineNative {
		return CodingEngineRunResult{}, errors.New("native engine is handled by LocalCode tools and has no external subprocess")
	}
	status := codingEngineStatus(ctx, cfg, engine)
	if !status.Installed {
		return CodingEngineRunResult{}, &CodingEngineNotInstalledError{Status: status}
	}
	if !status.Authenticated {
		return CodingEngineRunResult{}, fmt.Errorf("%s is installed but not authenticated; open the login terminal in settings", status.DisplayName)
	}
	actualTask := engineTaskForMode(project, task, mode, cfg)
	actualModel := selectedEngineModel(cfg, model)
	var args []string
	if engine == editingEngineClaude {
		args = buildClaudeCodeArgs(actualTask, actualModel, mode, threadID, cfg)
	} else {
		args = buildOpenCodeArgs(actualTask, actualModel, mode, cfg)
	}
	before := snapshotProjectFingerprints(project)
	backupDir := ""
	if mode != "repo-map" {
		var err error
		backupDir, err = createAiderBackup(project)
		if err != nil {
			return CodingEngineRunResult{}, fmt.Errorf("could not create pre-edit backup: %w", err)
		}
	}
	env := commandEnvironment(cfg)
	if engine == editingEngineOpenCode {
		env = openCodeCommandEnvironment(cfg, actualModel)
	}
	started := time.Now()
	output, exitCode, runErr := runCapturedCommand(ctx, status.Executable, args, env, project)
	after := snapshotProjectFingerprints(project)
	changed := changedFingerprintPaths(before, after)
	if backupDir != "" {
		if manifestErr := writeAiderBackupManifest(backupDir, project, before, after, changed); manifestErr != nil && runErr == nil {
			runErr = fmt.Errorf("engine edits completed but backup manifest could not be written: %w", manifestErr)
		}
	}
	result := CodingEngineRunResult{Engine: engine, Output: output, ChangedFiles: changed, BackupDir: backupDir, Executable: status.Executable, Arguments: args, Duration: time.Since(started), ExitCode: exitCode}
	if runErr != nil {
		return result, fmt.Errorf("%s failed with exit code %d: %w", status.DisplayName, exitCode, runErr)
	}
	return result, nil
}

func formatCodingEngineResult(result CodingEngineRunResult, cfg Config) string {
	name := codingEngineDisplayName(result.Engine)
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s\n%s: %s\n%s: %d\n%s: %s\n",
		localizeConfigText(cfg, "Engine", "Engine"), name,
		localizeConfigText(cfg, "Programmdatei", "Executable"), result.Executable,
		localizeConfigText(cfg, "Exitcode", "Exit code"), result.ExitCode,
		localizeConfigText(cfg, "Dauer", "Duration"), result.Duration.Round(time.Millisecond))
	if result.BackupDir != "" {
		fmt.Fprintf(&out, "%s: %s\n", localizeConfigText(cfg, "Backup", "Backup"), result.BackupDir)
	}
	if len(result.ChangedFiles) == 0 {
		out.WriteString(localizeConfigText(cfg, "Geänderte Dateien: keine erkannt\n", "Changed files: none detected\n"))
	} else {
		out.WriteString(localizeConfigText(cfg, "Geänderte Dateien:\n", "Changed files:\n"))
		for _, path := range result.ChangedFiles {
			out.WriteString("- " + path + "\n")
		}
	}
	if strings.TrimSpace(result.Output) != "" {
		out.WriteString("\n" + strings.ToUpper(name) + " OUTPUT:\n" + result.Output)
	}
	return truncateText(out.String(), 160000)
}

func (s *AppState) currentEngineThreadAndModel(cfg Config) (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentThread, selectedEngineModel(cfg, s.Model)
}

func (s *AppState) executeCodingEngineAction(ctx context.Context, project string, cfg Config, action AgentAction) (string, error) {
	engineCtx, cancel := context.WithTimeout(ctx, time.Duration(max(60, cfg.CommandTimeout))*time.Second)
	defer cancel()
	threadID, model := s.currentEngineThreadAndModel(cfg)
	mode := ""
	switch action.Action {
	case "engine_edit", "aider_edit":
		mode = "edit"
	case "engine_repo_map", "aider_repo_map":
		mode = "repo-map"
	case "engine_lint", "aider_lint":
		mode = "lint"
	case "engine_test", "aider_test":
		mode = "test"
	case "engine_undo", "aider_undo":
		s.mu.RLock()
		backup := s.LastEngineBackup
		if backup == "" {
			backup = s.LastAiderBackup
		}
		s.mu.RUnlock()
		if strings.TrimSpace(backup) == "" {
			backup = latestAiderBackup(project)
		}
		if strings.TrimSpace(backup) == "" {
			return "", errors.New("no editing-engine backup is available for this project")
		}
		return restoreAiderBackup(project, backup)
	default:
		return "", fmt.Errorf("unsupported editing-engine action: %s", action.Action)
	}
	task := strings.TrimSpace(action.Task)
	if task == "" {
		task = strings.TrimSpace(action.Message)
	}
	result, err := runCodingEngine(engineCtx, project, task, model, threadID, mode, cfg)
	if result.BackupDir != "" {
		s.mu.Lock()
		s.LastEngineBackup = result.BackupDir
		if result.Engine == editingEngineAider {
			s.LastAiderBackup = result.BackupDir
		}
		s.mu.Unlock()
	}
	formatted := formatCodingEngineResult(result, cfg)
	if err != nil {
		return formatted, err
	}
	return formatted, nil
}

func (s *AppState) offerInstallCodingEngine(ctx context.Context, project string, cfg Config) (Config, string, bool, error) {
	engine := normalizeEditingEngine(cfg.EditingEngine)
	status := codingEngineStatus(ctx, cfg, engine)
	if status.Installed {
		return cfg, status.DisplayName + " ist bereits installiert: " + status.Version, true, nil
	}
	if !codingEngineAutoInstall(cfg, engine) {
		return cfg, status.Error, false, &CodingEngineNotInstalledError{Status: status}
	}
	action := AgentAction{Action: "install_engine", Message: localizeConfigText(cfg, status.DisplayName+" installieren", "Install "+status.DisplayName), Task: engine}
	preview := localizeConfigText(cfg,
		"LocalCode installiert "+status.DisplayName+" benutzerlokal mit dem offiziellen Installationsweg und verifiziert Programmdatei und Version. Zugangsdaten werden nicht automatisiert erzeugt.",
		"LocalCode installs "+status.DisplayName+" for the current user through the official installation method and verifies the executable and version. Credentials are not generated automatically.")
	approved, err := s.requestApprovalWithPreview(ctx, project, action, preview)
	if err != nil {
		return cfg, "", false, err
	}
	if !approved {
		return cfg, localizeConfigText(cfg, "Installation wurde abgelehnt.", "Installation was declined."), false, nil
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: action.Message, Action: action.Action, Preview: preview})
	installCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	status, newCfg, output, installErr := installCodingEngine(installCtx, project, engine, cfg)
	cancel()
	if installErr != nil {
		detail := strings.TrimSpace(output + "\n\nERROR: " + installErr.Error())
		s.AddEvent(UIEvent{Type: "tool_error", Message: status.DisplayName + " konnte nicht installiert werden", Detail: detail, Action: action.Action})
		return cfg, detail, false, installErr
	}
	newCfg = normalizeConfig(newCfg)
	s.mu.Lock()
	s.Config = newCfg
	s.mu.Unlock()
	if err := saveConfig(newCfg); err != nil {
		return cfg, output, false, fmt.Errorf("engine installed but configuration could not be saved: %w", err)
	}
	detail := strings.TrimSpace(output + "\n\nVerified: " + status.Executable + "\n" + status.Version)
	s.AddEvent(UIEvent{Type: "action_done", Message: status.DisplayName + " installiert und verifiziert", Detail: truncateText(detail, 30000), Action: action.Action})
	return newCfg, detail, true, nil
}
