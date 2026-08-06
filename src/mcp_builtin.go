// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type mcpToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpTextResult(text string) string {
	data, _ := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	})
	return string(data)
}

func mcpErrorResult(err error) (string, error) {
	if err == nil {
		return "", nil
	}
	data, _ := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": err.Error()}},
		"isError": true,
	})
	return string(data), err
}

func decodeMCPParams(params any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	if values, ok := params.(map[string]any); ok {
		return values
	}
	data, _ := json.Marshal(params)
	var values map[string]any
	_ = json.Unmarshal(data, &values)
	if values == nil {
		values = map[string]any{}
	}
	return values
}

func mcpArgumentMap(params map[string]any) map[string]any {
	value, ok := params["arguments"]
	if !ok || value == nil {
		return map[string]any{}
	}
	if args, ok := value.(map[string]any); ok {
		return args
	}
	data, _ := json.Marshal(value)
	var args map[string]any
	_ = json.Unmarshal(data, &args)
	if args == nil {
		args = map[string]any{}
	}
	return args
}

func stringArg(args map[string]any, name string) string {
	value, _ := args[name]
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func boolArg(args map[string]any, name string, fallback bool) bool {
	value, ok := args[name]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func intArg(args map[string]any, name string, fallback int) int {
	value, ok := args[name]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case json.Number:
		n, err := typed.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		n, err := strconv.Atoi(typed)
		if err == nil {
			return n
		}
	}
	return fallback
}

func stringSliceArg(args map[string]any, name string) []string {
	value, ok := args[name]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{typed}
		}
	}
	return nil
}

