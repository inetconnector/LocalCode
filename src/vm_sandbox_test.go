// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverVMCapabilities(t *testing.T) {
	tools := t.TempDir()
	mockQEMU := writeWindowsCmdFixture(t, tools, "qemu-system-x86_64", `
if "%1"=="--version" (
  echo QEMU emulator version 8.2.0
  exit /b 0
)
exit /b 0`)

	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{
		"qemu": mockQEMU,
	}

	caps := discoverVMCapabilities(context.Background(), cfg)
	if !caps.QEMUAvailable {
		t.Fatalf("expected QEMU to be discovered via ToolOverrides, got: %#v", caps)
	}
	if !strings.Contains(caps.QEMUVersion, "8.2.0") {
		t.Errorf("expected QEMU version 8.2.0, got %q", caps.QEMUVersion)
	}
	if caps.Preferred != "qemu" {
		t.Errorf("expected preferred qemu, got %q", caps.Preferred)
	}
}

func TestRunVMSandboxTaskProcessIsolation(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	sandboxCfg := defaultVMSandboxConfig()
	sandboxCfg.Backend = VMSandboxBackendNone

	// Run echo command
	var taskCmd []string
	if runtime.GOOS == "windows" {
		taskCmd = []string{"cmd.exe", "/c", "echo", "SANDBOX_PROCESS_OK"}
	} else {
		taskCmd = []string{"echo", "SANDBOX_PROCESS_OK"}
	}

	res, err := runVMSandboxTask(context.Background(), project, taskCmd, sandboxCfg, cfg)
	if err != nil {
		t.Fatalf("runVMSandboxTask failed: %v", err)
	}
	if !res.Successful || !strings.Contains(res.Output, "SANDBOX_PROCESS_OK") {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestRunVMSandboxTaskMockQEMU(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()

	mockQEMU := writeWindowsCmdFixture(t, tools, "qemu-system-x86_64", `
echo MOCK_QEMU_RUNNING: %*
exit /b 0`)

	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{
		"qemu": mockQEMU,
	}

	sandboxCfg := defaultVMSandboxConfig()
	sandboxCfg.Backend = VMSandboxBackendQEMU
	sandboxCfg.BaseImage = filepath.Join(project, "base.qcow2")
	sandboxCfg.OverlayEnabled = true

	res, err := runVMSandboxTask(context.Background(), project, []string{"build-kernel"}, sandboxCfg, cfg)
	if err != nil {
		t.Fatalf("runVMSandboxTask failed: %v", err)
	}
	if !res.Successful || !strings.Contains(res.Output, "MOCK_QEMU_RUNNING") {
		t.Fatalf("unexpected QEMU result: %#v", res)
	}
}

func TestRunVMSandboxTaskErrors(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()

	// 1. Empty command
	_, err := runVMSandboxTask(context.Background(), project, nil, defaultVMSandboxConfig(), cfg)
	if err == nil {
		t.Error("expected error for empty command")
	}

	// 2. QEMU backend without QEMU available
	badSandboxCfg := defaultVMSandboxConfig()
	badSandboxCfg.Backend = VMSandboxBackendQEMU
	badCfg := defaultConfig()
	badCfg.ToolOverrides = map[string]string{"qemu": "C:\\nonexistent\\qemu.exe"}
	_, err = runVMSandboxTask(context.Background(), project, []string{"test"}, badSandboxCfg, badCfg)
	if err == nil {
		t.Error("expected error when QEMU is missing")
	}
}
