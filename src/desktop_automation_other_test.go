// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

import (
	"context"
	"strings"
	"testing"
)

func TestDesktopAutomationOtherSecurityGuardsAndParsers(t *testing.T) {
	// 1. Blocked security/credential windows must fail closed on macOS & Linux
	for _, sensitive := range []string{"Keychain Access", "1Password Vault", "Bitwarden", "polkit-agent", "Task Manager"} {
		if !isBlockedDesktopWindow(sensitive, "") {
			t.Fatalf("expected %q to be blocked", sensitive)
		}
	}
	if isBlockedDesktopWindow("NomaMica Studio", "chrome") {
		t.Fatalf("regular window should not be blocked")
	}

	// 2. DesktopScreenshot must reject path traversal outside project root fail-closed
	root := t.TempDir()
	_, err := DesktopScreenshot(context.Background(), defaultConfig(), root, "", "../../outside.png")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "outside") {
		t.Fatalf("expected path containment error, got %v", err)
	}

	// 3. Parsers handle macOS tab output and Linux wmctrl output deterministically
	macWins := parseTabSeparatedWindows("1234\tTerminal\tzsh — NomaMica\n")
	if len(macWins) != 1 || macWins[0].PID != 1234 || macWins[0].ProcessName != "Terminal" {
		t.Fatalf("unexpected macOS parsed windows: %+v", macWins)
	}
	linWins := parseWmctrlWindows("0x04000003  0 5678  workstation NomaMica Studio\n")
	if len(linWins) != 1 || linWins[0].PID != 5678 || linWins[0].Title != "NomaMica Studio" {
		t.Fatalf("unexpected Linux wmctrl parsed windows: %+v", linWins)
	}

	// 4. DesktopType safety validation & AppleScript escaping
	if err := validateDesktopTypePayload(""); err == nil {
		t.Fatalf("empty text should fail")
	}
	if err := validateDesktopTypePayload("abc\x00def"); err == nil {
		t.Fatalf("NUL byte should fail")
	}
	if err := validateDesktopTypePayload(strings.Repeat("a", maxDesktopTypeChars+1)); err == nil {
		t.Fatalf("oversized text should fail")
	}
	if err := validateDesktopTypePayload("safe text"); err != nil {
		t.Fatalf("valid text failed: %v", err)
	}
	script := buildMacTypeScript(`Studio "Main"`, `hello "world"`)
	if !strings.Contains(script, `\"Main\"`) || !strings.Contains(script, `\"world\"`) {
		t.Fatalf("AppleScript escaping failed: %s", script)
	}
}

func TestTrayManagerOtherGracefulStop(t *testing.T) {
	tm := NewTrayManager("http://127.0.0.1:32145", "de", true)
	done := make(chan error, 1)
	go func() {
		done <- tm.Run()
	}()
	tm.Stop()
	tm.Stop() // idempotent double stop must not panic
	if err := <-done; err != nil {
		t.Fatalf("TrayManager.Run returned unexpected error: %v", err)
	}
}
