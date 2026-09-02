// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	cfg.NetworkEnabled = true
	cfg.ToolOverrides = map[string]string{
		"qemu": mockQEMU,
	}

	// 1. QEMU with snapshot overlay and user network
	sandboxCfg := defaultVMSandboxConfig()
	sandboxCfg.Backend = VMSandboxBackendQEMU
	sandboxCfg.NetworkMode = "user"
	sandboxCfg.BaseImage = filepath.Join(project, "base.qcow2")
	sandboxCfg.OverlayEnabled = true

	res, err := runVMSandboxTask(context.Background(), project, []string{"build-kernel"}, sandboxCfg, cfg)
	if err != nil {
		t.Fatalf("runVMSandboxTask failed: %v", err)
	}
	if !res.Successful || !strings.Contains(res.Output, "MOCK_QEMU_RUNNING") {
		t.Fatalf("unexpected QEMU result: %#v", res)
	}

	// 2. QEMU without snapshot overlay and with auto-detection
	sandboxCfg2 := defaultVMSandboxConfig()
	sandboxCfg2.Backend = VMSandboxBackendAuto
	sandboxCfg2.NetworkMode = "none"
	sandboxCfg2.BaseImage = filepath.Join(project, "base.qcow2")
	sandboxCfg2.OverlayEnabled = false

	res2, err := runVMSandboxTask(context.Background(), project, []string{"build-kernel"}, sandboxCfg2, cfg)
	if err != nil {
		t.Fatalf("runVMSandboxTask auto failed: %v", err)
	}
	if !res2.Successful || res2.Backend != "qemu" {
		t.Fatalf("expected successful auto qemu run, got: %#v", res2)
	}
}

func TestRunVMSandboxTaskMockWSL(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()

	mockWSL := writeWindowsCmdFixture(t, tools, "wsl", `
echo MOCK_WSL_RUNNING: %*
exit /b 0`)

	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{
		"wsl": mockWSL,
	}

	// 1. WSL backend explicit
	sandboxCfg := defaultVMSandboxConfig()
	sandboxCfg.Backend = VMSandboxBackendWSL

	res, err := runVMSandboxTask(context.Background(), project, []string{"uname", "-a"}, sandboxCfg, cfg)
	if err != nil {
		t.Fatalf("WSL run failed: %v", err)
	}
	if !res.Successful || !strings.Contains(res.Output, "MOCK_WSL_RUNNING") {
		t.Fatalf("unexpected WSL result: %#v", res)
	}

	// 2. WSL backend auto fallback when QEMU is absent
	sandboxCfgAuto := defaultVMSandboxConfig()
	sandboxCfgAuto.Backend = VMSandboxBackendAuto

	resAuto, err := runVMSandboxTask(context.Background(), project, []string{"make"}, sandboxCfgAuto, cfg)
	if err != nil {
		t.Fatalf("auto WSL run failed: %v", err)
	}
	if !resAuto.Successful || resAuto.Backend != "wsl" {
		t.Fatalf("expected successful auto wsl run, got: %#v", resAuto)
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

	// 3. WSL backend without WSL available
	badWSLSandbox := defaultVMSandboxConfig()
	badWSLSandbox.Backend = VMSandboxBackendWSL
	badWSLCfg := defaultConfig()
	badWSLCfg.ToolOverrides = map[string]string{"wsl": "C:\\nonexistent\\wsl.exe"}
	_, err = runVMSandboxTask(context.Background(), project, []string{"test"}, badWSLSandbox, badWSLCfg)
	if err == nil {
		t.Error("expected error when WSL is missing")
	}

	// 4. Timeout handling
	timeoutSandbox := defaultVMSandboxConfig()
	timeoutSandbox.Backend = VMSandboxBackendNone
	timeoutSandbox.TimeoutSeconds = 1
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	var dummyCmd []string
	if runtime.GOOS == "windows" {
		dummyCmd = []string{"cmd.exe", "/c", "echo", "hi"}
	} else {
		dummyCmd = []string{"echo", "hi"}
	}
	res, err := runVMSandboxTask(ctxCancelled, project, dummyCmd, timeoutSandbox, cfg)
	if err == nil && res.Successful {
		t.Error("expected error for cancelled context")
	}
}
