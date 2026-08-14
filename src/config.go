// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const productDirName = "LocalCode"
const legacyProductDirName = "Local" + "Codex"

func userProfileDir() string {
	if override := strings.TrimSpace(os.Getenv("LOCALCODE_USER_HOME")); override != "" {
		return filepath.Clean(override)
	}
	if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
		return filepath.Clean(profile)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Clean(home)
	}
	return "."
}

func preferredProjectRoot() string {
	profile := userProfileDir()
	candidates := []string{filepath.Join(profile, "Projekte"), filepath.Join(profile, "Projects")}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	root := candidates[0]
	_ = os.MkdirAll(root, 0o755)
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return filepath.Clean(root)
}

func defaultBlockedPatterns() []string {
	return []string{
		`(?i)\bformat\s+[a-z]:`, `(?i)\bdiskpart\b`, `(?i)\bshutdown\b`, `(?i)\brestart-computer\b`,
		`(?i)\bstop-computer\b`, `(?i)\brm\s+-rf\s+/$`, `(?i)\bdel\s+/s\s+/q\s+[a-z]:\\`,
		`(?i)\brd\s+/s\s+/q\s+[a-z]:\\`, `(?i)\bgit\s+clean\s+-fdx\b`, `(?i)\bgit\s+reset\s+--hard\b`,
	}
}

func defaultMCPServers() []MCPServerConfig {
	return []MCPServerConfig{
		{
			Name: "filesystem", DisplayName: "Filesystem", Description: "Sichere Datei- und Verzeichnisoperationen innerhalb des aktiven Projekts.",
			Enabled: true, Transport: "builtin", Preset: "filesystem", ProjectScoped: true, TimeoutSec: 60,
		},
		{
			Name: "powershell", DisplayName: "PowerShell", Description: "PowerShell-Befehle, Cmdlet-Erkennung und Hilfetexte mit LocalCode-Genehmigungen.",
			Enabled: true, Transport: "builtin", Preset: "powershell", ProjectScoped: true, AutoInstall: true, TimeoutSec: 300,
		},
		{
			Name: "git", DisplayName: "Git", Description: "Git-Status, Diff, Historie, Branches, Staging und Commits mit sicherer Argumentübergabe.",
			Enabled: true, Transport: "builtin", Preset: "git", ProjectScoped: true, AutoInstall: true, TimeoutSec: 300,
		},
		{
			Name: "fetch", DisplayName: "Fetch", Description: "Offizieller MCP-Referenzserver zum Abrufen und Umwandeln von Webseiten.",
			Enabled: true, Transport: "stdio", Preset: "fetch", Command: "uvx", Args: []string{"mcp-server-fetch"},
			Env: map[string]string{"PYTHONIOENCODING": "utf-8"}, AutoInstall: true, TimeoutSec: 120,
		},
		{
			Name: "github", DisplayName: "GitHub", Description: "Offizieller GitHub MCP Server für Repositories, Issues, Pull Requests und Actions.",
			Enabled: false, Transport: "streamable-http", Preset: "github", URL: "https://api.githubcopilot.com/mcp/x/all",
			Headers: map[string]string{"Authorization": "Bearer ${GITHUB_PAT_TOKEN}"}, AuthEnv: "GITHUB_PAT_TOKEN", TimeoutSec: 120,
		},
		{
			Name: "playwright", DisplayName: "Playwright Browser", Description: "Offizieller Microsoft Playwright MCP Server für Browserautomation; ersetzt den archivierten Puppeteer-Server.",
			Enabled: true, Transport: "stdio", Preset: "playwright", Command: "npx", Args: []string{"-y", "@playwright/mcp@latest", "--browser", "msedge", "--user-data-dir", "${APP_DATA}\browser-profile", "--output-dir", "${APP_DATA}\browser-output"},
			AutoInstall: true, ProjectScoped: true, TimeoutSec: 180,
		},
	}
}

