// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func gitAvailable() bool { _, err := exec.LookPath("git"); return err == nil }

func runGit(ctx context.Context, project string, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("git arguments are empty")
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", project, "--no-pager"}, args...)...)
	hideCommandWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return truncateText(buf.String(), 160000), err
}

func gitRead(project string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return runGit(ctx, project, args)
}

func gitStatusSummary(project string) string {
	if !gitAvailable() {
		return "Git ist nicht installiert."
	}
	out, err := gitRead(project, "status", "--short", "--branch")
	if err != nil {
		return "Kein Git-Repository oder git status fehlgeschlagen: " + err.Error()
	}
	if strings.TrimSpace(out) == "" {
		return "Arbeitsbaum sauber."
	}
	return strings.TrimSpace(out)
}

func gitBranchName(project string) string {
	out, err := gitRead(project, "branch", "--show-current")
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
