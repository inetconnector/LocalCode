// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGitReadOnlyClassifierRejectsMutatingRefOperations(t *testing.T) {
	tests := [][]string{
		{"branch", "new-feature"},
		{"branch", "-D", "old-feature"},
		{"tag", "v1.2.3"},
		{"tag", "-d", "v1.2.3"},
		{"remote", "add", "origin", "https://example.invalid/repo.git"},
		{"remote", "set-url", "origin", "https://example.invalid/new.git"},
		{"remote", "remove", "origin"},
	}
	for _, args := range tests {
		if gitActionIsReadOnly(args) {
			t.Fatalf("mutating git form classified read-only: git %s", strings.Join(args, " "))
		}
	}
}

func TestGitReadOnlyClassifierAllowsKnownInspectionForms(t *testing.T) {
	tests := [][]string{
		{"status", "--short", "--branch"},
		{"branch", "--show-current"},
		{"branch", "--list"},
		{"tag", "--list"},
		{"remote"},
		{"remote", "-v"},
		{"remote", "get-url", "origin"},
		{"rev-parse", "--is-inside-work-tree"},
		{"ls-files"},
		{"log", "--oneline", "-5"},
		{"diff", "--stat"},
		{"show", "HEAD"},
	}
	for _, args := range tests {
		if !gitActionIsReadOnly(args) {
			t.Fatalf("known inspection form classified mutating: git %s", strings.Join(args, " "))
		}
	}
}

func TestGitReadOnlyClassifierRejectsExternalOrSandboxBypassForms(t *testing.T) {
	tests := [][]string{
		{"diff", "--no-index", "C:/outside/a.txt", "C:/outside/b.txt"},
		{"diff", "--ext-diff"},
		{"show", "--textconv", "HEAD:file.txt"},
		{"log", "--ext-diff"},
		{"grep", "needle"},
		{"blame", "--contents", "C:/outside/file.txt", "tracked.txt"},
	}
	for _, args := range tests {
		if gitActionIsReadOnly(args) {
			t.Fatalf("unsafe git form classified read-only: git %s", strings.Join(args, " "))
		}
	}
}

func TestValidateGitArgsBlocksNoIndexDiff(t *testing.T) {
	if err := validateGitArgs([]string{"diff", "--no-index", "a", "b"}); err == nil {
		t.Fatal("git diff --no-index must be rejected by safe git execution")
	}
}

func TestDeriveCommitMessageKeepsUTF8Valid(t *testing.T) {
	input := strings.Repeat("Änderung-🚀-", 20)
	message := deriveCommitMessage(input)
	if !utf8.ValidString(message) {
		t.Fatalf("derived commit message is invalid UTF-8: %q", message)
	}
	parts := strings.SplitN(message, ": ", 2)
	if len(parts) != 2 {
		t.Fatalf("missing conventional-commit prefix: %q", message)
	}
	if got := len([]rune(parts[1])); got > 72 {
		t.Fatalf("commit subject has %d runes, want <= 72", got)
	}
}
