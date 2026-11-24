// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func hideCommandWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Give every managed subprocess its own process group. Cancellation can
	// then terminate linters, test runners and model helper children together
	// instead of leaving an inherited stdout/stderr pipe open.
	cmd.SysProcAttr.Setpgid = true
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func appleScriptStringLiteral(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", ``)
	return `"` + replacer.Replace(s) + `"`
}

func openBrowser(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Start()
	}
	if p, err := exec.LookPath("xdg-open"); err == nil {
		return exec.Command(p, url).Start()
	}
	if p, err := exec.LookPath("gio"); err == nil {
		return exec.Command(p, "open", url).Start()
	}
	for _, candidate := range chromiumBrowserCandidates() {
		if err := exec.Command(candidate, url).Start(); err == nil {
			return nil
		}
	}
	return errors.New("no browser opener found (install xdg-open)")
}

func openStartupBrowser(url string) error { return openBrowser(url) }

func showFatal(title, message string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, message) }

func startOllamaDetached(path string) error {
	cmd := exec.Command(path, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func findOllamaExecutable() string {
	if p, err := exec.LookPath("ollama"); err == nil {
		return p
	}
	candidates := []string{"/usr/local/bin/ollama", "/usr/bin/ollama"}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{
			"/opt/homebrew/bin/ollama",
			"/Applications/Ollama.app/Contents/Resources/ollama",
		}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func chromiumBrowserCandidates() []string {
	var out []string
	if runtime.GOOS == "darwin" {
		macApps := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, app := range macApps {
			if st, err := os.Stat(app); err == nil && !st.IsDir() {
				out = append(out, app)
			}
		}
	}
	names := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "microsoft-edge", "msedge"}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func runProbeCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideCommandWindow(cmd)
	out, err := cmd.Output()
	return string(out), err
}

func parseMacOSDisplaysOutput(raw string) string {
	var chipset, metal, vram string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "chipset model:") && chipset == "":
			chipset = strings.TrimSpace(trimmed[len("chipset model:"):])
		case (strings.HasPrefix(lower, "metal support:") || strings.HasPrefix(lower, "metal family:") || strings.HasPrefix(lower, "metal:")) && metal == "":
			if idx := strings.IndexByte(trimmed, ':'); idx >= 0 {
				metal = strings.TrimSpace(trimmed[idx+1:])
			}
		case strings.HasPrefix(lower, "vram") && vram == "":
			if idx := strings.IndexByte(trimmed, ':'); idx >= 0 {
				vram = strings.TrimSpace(trimmed[idx+1:])
			}
		}
	}
	if chipset == "" {
		return ""
	}
	var extras []string
	if metal != "" {
		extras = append(extras, metal)
	}
	if vram != "" {
		extras = append(extras, vram)
	}
	if len(extras) > 0 {
		return fmt.Sprintf("%s (%s)", chipset, strings.Join(extras, ", "))
	}
	return chipset
}

func parseROCmSMIOutput(raw string) string {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "-") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "card series:") || strings.Contains(lower, "card model:") || strings.Contains(lower, "device name:") {
			if idx := strings.LastIndexByte(trimmed, ':'); idx >= 0 {
				val := strings.TrimSpace(trimmed[idx+1:])
				if val != "" {
					return "AMD " + strings.TrimPrefix(val, "AMD ")
				}
			}
		}
	}
	return ""
}

func parseLspciGPUOutput(raw string) string {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !strings.Contains(lower, "vga compatible controller") &&
			!strings.Contains(lower, "3d controller") &&
			!strings.Contains(lower, "display controller") {
			continue
		}
		if idx := strings.Index(trimmed, ": "); idx >= 0 {
			desc := strings.TrimSpace(trimmed[idx+2:])
			if desc != "" {
				return desc
			}
		}
		return trimmed
	}
	return ""
}

func detectGPU() string {
	const probeTimeout = 4 * time.Second

	if runtime.GOOS == "darwin" {
		if out, err := runProbeCommand(probeTimeout, "system_profiler", "SPDisplaysDataType"); err == nil {
			if parsed := parseMacOSDisplaysOutput(out); parsed != "" {
				return parsed
			}
		}
		return ""
	}

	// Linux / BSD: prefer NVIDIA, then AMD ROCm, then Vulkan/PCI (lspci).
	if p, err := exec.LookPath("nvidia-smi"); err == nil {
		if out, err := runProbeCommand(probeTimeout, p, "--query-gpu=name,memory.total", "--format=csv,noheader"); err == nil {
			line := strings.TrimSpace(out)
			if i := strings.IndexByte(line, '\n'); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line != "" {
				return line
			}
		}
	}

	if p, err := exec.LookPath("rocm-smi"); err == nil {
		if out, err := runProbeCommand(probeTimeout, p, "--showproductname"); err == nil {
			if parsed := parseROCmSMIOutput(out); parsed != "" {
				return parsed
			}
		}
	}

	if p, err := exec.LookPath("lspci"); err == nil {
		if out, err := runProbeCommand(probeTimeout, p); err == nil {
			if parsed := parseLspciGPUOutput(out); parsed != "" {
				return parsed
			}
		}
	}

	return ""
}

func openProjectTarget(project, target string, cfg Config) error {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "vscode":
		if code, err := exec.LookPath("code"); err == nil {
			return exec.Command(code, project).Start()
		}
		if runtime.GOOS == "darwin" {
			return exec.Command("open", "-a", "Visual Studio Code", project).Start()
		}
		return fmt.Errorf("%s", localizeConfigText(cfg, "Visual Studio Code wurde nicht gefunden", "Visual Studio Code was not found"))
	default:
		if runtime.GOOS == "darwin" {
			return exec.Command("open", project).Start()
		}
		return openBrowser(project)
	}
}

