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
	required := []string{
		"# Visual Studio and IDE state",
		".vs/", "*.suo", "*.user", "*.userosscache", "*.sln.docstates", ".idea/", ".vscode/",
		"",
		"# Build output",
		"bin/", "obj/", "build/", "dist/", "out/", "Debug/", "Release/", "x64/", "x86/",
		"",
		"# Logs, caches and temporary files",
		"*.log", "*.tmp", "*.temp", "*.cache", "*.bak", "*~", ".DS_Store", "Thumbs.db",
		"",
		"# Secrets and machine-local settings",
		".env", ".env.*", "!.env.example", "*.pfx", "*.p12", "*.key", "*.pem", "local.properties",
		"",
		"# Language ecosystems",
		"node_modules/", ".gradle/", "**/.gradle/", "**/build/", "__pycache__/", "*.pyc", "target/", "coverage/",
	}
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = strings.ReplaceAll(string(data), "\r\n", "\n")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	present := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var additions []string
	for _, line := range required {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(additions) > 0 && additions[len(additions)-1] != "" {
				additions = append(additions, "")
			}
			continue
		}
		if !present[trimmed] {
			additions = append(additions, line)
			present[trimmed] = true
		}
	}
	if len(additions) == 0 {
		return ".gitignore enthält bereits die erforderlichen Visual-Studio-, Build-, Cache- und Secret-Regeln.", nil
	}
	content := strings.TrimRight(existing, "\n")
	if content != "" {
		content += "\n\n"
	}
	content += "# Added and maintained by LocalCode\n" + strings.TrimSpace(strings.Join(additions, "\n")) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf(".gitignore wurde ergänzt; %d fehlende Regeln wurden hinzugefügt.", len(additions)), nil
}

func deriveCommitMessage(task string) string {
	message := strings.TrimSpace(task)
	if message == "" {
		return "chore: update project files"
	}
	message = strings.Join(strings.Fields(message), " ")
	lower := strings.ToLower(message)
	prefix := "chore"
	switch {
	case strings.Contains(lower, "fix") || strings.Contains(lower, "fehler") || strings.Contains(lower, "bug"):
		prefix = "fix"
	case strings.Contains(lower, "add") || strings.Contains(lower, "erstelle") || strings.Contains(lower, "implement") || strings.Contains(lower, "feature"):
		prefix = "feat"
	case strings.Contains(lower, "readme") || strings.Contains(lower, "dokument"):
		prefix = "docs"
	case strings.Contains(lower, "test"):
		prefix = "test"
	case strings.Contains(lower, "refactor") || strings.Contains(lower, "umstruktur"):
		prefix = "refactor"
	}
	if len(message) > 72 {
		message = strings.TrimSpace(message[:72])
	}
	message = strings.TrimRight(message, ".;:, ")
	if message == "" {
		message = "update project files"
	}
	return prefix + ": " + strings.ToLower(message[:1]) + message[1:]
}

func commitMessageFromProject(project, fallback string) (message string, useFile bool) {
	path := filepath.Join(project, "COMMIT_MESSAGE.txt")
	if data, err := os.ReadFile(path); err == nil {
		if value := strings.TrimSpace(string(data)); value != "" {
			return value, true
		}
	}
	return deriveCommitMessage(fallback), false
}

func commitGitChanges(ctx context.Context, project string, cfg Config, requestedMessage string, stageAll bool) (string, error) {
	var report strings.Builder
	if !isGitRepository(project, cfg) {
		out, err := initializeGitRepository(ctx, project, cfg)
		report.WriteString("GIT INIT:\n" + out + "\n\n")
		if err != nil {
			return report.String(), err
		}
	}
	ignoreNote, err := ensureProjectGitIgnore(project)
	if err != nil {
		return report.String(), fmt.Errorf("prepare .gitignore: %w", err)
	}
	report.WriteString("GITIGNORE:\n" + ignoreNote + "\n\n")

	// Never keep Visual Studio's private, frequently locked .vs index in Git.
	if out, rmErr := runGit(ctx, project, []string{"rm", "-r", "--cached", "--ignore-unmatch", "--", ".vs"}, cfg); strings.TrimSpace(out) != "" {
		report.WriteString("UNTRACK .VS:\n" + out + "\n\n")
		_ = rmErr // --ignore-unmatch is best effort; the real verification is below.
	}

	if !stageAll {
		stageAll = true
	}
	stageArgs := []string{"add", "-A", "--", ".", ":(exclude).vs/**"}
	stageOut, stageErr := runGit(ctx, project, stageArgs, cfg)
	report.WriteString("STAGE:\n" + stageOut + "\n\n")
	if stageErr != nil {
		return report.String(), fmt.Errorf("git add failed: %w", stageErr)
	}

	check := runDirectTool(ctx, project, discoverTool(project, "git", cfg, false).Path, []string{"diff", "--cached", "--quiet"}, cfg)
	if check.ExitCode == 0 {
		status, _ := gitRead(project, cfg, "status", "--short", "--branch")
		report.WriteString("VERIFY:\nNo staged changes.\n" + status)
		return report.String(), nil
	}
	if check.ExitCode != 1 {
		report.WriteString("STAGED CHECK:\n" + check.Text())
		return report.String(), errors.New("could not verify staged changes")
	}

	message := strings.TrimSpace(requestedMessage)
	useFile := false
	if message == "" {
		message, useFile = commitMessageFromProject(project, requestedMessage)
	}
	var commitArgs []string
	if useFile {
		commitArgs = []string{"commit", "-F", "COMMIT_MESSAGE.txt"}
	} else {
		commitArgs = []string{"commit", "-m", message}
	}
	commitOut, commitErr := runGit(ctx, project, commitArgs, cfg)
	report.WriteString("COMMIT:\n" + commitOut + "\n\n")
	if commitErr != nil {
		lower := strings.ToLower(commitOut + "\n" + commitErr.Error())
		if strings.Contains(lower, "author identity unknown") || strings.Contains(lower, "please tell me who you are") {
			return report.String(), errors.New("Git user.name and user.email are not configured. Configure them locally or globally before committing")
		}
		return report.String(), commitErr
	}

	head, headErr := gitRead(project, cfg, "rev-parse", "--verify", "HEAD")
	status, statusErr := gitRead(project, cfg, "status", "--short", "--branch")
	report.WriteString("VERIFICATION:\nHEAD: " + strings.TrimSpace(head) + "\n" + status)
	if headErr != nil {
		return report.String(), headErr
	}
	if statusErr != nil {
		return report.String(), statusErr
	}
	return report.String(), nil
}