func builtinTools(preset string, cfg Config) []mcpToolDefinition {
	languageDE := resolvedLanguage(cfg) == "de"
	de := func(german, english string) string {
		if languageDE {
			return german
		}
		return english
	}
	stringProp := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	boolProp := func(description string) map[string]any {
		return map[string]any{"type": "boolean", "description": description}
	}
	arrayProp := func(description string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
	}

	switch strings.ToLower(preset) {
	case "filesystem":
		return []mcpToolDefinition{
			{Name: "list_directory", Description: de("Listet Dateien und Ordner sicher innerhalb des aktiven Projekts.", "Lists files and directories safely inside the active project."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Projektpfad.", "Relative project path.")), "depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 12}}, "path")},
			{Name: "read_text_file", Description: de("Liest eine UTF-8-Textdatei aus dem aktiven Projekt.", "Reads a UTF-8 text file from the active project."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Dateipfad.", "Relative file path."))}, "path")},
			{Name: "write_file", Description: de("Schreibt eine Datei atomar und legt vorher eine LocalCode-Sicherung an.", "Writes a file atomically and creates a LocalCode backup first."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Dateipfad.", "Relative file path.")), "content": stringProp(de("Vollständiger Dateiinhalt.", "Complete file content."))}, "path", "content")},
			{Name: "create_directory", Description: de("Erstellt ein Verzeichnis innerhalb des Projekts.", "Creates a directory inside the project."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Verzeichnispfad.", "Relative directory path."))}, "path")},
			{Name: "search_files", Description: de("Durchsucht Projektdateien nach Text.", "Searches project files for text."), InputSchema: objectSchema(map[string]any{"query": stringProp(de("Suchtext.", "Search text.")), "path": stringProp(de("Optionaler relativer Startpfad.", "Optional relative start path."))}, "query")},
			{Name: "get_file_info", Description: de("Liefert Metadaten zu einer Datei oder einem Verzeichnis.", "Returns metadata for a file or directory."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Pfad.", "Relative path."))}, "path")},
			{Name: "copy_path", Description: de("Kopiert eine Datei oder ein Verzeichnis innerhalb erlaubter Wurzeln.", "Copies a file or directory within allowed roots."), InputSchema: objectSchema(map[string]any{"source": stringProp(de("Quellpfad.", "Source path.")), "destination": stringProp(de("Zielpfad.", "Destination path."))}, "source", "destination")},
			{Name: "move_path", Description: de("Verschiebt oder benennt eine Datei beziehungsweise ein Verzeichnis um.", "Moves or renames a file or directory."), InputSchema: objectSchema(map[string]any{"source": stringProp(de("Quellpfad.", "Source path.")), "destination": stringProp(de("Zielpfad.", "Destination path."))}, "source", "destination")},
			{Name: "delete_path", Description: de("Löscht einen Pfad innerhalb des Projekts. Diese Aktion erfordert eine Genehmigung.", "Deletes a path inside the project. This action requires approval."), InputSchema: objectSchema(map[string]any{"path": stringProp(de("Relativer Pfad.", "Relative path.")), "recursive": boolProp(de("Verzeichnisse rekursiv löschen.", "Delete directories recursively."))}, "path")},
		}
	case "powershell":
		return []mcpToolDefinition{
			{Name: "powershell_run", Description: de("Führt ein PowerShell-Skript ohne sichtbares Konsolenfenster im Projektordner aus und liefert Exitcode, STDOUT und STDERR.", "Runs a PowerShell script without a visible console window in the project directory and returns exit code, STDOUT, and STDERR."), InputSchema: objectSchema(map[string]any{"script": stringProp(de("PowerShell-Skript.", "PowerShell script.")), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 3600}}, "script")},
			{Name: "powershell_get_command", Description: de("Ermittelt Cmdlet, Anwendung, Alias oder Funktion mit Get-Command.", "Resolves a cmdlet, application, alias, or function with Get-Command."), InputSchema: objectSchema(map[string]any{"name": stringProp(de("Befehlsname oder Suchmuster.", "Command name or pattern."))}, "name")},
			{Name: "powershell_get_help", Description: de("Ruft PowerShell-Hilfe zu einem Befehl ab.", "Retrieves PowerShell help for a command."), InputSchema: objectSchema(map[string]any{"name": stringProp(de("Befehlsname.", "Command name.")), "online": boolProp(de("Online-Hilfe im Browser öffnen.", "Open online help in the browser."))}, "name")},
		}
	case "git":
		return []mcpToolDefinition{
			{Name: "git_status", Description: de("Zeigt Branch und Arbeitsbaumstatus.", "Shows branch and working-tree status."), InputSchema: objectSchema(map[string]any{})},
			{Name: "git_diff", Description: de("Zeigt Änderungen im Arbeitsbaum oder Index.", "Shows working-tree or index changes."), InputSchema: objectSchema(map[string]any{"staged": boolProp(de("Nur Änderungen im Index anzeigen.", "Show staged changes only.")), "path": stringProp(de("Optionaler Pfadfilter.", "Optional path filter."))})},
			{Name: "git_log", Description: de("Zeigt die Commit-Historie.", "Shows commit history."), InputSchema: objectSchema(map[string]any{"max_count": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}})},
			{Name: "git_init", Description: de("Initialisiert ein Git-Repository und ergänzt eine Visual-Studio-taugliche .gitignore.", "Initializes a Git repository and adds a Visual Studio-ready .gitignore."), InputSchema: objectSchema(map[string]any{})},
			{Name: "git_add", Description: de("Nimmt Dateien in den Index auf.", "Stages files."), InputSchema: objectSchema(map[string]any{"paths": arrayProp(de("Pfade; leer bedeutet alle Änderungen.", "Paths; empty means all changes."))})},
			{Name: "git_commit", Description: de("Erstellt einen Commit mit der angegebenen Nachricht.", "Creates a commit with the supplied message."), InputSchema: objectSchema(map[string]any{"message": stringProp(de("Commit-Nachricht.", "Commit message.")), "stage_all": boolProp(de("Vorher alle Änderungen aufnehmen.", "Stage all changes first."))}, "message")},
			{Name: "git_branch", Description: de("Listet oder erstellt Branches.", "Lists or creates branches."), InputSchema: objectSchema(map[string]any{"name": stringProp(de("Optionaler neuer Branchname.", "Optional new branch name."))})},
			{Name: "git_checkout", Description: de("Wechselt Branch oder stellt Pfade wieder her.", "Checks out a branch or restores paths."), InputSchema: objectSchema(map[string]any{"target": stringProp(de("Branch, Commit oder Pfad.", "Branch, commit, or path.")), "create": boolProp(de("Neuen Branch erstellen.", "Create a new branch."))}, "target")},
			{Name: "git_pull", Description: de("Lädt und integriert Remote-Änderungen.", "Fetches and integrates remote changes."), InputSchema: objectSchema(map[string]any{"remote": stringProp(de("Optionaler Remote-Name.", "Optional remote name.")), "branch": stringProp(de("Optionaler Branch.", "Optional branch."))})},
			{Name: "git_push", Description: de("Überträgt Commits zu einem Remote. Force-Push ist blockiert.", "Pushes commits to a remote. Force push is blocked."), InputSchema: objectSchema(map[string]any{"remote": stringProp(de("Optionaler Remote-Name.", "Optional remote name.")), "branch": stringProp(de("Optionaler Branch.", "Optional branch.")), "set_upstream": boolProp(de("Upstream setzen.", "Set upstream."))})},
			{Name: "git_show", Description: de("Zeigt einen Commit oder ein Objekt.", "Shows a commit or object."), InputSchema: objectSchema(map[string]any{"object": stringProp(de("Git-Objekt; Standard HEAD.", "Git object; defaults to HEAD."))})},
		}
	}
	return nil
}

func mcpCallBuiltin(ctx context.Context, cfg Config, project string, server MCPServerConfig, method string, params any) (string, error) {
	if strings.TrimSpace(project) == "" && server.ProjectScoped && method != "tools/list" && method != "prompts/list" {
		return "", errors.New(localizeConfigText(cfg, "Kein aktives Projekt für diesen MCP-Server.", "No active project for this MCP server."))
	}
	switch method {
	case "tools/list":
		data, _ := json.Marshal(map[string]any{"tools": builtinTools(server.Preset, cfg)})
		return string(data), nil
	case "resources/list":
		resources := []map[string]any{}
		if server.Preset == "filesystem" && project != "" {
			uri, _ := fileURI(project)
			resources = append(resources, map[string]any{"uri": uri, "name": filepath.Base(project), "mimeType": "inode/directory"})
		}
		data, _ := json.Marshal(map[string]any{"resources": resources})
		return string(data), nil
	case "resources/read":
		values := decodeMCPParams(params)
		uri := stringArg(values, "uri")
		if !strings.HasPrefix(uri, "file:") {
			return "", errors.New("only file resources are supported")
		}
		parsedPath, err := pathFromFileURI(uri)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(project, parsedPath)
		if err != nil {
			return "", err
		}
		content, err := readProjectFile(project, relative)
		if err != nil {
			return "", err
		}
		data, _ := json.Marshal(map[string]any{"contents": []map[string]any{{"uri": uri, "mimeType": "text/plain", "text": content}}})
		return string(data), nil
	case "prompts/list":
		data, _ := json.Marshal(map[string]any{"prompts": []any{}})
		return string(data), nil
	case "prompts/get":
		return "", errors.New("this built-in MCP server does not expose prompts")
	case "tools/call":
		values := decodeMCPParams(params)
		name := stringArg(values, "name")
		args := mcpArgumentMap(values)
		text, err := executeBuiltinMCPTool(ctx, cfg, project, server.Preset, name, args)
		if err != nil {
			return mcpErrorResult(err)
		}
		return mcpTextResult(text), nil
	default:
		return "", fmt.Errorf("unsupported built-in MCP method %q", method)
	}
}

func pathFromFileURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return "", errors.New("not a valid file URI")
	}
	value, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(parsed.Host)
	// Some Windows clients emit file://C:/path instead of the canonical
	// file:///C:/path. net/url interprets C: as the authority, which would drop
	// the drive letter and make filepath.Rel fail with different volumes.
	if len(host) == 2 && host[1] == ':' && ((host[0] >= 'A' && host[0] <= 'Z') || (host[0] >= 'a' && host[0] <= 'z')) {
		value = host + value
	} else if host != "" && !strings.EqualFold(host, "localhost") {
		// Preserve UNC authorities (file://server/share/file.txt).
		value = "//" + host + "/" + strings.TrimPrefix(value, "/")
	}
	value = filepath.FromSlash(value)
	if runtime.GOOS == "windows" && strings.HasPrefix(value, string(filepath.Separator)) && len(value) > 3 && value[2] == ':' {
		value = value[1:]
	}
	return filepath.Clean(value), nil
}

func executeBuiltinMCPTool(ctx context.Context, cfg Config, project, preset, name string, args map[string]any) (string, error) {
	switch strings.ToLower(preset) {
	case "filesystem":
		return executeFilesystemMCPTool(cfg, project, name, args)
	case "powershell":
		return executePowerShellMCPTool(ctx, cfg, project, name, args)
	case "git":
		return executeGitMCPTool(ctx, cfg, project, name, args)
	default:
		return "", fmt.Errorf("unknown built-in MCP preset %q", preset)
	}
}

func secureProjectPath(project, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || raw == "." {
		return filepath.Clean(project), nil
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(project, candidate)
	}
	return ensureWithinRoot(project, candidate)
}

func executeFilesystemMCPTool(cfg Config, project, name string, args map[string]any) (string, error) {
	switch name {
	case "list_directory":
		depth := intArg(args, "depth", 3)
		if depth < 1 {
			depth = 1
		}
		if depth > 12 {
			depth = 12
		}
		return projectTree(project, stringArg(args, "path"), depth, 2500)
	case "read_text_file":
		return readProjectFile(project, stringArg(args, "path"))
	case "write_file":
		return writeProjectFile(project, stringArg(args, "path"), stringArg(args, "content"))
	case "create_directory":
		path, err := secureProjectPath(project, stringArg(args, "path"))
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", err
		}
		return "Created directory: " + path, nil
	case "search_files":
		return searchProject(project, stringArg(args, "query"), stringArg(args, "path"), 300)
	case "get_file_info":
		path, err := secureProjectPath(project, stringArg(args, "path"))
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		result := map[string]any{"path": path, "name": info.Name(), "size": info.Size(), "mode": info.Mode().String(), "is_directory": info.IsDir(), "modified": info.ModTime().Format(time.RFC3339)}
		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data), nil
	case "copy_path":
		return copyPath(cfg, project, stringArg(args, "source"), stringArg(args, "destination"))
	case "move_path":
		return movePath(cfg, project, stringArg(args, "source"), stringArg(args, "destination"))
	case "delete_path":
		path, err := secureProjectPath(project, stringArg(args, "path"))
		if err != nil {
			return "", err
		}
		if filepath.Clean(path) == filepath.Clean(project) {
			return "", errors.New("refusing to delete the project root")
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			if !boolArg(args, "recursive", false) {
				return "", errors.New("recursive=true is required to delete a directory")
			}
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			return "", err
		}
		return "Deleted: " + path, nil
	default:
		return "", fmt.Errorf("unknown filesystem tool %q", name)
	}
}

