// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func gitAvailable(project string, cfg Config) bool {
	return discoverTool(project, "git", cfg, false).Available
}

func runGit(ctx context.Context, project string, args []string, cfg Config) (string, error) {
	if len(args) == 0 {
		return "", errors.New("git arguments are empty")
	}
	resolvedArgs := append([]string(nil), args...)
	if gitActionIsReadOnly(args) {
		resolvedArgs = append([]string{"--no-pager"}, args...)
	}
	return runResolvedTool(ctx, project, "git", resolvedArgs, cfg)
}

func gitRead(project string, cfg Config, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return runGit(ctx, project, args, cfg)
}

func gitStatusSummary(project string, cfg Config) string {
	if !gitAvailable(project, cfg) {
		return "Git wurde nach vollständiger Werkzeugerkennung nicht gefunden. LocalCode kann bei Bedarf eine portable Git-Version installieren."
	}
	out, err := gitRead(project, cfg, "status", "--short", "--branch")
	if err != nil {
		return "Kein Git-Repository oder git status fehlgeschlagen:\n" + truncateText(out, 6000) + "\n" + err.Error()
	}
	if strings.TrimSpace(out) == "" {
		return "Arbeitsbaum sauber."
	}
	return strings.TrimSpace(out)
}

func gitBranchName(project string, cfg Config) string {
	out, err := gitRead(project, cfg, "branch", "--show-current")
	if err != nil || strings.TrimSpace(out) == "" {
		return "(kein Git-Branch)"
	}
	return strings.TrimSpace(out)
}

func gitActionIsReadOnly(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "status", "diff", "log", "show", "branch", "rev-parse", "ls-files", "remote", "tag", "grep", "blame":
		if strings.EqualFold(args[0], "branch") {
			for _, a := range args[1:] {
				if a == "-d" || a == "-D" || a == "-m" || a == "-M" || a == "--delete" || a == "--move" {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}

func previewGit(args []string) string { return "$ git " + strings.Join(args, " ") }

func validateGitArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("git arguments are empty")
	}
	for _, a := range args {
		if strings.ContainsRune(a, '\x00') {
			return errors.New("invalid NUL in git argument")
		}
	}
	destructive := strings.ToLower(strings.Join(args, " "))
	for _, forbidden := range []string{"clean -fdx", "reset --hard", "push --force", "push -f", "rebase --onto"} {
		if strings.Contains(destructive, forbidden) {
			return fmt.Errorf("destructive git operation requires manual terminal execution: %s", forbidden)
		}
	}
	return nil
}

func initializeGitRepository(ctx context.Context, project string, cfg Config) (string, error) {
	if isGitRepository(project, cfg) {
		verify, err := runResolvedTool(ctx, project, "git", []string{"rev-parse", "--is-inside-work-tree"}, cfg)
		if err == nil && strings.Contains(strings.ToLower(verify), "true") {
			return "Git-Repository war bereits initialisiert.\n\n" + verify, nil
		}
	}
	info := discoverTool(project, "git", cfg, true)
	if !info.Available {
		return "", &ToolNotFoundError{Info: info, Detail: "Git wurde für git init nicht gefunden."}
	}
	var attempts []string
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"init"}} {
		res := runDirectTool(ctx, project, info.Path, args, cfg)
		attempts = append(attempts, res.Text())
		if res.Err == nil && isGitRepository(project, cfg) {
			break
		}
	}
	if !isGitRepository(project, cfg) {
		return strings.Join(attempts, "\n\n--- RETRY ---\n\n"), errors.New("git init did not create a repository")
	}
	ignoreNote, ignoreErr := ensureProjectGitIgnore(project)
	verify, verifyErr := runResolvedTool(ctx, project, "git", []string{"rev-parse", "--is-inside-work-tree"}, cfg)
	output := strings.Join(attempts, "\n\n--- RETRY ---\n\n") + "\n\nVERIFIKATION:\n" + verify
	if ignoreNote != "" {
		output += "\n\nGITIGNORE:\n" + ignoreNote
	}
	if ignoreErr != nil {
		output += "\nWARNUNG: .gitignore konnte nicht erstellt werden: " + ignoreErr.Error()
	}
	if verifyErr != nil || !strings.Contains(strings.ToLower(verify), "true") {
		if verifyErr == nil {
			verifyErr = errors.New("git rev-parse did not return true")
		}
		return output, verifyErr
	}
	return output, nil
}

func ensureProjectGitIgnore(project string) (string, error) {
	path := filepath.Join(project, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return ".gitignore war bereits vorhanden und wurde nicht überschrieben.", nil
	}
	content := `# IDEs and editors
.vs/
.vscode/
.idea/
*.user
*.suo

# Build output
bin/
obj/
build/
dist/
out/
Debug/
Release/
x64/
x86/

# Logs, caches and temporary files
*.log
*.tmp
*.temp
*.cache
*.bak
*~
.DS_Store
Thumbs.db

# Secrets and machine-local settings
.env
.env.*
!.env.example
*.pfx
*.p12
*.key
*.pem
local.properties

# Language ecosystems
node_modules/
.gradle/
**/.gradle/
**/build/
__pycache__/
*.pyc
target/
coverage/
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return ".gitignore wurde mit Visual-Studio-, Build-, Cache-, Secret- und gängigen Sprachregeln angelegt.", nil
}
