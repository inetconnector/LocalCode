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
	"sort"
	"strings"
	"time"
)

func commandAvailable(command, project string) (string, bool) {
	command = strings.TrimSpace(expandMCPValue(command, project))
	if command == "" {
		return "", false
	}
	if filepath.IsAbs(command) {
		if info, err := os.Stat(command); err == nil && !info.IsDir() {
			return command, true
		}
		return command, false
	}
	if path, err := exec.LookPath(command); err == nil {
		return path, true
	}
	if runtime.GOOS == "windows" {
		candidates := []string{
			filepath.Join(userProfileDir(), ".local", "bin", command+".exe"),
			filepath.Join(appDataDir(), "tools", "uv", command+".exe"),
			filepath.Join(appDataDir(), "tools", "node", command+".cmd"),
			filepath.Join(appDataDir(), "tools", "node", command+".exe"),
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
	}
	return command, false
}

func parseMCPToolList(raw string) ([]string, error) {
	var response struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(response.Tools))
	for _, tool := range response.Tools {
		if strings.TrimSpace(tool.Name) != "" {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func mcpServerStatus(ctx context.Context, cfg Config, project string, server MCPServerConfig, connect bool) MCPServerStatus {
	status := MCPServerStatus{Name: server.Name, DisplayName: server.DisplayName, Preset: server.Preset, Enabled: server.Enabled}
	if status.DisplayName == "" {
		status.DisplayName = server.Name
	}
	switch strings.ToLower(server.Transport) {
	case "builtin":
		status.Source = "LocalCode built-in"
		status.Tools = builtinToolNames(server.Preset, cfg)
		status.ToolCount = len(status.Tools)
		switch strings.ToLower(server.Preset) {
		case "filesystem":
			status.Installed = true
		case "powershell":
			path := powershellExecutable(project, cfg)
			status.Installed = path != ""
			if path != "" {
				status.Source = path
			} else {
				status.Error = localizeConfigText(cfg, "PowerShell wurde nicht gefunden. LocalCode kann PowerShell 7 nach Genehmigung installieren.", "PowerShell was not found. LocalCode can install PowerShell 7 after approval.")
			}
		case "git":
			info := discoverTool(project, "git", cfg, false)
			status.Installed = info.Available
			if info.Available {
				status.Source = info.Path
			} else {
				status.Error = localizeConfigText(cfg, "Git wurde nicht gefunden. LocalCode kann MinGit nach Genehmigung benutzerlokal installieren.", "Git was not found. LocalCode can install MinGit for the current user after approval.")
			}
		default:
			status.Installed = true
		}
		status.Connected = server.Enabled && status.Installed
		return status
	case "stdio":
		checkCommand := server.Command
		if strings.EqualFold(server.Preset, "fetch") {
			checkCommand = "uvx"
		}
		if strings.EqualFold(server.Preset, "playwright") {
			checkCommand = "npx"
		}
		command, available := commandAvailable(checkCommand, project)
		status.Installed = available
		status.Source = command
		if !available {
			switch server.Preset {
			case "fetch":
				status.Error = localizeConfigText(cfg, "uvx fehlt. Über Installieren kann LocalCode uv benutzerlokal einrichten.", "uvx is missing. LocalCode can install uv for the current user.")
			case "playwright":
				status.Error = localizeConfigText(cfg, "Node.js/npx fehlt. Über Installieren kann LocalCode Node.js LTS benutzerlokal einrichten.", "Node.js/npx is missing. LocalCode can install Node.js LTS for the current user.")
			default:
				status.Error = localizeConfigText(cfg, "Startbefehl wurde nicht gefunden.", "The start command was not found.")
			}
			return status
		}
	case "streamable-http", "http":
		status.Installed = strings.TrimSpace(server.URL) != ""
		status.Source = server.URL
		if server.AuthEnv != "" && strings.TrimSpace(os.Getenv(server.AuthEnv)) == "" && githubCLIToken(ctx, project, cfg) == "" {
			status.AuthRequired = true
			status.Error = localizeConfigText(cfg, "GitHub-Anmeldung erforderlich. Verwende Anmelden oder setze GITHUB_PAT_TOKEN.", "GitHub authentication is required. Use Sign in or set GITHUB_PAT_TOKEN.")
			return status
		}
	default:
		status.Error = localizeConfigText(cfg, "Unbekannter MCP-Transport.", "Unknown MCP transport.")
		return status
	}
	if !server.Enabled || !connect {
		return status
	}
	testCtx, cancel := context.WithTimeout(ctx, time.Duration(max(server.TimeoutSec, 30))*time.Second)
	defer cancel()
	output, err := mcpCall(testCtx, cfg, project, server.Name, "tools/list", map[string]any{})
	if err != nil {
		status.Error = err.Error()
		return status
	}
	tools, err := parseMCPToolList(output)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Connected = true
	status.Tools = tools
	status.ToolCount = len(tools)
	return status
}

func allMCPStatuses(ctx context.Context, cfg Config, project string, connect bool) []MCPServerStatus {
	statuses := make([]MCPServerStatus, 0, len(cfg.MCPServers))
	for _, server := range cfg.MCPServers {
		statuses = append(statuses, mcpServerStatus(ctx, cfg, project, server, connect))
	}
	return statuses
}

func findMCPServerIndex(cfg Config, name string) int {
	for index := range cfg.MCPServers {
		if strings.EqualFold(cfg.MCPServers[index].Name, strings.TrimSpace(name)) {
			return index
		}
	}
	return -1
}

func installMCPDependency(ctx context.Context, project string, cfg Config, server MCPServerConfig) (Config, string, error) {
	switch strings.ToLower(server.Preset) {
	case "git":
		return installKnownTool(ctx, project, "git", cfg)
	case "powershell":
		return installKnownTool(ctx, project, "powershell", cfg)
	case "fetch":
		path, output, err := installUV(ctx)
		if err != nil {
			return cfg, output, err
		}
		index := findMCPServerIndex(cfg, server.Name)
		if index >= 0 {
			cfg.MCPServers[index].Command = path
		}
		return cfg, output, nil
	case "playwright":
		updated, output, err := installKnownTool(ctx, project, "node", cfg)
		if err != nil {
			return cfg, output, err
		}
		npx := discoverTool(project, "npx", updated, false)
		if !npx.Available {
			return updated, output, errors.New("npx was not found after installing Node.js")
		}
		index := findMCPServerIndex(updated, server.Name)
		if index >= 0 {
			updated.MCPServers[index].Command = npx.Path
			args := updated.MCPServers[index].Args
			if len(args) >= 2 && strings.EqualFold(args[0], "/c") && strings.EqualFold(args[1], "npx") {
				args = args[2:]
			}
			updated.MCPServers[index].Args = args
		}
		return updated, output + "\n\nnpx: " + npx.Path, nil
	case "github":
		return installKnownTool(ctx, project, "gh", cfg)
	default:
		return cfg, "", fmt.Errorf("no managed installer for MCP server %s", server.Name)
	}
}

func installUV(ctx context.Context) (string, string, error) {
	if runtime.GOOS != "windows" {
		if path, err := exec.LookPath("uvx"); err == nil {
			return path, "uvx already installed: " + path, nil
		}
		return "", "", errors.New("automatic uv installation is currently implemented for Windows")
	}
	powershell := powershellExecutable("", defaultConfig())
	if powershell == "" {
		return "", "", errors.New("Windows PowerShell is required to install uv")
	}
	installCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	command := "$ErrorActionPreference='Stop'; $env:UV_INSTALL_DIR=" + quotePowerShellLiteral(filepath.Join(appDataDir(), "tools", "uv")) + "; irm https://astral.sh/uv/install.ps1 | iex"
	output, err := runPowerShell(installCtx, powershell, appDataDir(), command, defaultConfig())
	if err != nil {
		return "", output, err
	}
	candidates := []string{
		filepath.Join(appDataDir(), "tools", "uv", "uvx.exe"),
		filepath.Join(userProfileDir(), ".local", "bin", "uvx.exe"),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, output + "\nuvx: " + candidate, nil
		}
	}
	return "", output, errors.New("uv installer completed but uvx.exe was not found")
}

func (s *AppState) prepareMCPServer(ctx context.Context, project string, cfg Config, action AgentAction) (Config, string, bool, error) {
	server, err := findMCPServer(cfg, action.Server)
	if err != nil {
		return cfg, "", false, err
	}
	status := mcpServerStatus(ctx, cfg, project, server, false)
	if !status.Installed {
		if !server.AutoInstall {
			return cfg, "", false, fmt.Errorf("MCP server %s is not installed: %s", server.Name, status.Error)
		}
		preview := localizeConfigText(cfg,
			fmt.Sprintf("LocalCode installiert die benötigte Laufzeit für den MCP-Server %s benutzerlokal und prüft anschließend tools/list.\n\n%s", server.DisplayName, status.Error),
			fmt.Sprintf("LocalCode will install the runtime required by MCP server %s for the current user, then verify tools/list.\n\n%s", server.DisplayName, status.Error))
		approved, approvalErr := s.requestApprovalWithPreview(ctx, project, action, preview)
		if approvalErr != nil {
			return cfg, "", false, approvalErr
		}
		if !approved {
			return cfg, localizeConfigText(cfg, "MCP-Installation abgelehnt.", "MCP installation rejected."), false, nil
		}
		updated, detail, installErr := installMCPDependency(ctx, project, cfg, server)
		if installErr != nil {
			return cfg, detail, false, installErr
		}
		updated = normalizeConfig(updated)
		if saveErr := saveConfig(updated); saveErr != nil {
			return cfg, detail, false, saveErr
		}
		s.mu.Lock()
		s.Config = updated
		s.mu.Unlock()
		defaultMCPManager.ResetServer(server.Name)
		cfg = updated
		server, _ = findMCPServer(cfg, action.Server)
		s.AddEvent(UIEvent{Type: "tool_result", Message: localizeConfigText(cfg, "MCP-Abhängigkeit installiert", "MCP dependency installed"), Detail: detail, Action: action.Action})
		verify := mcpServerStatus(ctx, cfg, project, server, true)
		if !verify.Connected {
			return cfg, detail, false, fmt.Errorf("MCP server %s could not be verified after installation: %s", server.Name, verify.Error)
		}
	}
	status = mcpServerStatus(ctx, cfg, project, server, false)
	if status.AuthRequired {
		preview := localizeConfigText(cfg,
			"GitHub MCP benötigt eine Anmeldung. LocalCode öffnet gh auth login in einem sichtbaren Terminal. Zugangsdaten werden ausschließlich von GitHub CLI verarbeitet und nicht in LocalCode gespeichert.",
			"GitHub MCP requires sign-in. LocalCode will open gh auth login in a visible terminal. Credentials are handled exclusively by GitHub CLI and are not stored by LocalCode.")
		approved, approvalErr := s.requestApprovalWithPreview(ctx, project, action, preview)
		if approvalErr != nil {
			return cfg, "", false, approvalErr
		}
		if !approved {
			return cfg, localizeConfigText(cfg, "GitHub-Anmeldung abgelehnt.", "GitHub sign-in rejected."), false, nil
		}
		gh := discoverTool(project, "gh", cfg, false)
		if !gh.Available {
			updated, detail, installErr := installKnownTool(ctx, project, "gh", cfg)
			if installErr != nil {
				return cfg, detail, false, installErr
			}
			cfg = normalizeConfig(updated)
			if saveErr := saveConfig(cfg); saveErr != nil {
				return cfg, detail, false, saveErr
			}
			s.mu.Lock()
			s.Config = cfg
			s.mu.Unlock()
			gh = discoverTool(project, "gh", cfg, false)
		}
		if !gh.Available {
			return cfg, "", false, errors.New("GitHub CLI is unavailable after installation")
		}
		if err := openInteractiveTerminal(project, fmt.Sprintf("\"%s\" auth login", gh.Path), cfg); err != nil {
			return cfg, "", false, err
		}
		message := localizeConfigText(cfg,
			"GitHub-Anmeldung wurde in einem Terminal geöffnet. Schließe die Anmeldung dort ab und antworte danach mit ‚fertig‘; LocalCode prüft die Verbindung erneut.",
			"GitHub sign-in was opened in a terminal. Complete sign-in there, then reply ‘done’; LocalCode will test the connection again.")
		return cfg, message, true, nil
	}
	return cfg, "", false, nil
}
