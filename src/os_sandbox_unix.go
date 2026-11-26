// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func isSandboxProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func defaultSandboxBackendName() string {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("bwrap"); err == nil {
			return "linux-bwrap"
		}
		return "linux-pgid"
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return "darwin-seatbelt"
		}
		return "darwin-pgid"
	default:
		return "unix-pgid"
	}
}

// BuildBubblewrapArgs constructs Linux bubblewrap (bwrap) invocation arguments
// providing read-only rootfs, writable workspace bind, tmpfs masking of
// protected credential directories, optional network namespace isolation, and
// parent-death termination.
func BuildBubblewrapArgs(policy OSSandboxPolicy, homeDir, workDir, name string, args []string) []string {
	bwrapArgs := []string{
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--bind", policy.WorkspaceRoot, policy.WorkspaceRoot,
	}

	for _, ro := range policy.ReadOnlyPaths {
		ro = strings.TrimSpace(ro)
		if ro != "" {
			bwrapArgs = append(bwrapArgs, "--ro-bind-try", ro, ro)
		}
	}

	seenMasked := map[string]bool{}
	maskPath := func(p string) {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "/" || seenMasked[p] {
			return
		}
		seenMasked[p] = true
		bwrapArgs = append(bwrapArgs, "--tmpfs", p)
	}

	blocked := policy.BlockedPaths
	if len(blocked) == 0 {
		blocked = DefaultSandboxBlockedPaths()
	}
	for _, b := range blocked {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if filepath.IsAbs(b) {
			maskPath(b)
			continue
		}
		if homeDir != "" && !strings.HasPrefix(b, ".git/") {
			maskPath(filepath.Join(homeDir, filepath.FromSlash(b)))
		}
		if policy.WorkspaceRoot != "" {
			wsTarget := filepath.Join(policy.WorkspaceRoot, filepath.FromSlash(b))
			if st, err := os.Stat(wsTarget); err == nil && st.IsDir() {
				maskPath(wsTarget)
			} else if strings.HasPrefix(strings.ToLower(filepath.ToSlash(b)), ".git/hooks") {
				if stGit, err := os.Stat(filepath.Join(policy.WorkspaceRoot, ".git")); err == nil && stGit.IsDir() {
					maskPath(wsTarget)
				}
			}
		}
	}

	if !policy.AllowNetwork {
		bwrapArgs = append(bwrapArgs, "--unshare-net")
	}
	bwrapArgs = append(bwrapArgs, "--die-with-parent")
	if workDir != "" {
		bwrapArgs = append(bwrapArgs, "--chdir", workDir)
	}
	bwrapArgs = append(bwrapArgs, "--", name)
	bwrapArgs = append(bwrapArgs, args...)
	return bwrapArgs
}

// BuildSeatbeltProfile constructs an Apple Seatbelt (SBPL) policy string for
// macOS sandbox-exec enforcing default-deny, workspace read/write containment,
// explicit credential directory denials, and optional loopback proxy egress.
func BuildSeatbeltProfile(policy OSSandboxPolicy, homeDir, workDir, proxyURL string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow signal (target self))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow file-read*\n")
	for _, sysPath := range []string{"/usr", "/bin", "/sbin", "/System", "/Library", "/private/var/db", "/opt/homebrew"} {
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(sysPath))
	}
	b.WriteString("  (literal \"/dev/null\")\n")
	b.WriteString("  (literal \"/dev/urandom\")\n")
	b.WriteString("  (literal \"/dev/random\"))\n")
	b.WriteString("(allow file-read* file-write*\n")
	fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(policy.WorkspaceRoot))
	if workDir != "" && workDir != policy.WorkspaceRoot {
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(workDir))
	}
	b.WriteString("  (subpath \"/private/tmp\")\n")
	b.WriteString("  (subpath \"/tmp\"))\n")

	for _, ro := range policy.ReadOnlyPaths {
		ro = strings.TrimSpace(ro)
		if ro != "" {
			fmt.Fprintf(&b, "(allow file-read* (subpath \"%s\"))\n", escapeSBPLString(ro))
		}
	}

	blocked := policy.BlockedPaths
	if len(blocked) == 0 {
		blocked = DefaultSandboxBlockedPaths()
	}
	seenDenied := map[string]bool{}
	denyPath := func(p string) {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "/" || seenDenied[p] {
			return
		}
		seenDenied[p] = true
		fmt.Fprintf(&b, "(deny file-read* file-write* (subpath \"%s\"))\n", escapeSBPLString(p))
	}
	for _, bl := range blocked {
		bl = strings.TrimSpace(bl)
		if bl == "" {
			continue
		}
		if filepath.IsAbs(bl) {
			denyPath(bl)
			continue
		}
		if homeDir != "" {
			denyPath(filepath.Join(homeDir, filepath.FromSlash(bl)))
		}
		if policy.WorkspaceRoot != "" {
			denyPath(filepath.Join(policy.WorkspaceRoot, filepath.FromSlash(bl)))
		}
	}

	if policy.AllowNetwork {
		if proxyURL != "" {
			if u, err := url.Parse(proxyURL); err == nil && u.Port() != "" {
				fmt.Fprintf(&b, "(allow network-outbound (remote ip \"localhost:%s\"))\n", u.Port())
			} else {
				b.WriteString("(allow network-outbound (remote ip \"localhost:*\"))\n")
			}
		} else {
			b.WriteString("(allow network-outbound)\n")
		}
	}

	return b.String()
}

func escapeSBPLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func runOSSandboxPlatform(ctx context.Context, policy OSSandboxPolicy, workDir, name string, args, env []string, proxyURL string) (OSSandboxResult, error) {
	res := OSSandboxResult{
		ExitCode:       -1,
		SandboxBackend: defaultSandboxBackendName(),
	}

	homeDir, _ := os.UserHomeDir()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if bwrapPath, err := exec.LookPath("bwrap"); err == nil {
			res.SandboxBackend = "linux-bwrap"
			bwrapArgs := BuildBubblewrapArgs(policy, homeDir, workDir, name, args)
			cmd = exec.Command(bwrapPath, bwrapArgs...)
		} else {
			res.SandboxBackend = "linux-pgid"
			cmd = exec.Command(name, args...)
		}
	case "darwin":
		if sbPath, err := exec.LookPath("sandbox-exec"); err == nil {
			res.SandboxBackend = "darwin-seatbelt"
			profile := BuildSeatbeltProfile(policy, homeDir, workDir, proxyURL)
			sbArgs := append([]string{"-p", profile, name}, args...)
			cmd = exec.Command(sbPath, sbArgs...)
		} else {
			res.SandboxBackend = "darwin-pgid"
			cmd = exec.Command(name, args...)
		}
	default:
		res.SandboxBackend = "unix-pgid"
		cmd = exec.Command(name, args...)
	}

	cmd.Dir = workDir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := newBoundedBuffer(defaultSandboxMaxOutput)
	stderr := newBoundedBuffer(defaultSandboxMaxOutput)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return res, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		res.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		waitErr = ctx.Err()
	}

	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	} else if waitErr == nil {
		res.ExitCode = 0
	}
	if res.TimedOut {
		return res, fmt.Errorf("sandbox execution timed out after %s: %w", policy.Timeout, waitErr)
	}
	if waitErr != nil {
		return res, waitErr
	}
	return res, nil
}
