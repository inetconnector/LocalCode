// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const moveFileWriteThrough = 0x00000008

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	replaceFileWProc = kernel32DLL.NewProc("ReplaceFileW")
	moveFileExWProc  = kernel32DLL.NewProc("MoveFileExW")
)

func replaceFileAtomic(stagedPath, targetPath string, targetExists bool) error {
	staged, err := syscall.UTF16PtrFromString(stagedPath)
	if err != nil {
		return err
	}
	target, err := syscall.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	if targetExists {
		// ReplaceFileW atomically replaces the target with the staged file on the
		// same volume. LocalCode already owns its separate content backup, so the
		// Windows backup-file parameter is intentionally nil.
		ok, _, callErr := replaceFileWProc.Call(
			uintptr(unsafe.Pointer(target)),
			uintptr(unsafe.Pointer(staged)),
			0,
			0,
			0,
			0,
		)
		if ok == 0 {
			return windowsAtomicReplaceError("ReplaceFileW", callErr)
		}
		return nil
	}

	// For a new destination do not request REPLACE_EXISTING: if another process
	// creates the path after optimistic validation, the move fails safely instead
	// of overwriting that new file. WRITE_THROUGH asks Windows not to return until
	// the move has been flushed to disk.
	ok, _, callErr := moveFileExWProc.Call(
		uintptr(unsafe.Pointer(staged)),
		uintptr(unsafe.Pointer(target)),
		uintptr(moveFileWriteThrough),
	)
	if ok == 0 {
		return windowsAtomicReplaceError("MoveFileExW", callErr)
	}
	return nil
}

func windowsAtomicReplaceError(api string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s failed", api)
	}
	return fmt.Errorf("%s failed: %w", api, err)
}
