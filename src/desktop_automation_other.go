// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type DesktopWindowInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	PID         int    `json:"pid"`
	Handle      int64  `json:"handle"`
	IsActive    bool   `json:"is_active,omitempty"`
}

func DesktopListWindows(ctx context.Context, cfg Config) (string, error) {
	var windows []DesktopWindowInfo
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return "", errors.New("osascript not available on macOS")
		}
		script := `tell application "System Events"
			set out to ""
			repeat with p in (every process whose background only is false)
				set pname to name of p
				set pidVal to unix id of p
				try
					repeat with w in (every window of p)
						set wname to name of w
						set out to out & pidVal & tab & pname & tab & wname & linefeed
					end repeat
				end try
			end repeat
			return out
		end tell`
		cmd := exec.CommandContext(ctx, "osascript", "-e", script)
		hideCommandWindow(cmd)
		raw, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("macOS window enumeration failed: %w", err)
		}
		windows = parseTabSeparatedWindows(string(raw))
	default:
		if wmctrlPath, err := exec.LookPath("wmctrl"); err == nil {
			cmd := exec.CommandContext(ctx, wmctrlPath, "-lp")
			hideCommandWindow(cmd)
			raw, err := cmd.Output()
			if err != nil {
				return "", fmt.Errorf("wmctrl window enumeration failed: %w", err)
			}
			windows = parseWmctrlWindows(string(raw))
		} else {
			return "", errors.New("no Linux window lister found (install wmctrl)")
		}
	}

	filtered := make([]DesktopWindowInfo, 0, len(windows))
	for _, w := range windows {
		if isBlockedDesktopWindow(w.Title, w.ProcessName) {
			continue
		}
		filtered = append(filtered, w)
	}
	encoded, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func parseTabSeparatedWindows(raw string) []DesktopWindowInfo {
	var out []DesktopWindowInfo
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		out = append(out, DesktopWindowInfo{
			PID:         pid,
			ProcessName: strings.TrimSpace(parts[1]),
			Title:       strings.TrimSpace(parts[2]),
		})
	}
	return out
}

func parseWmctrlWindows(raw string) []DesktopWindowInfo {
	var out []DesktopWindowInfo
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		handle, _ := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(fields[0]), "0x"), 16, 64)
		pid, _ := strconv.Atoi(fields[2])
		title := strings.Join(fields[4:], " ")
		out = append(out, DesktopWindowInfo{
			Handle:      handle,
			PID:         pid,
			ProcessName: fields[3],
			Title:       title,
		})
	}
	return out
}

func DesktopInspect(ctx context.Context, cfg Config, windowTitle, selector string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("access to protected system/security window is forbidden")
	}
	return "", fmt.Errorf("desktop UI control inspection is not yet supported on %s", runtime.GOOS)
}

func DesktopClick(ctx context.Context, cfg Config, windowTitle, controlName string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("interaction with protected system/security window is forbidden")
	}
	return "", fmt.Errorf("desktop UI control click is not yet supported on %s", runtime.GOOS)
}

func DesktopType(ctx context.Context, cfg Config, windowTitle, controlName, text string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("typing into protected system/security window is forbidden")
	}
	return "", fmt.Errorf("desktop UI typing is not yet supported on %s", runtime.GOOS)
}

func DesktopScreenshot(ctx context.Context, cfg Config, project, windowTitle, destination string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("screenshots of protected system/security windows are forbidden")
	}
	dest := strings.TrimSpace(destination)
	if dest == "" {
		dest = "desktop-screenshot.png"
	}
	fullDest, err := ensureWithinRoot(project, dest)
	if err != nil {
		return "", fmt.Errorf("screenshot path outside workspace root: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(fullDest), 0o755); err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("screencapture"); err != nil {
			return "", errors.New("macOS screencapture tool not available")
		}
		cmd = exec.CommandContext(ctx, "screencapture", "-x", fullDest)
	} else {
		switch {
		case commandExists("grim"):
			cmd = exec.CommandContext(ctx, "grim", fullDest)
		case commandExists("gnome-screenshot"):
			cmd = exec.CommandContext(ctx, "gnome-screenshot", "-f", fullDest)
		case commandExists("scrot"):
			cmd = exec.CommandContext(ctx, "scrot", fullDest)
		case commandExists("import"):
			cmd = exec.CommandContext(ctx, "import", "-window", "root", fullDest)
		default:
			return "", errors.New("no Linux screenshot tool found (install grim, gnome-screenshot, scrot, or imagemagick)")
		}
	}
	hideCommandWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("desktop screenshot failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("Screenshot saved to %s", fullDest), nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

var blockedDesktopWindows = []string{
	"task manager", "taskmgr", "windows security", "sicherheitscenter", "logonui", "uac",
	"benutzerkontensteuerung", "credential", "passwort", "anmeldeinformationen",
	"keychain access", "schlüsselbundverwaltung", "1password", "bitwarden", "keepass",
	"polkit", "gnome-keyring",
}

func isBlockedDesktopWindow(title, processName string) bool {
	lowerT := strings.ToLower(title)
	lowerP := strings.ToLower(processName)
	for _, blocked := range blockedDesktopWindows {
		if strings.Contains(lowerT, blocked) || strings.Contains(lowerP, blocked) {
			return true
		}
	}
	return false
}

func escapePowerShellString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
