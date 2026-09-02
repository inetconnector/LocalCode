// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type VMSandboxBackend string

const (
	VMSandboxBackendNone VMSandboxBackend = "none"
	VMSandboxBackendQEMU VMSandboxBackend = "qemu"
	VMSandboxBackendWSL  VMSandboxBackend = "wsl"
	VMSandboxBackendAuto VMSandboxBackend = "auto"
)

type VMSandboxConfig struct {
	Backend        VMSandboxBackend `json:"backend"`
	QEMUPath       string           `json:"qemu_path,omitempty"`
	WSLPath        string           `json:"wsl_path,omitempty"`
	MemoryMB       int              `json:"memory_mb"`
	CPUs           int              `json:"cpus"`
	NetworkMode    string           `json:"network_mode"` // "none" (isolated) | "user" (restricted outbound)
	TimeoutSeconds int              `json:"timeout_seconds"`
	BaseImage      string           `json:"base_image,omitempty"`
	OverlayEnabled bool             `json:"overlay_enabled"`
}

type VMCapabilities struct {
	HyperVAvailable bool     `json:"hyperv_available"`
	QEMUAvailable   bool     `json:"qemu_available"`
	QEMUPath        string   `json:"qemu_path,omitempty"`
	QEMUVersion     string   `json:"qemu_version,omitempty"`
	WSLAvailable    bool     `json:"wsl_available"`
	WSLPath         string   `json:"wsl_path,omitempty"`
	Preferred       string   `json:"preferred_backend"`
	SupportedModes  []string `json:"supported_modes"`
}

func defaultVMSandboxConfig() VMSandboxConfig {
	return VMSandboxConfig{
		Backend:        VMSandboxBackendAuto,
		MemoryMB:       2048,
		CPUs:           2,
		NetworkMode:    "none",
		TimeoutSeconds: 600,
		OverlayEnabled: true,
	}
}

func discoverVMCapabilities(ctx context.Context, cfg Config) VMCapabilities {
	caps := VMCapabilities{
		SupportedModes: []string{"none", "process"},
	}

	// 1. QEMU check
	qemuCandidates := []string{
		"qemu-system-x86_64",
		"C:\\Program Files\\qemu\\qemu-system-x86_64.exe",
		filepath.Join(appDataDir(), "tools", "qemu", "qemu-system-x86_64.exe"),
	}
	if override, ok := cfg.ToolOverrides["qemu"]; ok && override != "" {
		qemuCandidates = append([]string{override}, qemuCandidates...)
	}

	for _, candidate := range qemuCandidates {
		if path, err := exec.LookPath(candidate); err == nil {
			caps.QEMUAvailable = true
			caps.QEMUPath = path
			if out, err := exec.CommandContext(ctx, path, "--version").Output(); err == nil {
				caps.QEMUVersion = strings.TrimSpace(strings.Split(string(out), "\n")[0])
			}
			caps.SupportedModes = append(caps.SupportedModes, "qemu-isolated")
			break
		}
	}

	// 2. WSL check on Windows
	if runtime.GOOS == "windows" {
		wslCandidates := []string{
			"wsl.exe",
			"C:\\Windows\\System32\\wsl.exe",
		}
		if override, ok := cfg.ToolOverrides["wsl"]; ok && override != "" {
			wslCandidates = append([]string{override}, wslCandidates...)
		}
		for _, candidate := range wslCandidates {
			if path, err := exec.LookPath(candidate); err == nil {
				caps.WSLAvailable = true
				caps.WSLPath = path
				caps.HyperVAvailable = true
				caps.SupportedModes = append(caps.SupportedModes, "wsl2-container")
				break
			}
		}
	}

	if caps.QEMUAvailable {
		caps.Preferred = "qemu"
	} else if caps.WSLAvailable {
		caps.Preferred = "wsl"
	} else {
		caps.Preferred = "process"
	}

	return caps
}

