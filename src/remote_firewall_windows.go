// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func remoteFirewallRuleName(port int) string {
	return fmt.Sprintf("LocalCode Remote %d", port)
}

func firewallRulePresent(name string) bool {
	cmd := exec.Command("netsh.exe", "advfirewall", "firewall", "show", "rule", "name="+name)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(out))
	return !strings.Contains(text, "no rules match") && !strings.Contains(text, "keine regeln")
}

func addRemoteFirewallRule(args []string) error {
	cmd := exec.Command("netsh.exe", args...)
	hideCommandWindow(cmd)
	if out, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else if len(out) > 0 && !strings.Contains(strings.ToLower(string(out)), "requires elevation") && !strings.Contains(strings.ToLower(string(out)), "erhöhte rechte") {
		// Keep going to the explicit elevation path. Windows localizations vary,
		// and an access-denied result is expected for non-admin processes.
	}

	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
	}
	argumentList := strings.Join(quoted, ",")
	ps := "$p=Start-Process -FilePath 'netsh.exe' -Verb RunAs -Wait -PassThru -ArgumentList @(" + argumentList + "); exit $p.ExitCode"
	elevated := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err := elevated.Run(); err != nil {
		return fmt.Errorf("firewall elevation was declined or failed: %w", err)
	}
	return nil
}

func ensureRemoteFirewallRule(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	name := remoteFirewallRuleName(port)
	if firewallRulePresent(name) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + name,
		"dir=in",
		"action=allow",
		"protocol=TCP",
		"localport=" + strconv.Itoa(port),
		"profile=private",
		"remoteip=LocalSubnet",
		"program=" + exe,
		"enable=yes",
	}
	if err := addRemoteFirewallRule(args); err != nil {
		return err
	}
	if !firewallRulePresent(name) {
		return fmt.Errorf("firewall rule %q was not created", name)
	}
	return nil
}
