// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRemoteFirewallRuleValidationAndScope(t *testing.T) {
	if ensureRemoteFirewallRule(0) == nil || ensureRemoteFirewallRule(70000) == nil {
		t.Fatal("invalid firewall ports must be rejected")
	}
	original := runRemoteFirewallPowerShell
	t.Cleanup(func() { runRemoteFirewallPowerShell = original })
	var script string
	runRemoteFirewallPowerShell = func(value string) error {
		script = value
		return nil
	}
	if err := ensureRemoteFirewallRule(32146); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"LocalCode Remote 32146", "-Profile Private", "-RemoteAddress LocalSubnet", "-LocalPort 32146", "-Verb RunAs", "-WindowStyle Hidden"} {
		if !strings.Contains(script, marker) {
			t.Fatalf("firewall script missing %q: %s", marker, script)
		}
	}
	if remoteFirewallRuleName(32146) != "LocalCode Remote 32146" {
		t.Fatal("unexpected firewall rule name")
	}
	if powershellSingleQuoted("a'b") != "'a''b'" {
		t.Fatal("PowerShell quoting is unsafe")
	}
}

func TestRemoteFirewallElevationFailureIsReturned(t *testing.T) {
	original := runRemoteFirewallPowerShell
	t.Cleanup(func() { runRemoteFirewallPowerShell = original })
	runRemoteFirewallPowerShell = func(string) error { return errors.New("declined") }
	if err := ensureRemoteFirewallRule(32146); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("expected elevation failure, got %v", err)
	}
}

func TestRemoteFirewallRunnerHidesOuterPowerShell(t *testing.T) {
	source, err := os.ReadFile("remote_firewall_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, marker := range []string{"exec.Command(\"powershell.exe\"", "hideCommandWindow(cmd)", "cmd.Run()"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("firewall runner no longer hides the outer PowerShell process; missing %q", marker)
		}
	}
}
