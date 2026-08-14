// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const maxProjectCommandBytes = 128 * 1024

var projectCommandNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type ProjectCommand struct {
	Name        string
	Description string
	Source      string
	Scope       string
	Body        string
}

func listProjectCommands(project string) []ProjectCommand {
	roots := projectCommandRoots(project)
	seen := map[string]bool{}
	var out []ProjectCommand
	for _, root := range roots {
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name, ok := projectCommandName(entry.Name())
			if !ok {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			cmd, err := loadProjectCommandFromFile(root, filepath.Join(root.Path, entry.Name()), name)
			if err != nil {
				continue
			}
			seen[key] = true
			out = append(out, cmd)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope == "project"
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func readProjectCommand(project, name string) (ProjectCommand, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if !projectCommandNamePattern.MatchString(name) {
		return ProjectCommand{}, fmt.Errorf("invalid command name: %s", name)
	}
	for _, cmd := range listProjectCommands(project) {
		if strings.EqualFold(cmd.Name, name) {
			return cmd, nil
		}
	}
	return ProjectCommand{}, fmt.Errorf("project command not found: /%s", name)
}

func formatProjectCommandList(project string) string {
	commands := listProjectCommands(project)
	if len(commands) == 0 {
		return "Keine Projekt-Commands gefunden.\nNo project commands found."
	}
	var b strings.Builder
	b.WriteString("PROJEKT-COMMANDS / PROJECT COMMANDS\n")
	for _, cmd := range commands {
		fmt.Fprintf(&b, "/%s [%s]", cmd.Name, cmd.Scope)
		if cmd.Description != "" {
			b.WriteString(" - ")
			b.WriteString(cmd.Description)
		}
		b.WriteByte('\n')
		b.WriteString("  ")
		b.WriteString(cmd.Source)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatProjectCommand(cmd ProjectCommand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COMMAND /%s\nScope: %s\nSource: %s\n", cmd.Name, cmd.Scope, cmd.Source)
	if cmd.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", cmd.Description)
	}
	b.WriteString("\nBODY:\n")
	b.WriteString(cmd.Body)
	if !strings.HasSuffix(cmd.Body, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

func expandSlashCommandPrompt(project, message string) (string, *ProjectCommand, bool, error) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return message, nil, false, nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return message, nil, false, nil
	}
	name := strings.TrimPrefix(fields[0], "/")
	if !projectCommandNamePattern.MatchString(name) {
		return message, nil, false, nil
	}
	args := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
	cmd, err := readProjectCommand(project, name)
	if err != nil {
		return message, nil, false, err
	}
	body := expandProjectCommandBody(cmd.Body, project, args)
	var b strings.Builder
	fmt.Fprintf(&b, "SLASH COMMAND /%s\n", cmd.Name)
	fmt.Fprintf(&b, "Source: %s\n", cmd.Source)
	if cmd.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", cmd.Description)
	}
	fmt.Fprintf(&b, "Arguments: %s\n\n", args)
	b.WriteString(body)
	return b.String(), &cmd, true, nil
}

func expandProjectCommandBody(body, project, args string) string {
	replacements := map[string]string{
		"{{args}}":    args,
		"{{project}}": filepath.Base(project),
		"{{cwd}}":     filepath.Clean(project),
	}
	for key, value := range replacements {
		body = strings.ReplaceAll(body, key, value)
	}
	return strings.TrimSpace(body)
}

type projectCommandRoot struct {
	Path  string
	Scope string
}

func projectCommandRoots(project string) []projectCommandRoot {
	var roots []projectCommandRoot
	if strings.TrimSpace(project) != "" {
		for _, rel := range []string{
			filepath.Join(".localcode", "commands"),
			filepath.Join(".codex", "commands"),
			filepath.Join(".opencode", "commands"),
			filepath.Join(".cursor", "commands"),
		} {
			if full, err := ensureWithinRoot(project, rel); err == nil {
				roots = append(roots, projectCommandRoot{Path: full, Scope: "project"})
			}
		}
	}
	roots = append(roots, projectCommandRoot{Path: filepath.Join(appDataDir(), "commands"), Scope: "global"})
	return roots
}

func loadProjectCommandFromFile(root projectCommandRoot, path, name string) (ProjectCommand, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ProjectCommand{}, err
	}
	if info.IsDir() {
		return ProjectCommand{}, errors.New("command path is a directory")
	}
	if info.Size() <= 0 || info.Size() > maxProjectCommandBytes {
		return ProjectCommand{}, fmt.Errorf("command file size is unsupported: %d bytes", info.Size())
	}
	if err := ensureCommandPathInsideRoot(root.Path, path); err != nil {
		return ProjectCommand{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectCommand{}, err
	}
	if !isProbablyText(data) {
		return ProjectCommand{}, errors.New("command file is not UTF-8 text")
	}
	content := string(data)
	body := strings.TrimSpace(stripFrontmatter(content))
	if body == "" {
		return ProjectCommand{}, errors.New("command body is empty")
	}
	description := frontmatterValue(content, "description")
	if description == "" {
		description = firstMarkdownParagraph(body)
	}
	return ProjectCommand{
		Name:        name,
		Description: truncateText(strings.Join(strings.Fields(description), " "), 240),
		Source:      displayCommandPath(root.Path, path),
		Scope:       root.Scope,
		Body:        body,
	}, nil
}

func projectCommandName(filename string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".md" && ext != ".txt" {
		return "", false
	}
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	if !projectCommandNamePattern.MatchString(name) {
		return "", false
	}
	return name, true
}

func stripFrontmatter(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	trimmed := strings.TrimSpace(normalized)
	if !strings.HasPrefix(trimmed, "---") {
		return content
	}
	rest := strings.TrimPrefix(trimmed, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return content
	}
	return strings.TrimSpace(rest[end+len("\n---"):])
}

func ensureCommandPathInsideRoot(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rootResolved := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootResolved = resolved
	}
	pathResolved := pathAbs
	if resolved, err := filepath.EvalSymlinks(pathAbs); err == nil {
		pathResolved = resolved
	}
	if !pathWithin(rootResolved, pathResolved) {
		return fmt.Errorf("command path escapes command root: %s", path)
	}
	return nil
}

func displayCommandPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(root)), filepath.Base(root), rel))
	}
	return filepath.Clean(path)
}

func projectCommandsContext(project string) string {
	commands := listProjectCommands(project)
	if len(commands) == 0 {
		return "Keine Projekt-Commands gefunden."
	}
	var b strings.Builder
	b.WriteString("Verfügbare Slash-/Projekt-Commands:\n")
	for _, cmd := range commands {
		fmt.Fprintf(&b, "- /%s [%s]", cmd.Name, cmd.Scope)
		if cmd.Description != "" {
			b.WriteString(": ")
			b.WriteString(cmd.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