func mergeDefaultMCPServers(existing []MCPServerConfig) []MCPServerConfig {
	defaults := defaultMCPServers()
	if len(existing) == 0 {
		return defaults
	}
	seen := map[string]bool{}
	out := make([]MCPServerConfig, 0, len(existing)+len(defaults))
	for _, server := range existing {
		name := strings.ToLower(strings.TrimSpace(server.Name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, server)
	}
	for _, server := range defaults {
		name := strings.ToLower(server.Name)
		if !seen[name] {
			out = append(out, server)
		}
	}
	return out
}

func defaultConfig() Config {
	return Config{
		SchemaVersion: 10, RootProjectDir: preferredProjectRoot(), Port: 32145, ProjectAliases: map[string]string{},
		SetupDownloadsEnabled: true, OllamaAutoInstall: true, OllamaAutoPull: true, OllamaDefaultModel: defaultCodingModel,
		EditingEngine: "aider", AiderEnabled: true, AiderAutoInstall: true, AiderVersion: aiderPinnedVersion,
		AiderEditFormat: "diff", AiderEditorEditFormat: "editor-diff", AiderMapTokens: 4096, AiderMaxChatHistoryTokens: 8192,
		AiderAutoLint: true, AiderAutoTest: true, AiderUseGit: true, AiderAutoCommits: false,
		ClaudeCodeEnabled: true, ClaudeCodeAutoInstall: true, ClaudeCodeChannel: "stable", ClaudeCodeModel: "sonnet", ClaudeCodePermissionMode: "acceptEdits", ClaudeCodeMaxTurns: 24,
		OpenCodeEnabled: true, OpenCodeAutoInstall: true, OpenCodeVersion: "latest", OpenCodeAgent: "build", OpenCodeAutoApprove: true,
		ContextLength: 32768, ContextCompactionEnabled: true, ContextCompactionThresholdPercent: 68, ContextCompactionKeepRecent: 12, MaxAgentSteps: 60, CommandTimeout: 300, ModelTimeout: 240,
		ApprovalMode: "strict", SandboxMode: "project", NetworkEnabled: true,
		WebSearchProvider: "duckduckgo", WebSearchAPIKeyEnv: "OLLAMA_API_KEY", WebSearchMaxResults: 6,
		WebFetchMaxBytes: 2 << 20, ImageGeneratorProvider: "automatic1111", ImageGeneratorURL: "http://127.0.0.1:7860", ImageGeneratorSteps: 20, ImageGeneratorCFGScale: 7.0,
		GitEnabled: true, AutoStateUpdate: true,
		StateFile: "STATE.md", CreateProjectDocs: true, BlockedCommandPatterns: defaultBlockedPatterns(),
		UITheme: "dark", UIAccent: "#2f81f7", UIBackground: "#171717", UIForeground: "#f3f4f6",
		UIFont: "Segoe UI", CodeFont: "Cascadia Mono", UILeftWidth: 296, UIRightWidth: 340,
		UITerminalHeight: 260, ShowBottomBar: true, TerminalDock: "bottom", TerminalShell: "powershell",
		AgentEnvironment: "windows-native", DefaultOpenTarget: "vscode", Language: "auto",
		ResponseSpeed: "balanced", ProfileName: "Local user", AvatarInitials: "LC",
		PreferredLanguage: "auto", VoiceEnabled: false, PetEnabled: false, PetName: "Codey",
		Shortcuts: map[string]string{
			"new_chat": "Ctrl+N", "settings": "Ctrl+,", "terminal": "Ctrl+`", "send": "Enter",
			"newline": "Shift+Enter", "toggle_left": "Ctrl+Shift+L", "toggle_right": "Ctrl+Shift+R",
		},
		EnvironmentVars:   map[string]string{},
		MCPServers:        defaultMCPServers(),
		AutoDiscoverTools: true, AutoResearchToolHelp: true, ToolOverrides: map[string]string{},
	}
}

func userConfigBaseDir() string {
	if override := strings.TrimSpace(os.Getenv("LOCALCODE_CONFIG_HOME")); override != "" {
		return filepath.Clean(override)
	}
	// Honor XDG_CONFIG_HOME on every platform. Besides making portable and
	// enterprise deployments predictable, this keeps tests isolated on Windows.
	if override := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); override != "" {
		return filepath.Clean(override)
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = filepath.Join(userProfileDir(), ".config")
	}
	return base
}

func userCacheBaseDir() string {
	if override := strings.TrimSpace(os.Getenv("LOCALCODE_CACHE_HOME")); override != "" {
		return filepath.Clean(override)
	}
	if override := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); override != "" {
		return filepath.Clean(override)
	}
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = filepath.Join(userProfileDir(), ".cache")
	}
	return base
}

