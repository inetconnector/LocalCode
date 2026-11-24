// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	// Linux PTY ioctl request codes (amd64/arm64/386/arm).
	linuxTIOCSPTLCK uintptr = 0x40045431
	linuxTIOCGPTN   uintptr = 0x80045430

	// Darwin (macOS) PTY ioctl request codes.
	darwinTIOCPTYGRANT uintptr = 0x20007454
	darwinTIOCPTYUNLK  uintptr = 0x20007452
	darwinTIOCPTYGNAME uintptr = 0x40807453
)

type unixWinsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func defaultUnixPTYShell() string {
	if sh := strings.TrimSpace(os.Getenv("SHELL")); sh != "" {
		if st, err := os.Stat(sh); err == nil && !st.IsDir() {
			return sh
		}
	}
	for _, candidate := range []string{"/bin/bash", "/bin/zsh", "/bin/sh"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return "/bin/sh"
}

func startPTYProcess(workspaceRoot string, cols, rows int) (ptyBackend, error) {
	if backend, err := startUnixPTY(workspaceRoot, cols, rows); err == nil {
		return backend, nil
	}
	return startUnixPipeFallback(workspaceRoot, cols, rows)
}

func openUnixPTYPair() (*os.File, *os.File, error) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	var slavePath string
	switch runtime.GOOS {
	case "linux":
		var unlock int32
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), linuxTIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("TIOCSPTLCK: %w", errno)
		}
		var ptyNum uint32
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), linuxTIOCGPTN, uintptr(unsafe.Pointer(&ptyNum))); errno != 0 {
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("TIOCGPTN: %w", errno)
		}
		slavePath = fmt.Sprintf("/dev/pts/%d", ptyNum)
	case "darwin":
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), darwinTIOCPTYGRANT, 0); errno != 0 {
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("TIOCPTYGRANT: %w", errno)
		}
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), darwinTIOCPTYUNLK, 0); errno != 0 {
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("TIOCPTYUNLK: %w", errno)
		}
		var nameBuf [128]byte
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), darwinTIOCPTYGNAME, uintptr(unsafe.Pointer(&nameBuf[0]))); errno != 0 {
			_ = ptmx.Close()
			return nil, nil, fmt.Errorf("TIOCPTYGNAME: %w", errno)
		}
		if idx := bytes.IndexByte(nameBuf[:], 0); idx >= 0 {
			slavePath = string(nameBuf[:idx])
		} else {
			slavePath = string(nameBuf[:])
		}
	default:
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("unsupported unix PTY platform: %s", runtime.GOOS)
	}

	if slavePath == "" {
		_ = ptmx.Close()
		return nil, nil, errors.New("empty PTY slave path")
	}
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("open slave PTY %s: %w", slavePath, err)
	}
	return ptmx, slave, nil
}

func setUnixWinsize(fd uintptr, cols, rows int) error {
	ws := unixWinsize{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return errno
	}
	return nil
}

type unixPTYBackend struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	mu        sync.Mutex
	closed    bool
	waitOnce  sync.Once
	exitCode  int
	waitErr   error
	closeOnce sync.Once
}

func startUnixPTY(workspaceRoot string, cols, rows int) (*unixPTYBackend, error) {
	ptmx, slave, err := openUnixPTYPair()
	if err != nil {
		return nil, err
	}
	_ = setUnixWinsize(ptmx.Fd(), cols, rows)

	shell := defaultUnixPTYShell()
	cmd := exec.Command(shell)
	cmd.Dir = workspaceRoot
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		_ = slave.Close()
		_ = ptmx.Close()
		return nil, err
	}
	// Parent closes slave side immediately so EOF propagates when the child exits.
	_ = slave.Close()

	return &unixPTYBackend{
		ptmx: ptmx,
		cmd:  cmd,
	}, nil
}

func (u *unixPTYBackend) BackendName() string { return "unix-pty" }

func (u *unixPTYBackend) Read(p []byte) (int, error) {
	return u.ptmx.Read(p)
}

func (u *unixPTYBackend) Write(p []byte) (int, error) {
	return u.ptmx.Write(p)
}

func (u *unixPTYBackend) Resize(cols, rows int) error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return ErrPTYSessionClosed
	}
	fd := u.ptmx.Fd()
	u.mu.Unlock()
	return setUnixWinsize(fd, cols, rows)
}

func (u *unixPTYBackend) Wait() (int, error) {
	u.waitOnce.Do(func() {
		err := u.cmd.Wait()
		u.waitErr = err
		if u.cmd.ProcessState != nil {
			u.exitCode = u.cmd.ProcessState.ExitCode()
		} else if err != nil {
			u.exitCode = 1
		}
	})
	return u.exitCode, u.waitErr
}

func (u *unixPTYBackend) Close() error {
	u.closeOnce.Do(func() {
		u.mu.Lock()
		u.closed = true
		u.mu.Unlock()

		if u.cmd != nil && u.cmd.Process != nil {
			_ = syscall.Kill(-u.cmd.Process.Pid, syscall.SIGHUP)
			_ = syscall.Kill(-u.cmd.Process.Pid, syscall.SIGKILL)
			_ = u.cmd.Process.Kill()
		}
		_ = u.ptmx.Close()
	})
	return nil
}

type unixPipeFallback struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	waitOnce  sync.Once
	exitCode  int
	waitErr   error
	closeOnce sync.Once
}

func startUnixPipeFallback(workspaceRoot string, cols, rows int) (*unixPipeFallback, error) {
	cmd := exec.Command(defaultUnixPTYShell())
	cmd.Dir = workspaceRoot
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
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

	b := &unixPipeFallback{
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

func (b *unixPipeFallback) BackendName() string { return "pipe-fallback" }

func (b *unixPipeFallback) Read(p []byte) (int, error) { return b.stdout.Read(p) }

func (b *unixPipeFallback) Write(p []byte) (int, error) { return b.stdin.Write(p) }

func (b *unixPipeFallback) Resize(cols, rows int) error { return nil }

func (b *unixPipeFallback) Wait() (int, error) {
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

func (b *unixPipeFallback) Close() error {
	b.closeOnce.Do(func() {
		_ = b.stdin.Close()
		killProcessTree(b.cmd)
		_ = b.stdout.Close()
	})
	return nil
}
