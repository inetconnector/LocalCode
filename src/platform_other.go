// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
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

func openBrowser(url string) error          { return exec.Command("xdg-open", url).Start() }
func openStartupBrowser(url string) error   { return openBrowser(url) }
func showFatal(title, message string)       { fmt.Printf("%s: %s\n", title, message) }
func startOllamaDetached(path string) error { return exec.Command(path, "serve").Start() }
func findOllamaExecutable() string          { p, _ := exec.LookPath("ollama"); return p }
func detectGPU() string {
	out, _ := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader").Output()
	return strings.TrimSpace(string(out))
}
func openProjectTarget(project, target string, cfg Config) error {
	return exec.Command("xdg-open", project).Start()
}
func selectDirectory(initial, language string) (string, error) { return initial, nil }
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
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = project
	cmd.Env = commandEnvironment(cfg)
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	return truncateText(b.String(), 160000), err
}
func openInteractiveTerminal(project, command string, cfg Config) error {
	if strings.TrimSpace(command) == "" {
		command = "exec $SHELL"
	}
	return exec.Command("x-terminal-emulator", "-e", "sh", "-lc", "cd '"+strings.ReplaceAll(project, "'", "'\\''")+"'; "+command+"; exec $SHELL").Start()
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