func appDataDir() string {
	return filepath.Join(userConfigBaseDir(), productDirName)
}

func configPath() string {
	return filepath.Join(appDataDir(), "config.json")
}

func legacyAppDataDir() string {
	return filepath.Join(userConfigBaseDir(), legacyProductDirName)
}

func copyFileIfMissing(source, target string) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	return closeErr
}

func copyDirIfMissing(source, target string) error {
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyFileIfMissing(path, dst)
	})
}

func migrateLegacyProductData() {
	legacyDir := legacyAppDataDir()
	if info, err := os.Stat(legacyDir); err == nil && info.IsDir() {
		for _, name := range []string{"config.json", "threads.json"} {
			_ = copyFileIfMissing(filepath.Join(legacyDir, name), filepath.Join(appDataDir(), name))
		}
	}

	legacyCache := filepath.Join(userCacheBaseDir(), legacyProductDirName)
	newCache := filepath.Join(userCacheBaseDir(), productDirName)
	_ = copyDirIfMissing(filepath.Join(legacyCache, "backups"), filepath.Join(newCache, "backups"))
	_ = copyFileIfMissing(filepath.Join(legacyCache, "local"+"codex.log"), filepath.Join(newCache, "localcode-migrated.log"))
}

func pathWithin(base, candidate string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(candidate) == "" {
		return false
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func suspiciousProjectRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" {
		return true
	}
	// Only LocalCode's own managed data directories are unsafe as workspace
	// roots. Rejecting all of APPDATA/LOCALAPPDATA also rejected legitimate
	// temporary projects and made Windows tests depend on the real user profile.
	managed := []string{
		appDataDir(),
		legacyAppDataDir(),
		filepath.Join(userCacheBaseDir(), productDirName),
		filepath.Join(userCacheBaseDir(), legacyProductDirName),
	}
	for _, key := range []string{"LOCALAPPDATA", "APPDATA"} {
		if base := strings.TrimSpace(os.Getenv(key)); base != "" {
			managed = append(managed,
				filepath.Join(base, productDirName),
				filepath.Join(base, legacyProductDirName),
			)
		}
	}
	for _, base := range managed {
		if pathWithin(base, root) || pathWithin(root, base) {
			return true
		}
	}
	return false
}

func normalizeProjectAliases(values map[string]string) map[string]string {
	out := map[string]string{}
	for path, alias := range values {
		path = strings.TrimSpace(path)
		alias = strings.TrimSpace(alias)
		if path == "" || alias == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = filepath.Clean(abs)
		}
		runes := []rune(alias)
		if len(runes) > 120 {
			alias = strings.TrimSpace(string(runes[:120]))
		}
		if alias != "" {
			out[path] = alias
		}
	}
	return out
}

