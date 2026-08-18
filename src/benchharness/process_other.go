// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package benchharness

import (
	"errors"
	"os/exec"
	"syscall"
)

func prepareBenchmarkCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killBenchmarkCommandTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}
