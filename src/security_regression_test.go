// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation unavailable without Windows developer mode/privilege: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := ensureWithinRoot(root, filepath.Join("escape", "secret.txt")); err == nil {
		t.Fatal("symlink/junction escape must be rejected")
	}
	if _, err := readProjectFile(root, filepath.Join("escape", "secret.txt")); err == nil {
		t.Fatal("readProjectFile must not follow a project symlink outside the root")
	}
}

func TestShellReadOnlyClassifierRejectsCompoundAndRedirectedCommands(t *testing.T) {
	for _, command := range []string{
		`echo hello > changed.txt`,
		`git status && echo changed > changed.txt`,
		`git status; echo changed > changed.txt`,
		`go test ./... && echo changed > changed.txt`,
		`npm test && echo changed > changed.txt`,
	} {
		if commandLooksReadOnly(command) {
			t.Errorf("compound or redirected shell command must require approval: %q", command)
		}
	}
}

func TestGitReadOnlyClassifierRejectsMutatingBranchTagAndRemote(t *testing.T) {
	mutating := [][]string{
		{"branch", "new-branch"},
		{"tag", "v1.2.3"},
		{"tag", "-d", "v1.2.3"},
		{"remote", "add", "origin2", "https://example.invalid/repo.git"},
		{"remote", "set-url", "origin", "https://example.invalid/repo.git"},
	}
	for _, args := range mutating {
		if gitActionIsReadOnly(args) {
			t.Errorf("mutating git command classified as read-only: git %v", args)
		}
	}

	readonly := [][]string{
		{"status", "--short"},
		{"log", "--oneline", "-5"},
		{"branch", "--show-current"},
		{"branch", "--all", "--verbose"},
		{"tag", "--list"},
		{"remote", "-v"},
	}
	for _, args := range readonly {
		if !gitActionIsReadOnly(args) {
			t.Errorf("read-only git command classified as mutating: git %v", args)
		}
	}
}

func TestDefaultRemoteIsOptIn(t *testing.T) {
	cfg := defaultConfig()
	if cfg.RemoteEnabled {
		t.Fatal("LAN remote must be opt-in by default")
	}
	if cfg.RemoteBindHost != "127.0.0.1" {
		t.Fatalf("default remote bind host = %q, want loopback", cfg.RemoteBindHost)
	}
}

func TestDefaultExternalMCPServersAreOptInAndPinned(t *testing.T) {
	for _, server := range defaultMCPServers() {
		if server.Transport == "stdio" && server.Enabled {
			t.Errorf("external stdio MCP server %q must be opt-in", server.Name)
		}
		for _, arg := range server.Args {
			if arg == "@latest" || containsLatestPackageSelector(arg) {
				t.Errorf("MCP server %q uses unpinned package selector %q", server.Name, arg)
			}
		}
	}
}

func containsLatestPackageSelector(value string) bool {
	for i := 0; i+7 <= len(value); i++ {
		if value[i:i+7] == "@latest" {
			return true
		}
	}
	return false
}
