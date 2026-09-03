// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var runRemoteFirewallPowerShell = func(script string) error {
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
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
	script := "$rule=Get-NetFirewallRule -DisplayName " + powershellSingleQuoted(name) + " -ErrorAction SilentlyContinue | Where-Object {$_.Enabled -eq 'True' -and $_.Direction -eq 'Inbound' -and $_.Action -eq 'Allow'} | Select-Object -First 1; " +
		"if($rule){" +
		"$port=$rule | Get-NetFirewallPortFilter | Where-Object {$_.Protocol -eq 'TCP' -and $_.LocalPort -eq " + powershellSingleQuoted(portText) + "} | Select-Object -First 1; " +
		"$app=$rule | Get-NetFirewallApplicationFilter | Where-Object {$_.Program -eq " + powershellSingleQuoted(exe) + "} | Select-Object -First 1; " +
		"if($port -and $app){exit 0}" +
		"}; " +
		"Write-Output " + powershellSingleQuoted("LocalCode Remote firewall rule missing; continuing without elevation.") + "; exit 2"
	if err := runRemoteFirewallPowerShell(script); err != nil {
		log.Printf("LocalCode Remote firewall rule %q is not installed for TCP %d; skipping automatic elevation: %v", name, port, err)
		return nil
	}
	return nil
}