type VMSandboxResult struct {
	Backend    string        `json:"backend"`
	Command    []string      `json:"command"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration"`
	Output     string        `json:"output"`
	Successful bool          `json:"successful"`
	TimedOut   bool          `json:"timed_out,omitempty"`
	Ephemeral  bool          `json:"ephemeral_overlay"`
}

func runVMSandboxTask(ctx context.Context, project string, taskCommand []string, sandboxCfg VMSandboxConfig, globalCfg Config) (VMSandboxResult, error) {
	started := time.Now()
	res := VMSandboxResult{
		Backend:   string(sandboxCfg.Backend),
		Ephemeral: sandboxCfg.OverlayEnabled,
	}

	if len(taskCommand) == 0 {
		return res, errors.New("empty task command")
	}

	timeoutSec := sandboxCfg.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	taskCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	caps := discoverVMCapabilities(taskCtx, globalCfg)

	selectedBackend := sandboxCfg.Backend
	if selectedBackend == VMSandboxBackendAuto {
		if caps.QEMUAvailable {
			selectedBackend = VMSandboxBackendQEMU
		} else if caps.WSLAvailable {
			selectedBackend = VMSandboxBackendWSL
		} else {
			selectedBackend = VMSandboxBackendNone
		}
	}

	res.Backend = string(selectedBackend)

	switch selectedBackend {
	case VMSandboxBackendQEMU:
		if !caps.QEMUAvailable {
			return res, errors.New("QEMU backend selected but qemu-system-x86_64 is not installed")
		}
		qemuArgs := []string{
			"-m", strconv.Itoa(sandboxCfg.MemoryMB),
			"-smp", strconv.Itoa(sandboxCfg.CPUs),
			"-nographic",
		}
		if sandboxCfg.NetworkMode == "none" || !globalCfg.NetworkEnabled {
			qemuArgs = append(qemuArgs, "-net", "none")
		} else {
			qemuArgs = append(qemuArgs, "-net", "nic", "-net", "user")
		}
		if sandboxCfg.BaseImage != "" {
			if sandboxCfg.OverlayEnabled {
				qemuArgs = append(qemuArgs, "-snapshot", sandboxCfg.BaseImage)
			} else {
				qemuArgs = append(qemuArgs, sandboxCfg.BaseImage)
			}
		}
		res.Command = append([]string{caps.QEMUPath}, qemuArgs...)
		cmd := exec.CommandContext(taskCtx, caps.QEMUPath, qemuArgs...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		res.Duration = time.Since(started)
		res.Output = string(out)
		if taskCtx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
			return res, fmt.Errorf("VM sandbox execution timed out after %ds", timeoutSec)
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				res.ExitCode = exitErr.ExitCode()
			} else {
				res.ExitCode = 1
			}
			return res, err
		}
		res.Successful = true
		return res, nil

	case VMSandboxBackendWSL:
		if !caps.WSLAvailable {
			return res, errors.New("WSL backend selected but wsl.exe is not available")
		}
		wslArgs := append([]string{"-e"}, taskCommand...)
		res.Command = append([]string{caps.WSLPath}, wslArgs...)
		cmd := exec.CommandContext(taskCtx, caps.WSLPath, wslArgs...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		res.Duration = time.Since(started)
		res.Output = string(out)
		if taskCtx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
			return res, fmt.Errorf("WSL sandbox execution timed out after %ds", timeoutSec)
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				res.ExitCode = exitErr.ExitCode()
			} else {
				res.ExitCode = 1
			}
			return res, err
		}
		res.Successful = true
		return res, nil

	default:
		// Process isolation within project directory
		res.Command = taskCommand
		head := taskCommand[0]
		args := taskCommand[1:]
		cmd := exec.CommandContext(taskCtx, head, args...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		res.Duration = time.Since(started)
		res.Output = string(out)
		if taskCtx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
			return res, fmt.Errorf("sandbox execution timed out after %ds", timeoutSec)
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				res.ExitCode = exitErr.ExitCode()
			} else {
				res.ExitCode = 1
			}
			return res, err
		}
		res.Successful = true
		return res, nil
	}
}
