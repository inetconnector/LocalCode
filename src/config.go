// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const productDirName = "LocalCode"
const legacyProductDirName = "Local" + "Codex"

func userProfileDir() string {
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

func defaultConfig() Config {
	return Config{
		RootProjectDir: preferredProjectRoot(), Port: 32145,
		ContextLength: 32768, ContextCompactionEnabled: true, ContextCompactionThresholdPercent: 68, ContextCompactionKeepRecent: 12, MaxAgentSteps: 60, CommandTimeout: 300, ModelTimeout: 240,
		ApprovalMode: "strict", SandboxMode: "project", NetworkEnabled: true,
		WebSearchProvider: "duckduckgo", WebSearchAPIKeyEnv: "OLLAMA_API_KEY", WebSearchMaxResults: 6,
		WebFetchMaxBytes: 2 << 20, GitEnabled: true, AutoStateUpdate: true,
		StateFile: "STATE.md", CreateProjectDocs: true, BlockedCommandPatterns: defaultBlockedPatterns(),
		UITheme: "dark", UIAccent: "#2f81f7", UIBackground: "#171717", UIForeground: "#f3f4f6",
		UIFont: "Segoe UI", CodeFont: "Cascadia Mono", UILeftWidth: 296, UIRightWidth: 340,
		UITerminalHeight: 260, ShowBottomBar: true, TerminalDock: "bottom", TerminalShell: "powershell",
		AgentEnvironment: "windows-native", DefaultOpenTarget: "vscode", Language: "de",
		ResponseSpeed: "balanced", ProfileName: "Local user", AvatarInitials: "LC",
		PreferredLanguage: "Deutsch", VoiceEnabled: false, PetEnabled: false, PetName: "Codey",
		Shortcuts: map[string]string{
			"new_chat": "Ctrl+N", "settings": "Ctrl+,", "terminal": "Ctrl+`", "send": "Enter",
			"newline": "Shift+Enter", "toggle_left": "Ctrl+Shift+L", "toggle_right": "Ctrl+Shift+R",
		},
		EnvironmentVars:   map[string]string{},
		AutoDiscoverTools: true, AutoResearchToolHelp: true, ToolOverrides: map[string]string{},
	}
}

func userConfigBaseDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = filepath.Join(userProfileDir(), ".config")
	}
	return base
}

func userCacheBaseDir() string {
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
	for _, key := range []string{"LOCALAPPDATA", "APPDATA"} {
		if base := strings.TrimSpace(os.Getenv(key)); base != "" && pathWithin(base, root) {
			return true
		}
	}
	return false
}

func normalizeConfig(cfg Config) Config {
	d := defaultConfig()
	oldSchema := cfg.SchemaVersion
	if cfg.SchemaVersion < 4 {
		cfg.SchemaVersion = 4
	}
	if oldSchema < 3 {
		cfg.AutoDiscoverTools = true
		cfg.AutoResearchToolHelp = true
	}
	if cfg.Port == 0 {
		cfg.Port = d.Port
	}
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
	if cfg.StateFile == "" {
		cfg.StateFile = d.StateFile
	}
	if cfg.BlockedCommandPatterns == nil {
		cfg.BlockedCommandPatterns = d.BlockedCommandPatterns
	}
	if cfg.AllowedRoots == nil {
		cfg.AllowedRoots = []string{}
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = []MCPServerConfig{}
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
	if cfg.Language == "" {
		cfg.Language = d.Language
	}
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
		cfg.MCPServers[i].Transport = strings.ToLower(strings.TrimSpace(cfg.MCPServers[i].Transport))
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
	return os.WriteFile(path, data, 0o600)
}

func logPath() string {
	return filepath.Join(userCacheBaseDir(), productDirName, "localcode.log")
}
