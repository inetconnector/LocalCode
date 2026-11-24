// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	winProcThreadAttributePseudoConsole = 0x00020016
	winExtendedStartupInfoPresent       = 0x00080000
)

var (
	ptyKernel32                        = syscall.NewLazyDLL("kernel32.dll")
	procCreatePseudoConsole            = ptyKernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole            = ptyKernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole             = ptyKernel32.NewProc("ClosePseudoConsole")
	procInitProcThreadAttributeList    = ptyKernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute      = ptyKernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList  = ptyKernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateProcessW                 = ptyKernel32.NewProc("CreateProcessW")
)

type winStartupInfoExW struct {
	StartupInfo   syscall.StartupInfo
	AttributeList *byte
}

func packWinCoord(cols, rows int) uintptr {
	x := uint16(int16(cols))
	y := uint16(int16(rows))
	return uintptr(uint32(x) | (uint32(y) << 16))
}

func conPTYAvailable() bool {
	return procCreatePseudoConsole.Find() == nil &&
		procResizePseudoConsole.Find() == nil &&
		procClosePseudoConsole.Find() == nil &&
		procInitProcThreadAttributeList.Find() == nil &&
		procUpdateProcThreadAttribute.Find() == nil &&
		procDeleteProcThreadAttributeList.Find() == nil &&
		procCreateProcessW.Find() == nil
}

func defaultWindowsPTYShell() string {
	if comspec := strings.TrimSpace(os.Getenv("COMSPEC")); comspec != "" {
		if st, err := os.Stat(comspec); err == nil && !st.IsDir() {
			return comspec
		}
	}
	if p, err := exec.LookPath("cmd.exe"); err == nil {
		return p
	}
	return "cmd.exe"
}

func startPTYProcess(workspaceRoot string, cols, rows int) (ptyBackend, error) {
	if conPTYAvailable() {
		backend, err := startWindowsConPTY(workspaceRoot, cols, rows)
		if err == nil {
			return backend, nil
		}
		// Fall back to interactive pipe shell if ConPTY creation fails on restricted/older environments.
	}
	return startWindowsPipeFallback(workspaceRoot, cols, rows)
}

type windowsConPTY struct {
	hPC        uintptr
	hProcess   syscall.Handle
	inFile     *os.File
	outFile    *os.File
	mu         sync.Mutex
	pcClosed   bool
	closed     bool
	exitCode   int
	waitErr    error
	waitDone   chan struct{}
	closeOnce  sync.Once
}

func startWindowsConPTY(workspaceRoot string, cols, rows int) (*windowsConPTY, error) {
	var ptyInRead, ptyInWrite syscall.Handle
	var ptyOutRead, ptyOutWrite syscall.Handle

	if err := syscall.CreatePipe(&ptyInRead, &ptyInWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe(in): %w", err)
	}
	if err := syscall.CreatePipe(&ptyOutRead, &ptyOutWrite, nil, 0); err != nil {
		_ = syscall.CloseHandle(ptyInRead)
		_ = syscall.CloseHandle(ptyInWrite)
		return nil, fmt.Errorf("CreatePipe(out): %w", err)
	}

	var hPC uintptr
	r1, _, callErr := procCreatePseudoConsole.Call(
		packWinCoord(cols, rows),
		uintptr(ptyInRead),
		uintptr(ptyOutWrite),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)
	if int32(r1) != 0 {
		_ = syscall.CloseHandle(ptyInRead)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		_ = syscall.CloseHandle(ptyOutWrite)
		return nil, fmt.Errorf("CreatePseudoConsole failed (HRESULT=0x%x): %v", uint32(r1), callErr)
	}

	// Close the ConPTY-side ends now that CreatePseudoConsole has duplicated/adopted them.
	_ = syscall.CloseHandle(ptyInRead)
	_ = syscall.CloseHandle(ptyOutWrite)

	var attrListSize uintptr
	_, _, _ = procInitProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrListSize)))
	if attrListSize == 0 {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, errors.New("InitializeProcThreadAttributeList returned zero size")
	}

	attrList := make([]byte, attrListSize)
	ok, _, err := procInitProcThreadAttributeList.Call(
		uintptr(unsafe.Pointer(&attrList[0])),
		1,
		0,
		uintptr(unsafe.Pointer(&attrListSize)),
	)
	if ok == 0 {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, fmt.Errorf("InitializeProcThreadAttributeList failed: %w", err)
	}
	defer func() {
		_, _, _ = procDeleteProcThreadAttributeList.Call(uintptr(unsafe.Pointer(&attrList[0])))
		runtime.KeepAlive(attrList)
	}()

	// Note: For PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, Win32 expects the HPCON value itself
	// (not a pointer to HPCON) as lpValue, with cbSize = sizeof(HPCON).
	ok, _, err = procUpdateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(&attrList[0])),
		0,
		winProcThreadAttributePseudoConsole,
		hPC,
		unsafe.Sizeof(hPC),
		0,
		0,
	)
	if ok == 0 {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, fmt.Errorf("UpdateProcThreadAttribute failed: %w", err)
	}

	var siEx winStartupInfoExW
	siEx.StartupInfo.Cb = uint32(unsafe.Sizeof(siEx))
	// Explicitly clear inherited stdio handles when spawning from a process whose
	// own stdin/stdout may be redirected pipes (e.g. HTTP server or go test),
	// so the child attaches strictly to the ConPTY console handles.
	siEx.StartupInfo.Flags = syscall.STARTF_USESTDHANDLES
	siEx.AttributeList = &attrList[0]

	cmdLineSlice, err := syscall.UTF16FromString(defaultWindowsPTYShell())
	if err != nil {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, err
	}
	dirPtr, err := syscall.UTF16PtrFromString(workspaceRoot)
	if err != nil {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, err
	}

	var pi syscall.ProcessInformation
	ok, _, err = procCreateProcessW.Call(
		0,
		uintptr(unsafe.Pointer(&cmdLineSlice[0])),
		0,
		0,
		0,
		winExtendedStartupInfoPresent,
		0,
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&siEx)),
		uintptr(unsafe.Pointer(&pi)),
	)
	runtime.KeepAlive(cmdLineSlice)
	runtime.KeepAlive(dirPtr)
	runtime.KeepAlive(&siEx)

	if ok == 0 {
		_, _, _ = procClosePseudoConsole.Call(hPC)
		_ = syscall.CloseHandle(ptyInWrite)
		_ = syscall.CloseHandle(ptyOutRead)
		return nil, fmt.Errorf("CreateProcessW(ConPTY) failed: %w", err)
	}

	_ = syscall.CloseHandle(pi.Thread)

	c := &windowsConPTY{
		hPC:      hPC,
		hProcess: pi.Process,
		inFile:   os.NewFile(uintptr(ptyInWrite), "conpty-in"),
		outFile:  os.NewFile(uintptr(ptyOutRead), "conpty-out"),
		waitDone: make(chan struct{}),
	}

	go c.watchProcess()
	return c, nil
}