func powershellExecutable(project string, cfg Config) string {
	for _, name := range []string{"powershell", "pwsh"} {
		if info := discoverTool(project, name, cfg, false); info.Available {
			return info.Path
		}
	}
	if runtime.GOOS == "windows" {
		if windir := os.Getenv("WINDIR"); windir != "" {
			path := filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	for _, name := range []string{"pwsh", "powershell", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func executePowerShellMCPTool(ctx context.Context, cfg Config, project, name string, args map[string]any) (string, error) {
	executable := powershellExecutable(project, cfg)
	if executable == "" {
		return "", errors.New("PowerShell was not found. Install PowerShell or enable Windows PowerShell")
	}
	switch name {
	case "powershell_run":
		script := stringArg(args, "script")
		if script == "" {
			return "", errors.New("script is required")
		}
		timeout := intArg(args, "timeout_seconds", cfg.CommandTimeout)
		if timeout <= 0 {
			timeout = 300
		}
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		return runPowerShell(runCtx, executable, project, script, cfg)
	case "powershell_get_command":
		command := stringArg(args, "name")
		if command == "" {
			return "", errors.New("name is required")
		}
		script := fmt.Sprintf("Get-Command -Name %s -ErrorAction Stop | Select-Object Name,CommandType,Source,Version,Path,Definition | Format-List | Out-String -Width 240", quotePowerShellLiteral(command))
		return runPowerShell(ctx, executable, project, script, cfg)
	case "powershell_get_help":
		command := stringArg(args, "name")
		if command == "" {
			return "", errors.New("name is required")
		}
		if boolArg(args, "online", false) {
			script := fmt.Sprintf("Get-Help -Name %s -Online", quotePowerShellLiteral(command))
			return runPowerShell(ctx, executable, project, script, cfg)
		}
		script := fmt.Sprintf("Get-Help -Name %s -Full | Out-String -Width 240", quotePowerShellLiteral(command))
		return runPowerShell(ctx, executable, project, script, cfg)
	default:
		return "", fmt.Errorf("unknown PowerShell tool %q", name)
	}
}

func quotePowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func runPowerShell(ctx context.Context, executable, project, script string, cfg Config) (string, error) {
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "$ErrorActionPreference='Stop'; " + script}
	cmd := exec.CommandContext(ctx, executable, args...)
	hideCommandWindow(cmd)
	cmd.Dir = project
	env := os.Environ()
	for key, value := range cfg.EnvironmentVars {
		env = append(env, key+"="+os.ExpandEnv(value))
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n\nSTDERR:\n"
		}
		result += strings.TrimSpace(stderr.String())
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func executeGitMCPTool(ctx context.Context, cfg Config, project, name string, args map[string]any) (string, error) {
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var gitArgs []string
	switch name {
	case "git_status":
		gitArgs = []string{"status", "--short", "--branch"}
	case "git_diff":
		gitArgs = []string{"diff"}
		if boolArg(args, "staged", false) {
			gitArgs = append(gitArgs, "--cached")
		}
		if path := stringArg(args, "path"); path != "" {
			gitArgs = append(gitArgs, "--", path)
		}
	case "git_log":
		count := intArg(args, "max_count", 30)
		if count < 1 {
			count = 1
		}
		if count > 200 {
			count = 200
		}
		gitArgs = []string{"log", "--decorate", "--stat", "--max-count", strconv.Itoa(count)}
	case "git_init":
		return initializeGitRepository(callCtx, project, cfg)
	case "git_add":
		paths := stringSliceArg(args, "paths")
		if len(paths) == 0 {
			paths = []string{"-A"}
		}
		gitArgs = append([]string{"add"}, paths...)
	case "git_commit":
		message := stringArg(args, "message")
		if message == "" {
			return "", errors.New("commit message is required")
		}
		return commitGitChanges(callCtx, project, cfg, message, boolArg(args, "stage_all", false))
	case "git_branch":
		branch := stringArg(args, "name")
		if branch == "" {
			gitArgs = []string{"branch", "--all", "--verbose"}
		} else {
			gitArgs = []string{"branch", branch}
		}
	case "git_checkout":
		target := stringArg(args, "target")
		if target == "" {
			return "", errors.New("target is required")
		}
		if boolArg(args, "create", false) {
			gitArgs = []string{"switch", "-c", target}
		} else {
			gitArgs = []string{"switch", target}
		}
	case "git_pull":
		gitArgs = []string{"pull", "--ff-only"}
		if remote := stringArg(args, "remote"); remote != "" {
			gitArgs = append(gitArgs, remote)
			if branch := stringArg(args, "branch"); branch != "" {
				gitArgs = append(gitArgs, branch)
			}
		}
	case "git_push":
		gitArgs = []string{"push"}
		if boolArg(args, "set_upstream", false) {
			gitArgs = append(gitArgs, "--set-upstream")
		}
		if remote := stringArg(args, "remote"); remote != "" {
			gitArgs = append(gitArgs, remote)
			if branch := stringArg(args, "branch"); branch != "" {
				gitArgs = append(gitArgs, branch)
			}
		}
	case "git_show":
		object := stringArg(args, "object")
		if object == "" {
			object = "HEAD"
		}
		gitArgs = []string{"show", "--stat", "--decorate", object}
	default:
		return "", fmt.Errorf("unknown Git MCP tool %q", name)
	}
	if err := validateGitArgs(gitArgs); err != nil {
		return "", err
	}
	return runGit(callCtx, project, gitArgs, cfg)
}

func builtinToolNames(preset string, cfg Config) []string {
	tools := builtinTools(preset, cfg)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}
