//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unsafe"
)

func hideCommandWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}

func openBrowser(url string) error {
	// Prefer Chromium app mode so LocalCodex opens as a focused desktop-style
	// window without browser tabs or an address bar. Fall back to the default
	// browser when Edge/Chrome cannot be located.
	candidates := []string{}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		candidates = append(candidates, p)
	}
	if p, err := exec.LookPath("chrome.exe"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
	} {
		if p != "" {
			candidates = append(candidates, p)
		}
	}
	seen := map[string]bool{}
	for _, browser := range candidates {
		if browser == "" || seen[strings.ToLower(browser)] {
			continue
		}
		seen[strings.ToLower(browser)] = true
		if st, err := os.Stat(browser); err != nil || st.IsDir() {
			continue
		}
		cmd := exec.Command(browser, "--app="+url, "--start-maximized", "--no-first-run")
		hideCommandWindow(cmd)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	hideCommandWindow(cmd)
	return cmd.Start()
}

func showFatal(title, message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}

func startOllamaDetached(path string) error {
	cmd := exec.Command(path, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Start()
}

func findOllamaExecutable() string {
	if p, err := exec.LookPath("ollama.exe"); err == nil {
		return p
	}
	candidates := []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Ollama", "ollama.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Ollama", "ollama.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Ollama", "ollama.exe")}
	for _, p := range candidates {
		if p != "" {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func commandBlocked(cfg Config, command string) error {
	for _, pattern := range cfg.BlockedCommandPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(command) {
			return fmt.Errorf("blocked by command safety rule: %s", pattern)
		}
	}
	return nil
}

func windowsPathToWSL(path string) string {
	volume := filepath.VolumeName(path)
	if len(volume) < 2 || volume[1] != ':' {
		return strings.ReplaceAll(path, "\\", "/")
	}
	drive := strings.ToLower(volume[:1])
	rest := strings.TrimPrefix(path, volume)
	rest = strings.TrimLeft(rest, "\\/")
	return "/mnt/" + drive + "/" + strings.ReplaceAll(rest, "\\", "/")
}

func commandEnvironment(cfg Config) []string {
	env := os.Environ()
	for key, value := range cfg.EnvironmentVars {
		key = strings.TrimSpace(key)
		if key != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func runProjectCommand(ctx context.Context, project, command string, cfg Config) (string, error) {
	if err := commandBlocked(cfg, command); err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	if cfg.AgentEnvironment == "wsl" || cfg.TerminalShell == "wsl" {
		wslProject := windowsPathToWSL(project)
		line := "cd '" + strings.ReplaceAll(wslProject, "'", "'\\''") + "' && " + command
		cmd = exec.CommandContext(ctx, "wsl.exe", "bash", "-lc", line)
	} else if cfg.TerminalShell == "cmd" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command)
		cmd.Dir = project
	} else {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
		cmd.Dir = project
	}
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return truncateText(buf.String(), 160000), err
}

func openInteractiveTerminal(project, command string, cfg Config) error {
	title := "LocalCodex Terminal"
	var args []string
	if cfg.AgentEnvironment == "wsl" || cfg.TerminalShell == "wsl" {
		wslProject := windowsPathToWSL(project)
		line := "cd '" + strings.ReplaceAll(wslProject, "'", "'\\''") + "'"
		if strings.TrimSpace(command) != "" {
			line += " && " + command
		}
		line += "; exec bash -i"
		args = []string{"wsl.exe", "bash", "-lc", line}
	} else if cfg.TerminalShell == "cmd" {
		line := "cd /d \"" + strings.ReplaceAll(project, "\"", "\"\"") + "\""
		if strings.TrimSpace(command) != "" {
			line += " && " + command
		}
		args = []string{"cmd.exe", "/K", line}
	} else {
		escapedProject := strings.ReplaceAll(project, "'", "''")
		line := "Set-Location -LiteralPath '" + escapedProject + "'"
		if strings.TrimSpace(command) != "" {
			line += "; " + command
		}
		args = []string{"powershell.exe", "-NoLogo", "-NoExit", "-ExecutionPolicy", "Bypass", "-Command", line}
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		wtArgs := []string{"-w", "new", "new-tab", "--title", title}
		wtArgs = append(wtArgs, args...)
		cmd := exec.Command(wt, wtArgs...)
		cmd.Env = commandEnvironment(cfg)
		return cmd.Start()
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = commandEnvironment(cfg)
	return cmd.Start()
}

func openProjectTarget(project, target string) error {
	startHidden := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		hideCommandWindow(cmd)
		return cmd.Start()
	}
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "vscode":
		if code, err := exec.LookPath("code.exe"); err == nil {
			return startHidden(code, project)
		}
		if code, err := exec.LookPath("code.cmd"); err == nil {
			return startHidden("cmd.exe", "/D", "/S", "/C", "\""+code+"\" \""+project+"\"")
		}
		return fmt.Errorf("Visual Studio Code wurde nicht gefunden")
	case "visualstudio":
		if devenv, err := exec.LookPath("devenv.exe"); err == nil {
			return startHidden(devenv, project)
		}
		return fmt.Errorf("Visual Studio wurde nicht gefunden")
	default:
		return startHidden("explorer.exe", project)
	}
}

func selectDirectory(initial string) (string, error) {
	escaped := strings.ReplaceAll(initial, "'", "''")
	script := `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; ` +
		`$d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Projektordner auswählen'; $d.ShowNewFolderButton=$true; ` +
		`if ('` + escaped + `' -ne '' -and (Test-Path -LiteralPath '` + escaped + `')) { $d.SelectedPath='` + escaped + `' }; ` +
		`if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::OutputEncoding=[Text.Encoding]::UTF8; [Console]::Write($d.SelectedPath) }`
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", nil
	}
	full, err := filepath.Abs(selected)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("selected directory not found: %s", full)
	}
	return full, nil
}

func detectGPU() string {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader")
	hideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return line
}
