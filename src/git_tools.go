// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
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
	resolvedArgs := append([]string{"--no-pager"}, args...)
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
