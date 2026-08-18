// SPDX-License-Identifier: Apache-2.0
//go:build windows

package benchharness

import (
	"fmt"
	"os/exec"
	"syscall"
)

const benchmarkCreateNewProcessGroup = 0x00000200

func prepareBenchmarkCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: benchmarkCreateNewProcessGroup}
}

func killBenchmarkCommandTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// taskkill /T terminates descendants as well as the direct adapter process.
	// Ignore taskkill's own output; Process.Kill remains the final fallback.
	killer := exec.Command("taskkill", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F")
	_ = killer.Run()
	return cmd.Process.Kill()
}
