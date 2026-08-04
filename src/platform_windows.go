// SPDX-License-Identifier: Apache-2.0

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
	"sync"
	"syscall"
	"time"
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

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := fmt.Sprintf("%d", cmd.Process.Pid)
	kill := exec.Command("taskkill.exe", "/PID", pid, "/T", "/F")
	hideCommandWindow(kill)
	_ = kill.Run()
	_ = cmd.Process.Kill()
}

func openBrowser(url string) error {
	// Prefer Chromium app mode so LocalCode opens as a focused desktop-style
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
	resolutionNote := ""
	if cfg.AgentEnvironment == "wsl" || cfg.TerminalShell == "wsl" {
		wslProject := windowsPathToWSL(project)
		line := "cd '" + strings.ReplaceAll(wslProject, "'", "'\\''") + "' && " + command
		cmd = exec.Command("wsl.exe", "bash", "-lc", line)
	} else if cfg.TerminalShell == "cmd" {
		rewritten, note, err := rewriteKnownToolCommand(project, command, cfg, "cmd")
		if err != nil {
			return note, err
		}
		resolutionNote = note
		cmd = exec.Command("cmd.exe", "/D", "/S", "/C", rewritten)
		cmd.Dir = project
	} else {
		rewritten, note, err := rewriteKnownToolCommand(project, command, cfg, "powershell")
		if err != nil {
			return note, err
		}
		resolutionNote = note
		cmd = exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", rewritten)
		cmd.Dir = project
	}
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return resolutionNote, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killProcessTree(cmd)
		err = ctx.Err()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	var out strings.Builder
	if resolutionNote != "" {
		out.WriteString(resolutionNote + "\n")
	}
	fmt.Fprintf(&out, "Arbeitsordner: %s\nExitcode: %d\nDauer: %s\n", project, exitCode, time.Since(started).Round(time.Millisecond))
	if stdout.Len() > 0 {
		out.WriteString("\nSTDOUT:\n" + stdout.String())
	}
	if stderr.Len() > 0 {
		out.WriteString("\nSTDERR:\n" + stderr.String())
	}
	return truncateText(out.String(), 160000), err
}

func openInteractiveTerminal(project, command string, cfg Config) error {
	title := "LocalCode Terminal"
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
		for _, candidate := range visualStudioToolPaths("devenv") {
			if st, err := os.Stat(candidate[0]); err == nil && !st.IsDir() {
				return startHidden(candidate[0], project)
			}
		}
		return fmt.Errorf("Visual Studio wurde weder über PATH noch über vswhere/Visual-Studio-Installationspfade gefunden")
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

var visualStudioRootsOnce sync.Once
var cachedVisualStudioRoots []string

func visualStudioInstallRoots() []string {
	visualStudioRootsOnce.Do(func() {
		roots := []string{}
		addRoot := func(root string) {
			root = strings.TrimSpace(root)
			if root == "" {
				return
			}
			for _, existing := range roots {
				if strings.EqualFold(existing, root) {
					return
				}
			}
			roots = append(roots, filepath.Clean(root))
		}
		vswhereCandidates := []string{}
		if p, err := exec.LookPath("vswhere.exe"); err == nil {
			vswhereCandidates = append(vswhereCandidates, p)
		}
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if base != "" {
				vswhereCandidates = append(vswhereCandidates, filepath.Join(base, "Microsoft Visual Studio", "Installer", "vswhere.exe"))
			}
		}
		for _, vswhere := range vswhereCandidates {
			if st, err := os.Stat(vswhere); err != nil || st.IsDir() {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			cmd := exec.CommandContext(ctx, vswhere, "-all", "-products", "*", "-property", "installationPath")
			hideCommandWindow(cmd)
			out, err := cmd.Output()
			cancel()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n") {
				addRoot(line)
			}
			break
		}
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if base == "" {
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(base, "Microsoft Visual Studio", "*", "*"))
			for _, match := range matches {
				addRoot(match)
			}
		}
		cachedVisualStudioRoots = roots
	})
	return append([]string(nil), cachedVisualStudioRoots...)
}

func visualStudioToolPaths(name string) [][2]string {
	result := [][2]string{}
	add := func(path, source string) {
		if path != "" {
			result = append(result, [2]string{path, source})
		}
	}
	for _, root := range visualStudioInstallRoots() {
		source := "Visual Studio: " + root
		switch canonicalToolName(name) {
		case "msbuild":
			add(filepath.Join(root, "MSBuild", "Current", "Bin", "MSBuild.exe"), source)
			add(filepath.Join(root, "MSBuild", "Current", "Bin", "amd64", "MSBuild.exe"), source)
		case "git":
			add(filepath.Join(root, "Common7", "IDE", "CommonExtensions", "Microsoft", "TeamFoundation", "Team Explorer", "Git", "cmd", "git.exe"), source)
			add(filepath.Join(root, "Common7", "IDE", "CommonExtensions", "Microsoft", "TeamFoundation", "Team Explorer", "Git", "bin", "git.exe"), source)
		case "cmake":
			add(filepath.Join(root, "Common7", "IDE", "CommonExtensions", "Microsoft", "CMake", "CMake", "bin", "cmake.exe"), source)
		case "ninja":
			add(filepath.Join(root, "Common7", "IDE", "CommonExtensions", "Microsoft", "CMake", "Ninja", "ninja.exe"), source)
		case "java", "keytool", "jarsigner":
			add(filepath.Join(root, "Common7", "IDE", "Extensions", "Xamarin", "Android", "OpenJDK", "bin", executableName(canonicalToolName(name))), source)
		case "devenv":
			add(filepath.Join(root, "Common7", "IDE", "devenv.exe"), source)
		case "nuget":
			add(filepath.Join(root, "Common7", "IDE", "CommonExtensions", "Microsoft", "NuGet", "nuget.exe"), source)
		}
	}
	return result
}

func androidHostDeviceDiagnostic() string {
	script := `$ErrorActionPreference='SilentlyContinue'; ` +
		`Get-PnpDevice -PresentOnly | Where-Object { ($_.FriendlyName -match 'Android|ADB|MTP|Google|Samsung|Pixel') -or ($_.InstanceId -match 'VID_18D1|VID_04E8|ADB') } | ` +
		`Select-Object -First 12 Status,Class,FriendlyName,InstanceId | Format-Table -AutoSize | Out-String -Width 260`
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
	out, err := cmd.Output()
	text := strings.TrimSpace(string(out))
	if err != nil || text == "" {
		return "Windows meldet über Plug-and-Play kein eindeutig erkennbares Android-/ADB-Gerät. Prüfe Datenkabel, USB-Modus und OEM-USB-Treiber."
	}
	return "Windows Plug-and-Play erkennt folgende mögliche Android-Geräte:\n" + text
}
