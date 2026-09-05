// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DesktopWindowInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	PID         int    `json:"pid"`
	Handle      int64  `json:"handle"`
	IsActive    bool   `json:"is_active,omitempty"`
}

type DesktopControlInfo struct {
	Name         string `json:"name"`
	AutomationID string `json:"automation_id,omitempty"`
	ControlType  string `json:"control_type"`
	ClassName    string `json:"class_name,omitempty"`
	IsEnabled    bool   `json:"is_enabled"`
	BoundingRect string `json:"bounding_rect,omitempty"`
}

var blockedDesktopWindows = []string{
	"task manager", "taskmgr", "windows security", "sicherheitscenter", "logonui", "uac",
	"benutzerkontensteuerung", "credential", "passwort", "anmeldeinformationen",
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

// DesktopListWindows enumerates open top-level desktop application windows.
func DesktopListWindows(ctx context.Context, cfg Config) (string, error) {
	psScript := `
Add-Type @"
  using System;
  using System.Collections.Generic;
  using System.Runtime.InteropServices;
  using System.Text;

  public class WinList {
    [DllImport("user32.dll")]
    private static extern bool EnumWindows(EnumWindowsProc enumProc, IntPtr lParam);

    [DllImport("user32.dll")]
    private static extern bool IsWindowVisible(IntPtr hWnd);

    [DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
    private static extern int GetWindowText(IntPtr hWnd, StringBuilder lpString, int nMaxCount);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);

    [DllImport("user32.dll")]
    private static extern IntPtr GetForegroundWindow();

    private delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

    public class WindowItem {
      public string Title { get; set; }
      public string ProcessName { get; set; }
      public int PID { get; set; }
      public long Handle { get; set; }
      public bool IsActive { get; set; }
    }

    public static List<WindowItem> GetVisibleWindows() {
      var list = new List<WindowItem>();
      IntPtr fg = GetForegroundWindow();
      EnumWindows((hWnd, lParam) => {
        if (!IsWindowVisible(hWnd)) return true;
        var sb = new StringBuilder(256);
        GetWindowText(hWnd, sb, 256);
        string title = sb.ToString().Trim();
        if (string.IsNullOrEmpty(title)) return true;
        uint pid;
        GetWindowThreadProcessId(hWnd, out pid);
        string procName = "";
        try { procName = System.Diagnostics.Process.GetProcessById((int)pid).ProcessName; } catch {}
        list.Add(new WindowItem {
          Title = title,
          ProcessName = procName,
          PID = (int)pid,
          Handle = hWnd.ToInt64(),
          IsActive = (hWnd == fg)
        });
        return true;
      }, IntPtr.Zero);
      return list;
    }
  }
"@
[WinList]::GetVisibleWindows() | ConvertTo-Json -Depth 2
`
	out, err := runPowerShellScript(ctx, cfg, psScript)
	if err != nil {
		return "", fmt.Errorf("listing desktop windows failed: %w", err)
	}

	var raw []DesktopWindowInfo
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, "{") {
		var single DesktopWindowInfo
		if json.Unmarshal([]byte(trimmed), &single) == nil {
			raw = []DesktopWindowInfo{single}
		}
	} else {
		_ = json.Unmarshal([]byte(trimmed), &raw)
	}

	var allowed []DesktopWindowInfo
	for _, w := range raw {
		if !isBlockedDesktopWindow(w.Title, w.ProcessName) {
			allowed = append(allowed, w)
		}
	}

	data, _ := json.MarshalIndent(allowed, "", "  ")
	return fmt.Sprintf("VISIBLE DESKTOP WINDOWS (%d found):\n\n%s", len(allowed), string(data)), nil
}

// DesktopInspect inspects the UI Automation control tree of a target window.
func DesktopInspect(ctx context.Context, cfg Config, windowTitle, selector string) (string, error) {
	windowTitle = strings.TrimSpace(windowTitle)
	if windowTitle == "" {
		return "", errors.New("desktop_inspect requires a window_title")
	}
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("access to system/security window is blocked for safety")
	}

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

$targetTitle = "%s"
$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    [System.Windows.Automation.Condition]::TrueCondition
)

$found = $null
foreach ($w in $windows) {
    if ($w.Current.Name -like "*$targetTitle*") {
        $found = $w
        break
    }
}

if (-not $found) {
    Write-Error "Window with title matching '$targetTitle' not found."
    exit 1
}

