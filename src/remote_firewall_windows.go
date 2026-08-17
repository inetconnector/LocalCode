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

var runRemoteFirewallPowerShell = func(script string) error {
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	return cmd.Run()
}

func remoteFirewallRuleName(port int) string {
	return fmt.Sprintf("LocalCode Remote %d", port)
}

func powershellSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func ensureRemoteFirewallRule(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	name := remoteFirewallRuleName(port)
	portText := strconv.Itoa(port)
	// Restrict the automatic exception to the current executable, TCP port,
	// Windows Private profiles and the local subnet. The elevation prompt is
	// reached only when LAN Remote was explicitly enabled by the user.
	inner := "$existing=Get-NetFirewallRule -DisplayName " + powershellSingleQuoted(name) + " -ErrorAction SilentlyContinue; " +
		"if(-not $existing){New-NetFirewallRule -DisplayName " + powershellSingleQuoted(name) +
		" -Direction Inbound -Action Allow -Protocol TCP -LocalPort " + portText +
		" -Profile Private -RemoteAddress LocalSubnet -Program " + powershellSingleQuoted(exe) + " | Out-Null}"
	script := "$p=Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -PassThru -ArgumentList @('-NoLogo','-NoProfile','-ExecutionPolicy','Bypass','-Command'," + powershellSingleQuoted(inner) + "); exit $p.ExitCode"
	if err := runRemoteFirewallPowerShell(script); err != nil {
		return fmt.Errorf("firewall elevation was declined or failed: %w", err)
	}
	return nil
}
