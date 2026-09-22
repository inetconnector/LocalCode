// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func TestNotifyIconDataStructSize(t *testing.T) {
	var nid NOTIFYICONDATAW
	size := unsafe.Sizeof(nid)
	// On Windows amd64 NOTIFYICONDATAW is >= 500 bytes and <= 1024 bytes
	if size < 500 || size > 1024 {
		t.Fatalf("unexpected NOTIFYICONDATAW struct size: %d", size)
	}
}

func TestLoadAppIcon(t *testing.T) {
	hIcon := loadAppIcon()
	if hIcon == 0 {
		t.Log("loadAppIcon returned 0 (expected in headless test environment)")
	}
}

func TestTrayWndProcMock(t *testing.T) {
	tm := NewTrayManager("http://127.0.0.1:4040", "de", true)
	activeTrayMu.Lock()
	activeTray = tm
	activeTrayMu.Unlock()
	defer func() {
		activeTrayMu.Lock()
		activeTray = nil
		activeTrayMu.Unlock()
	}()

	var openCalled atomic.Bool
	origOpenHook := openBrowserMaximizedHook
	openBrowserMaximizedHook = func(url string) error {
		openCalled.Store(true)
		return nil
	}
	defer func() { openBrowserMaximizedHook = origOpenHook }()

	// Test WM_TRAY_CALLBACK with double click
	ret := trayWndProc(0, WM_TRAY_CALLBACK, 0, WM_LBUTTONDBLCLK)
	if ret != 0 {
		t.Errorf("expected 0, got %d", ret)
	}
	if !openCalled.Load() {
		t.Errorf("expected OpenUI to be called on double click")
	}

	// Test WM_TRAY_CALLBACK with right click
	ret = trayWndProc(0, WM_TRAY_CALLBACK, 0, WM_RBUTTONUP)
	if ret != 0 {
		t.Errorf("expected 0, got %d", ret)
	}

	// Test WM_TRAY_CALLBACK with context menu msg
	ret = trayWndProc(0, WM_TRAY_CALLBACK, 0, WM_CONTEXTMENU)
	if ret != 0 {
		t.Errorf("expected 0, got %d", ret)
	}

	// Test WM_TRAY_CALLBACK with unexpected mouse event
	ret = trayWndProc(0, WM_TRAY_CALLBACK, 0, 0x0200) // WM_MOUSEMOVE
	if ret != 0 {
		t.Errorf("expected 0, got %d", ret)
	}

	// Test other message (delegated to DefWindowProcW)
	_ = trayWndProc(0, WM_APP, 0, 0)
}

func TestTrayManagerRunLifecycle(t *testing.T) {
	tm := NewTrayManager("http://127.0.0.1:4040", "en", true)

	errCh := make(chan error, 1)
	go func() {
		errCh <- tm.Run()
	}()

	select {
	case <-tm.readyCh:
		// Tray manager is running and window message loop is active
		if tm.hwnd == 0 {
			t.Errorf("expected tm.hwnd to be non-zero")
		}
		// Test showContextMenu
		tm.showContextMenu()
		// Now stop the tray manager
		tm.Stop()
	case err := <-errCh:
		if err != nil {
			t.Fatalf("tm.Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tray manager to be ready")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("tm.Run exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tray manager to stop")
	}
}
