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

const maxDesktopTypeChars = 4096

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

func buildMacInspectScript(windowTitle, selector string) string {
	winLit := appleScriptStringLiteral(strings.TrimSpace(windowTitle))
	selLit := appleScriptStringLiteral(strings.TrimSpace(selector))
	return fmt.Sprintf(`tell application "System Events"
		set targetTitle to %s
		set filterText to %s
		set out to ""
		repeat with p in (every process whose background only is false)
			try
				repeat with w in (every window of p)
					if (targetTitle is "") or ((name of w) contains targetTitle) then
						set out to out & "WINDOW: " & (name of p) & " | " & (name of w) & linefeed
						repeat with elem in (entire contents of w)
							try
								set desc to (role of elem as text) & ": " & (name of elem as text)
								if (filterText is "") or (desc contains filterText) then
									set out to out & "  " & desc & linefeed
								end if
							end try
						end repeat
						return out
					end if
				end repeat
			end try
		end repeat
		return out
	end tell`, winLit, selLit)
}

func buildMacClickScript(windowTitle, controlName string) string {
	winLit := appleScriptStringLiteral(strings.TrimSpace(windowTitle))
	ctrlLit := appleScriptStringLiteral(strings.TrimSpace(controlName))
	return fmt.Sprintf(`tell application "System Events"
		set targetTitle to %s
		set targetCtrl to %s
		repeat with p in (every process whose background only is false)
			try
				repeat with w in (every window of p)
					if (targetTitle is "") or ((name of w) contains targetTitle) then
						set frontmost of p to true
						repeat with elem in (entire contents of w)
							try
								if (name of elem as text) contains targetCtrl then
									click elem
									return "CLICKED: " & (name of elem as text)
								end if
							end try
						end repeat
					end if
				end repeat
			end try
		end repeat
		error "control not found"
	end tell`, winLit, ctrlLit)
}

func buildMacTypeScript(windowTitle, text string) string {
	winLit := appleScriptStringLiteral(strings.TrimSpace(windowTitle))
	txtLit := appleScriptStringLiteral(text)
	return fmt.Sprintf(`tell application "System Events"
		set targetTitle to %s
		repeat with p in (every process whose background only is false)
			try
				repeat with w in (every window of p)
					if (targetTitle is "") or ((name of w) contains targetTitle) then
						set frontmost of p to true
						keystroke %s
						return "TYPED"
					end if
				end repeat
			end try
		end repeat
		error "window not found"
	end tell`, winLit, txtLit)
}

func validateDesktopTypePayload(text string) error {
	if len(text) == 0 {
		return errors.New("desktop type text must not be empty")
	}
	if len(text) > maxDesktopTypeChars {
		return fmt.Errorf("desktop type text exceeds %d byte safety limit", maxDesktopTypeChars)
	}
	if strings.ContainsRune(text, '\x00') {
		return errors.New("desktop type text must not contain NUL bytes")
	}
	return nil
}

func DesktopInspect(ctx context.Context, cfg Config, windowTitle, selector string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("access to protected system/security window is forbidden")
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("osascript"); err != nil {
			return "", errors.New("osascript not available on macOS")
		}
		cmd := exec.CommandContext(ctx, "osascript", "-e", buildMacInspectScript(windowTitle, selector))
		hideCommandWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("macOS UI inspect failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	// Linux: locate matching window via xdotool and inspect X11 properties via xprop
	if !commandExists("xdotool") {
		return "", errors.New("xdotool not found on Linux (install xdotool and x11-utils)")
	}
	title := strings.TrimSpace(windowTitle)
	if title == "" {
		return DesktopListWindows(ctx, cfg)
	}
	searchCmd := exec.CommandContext(ctx, "xdotool", "search", "--name", "--", title)
	hideCommandWindow(searchCmd)
	winOut, err := searchCmd.Output()
	if err != nil {
		return "", fmt.Errorf("linux window lookup failed: %w", err)
	}
	winIDs := strings.Fields(string(winOut))
	if len(winIDs) == 0 {
		return "", fmt.Errorf("no Linux window matching %q", title)
	}
	if !commandExists("xprop") {
		return fmt.Sprintf("window_id=%s title=%q", winIDs[0], title), nil
	}
	propCmd := exec.CommandContext(ctx, "xprop", "-id", winIDs[0])
	hideCommandWindow(propCmd)
	propOut, err := propCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("xprop failed: %w", err)
	}
	return strings.TrimSpace(string(propOut)), nil
}

func DesktopClick(ctx context.Context, cfg Config, windowTitle, controlName string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") || isBlockedDesktopWindow(controlName, "") {
		return "", errors.New("interaction with protected system/security window is forbidden")
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("osascript"); err != nil {
			return "", errors.New("osascript not available on macOS")
		}
		if strings.TrimSpace(controlName) == "" {
			return "", errors.New("controlName is required for macOS UI click")
		}
		cmd := exec.CommandContext(ctx, "osascript", "-e", buildMacClickScript(windowTitle, controlName))
		hideCommandWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("macOS UI click failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	// Linux: focus window via xdotool and click active window (or coordinates "x,y" if supplied)
	if !commandExists("xdotool") {
		return "", errors.New("xdotool not found on Linux (install xdotool)")
	}
	title := strings.TrimSpace(windowTitle)
	if title != "" {
		focusCmd := exec.CommandContext(ctx, "xdotool", "search", "--name", "--", title, "windowactivate", "--sync")
		hideCommandWindow(focusCmd)
		if out, err := focusCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("linux window focus failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	parts := strings.Split(strings.TrimSpace(controlName), ",")
	var clickCmd *exec.Cmd
	if len(parts) == 2 {
		x, errX := strconv.Atoi(strings.TrimSpace(parts[0]))
		y, errY := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errX == nil && errY == nil && x >= 0 && y >= 0 {
			clickCmd = exec.CommandContext(ctx, "xdotool", "mousemove", "--sync", strconv.Itoa(x), strconv.Itoa(y), "click", "1")
		}
	}
	if clickCmd == nil {
		clickCmd = exec.CommandContext(ctx, "xdotool", "click", "1")
	}
	hideCommandWindow(clickCmd)
	if out, err := clickCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("linux UI click failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("Clicked %q in window %q", controlName, title), nil
}

func DesktopType(ctx context.Context, cfg Config, windowTitle, controlName, text string) (string, error) {
	if isBlockedDesktopWindow(windowTitle, "") || isBlockedDesktopWindow(controlName, "") {
		return "", errors.New("typing into protected system/security window is forbidden")
	}
	if err := validateDesktopTypePayload(text); err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("osascript"); err != nil {
			return "", errors.New("osascript not available on macOS")
		}
		cmd := exec.CommandContext(ctx, "osascript", "-e", buildMacTypeScript(windowTitle, text))
		hideCommandWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("macOS UI type failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return fmt.Sprintf("Typed %d characters into window %q", len(text), strings.TrimSpace(windowTitle)), nil
	}
	if !commandExists("xdotool") {
		return "", errors.New("xdotool not found on Linux (install xdotool)")
	}
	title := strings.TrimSpace(windowTitle)
	if title != "" {
		focusCmd := exec.CommandContext(ctx, "xdotool", "search", "--name", "--", title, "windowactivate", "--sync")
		hideCommandWindow(focusCmd)
		if out, err := focusCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("linux window focus failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	typeCmd := exec.CommandContext(ctx, "xdotool", "type", "--clearmodifiers", "--delay", "5", "--", text)
	hideCommandWindow(typeCmd)
	if out, err := typeCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("linux UI type failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("Typed %d characters into window %q", len(text), title), nil
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
