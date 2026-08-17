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
	"unicode"
)

func gitAvailable(project string, cfg Config) bool {
	return discoverTool(project, "git", cfg, false).Available
}

func runGit(ctx context.Context, project string, args []string, cfg Config) (string, error) {
	if len(args) == 0 {
		return "", errors.New("git arguments are empty")
	}
	if err := validateGitArgs(args); err != nil {
		return "", err
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

// gitActionIsReadOnly is deliberately conservative because its result is used
// by the approval policy. Unknown forms are treated as mutating. In
// particular branch/tag/remote are not read-only merely because their
// subcommand name often appears in inspection commands.
func gitActionIsReadOnly(args []string) bool {
	if len(args) == 0 {
		return false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	switch cmd {
	case "status", "rev-parse", "ls-files":
		return true
	case "log", "diff", "show":
		return gitDiffLikeArgsReadOnly(rest)
	case "branch":
		return gitBranchArgsReadOnly(rest)
	case "tag":
		return gitTagArgsReadOnly(rest)
	case "remote":
		return gitRemoteArgsReadOnly(rest)
	// grep may invoke textconv or an external pager; blame can read a caller-
	// supplied --contents file. Keep both approval-gated rather than trying to
	// maintain a fragile negative flag list.
	case "grep", "blame":
		return false
	default:
		return false
	}
}

func gitDiffLikeArgsReadOnly(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(strings.TrimSpace(arg))
		if lower == "--no-index" || lower == "--ext-diff" || lower == "--textconv" ||
			strings.HasPrefix(lower, "--output=") || lower == "--output" {
			return false
		}
	}
	return true
}

func gitBranchArgsReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	valueFlags := map[string]bool{
		"--contains": true, "--no-contains": true, "--merged": true,
		"--no-merged": true, "--points-at": true, "--sort": true,
		"--format": true,
	}
	flagOnly := map[string]bool{
		"--show-current": true, "--list": true, "--all": true, "-a": true,
		"--remotes": true, "-r": true, "--verbose": true, "-v": true,
		"-vv": true, "--no-color": true, "--ignore-case": true,
		"--column": true, "--no-column": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "--sort=") || strings.HasPrefix(lower, "--format=") ||
			strings.HasPrefix(lower, "--color=") || strings.HasPrefix(lower, "--column=") ||
			strings.HasPrefix(lower, "--contains=") || strings.HasPrefix(lower, "--no-contains=") ||
			strings.HasPrefix(lower, "--merged=") || strings.HasPrefix(lower, "--no-merged=") ||
			strings.HasPrefix(lower, "--points-at=") {
			continue
		}
		if flagOnly[lower] {
			continue
		}
		if valueFlags[lower] {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		// A positional branch name, or an unknown flag, can create/delete/move
		// refs and therefore requires approval.
		return false
	}
	return true
}

func gitTagArgsReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	listing := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		lower := strings.ToLower(arg)
		switch {
		case lower == "--list" || lower == "-l":
			listing = true
		case lower == "-n" || strings.HasPrefix(lower, "-n"):
			listing = true
		case lower == "--contains" || lower == "--no-contains" || lower == "--points-at" || lower == "--sort" || lower == "--format":
			listing = true
			if i+1 >= len(args) {
				return false
			}
			i++
		case strings.HasPrefix(lower, "--contains=") || strings.HasPrefix(lower, "--no-contains=") ||
			strings.HasPrefix(lower, "--points-at=") || strings.HasPrefix(lower, "--sort=") ||
			strings.HasPrefix(lower, "--format=") || lower == "--merged" || lower == "--no-merged" ||
			strings.HasPrefix(lower, "--merged=") || strings.HasPrefix(lower, "--no-merged=") || lower == "--ignore-case":
			listing = true
		case strings.HasPrefix(arg, "-"):
			return false
		default:
			// Positional patterns are safe only after an explicit listing option;
			// otherwise `git tag NAME` creates a tag.
			if !listing {
				return false
			}
		}
	}
	return listing
}

func gitRemoteArgsReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) == 1 && (args[0] == "-v" || args[0] == "--verbose") {
		return true
	}
	if strings.EqualFold(args[0], "get-url") {
		// get-url accepts --all/--push and a remote name; none mutate config.
		for _, arg := range args[1:] {
			if strings.EqualFold(arg, "add") || strings.EqualFold(arg, "set-url") || strings.EqualFold(arg, "remove") || strings.EqualFold(arg, "rename") {
				return false
			}
		}
		return len(args) >= 2
	}
	return false
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
	for _, forbidden := range []string{"clean -fdx", "reset --hard", "push --force", "push -f", "rebase --onto", "diff --no-index"} {
		if strings.Contains(destructive, forbidden) {
			return fmt.Errorf("destructive or sandbox-bypassing git operation requires manual terminal execution: %s", forbidden)
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
	runes := []rune(message)
	if len(runes) > 72 {
		runes = runes[:72]
		message = strings.TrimSpace(string(runes))
	}
	message = strings.TrimRight(message, ".;:, ")
	if message == "" {
		message = "update project files"
	}
	runes = []rune(message)
	runes[0] = unicode.ToLower(runes[0])
	return prefix + ": " + string(runes)
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

	if stageAll {
		stageArgs := []string{"add", "-A", "--", ".", ":(exclude).vs/**"}
		stageOut, stageErr := runGit(ctx, project, stageArgs, cfg)
		report.WriteString("STAGE:\n" + stageOut + "\n\n")
		if stageErr != nil {
			return report.String(), fmt.Errorf("git add failed: %w", stageErr)
		}
	} else {
		report.WriteString("STAGE:\nstage_all=false; existing index preserved.\n\n")
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
