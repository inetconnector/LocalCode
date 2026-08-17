// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestToolReadOnlyClassifierRejectsExecutableProjectWork(t *testing.T) {
	unsafe := []struct {
		tool string
		args []string
	}{
		{"go", []string{"test", "./..."}},
		{"go", []string{"vet", "./..."}},
		{"go", []string{"list", "./..."}},
		{"npm", []string{"test"}},
		{"npm", []string{"run", "lint"}},
		{"npx", []string{"--yes", "some-package"}},
		{"python", []string{"-c", "print('x')"}},
		{"dotnet", []string{"test"}},
		{"cargo", []string{"test"}},
	}
	for _, tc := range unsafe {
		if toolActionLooksReadOnly(tc.tool, tc.args) {
			t.Errorf("executable project work classified read-only: %s %v", tc.tool, tc.args)
		}
	}
}

func TestToolReadOnlyClassifierAllowsExactInformationQueries(t *testing.T) {
	safe := []struct {
		tool string
		args []string
	}{
		{"go", []string{"version"}},
		{"node", []string{"--version"}},
		{"npm", []string{"--version"}},
		{"python", []string{"--version"}},
		{"dotnet", []string{"--info"}},
		{"cargo", []string{"--version"}},
		{"adb", []string{"devices", "-l"}},
		{"git", []string{"status", "--short"}},
	}
	for _, tc := range safe {
		if !toolActionLooksReadOnly(tc.tool, tc.args) {
			t.Errorf("exact information query requires unnecessary approval: %s %v", tc.tool, tc.args)
		}
	}
}

func TestToolReadOnlyClassifierRejectsGitMutationAndDiffEscape(t *testing.T) {
	for _, args := range [][]string{
		{"branch", "new-branch"},
		{"tag", "v1"},
		{"remote", "add", "other", "https://example.invalid/repo.git"},
		{"diff", "--no-index", "C:/outside-a", "C:/outside-b"},
	} {
		if toolActionLooksReadOnly("git", args) {
			t.Errorf("unsafe git run_tool classified read-only: %v", args)
		}
	}
}

func TestToolReadOnlyClassifierRejectsExtraArgsOnVersionQueries(t *testing.T) {
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{"node", []string{"--version", "payload"}},
		{"npm", []string{"--version", "payload"}},
		{"dotnet", []string{"--info", "payload"}},
		{"adb", []string{"devices", "-l", "payload"}},
	} {
		if toolActionLooksReadOnly(tc.tool, tc.args) {
			t.Errorf("version/info query with extra args classified read-only: %s %v", tc.tool, tc.args)
		}
	}
}