func (c *windowsConPTY) watchProcess() {
	defer close(c.waitDone)

	_, err := syscall.WaitForSingleObject(c.hProcess, syscall.INFINITE)
	var code uint32
	if err == nil {
		if gcErr := syscall.GetExitCodeProcess(c.hProcess, &code); gcErr != nil {
			err = gcErr
		}
	}

	c.mu.Lock()
	c.exitCode = int(code)
	c.waitErr = err
	hProc := c.hProcess
	c.hProcess = 0
	c.mu.Unlock()

	if hProc != 0 {
		_ = syscall.CloseHandle(hProc)
	}
	// Closing the pseudo console after process exit flushes remaining output and signals EOF to Read().
	c.closePseudoConsole()
}

func (c *windowsConPTY) closePseudoConsole() {
	c.mu.Lock()
	if c.pcClosed || c.hPC == 0 {
		c.mu.Unlock()
		return
	}
	c.pcClosed = true
	hPC := c.hPC
	c.hPC = 0
	c.mu.Unlock()

	_, _, _ = procClosePseudoConsole.Call(hPC)
}

func (c *windowsConPTY) BackendName() string { return "conpty" }

func (c *windowsConPTY) Read(p []byte) (int, error) {
	return c.outFile.Read(p)
}

func (c *windowsConPTY) Write(p []byte) (int, error) {
	return c.inFile.Write(p)
}

func (c *windowsConPTY) Resize(cols, rows int) error {
	c.mu.Lock()
	if c.pcClosed || c.hPC == 0 {
		c.mu.Unlock()
		return ErrPTYSessionClosed
	}
	hPC := c.hPC
	c.mu.Unlock()

	r1, _, err := procResizePseudoConsole.Call(hPC, packWinCoord(cols, rows))
	if int32(r1) != 0 {
		return fmt.Errorf("ResizePseudoConsole failed (HRESULT=0x%x): %v", uint32(r1), err)
	}
	return nil
}

func (c *windowsConPTY) Wait() (int, error) {
	<-c.waitDone
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitCode, c.waitErr
}

func (c *windowsConPTY) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		hProc := c.hProcess
		c.mu.Unlock()

		if hProc != 0 {
			_ = syscall.TerminateProcess(hProc, 1)
		}
		_ = c.inFile.Close()
		go c.closePseudoConsole()

		select {
		case <-c.waitDone:
		case <-time.After(2 * time.Second):
		}
		_ = c.outFile.Close()
	})
	return nil
}

// windowsPipeFallback provides an interactive pipe-backed shell fallback for Windows < 1809.
type windowsPipeFallback struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	waitOnce  sync.Once
	exitCode  int
	waitErr   error
	closeOnce sync.Once
}

func startWindowsPipeFallback(workspaceRoot string, cols, rows int) (*windowsPipeFallback, error) {
	cmd := exec.Command(defaultWindowsPTYShell())
	cmd.Dir = workspaceRoot
	hideCommandWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = pw.Close()
		_ = pr.Close()
		return nil, err
	}

	b := &windowsPipeFallback{
		cmd:    cmd,
		stdin:  stdin,
		stdout: pr,
	}
	go func() {
		_, _ = b.Wait()
		_ = pw.Close()
	}()
	return b, nil
}

func (b *windowsPipeFallback) BackendName() string { return "pipe-fallback" }

func (b *windowsPipeFallback) Read(p []byte) (int, error) {
	return b.stdout.Read(p)
}

func (b *windowsPipeFallback) Write(p []byte) (int, error) {
	return b.stdin.Write(p)
}

func (b *windowsPipeFallback) Resize(cols, rows int) error { return nil }

func (b *windowsPipeFallback) Wait() (int, error) {
	b.waitOnce.Do(func() {
		err := b.cmd.Wait()
		b.waitErr = err
		if b.cmd.ProcessState != nil {
			b.exitCode = b.cmd.ProcessState.ExitCode()
		} else if err != nil {
			b.exitCode = 1
		}
	})
	return b.exitCode, b.waitErr
}

func (b *windowsPipeFallback) Close() error {
	b.closeOnce.Do(func() {
		_ = b.stdin.Close()
		killProcessTree(b.cmd)
		_ = b.stdout.Close()
	})
	return nil
}