$controls = $found.FindAll(
    [System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition
)

$results = @()
foreach ($c in $controls) {
    $name = $c.Current.Name
    $id = $c.Current.AutomationId
    $type = $c.Current.ControlType.ProgrammaticName -replace "ControlType\.", ""
    $class = $c.Current.ClassName
    $enabled = $c.Current.IsEnabled
    
    if (-not [string]::IsNullOrWhiteSpace($name) -or -not [string]::IsNullOrWhiteSpace($id)) {
        $results += [PSCustomObject]@{
            name = $name
            automation_id = $id
            control_type = $type
            class_name = $class
            is_enabled = $enabled
        }
    }
    if ($results.Count -ge 80) { break }
}

$results | ConvertTo-Json -Depth 2
`, escapePowerShellString(windowTitle))

	out, err := runPowerShellScript(ctx, cfg, psScript)
	if err != nil {
		return "", fmt.Errorf("inspecting desktop window UI tree failed: %w", err)
	}

	return fmt.Sprintf("DESKTOP WINDOW UI TREE (%s):\n\n%s", windowTitle, out), nil
}

// DesktopClick clicks or invokes a named control in a target window.
func DesktopClick(ctx context.Context, cfg Config, windowTitle, controlName string) (string, error) {
	windowTitle = strings.TrimSpace(windowTitle)
	controlName = strings.TrimSpace(controlName)
	if windowTitle == "" || controlName == "" {
		return "", errors.New("desktop_click requires window_title and control_name")
	}
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("access to system/security window is blocked for safety")
	}

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

$targetTitle = "%s"
$targetControl = "%s"

$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    [System.Windows.Automation.Condition]::TrueCondition
)

$foundWin = $null
foreach ($w in $windows) {
    if ($w.Current.Name -like "*$targetTitle*") {
        $foundWin = $w
        break
    }
}

if (-not $foundWin) {
    Write-Error "Window matching '$targetTitle' not found."
    exit 1
}

$controls = $foundWin.FindAll(
    [System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition
)

$invoked = $false
foreach ($c in $controls) {
    if ($c.Current.Name -eq $targetControl -or $c.Current.AutomationId -eq $targetControl) {
        $pattern = $null
        if ($c.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
            $pattern.Invoke()
            $invoked = $true
            Write-Output "Invoked control '$targetControl' successfully."
            break
        }
        if ($c.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern, [ref]$pattern)) {
            $pattern.Toggle()
            $invoked = $true
            Write-Output "Toggled control '$targetControl' successfully."
            break
        }
    }
}

if (-not $invoked) {
    Write-Error "Control '$targetControl' found but does not support InvokePattern/TogglePattern."
    exit 1
}
`, escapePowerShellString(windowTitle), escapePowerShellString(controlName))

	out, err := runPowerShellScript(ctx, cfg, psScript)
	if err != nil {
		return "", fmt.Errorf("desktop click failed: %w", err)
	}

	return fmt.Sprintf("DESKTOP CLICKED\nWindow: %s\nControl: %s\nResult: %s", windowTitle, controlName, strings.TrimSpace(out)), nil
}

// DesktopType enters text into an edit control or focused element.
func DesktopType(ctx context.Context, cfg Config, windowTitle, controlName, text string) (string, error) {
	windowTitle = strings.TrimSpace(windowTitle)
	if windowTitle == "" {
		return "", errors.New("desktop_type requires window_title")
	}
	if isBlockedDesktopWindow(windowTitle, "") {
		return "", errors.New("access to system/security window is blocked for safety")
	}

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

$targetTitle = "%s"
$targetControl = "%s"
$textToType = @"
%s
"@

$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    [System.Windows.Automation.Condition]::TrueCondition
)

$foundWin = $null
foreach ($w in $windows) {
    if ($w.Current.Name -like "*$targetTitle*") {
        $foundWin = $w
        break
    }
}

if (-not $foundWin) {
    Write-Error "Window matching '$targetTitle' not found."
    exit 1
}

$controls = $foundWin.FindAll(
    [System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition
)

$set = $false
foreach ($c in $controls) {
    if ($c.Current.Name -eq $targetControl -or $c.Current.AutomationId -eq $targetControl -or [string]::IsNullOrEmpty($targetControl)) {
        $pattern = $null
        if ($c.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) {
            $pattern.SetValue($textToType)
            $set = $true
            Write-Output "Set value on '$($c.Current.Name)' successfully."
            break
        }
    }
}

if (-not $set) {
    $foundWin.SetFocus()
    [System.Windows.Forms.SendKeys]::SendWait($textToType)
    Write-Output "Typed text via SendKeys to window."
}
`, escapePowerShellString(windowTitle), escapePowerShellString(controlName), text)

	out, err := runPowerShellScript(ctx, cfg, psScript)
	if err != nil {
		return "", fmt.Errorf("desktop type failed: %w", err)
	}

	return fmt.Sprintf("DESKTOP TYPED\nWindow: %s\nControl: %s\nResult: %s", windowTitle, controlName, strings.TrimSpace(out)), nil
}

// DesktopScreenshot captures a visual screenshot of a desktop window or entire screen.
func DesktopScreenshot(ctx context.Context, cfg Config, project, windowTitle, destination string) (string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		destination = "desktop_screenshot.png"
	}
	outPath := destination
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(project, destination)
	}
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bitmap = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$bitmap.Save("%s", [System.Drawing.Imaging.ImageFormat]::Png)
$graphics.Dispose()
$bitmap.Dispose()
Write-Output "Saved screenshot."
`, escapePowerShellString(outPath))

	out, err := runPowerShellScript(ctx, cfg, psScript)
	if err != nil {
		return "", fmt.Errorf("desktop screenshot failed: %w", err)
	}

	info, _ := os.Stat(outPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	return fmt.Sprintf("DESKTOP SCREENSHOT CAPTURED\nDestination: %s\nSize: %d bytes\nOutput: %s", destination, size, strings.TrimSpace(out)), nil
}

func runPowerShellScript(ctx context.Context, cfg Config, script string) (string, error) {
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(pctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func escapePowerShellString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
