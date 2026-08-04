//go:build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func hideCommandWindow(cmd *exec.Cmd) {}

func openBrowser(url string) error          { return exec.Command("xdg-open", url).Start() }
func showFatal(title, message string)       { fmt.Printf("%s: %s\n", title, message) }
func startOllamaDetached(path string) error { return exec.Command(path, "serve").Start() }
func findOllamaExecutable() string          { p, _ := exec.LookPath("ollama"); return p }
func detectGPU() string {
	out, _ := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader").Output()
	return strings.TrimSpace(string(out))
}
func openProjectTarget(project, target string) error {
	return exec.Command("xdg-open", project).Start()
}
func selectDirectory(initial string) (string, error) { return initial, nil }
func commandBlocked(cfg Config, command string) error {
	for _, p := range cfg.BlockedCommandPatterns {
		if re, err := regexp.Compile(p); err == nil && re.MatchString(command) {
			return fmt.Errorf("blocked by command safety rule: %s", p)
		}
	}
	return nil
}
func runProjectCommand(ctx context.Context, project, command string, cfg Config) (string, error) {
	if err := commandBlocked(cfg, command); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = project
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