func normalizeProjectPathList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = filepath.Clean(abs)
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func normalizeConfig(cfg Config) Config {
	d := defaultConfig()
	oldSchema := cfg.SchemaVersion
	if cfg.SchemaVersion < 10 {
		cfg.SchemaVersion = 10
	}
	if oldSchema < 3 {
		cfg.AutoDiscoverTools = true
		cfg.AutoResearchToolHelp = true
		// These fields did not exist in early schemas. Preserve the historical
		// secure defaults instead of interpreting the zero-value as an explicit
		// opt-out during in-memory migrations and tests.
		cfg.GitEnabled = d.GitEnabled
		cfg.AutoStateUpdate = d.AutoStateUpdate
		cfg.CreateProjectDocs = d.CreateProjectDocs
		cfg.NetworkEnabled = d.NetworkEnabled
	}
	if cfg.Port == 0 {
		cfg.Port = d.Port
	}
	if oldSchema < 8 {
		cfg.OllamaAutoInstall = true
		cfg.OllamaAutoPull = true
	}
	if oldSchema < 10 {
		// Dependency downloads are intentionally separate from the agent/web
		// network switch. Earlier releases could deadlock during startup when
		// network_enabled was false, because the UI was not available yet to
		// re-enable it. Existing installations therefore migrate to allowing
		// only the explicitly enabled automatic setup downloads.
		cfg.SetupDownloadsEnabled = true
	}
	if strings.TrimSpace(cfg.OllamaDefaultModel) == "" {
		cfg.OllamaDefaultModel = d.OllamaDefaultModel
	}
	if strings.TrimSpace(cfg.EditingEngine) == "" {
		cfg.EditingEngine = d.EditingEngine
	}
	cfg.EditingEngine = normalizeEditingEngine(cfg.EditingEngine)
	if strings.TrimSpace(cfg.AiderVersion) == "" {
		cfg.AiderVersion = aiderPinnedVersion
	}
	if strings.TrimSpace(cfg.AiderEditFormat) == "" {
		cfg.AiderEditFormat = d.AiderEditFormat
	}
	if strings.TrimSpace(cfg.AiderEditorEditFormat) == "" {
		cfg.AiderEditorEditFormat = d.AiderEditorEditFormat
	}
	if cfg.AiderMapTokens < 0 || cfg.AiderMapTokens > 32768 {
		cfg.AiderMapTokens = d.AiderMapTokens
	}
	if cfg.AiderMapTokens == 0 && oldSchema < 7 {
		cfg.AiderMapTokens = d.AiderMapTokens
	}
	if cfg.AiderMaxChatHistoryTokens < 1024 || cfg.AiderMaxChatHistoryTokens > 131072 {
		cfg.AiderMaxChatHistoryTokens = d.AiderMaxChatHistoryTokens
	}
	validAiderFormats := map[string]bool{"auto": true, "diff": true, "whole": true, "udiff": true, "editor-diff": true, "editor-whole": true}
	if !validAiderFormats[cfg.AiderEditFormat] {
		cfg.AiderEditFormat = d.AiderEditFormat
	}
	if !validAiderFormats[cfg.AiderEditorEditFormat] {
		cfg.AiderEditorEditFormat = d.AiderEditorEditFormat
	}
	if !cfg.AiderUseGit {
		cfg.AiderAutoCommits = false
	}
	if oldSchema < 7 {
		cfg.AiderEnabled = true
		cfg.AiderAutoInstall = true
		cfg.AiderAutoLint = true
		cfg.AiderAutoTest = true
		cfg.AiderUseGit = true
	}
	if oldSchema < 9 {
		cfg.ClaudeCodeEnabled = true
		cfg.ClaudeCodeAutoInstall = true
		cfg.OpenCodeEnabled = true
		cfg.OpenCodeAutoInstall = true
		cfg.OpenCodeAutoApprove = true
	}
	if strings.TrimSpace(cfg.ClaudeCodeChannel) == "" {
		cfg.ClaudeCodeChannel = d.ClaudeCodeChannel
	}
	if strings.TrimSpace(cfg.ClaudeCodeModel) == "" {
		cfg.ClaudeCodeModel = d.ClaudeCodeModel
	}
	switch cfg.ClaudeCodePermissionMode {
	case "default", "acceptEdits", "plan", "dontAsk":
	default:
		cfg.ClaudeCodePermissionMode = d.ClaudeCodePermissionMode
	}
	if cfg.ClaudeCodeMaxTurns < 1 || cfg.ClaudeCodeMaxTurns > 200 {
		cfg.ClaudeCodeMaxTurns = d.ClaudeCodeMaxTurns
	}
	if strings.TrimSpace(cfg.OpenCodeVersion) == "" {
		cfg.OpenCodeVersion = d.OpenCodeVersion
	}
	if strings.TrimSpace(cfg.OpenCodeAgent) == "" {
		cfg.OpenCodeAgent = d.OpenCodeAgent
	}
	if cfg.ProjectAliases == nil {
		cfg.ProjectAliases = map[string]string{}
	}
	cfg.ProjectAliases = normalizeProjectAliases(cfg.ProjectAliases)
	cfg.PinnedProjects = normalizeProjectPathList(cfg.PinnedProjects)
	cfg.HiddenProjects = normalizeProjectPathList(cfg.HiddenProjects)
	if cfg.ContextLength < 4096 {
		cfg.ContextLength = d.ContextLength
	}
	if oldSchema < 4 {
		cfg.ContextCompactionEnabled = true
	}
	if cfg.ContextCompactionThresholdPercent < 45 || cfg.ContextCompactionThresholdPercent > 90 {
		cfg.ContextCompactionThresholdPercent = d.ContextCompactionThresholdPercent
	}
	if cfg.ContextCompactionKeepRecent < 6 || cfg.ContextCompactionKeepRecent > 40 {
		cfg.ContextCompactionKeepRecent = d.ContextCompactionKeepRecent
	}
	if cfg.MaxAgentSteps < 10 || cfg.MaxAgentSteps > 200 {
		cfg.MaxAgentSteps = d.MaxAgentSteps
	}
	if cfg.CommandTimeout < 10 || cfg.CommandTimeout > 3600 {
		cfg.CommandTimeout = d.CommandTimeout
	}
	if cfg.ModelTimeout < 30 || cfg.ModelTimeout > 1800 {
		cfg.ModelTimeout = d.ModelTimeout
	}
	if cfg.ApprovalMode == "" {
		cfg.ApprovalMode = d.ApprovalMode
	}
	switch cfg.ApprovalMode {
	case "strict", "balanced", "auto", "dangerous":
	default:
		cfg.ApprovalMode = d.ApprovalMode
	}
	if cfg.SandboxMode == "" {
		cfg.SandboxMode = d.SandboxMode
	}
	switch cfg.SandboxMode {
	case "project", "workspace", "unrestricted":
	default:
		cfg.SandboxMode = d.SandboxMode
	}
	if cfg.WebSearchProvider == "" {
		cfg.WebSearchProvider = d.WebSearchProvider
	}
	switch cfg.WebSearchProvider {
	case "disabled", "duckduckgo", "ollama":
	default:
		cfg.WebSearchProvider = d.WebSearchProvider
	}
	if cfg.WebSearchAPIKeyEnv == "" {
		cfg.WebSearchAPIKeyEnv = d.WebSearchAPIKeyEnv
	}
	if cfg.WebSearchMaxResults < 1 || cfg.WebSearchMaxResults > 10 {
		cfg.WebSearchMaxResults = d.WebSearchMaxResults
	}
	if cfg.WebFetchMaxBytes < 65536 || cfg.WebFetchMaxBytes > 16<<20 {
		cfg.WebFetchMaxBytes = d.WebFetchMaxBytes
	}
	if strings.TrimSpace(cfg.ImageGeneratorProvider) == "" {
		cfg.ImageGeneratorProvider = d.ImageGeneratorProvider
	}
	cfg.ImageGeneratorProvider = strings.ToLower(strings.TrimSpace(cfg.ImageGeneratorProvider))
	switch cfg.ImageGeneratorProvider {
	case "disabled", "automatic1111":
	default:
		cfg.ImageGeneratorProvider = d.ImageGeneratorProvider
	}
	if strings.TrimSpace(cfg.ImageGeneratorURL) == "" {
		cfg.ImageGeneratorURL = d.ImageGeneratorURL
	}
	cfg.ImageGeneratorURL = strings.TrimRight(strings.TrimSpace(cfg.ImageGeneratorURL), "/")
	if cfg.ImageGeneratorSteps < 1 || cfg.ImageGeneratorSteps > 80 {
		cfg.ImageGeneratorSteps = d.ImageGeneratorSteps
	}
	if cfg.ImageGeneratorCFGScale < 1 || cfg.ImageGeneratorCFGScale > 30 {
		cfg.ImageGeneratorCFGScale = d.ImageGeneratorCFGScale
	}
	if cfg.StateFile == "" {
		cfg.StateFile = d.StateFile
	}
	if cfg.BlockedCommandPatterns == nil {
		cfg.BlockedCommandPatterns = d.BlockedCommandPatterns
	}
	if cfg.AllowedRoots == nil {
		cfg.AllowedRoots = []string{}
	}
	if cfg.ApprovalRules == nil {
		cfg.ApprovalRules = []ApprovalRule{}
	}
	cfg.ApprovalRules = normalizeApprovalRules(cfg.ApprovalRules)
	cfg.MCPServers = mergeDefaultMCPServers(cfg.MCPServers)
	for index := range cfg.MCPServers {
		server := &cfg.MCPServers[index]
		server.Name = strings.TrimSpace(server.Name)
		if strings.TrimSpace(server.DisplayName) == "" {
			server.DisplayName = server.Name
		}
		server.Transport = strings.ToLower(strings.TrimSpace(server.Transport))
		if server.Transport == "http" {
			server.Transport = "streamable-http"
		}
		if server.TimeoutSec <= 0 {
			server.TimeoutSec = 60
		}
		if server.Env == nil {
			server.Env = map[string]string{}
		}
		if server.Headers == nil {
			server.Headers = map[string]string{}
		}
		if strings.EqualFold(server.Preset, "playwright") && (strings.EqualFold(filepath.Base(server.Command), "cmd.exe") || strings.EqualFold(server.Command, "cmd")) {
			if len(server.Args) >= 2 && strings.EqualFold(server.Args[0], "/c") && strings.EqualFold(server.Args[1], "npx") {
				server.Command = "npx"
				server.Args = append([]string(nil), server.Args[2:]...)
			}
		}
	}
	if cfg.UITheme == "" {
		cfg.UITheme = d.UITheme
	}
	switch cfg.UITheme {
	case "dark", "light", "system":
	default:
		cfg.UITheme = d.UITheme
	}
	if strings.TrimSpace(cfg.UIAccent) == "" {
		cfg.UIAccent = d.UIAccent
	}
	if strings.TrimSpace(cfg.UIBackground) == "" {
		cfg.UIBackground = d.UIBackground
	}
	if strings.TrimSpace(cfg.UIForeground) == "" {
		cfg.UIForeground = d.UIForeground
	}
	if strings.TrimSpace(cfg.UIFont) == "" {
		cfg.UIFont = d.UIFont
	}
	if strings.TrimSpace(cfg.CodeFont) == "" {
		cfg.CodeFont = d.CodeFont
	}
	if cfg.UILeftWidth < 220 || cfg.UILeftWidth > 520 {
		cfg.UILeftWidth = d.UILeftWidth
	}
	if cfg.UIRightWidth < 260 || cfg.UIRightWidth > 620 {
		cfg.UIRightWidth = d.UIRightWidth
	}
	if cfg.UITerminalHeight < 140 || cfg.UITerminalHeight > 700 {
		cfg.UITerminalHeight = d.UITerminalHeight
	}
	if cfg.TerminalDock == "" {
		cfg.TerminalDock = d.TerminalDock
	}
	if cfg.TerminalDock != "bottom" && cfg.TerminalDock != "right" {
		cfg.TerminalDock = d.TerminalDock
	}
	if cfg.TerminalShell == "" {
		cfg.TerminalShell = d.TerminalShell
	}
	switch cfg.TerminalShell {
	case "powershell", "cmd", "wsl":
	default:
		cfg.TerminalShell = d.TerminalShell
	}
	if cfg.AgentEnvironment == "" {
		cfg.AgentEnvironment = d.AgentEnvironment
	}
	if cfg.AgentEnvironment != "windows-native" && cfg.AgentEnvironment != "wsl" {
		cfg.AgentEnvironment = d.AgentEnvironment
	}
	if cfg.DefaultOpenTarget == "" {
		cfg.DefaultOpenTarget = d.DefaultOpenTarget
	}
	switch cfg.DefaultOpenTarget {
	case "explorer", "vscode", "visualstudio":
	default:
		cfg.DefaultOpenTarget = d.DefaultOpenTarget
	}
	if oldSchema < 5 && strings.EqualFold(strings.TrimSpace(cfg.Language), "de") && strings.EqualFold(strings.TrimSpace(cfg.PreferredLanguage), "Deutsch") {
		// Previous releases hard-coded German as the default. Migrate that
		// implicit default to Windows language detection while preserving any
		// explicit English or custom response-language selection.
		cfg.Language = "auto"
		cfg.PreferredLanguage = "auto"
	}
	cfg.Language = normalizeLanguageSetting(cfg.Language)
	if cfg.ResponseSpeed == "" {
		cfg.ResponseSpeed = d.ResponseSpeed
	}
	switch cfg.ResponseSpeed {
	case "fast", "balanced", "thorough":
	default:
		cfg.ResponseSpeed = d.ResponseSpeed
	}
	if strings.TrimSpace(cfg.ProfileName) == "" {
		cfg.ProfileName = d.ProfileName
	}
	if strings.TrimSpace(cfg.AvatarInitials) == "" {
		cfg.AvatarInitials = d.AvatarInitials
	}
	if strings.TrimSpace(cfg.PreferredLanguage) == "" {
		cfg.PreferredLanguage = d.PreferredLanguage
	}
	if strings.TrimSpace(cfg.PetName) == "" {
		cfg.PetName = d.PetName
	}
	if cfg.Shortcuts == nil {
		cfg.Shortcuts = d.Shortcuts
	} else {
		for k, v := range d.Shortcuts {
			if strings.TrimSpace(cfg.Shortcuts[k]) == "" {
				cfg.Shortcuts[k] = v
			}
		}
	}
	if cfg.EnvironmentVars == nil {
		cfg.EnvironmentVars = map[string]string{}
	}
	if cfg.ToolOverrides == nil {
		cfg.ToolOverrides = map[string]string{}
	}
	cfg.Memories = normalizeMemoryEntries(cfg.Memories)

	root := strings.TrimSpace(cfg.RootProjectDir)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	info, statErr := os.Stat(root)
	if root == "" || statErr != nil || !info.IsDir() || suspiciousProjectRoot(root) {
		root = d.RootProjectDir
	}
	cfg.RootProjectDir = filepath.Clean(root)

	cleanedRoots := make([]string, 0, len(cfg.AllowedRoots))
	for _, r := range cfg.AllowedRoots {
		if abs, err := filepath.Abs(strings.TrimSpace(r)); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				cleanedRoots = append(cleanedRoots, filepath.Clean(abs))
			}
		}
	}
	cfg.AllowedRoots = cleanedRoots

	for i := range cfg.MCPServers {
		cfg.MCPServers[i].Name = strings.TrimSpace(cfg.MCPServers[i].Name)
		cfg.MCPServers[i].DisplayName = strings.TrimSpace(cfg.MCPServers[i].DisplayName)
		if cfg.MCPServers[i].DisplayName == "" {
			cfg.MCPServers[i].DisplayName = cfg.MCPServers[i].Name
		}
		cfg.MCPServers[i].Preset = strings.ToLower(strings.TrimSpace(cfg.MCPServers[i].Preset))
		cfg.MCPServers[i].Transport = strings.ToLower(strings.TrimSpace(cfg.MCPServers[i].Transport))
		if cfg.MCPServers[i].Transport == "http" {
			cfg.MCPServers[i].Transport = "streamable-http"
		}
		if cfg.MCPServers[i].TimeoutSec <= 0 {
			cfg.MCPServers[i].TimeoutSec = 60
		}
		if cfg.MCPServers[i].Env == nil {
			cfg.MCPServers[i].Env = map[string]string{}
		}
		if cfg.MCPServers[i].Headers == nil {
			cfg.MCPServers[i].Headers = map[string]string{}
		}
	}

	if cfg.LastProject != "" {
		full, err := ensureWithinRoot(cfg.RootProjectDir, cfg.LastProject)
		if err != nil {
			cfg.LastProject = ""
		} else if info, err := os.Stat(full); err != nil || !info.IsDir() {
			cfg.LastProject = ""
		} else {
			cfg.LastProject = full
		}
	}
	cfg.OllamaURL = normalizeOllamaBaseURL(cfg.OllamaURL)
	return cfg
}

