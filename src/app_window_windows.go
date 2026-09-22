// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	appWindowMu      sync.Mutex
	appWindowVisitor func(uintptr)
	// Allocate one callback for the lifetime of the process, not one per launch.
	appWindowCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		appWindowVisitor(hwnd)
		return 1
	})
	appEnumWindows   = modUser32Sys.NewProc("EnumWindows")
	appWindowVisible = modUser32Sys.NewProc("IsWindowVisible")
	appWindowTitle   = modUser32Sys.NewProc("GetWindowTextW")
	appWindowClass   = modUser32Sys.NewProc("GetClassNameW")
	appWindowPID     = modUser32Sys.NewProc("GetWindowThreadProcessId")
	appShowWindow    = modUser32Sys.NewProc("ShowWindowAsync")
	appProcessImage  = modKernel32Sys.NewProc("QueryFullProcessImageNameW")
)

func isLocalCodeAppWindow(title, class, image, browser string) bool {
	return title == "LocalCode" && class == "Chrome_WidgetWin_1" &&
		strings.EqualFold(filepath.Clean(image), filepath.Clean(browser))
}

func localCodeAppWindows(browser string) []uintptr {
	appWindowMu.Lock()
	defer appWindowMu.Unlock()
	var handles []uintptr
	appWindowVisitor = func(hwnd uintptr) {
		visible, _, _ := appWindowVisible.Call(hwnd)
		if visible == 0 {
			return
		}
		var title, class [256]uint16
		appWindowTitle.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		appWindowClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if syscall.UTF16ToString(title[:]) != "LocalCode" || syscall.UTF16ToString(class[:]) != "Chrome_WidgetWin_1" {
			return
		}
		var pid uint32
		appWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		process, err := syscall.OpenProcess(0x1000, false, pid) // PROCESS_QUERY_LIMITED_INFORMATION
		if err != nil {
			return
		}
		defer syscall.CloseHandle(process)
		var image [32768]uint16
		size := uint32(len(image))
		ok, _, _ := appProcessImage.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&image[0])), uintptr(unsafe.Pointer(&size)))
		if ok != 0 && isLocalCodeAppWindow(syscall.UTF16ToString(title[:]), syscall.UTF16ToString(class[:]), syscall.UTF16ToString(image[:]), browser) {
			handles = append(handles, hwnd)
		}
	}
	appEnumWindows.Call(appWindowCallback, 0)
	appWindowVisitor = nil
	return handles
}

func maximizeLocalCodeAppWindows(browser string) {
	// Catch both restored windows and late-created app windows. Each window is
	// maximized only once per launch so subsequent manual resizing remains possible.
	seen := map[uintptr]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, hwnd := range localCodeAppWindows(browser) {
			if !seen[hwnd] {
				ok, _, _ := appShowWindow.Call(hwnd, 3) // SW_MAXIMIZE
				seen[hwnd] = ok != 0
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
