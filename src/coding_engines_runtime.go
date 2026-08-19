// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	case editingEngineClaw:
		return installClawCode(ctx, project, cfg)
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
	case editingEngineClaw:
		return clawSelectedModel(cfg, fallback)
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
	args := []string{"-p", "--output-format", "text", "--permission-mode", permission, "--max-turns", strconv.Itoa(maxTurns), "--no-chrome", "--append-system-prompt", "Work only inside the current project. Do not create commits or push unless explicitly requested. Summarize changed files and verification results."}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	if strings.TrimSpace(threadID) != "" {
		args = append(args, "--name", "localcode-"+safeThreadFileName(threadID))
	}
	return append(args, task)
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
	return append(args, task)
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
	content := map[string]any{"provider": map[string]any{"ollama": map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Ollama (local)", "options": map[string]any{"baseURL": ollamaOpenAIBaseURL(cfg.OllamaURL)}, "models": map[string]any{modelID: map[string]any{"name": modelID}}}}}
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
	switch engine {
	case editingEngineClaude:
		args = buildClaudeCodeArgs(actualTask, actualModel, mode, threadID, cfg)
	case editingEngineClaw:
		args = buildClawArgs(actualTask, actualModel, mode)
	default:
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
	} else if engine == editingEngineClaw {
		env = clawCommandEnvironment(cfg)
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
	fmt.Fprintf(&out, "%s: %s\n%s: %s\n%s: %d\n%s: %s\n", localizeConfigText(cfg, "Engine", "Engine"), name, localizeConfigText(cfg, "Programmdatei", "Executable"), result.Executable, localizeConfigText(cfg, "Exitcode", "Exit code"), result.ExitCode, localizeConfigText(cfg, "Dauer", "Duration"), result.Duration.Round(time.Millisecond))
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
	preview := localizeConfigText(cfg, "LocalCode installiert "+status.DisplayName+" benutzerlokal mit einem gepinnten beziehungsweise offiziellen Installationsweg und verifiziert Programmdatei und Version. Zugangsdaten werden nicht automatisiert erzeugt.", "LocalCode installs "+status.DisplayName+" for the current user through a pinned or official installation method and verifies the executable and version. Credentials are not generated automatically.")
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