func loadConfig() Config {
	migrateLegacyProductData()
	cfg := defaultConfig()
	if data, err := os.ReadFile(configPath()); err == nil {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(data, &raw)
		_ = json.Unmarshal(data, &cfg)
		defaults := defaultConfig()
		if _, ok := raw["network_enabled"]; !ok {
			cfg.NetworkEnabled = defaults.NetworkEnabled
		}
		if _, ok := raw["git_enabled"]; !ok {
			cfg.GitEnabled = defaults.GitEnabled
		}
		if _, ok := raw["auto_state_update"]; !ok {
			cfg.AutoStateUpdate = defaults.AutoStateUpdate
		}
		if _, ok := raw["create_project_docs"]; !ok {
			cfg.CreateProjectDocs = defaults.CreateProjectDocs
		}
		if _, ok := raw["show_bottom_bar"]; !ok {
			cfg.ShowBottomBar = defaults.ShowBottomBar
		}
	}
	normalized := normalizeConfig(cfg)
	if !configsEqual(normalized, cfg) {
		_ = saveConfig(normalized)
	}
	return normalized
}

func configsEqual(a, b Config) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func saveConfig(cfg Config) error {
	cfg = normalizeConfig(cfg)
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows does not replace an existing file with os.Rename. Keep a
	// recoverable backup while replacing the configuration atomically enough
	// for a local desktop application.
	backup := path + ".bak"
	_ = os.Remove(backup)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func logPath() string {
	return filepath.Join(userCacheBaseDir(), productDirName, "localcode.log")
}
