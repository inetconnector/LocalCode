// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformationClass = 9

	jobObjectLimitActiveProcess           = 0x00000008
	jobObjectLimitJobMemory               = 0x00000200
	jobObjectLimitDieOnUnhandledException = 0x00000400
	jobObjectLimitKillOnJobClose          = 0x00002000

	winProcessTerminate               = 0x0001
	winProcessSetQuota                = 0x0100
	winProcessQueryLimitedInformation = 0x1000
	winStillActive                    = 259
)

var (
	sandboxKernel32              = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = sandboxKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = sandboxKernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = sandboxKernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = sandboxKernel32.NewProc("TerminateJobObject")
	procOpenProcess              = sandboxKernel32.NewProc("OpenProcess")
	procGetExitCodeProcess       = sandboxKernel32.NewProc("GetExitCodeProcess")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectIOCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                jobObjectIOCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type windowsJobObject struct {
	handle syscall.Handle
}

func createWindowsSandboxJob(policy OSSandboxPolicy) (*windowsJobObject, error) {
	r1, _, err := procCreateJobObjectW.Call(0, 0)
	if r1 == 0 {
		return nil, fmt.Errorf("CreateJobObjectW failed: %w", err)
	}
	job := &windowsJobObject{handle: syscall.Handle(r1)}

	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose | jobObjectLimitDieOnUnhandledException

	if policy.MaxProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitActiveProcess
		info.BasicLimitInformation.ActiveProcessLimit = uint32(policy.MaxProcesses)
	}
	if policy.MemoryLimitMB > 0 {
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitJobMemory
		info.JobMemoryLimit = uintptr(policy.MemoryLimitMB) * 1024 * 1024
	}

	ok, _, setErr := procSetInformationJobObject.Call(
		uintptr(job.handle),
		uintptr(jobObjectExtendedLimitInformationClass),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if ok == 0 {
		_ = job.Close()
		return nil, fmt.Errorf("SetInformationJobObject failed: %w", setErr)
	}
	return job, nil
}

func (j *windowsJobObject) AssignPID(pid int) error {
	if j == nil || j.handle == 0 || pid <= 0 {
		return errors.New("invalid job object or process id")
	}
	hProc, _, openErr := procOpenProcess.Call(
		uintptr(winProcessSetQuota|winProcessTerminate),
		0,
		uintptr(uint32(pid)),
	)
	if hProc == 0 {
		return fmt.Errorf("OpenProcess(%d) failed: %w", pid, openErr)
	}
	defer syscall.CloseHandle(syscall.Handle(hProc))

	ok, _, assignErr := procAssignProcessToJobObject.Call(uintptr(j.handle), hProc)
	if ok == 0 {
		return fmt.Errorf("AssignProcessToJobObject(%d) failed: %w", pid, assignErr)
	}
	return nil
}

func (j *windowsJobObject) Terminate(exitCode uint32) error {
	if j == nil || j.handle == 0 {
		return nil
	}
	ok, _, err := procTerminateJobObject.Call(uintptr(j.handle), uintptr(exitCode))
	if ok == 0 {
		return fmt.Errorf("TerminateJobObject failed: %w", err)
	}
	return nil
}

func (j *windowsJobObject) Close() error {
	if j == nil || j.handle == 0 {
		return nil
	}
	h := j.handle
	j.handle = 0
	return syscall.CloseHandle(h)
}

func isSandboxProcessAlive(pid int) bool {
	return isWindowsProcessAlive(pid)
}

func isWindowsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	hProc, _, _ := procOpenProcess.Call(uintptr(winProcessQueryLimitedInformation), 0, uintptr(uint32(pid)))
	if hProc == 0 {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(hProc))
	var code uint32
	ok, _, _ := procGetExitCodeProcess.Call(hProc, uintptr(unsafe.Pointer(&code)))
	if ok == 0 {
		return false
	}
	return code == winStillActive
}

func defaultSandboxBackendName() string {
	return "windows-job-object"
}

func runOSSandboxPlatform(ctx context.Context, policy OSSandboxPolicy, workDir, name string, args, env []string, _ string) (OSSandboxResult, error) {
	res := OSSandboxResult{
		ExitCode:       -1,
		SandboxBackend: defaultSandboxBackendName(),
	}

	job, err := createWindowsSandboxJob(policy)
	if err != nil {
		return res, fmt.Errorf("initialize windows job object sandbox: %w", err)
	}
	defer job.Close()

	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	cmd.Env = env
	hideCommandWindow(cmd)

	stdout := newBoundedBuffer(defaultSandboxMaxOutput)
	stderr := newBoundedBuffer(defaultSandboxMaxOutput)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return res, err
	}

	// Immediately bind the spawned process into the Win32 Job Object so all
	// descendants inherit the job limits and are killed together on termination.
	if assignErr := job.AssignPID(cmd.Process.Pid); assignErr != nil {
		// If the process already exited before assignment, Wait() will reap it;
		// only treat assignment failure as fatal while the process is still alive.
		if isWindowsProcessAlive(cmd.Process.Pid) {
			killProcessTree(cmd)
			_ = cmd.Wait()
			return res, fmt.Errorf("assign process to sandbox job object: %w", assignErr)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		termErr := job.Terminate(1)
		_ = job.Close()
		if termErr != nil || (cmd.Process != nil && isWindowsProcessAlive(cmd.Process.Pid)) {
			killProcessTree(cmd)
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
