// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	editingEngineAider    = "aider"
	editingEngineClaude   = "claude"
	editingEngineOpenCode = "opencode"
	editingEngineClaw     = "claw"
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

type CodingEngineNotInstalledError struct{ Status CodingEngineStatus }

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
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case editingEngineAider, editingEngineClaude, editingEngineOpenCode, editingEngineClaw, editingEngineNative:
		return value
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
	case editingEngineClaw:
		return "Claw Code"
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
	case editingEngineClaw, editingEngineNative:
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
	case editingEngineClaw:
		return cfg.SetupDownloadsEnabled
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
		if abs, absErr := filepath.Abs(path); absErr == nil {
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

func claudeToolRoot() string   { return filepath.Join(appDataDir(), "tools", "claude-code") }
func openCodeToolRoot() string { return filepath.Join(appDataDir(), "tools", "opencode") }

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
	return firstExecutable(pathCandidate, filepath.Join(profile, ".local", "bin", name), filepath.Join(claudeToolRoot(), name), filepath.Join(localApp, "Programs", "Claude Code", name), filepath.Join(appData, "npm", "claude.cmd"))
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
		candidates = append(candidates, filepath.Join(openCodeToolRoot(), name), filepath.Join(openCodeToolRoot(), "bin", name), filepath.Join(os.Getenv("APPDATA"), "npm", name))
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
	return err == nil && code == 0, text
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
	status := CodingEngineStatus{Engine: engine, DisplayName: codingEngineDisplayName(engine), Enabled: codingEngineEnabled(cfg, engine)}
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
	switch engine {
	case editingEngineClaude:
		status.Executable = findClaudeCodeExecutable(cfg)
		status.InstallationRoot = claudeToolRoot()
		status.ExpectedVersion = strings.TrimSpace(cfg.ClaudeCodeChannel)
	case editingEngineOpenCode:
		status.Executable = findOpenCodeExecutable(cfg)
		status.InstallationRoot = openCodeToolRoot()
		status.ExpectedVersion = strings.TrimSpace(cfg.OpenCodeVersion)
	case editingEngineClaw:
		status.Executable = findClawExecutable()
		status.InstallationRoot = clawToolRoot()
		status.ExpectedVersion = clawPinnedCommit
	}
	if status.Executable == "" {
		status.Error = status.DisplayName + " executable not found"
		return status
	}
	versionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	var foundVersion string
	var err error
	if engine == editingEngineClaw {
		foundVersion, err = clawExecutableVersion(versionCtx, status.Executable, cfg)
	} else {
		foundVersion, err = engineExecutableVersion(versionCtx, status.Executable, cfg)
	}
	cancel()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = foundVersion
	status.Installed = true
	if engine == editingEngineClaw {
		status.Authenticated = true
		status.Authentication = "Local Ollama via process-scoped OLLAMA_HOST"
		return status
	}
	authCtx, authCancel := context.WithTimeout(ctx, 20*time.Second)
	if engine == editingEngineClaude {
		status.Authenticated, status.Authentication = claudeAuthenticationStatus(authCtx, status.Executable, cfg)
	} else {
		status.Authenticated, status.Authentication = openCodeAuthenticationStatus(authCtx, status.Executable, cfg)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.OpenCodeModel)), "ollama/") || strings.TrimSpace(cfg.OpenCodeModel) == "" {
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