func selectDirectory(initial, language string) (string, error) {
	prompt := localized(language, "Projektordner auswählen", "Choose project folder")
	initialDir := strings.TrimSpace(initial)
	if initialDir != "" {
		if st, err := os.Stat(initialDir); err != nil || !st.IsDir() {
			initialDir = ""
		}
	}

	var selected string
	if runtime.GOOS == "darwin" {
		script := "POSIX path of (choose folder with prompt " + appleScriptStringLiteral(prompt)
		if initialDir != "" {
			script += " default location (POSIX file " + appleScriptStringLiteral(initialDir) + ")"
		}
		script += ")"
		cmd := exec.Command("osascript", "-e", script)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			errText := stderr.String()
			if strings.Contains(errText, "-128") || strings.Contains(strings.ToLower(errText), "cancel") {
				return "", nil
			}
			return "", fmt.Errorf("osascript folder picker failed: %w (%s)", err, strings.TrimSpace(errText))
		}
		selected = strings.TrimSpace(stdout.String())
	} else {
		if zenity, err := exec.LookPath("zenity"); err == nil {
			args := []string{"--file-selection", "--directory", "--title=" + prompt}
			if initialDir != "" {
				args = append(args, "--filename="+strings.TrimRight(initialDir, "/")+"/")
			}
			cmd := exec.Command(zenity, args...)
			out, err := cmd.Output()
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
					return "", nil
				}
				return "", fmt.Errorf("zenity folder picker failed: %w", err)
			}
			selected = strings.TrimSpace(string(out))
		} else if kdialog, err := exec.LookPath("kdialog"); err == nil {
			startDir := initialDir
			if startDir == "" {
				if home, homeErr := os.UserHomeDir(); homeErr == nil {
					startDir = home
				} else {
					startDir = "."
				}
			}
			cmd := exec.Command(kdialog, "--getexistingdirectory", startDir, "--title", prompt)
			out, err := cmd.Output()
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
					return "", nil
				}
				return "", fmt.Errorf("kdialog folder picker failed: %w", err)
			}
			selected = strings.TrimSpace(string(out))
		} else {
			return "", errors.New("no graphical folder picker found (install zenity or kdialog)")
		}
	}

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

func commandBlocked(cfg Config, command string) error {
	for _, p := range cfg.BlockedCommandPatterns {
		if re, err := regexp.Compile(p); err == nil && re.MatchString(command) {
			return fmt.Errorf("blocked by command safety rule: %s", p)
		}
	}
	return nil
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
	command = normalizeCommandProjectRelativePaths(project, command)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = project
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	return truncateText(b.String(), 160000), err
}

func openInteractiveTerminal(project, command string, cfg Config) error {
	trimmedCmd := strings.TrimSpace(command)
	cdPart := "cd " + shellSingleQuote(project)

	if runtime.GOOS == "darwin" {
		scriptCmd := cdPart
		if trimmedCmd != "" {
			scriptCmd += " && " + trimmedCmd
		}
		cmd := exec.Command(
			"osascript",
			"-e", `tell application "Terminal"`,
			"-e", `activate`,
			"-e", `do script `+appleScriptStringLiteral(scriptCmd),
			"-e", `end tell`,
		)
		cmd.Env = commandEnvironment(cfg)
		return cmd.Start()
	}

	shellCmd := cdPart + "; "
	if trimmedCmd != "" {
		shellCmd += trimmedCmd + "; "
	}
	shellCmd += `exec "${SHELL:-/bin/sh}"`

	type terminalCandidate struct {
		bin  string
		args func(bin string) []string
	}
	candidates := []terminalCandidate{
		{
			bin: "x-terminal-emulator",
			args: func(_ string) []string {
				return []string{"-e", "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "gnome-terminal",
			args: func(_ string) []string {
				return []string{"--working-directory=" + project, "--", "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "konsole",
			args: func(_ string) []string {
				return []string{"--workdir", project, "-e", "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "xfce4-terminal",
			args: func(_ string) []string {
				return []string{"--working-directory=" + project, "-x", "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "alacritty",
			args: func(_ string) []string {
				return []string{"--working-directory", project, "-e", "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "kitty",
			args: func(_ string) []string {
				return []string{"--directory", project, "sh", "-lc", shellCmd}
			},
		},
		{
			bin: "wezterm",
			args: func(_ string) []string {
				return []string{"start", "--cwd", project, "--", "sh", "-lc", shellCmd}
			},
		},
	}

	for _, cand := range candidates {
		binPath, err := exec.LookPath(cand.bin)
		if err != nil {
			continue
		}
		cmd := exec.Command(binPath, cand.args(binPath)...)
		cmd.Env = commandEnvironment(cfg)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}

	return errors.New("no supported terminal emulator found (tried x-terminal-emulator, gnome-terminal, konsole, xfce4-terminal, alacritty, kitty, wezterm)")
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		// A negative pid targets the complete process group created above.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}
}

func visualStudioToolPaths(name string) [][2]string { return nil }

func androidHostDeviceDiagnostic() string { return "" }

func installOllama(ctx context.Context, progress ollamaInstallProgressFunc) (string, error) {
	return "", fmt.Errorf("automatic Ollama installation is currently supported on Windows only")
}
