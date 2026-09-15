// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func TestHasTrayFlag(t *testing.T) {
	testCases := []struct {
		args     []string
		expected bool
	}{
		{args: []string{}, expected: false},
		{args: []string{"--diagnose"}, expected: false},
		{args: []string{"/tray"}, expected: true},
		{args: []string{"/TRAY"}, expected: true},
		{args: []string{"-tray"}, expected: true},
		{args: []string{"--tray"}, expected: true},
		{args: []string{"--other", "/tray"}, expected: true},
		{args: []string{"--other", "tray"}, expected: false},
		{args: []string{"/tray-mode"}, expected: false},
	}

	for _, tc := range testCases {
		res := hasTrayFlag(tc.args)
		if res != tc.expected {
			t.Errorf("hasTrayFlag(%v) = %v; want %v", tc.args, res, tc.expected)
		}
	}
}

func TestTrayLocalization(t *testing.T) {
	deCfg := Config{Language: "de"}
	enCfg := Config{Language: "en"}

	deOpen := localizeConfigText(deCfg, "Öffnen", "Open")
	enOpen := localizeConfigText(enCfg, "Öffnen", "Open")
	if deOpen != "Öffnen" {
		t.Errorf("expected 'Öffnen', got %q", deOpen)
	}
	if enOpen != "Open" {
		t.Errorf("expected 'Open', got %q", enOpen)
	}

	deExit := localizeConfigText(deCfg, "Beenden", "Exit")
	enExit := localizeConfigText(enCfg, "Beenden", "Exit")
	if deExit != "Beenden" {
		t.Errorf("expected 'Beenden', got %q", deExit)
	}
	if enExit != "Exit" {
		t.Errorf("expected 'Exit', got %q", enExit)
	}
}

func TestTrayManagerInstantiation(t *testing.T) {
	tm := NewTrayManager("http://127.0.0.1:4040", "de", true)
	if tm == nil {
		t.Fatal("NewTrayManager returned nil")
	}
	if tm.url != "http://127.0.0.1:4040" {
		t.Errorf("expected url 'http://127.0.0.1:4040', got %q", tm.url)
	}
	if tm.language != "de" {
		t.Errorf("expected language 'de', got %q", tm.language)
	}
	if !tm.startMinimized {
		t.Errorf("expected startMinimized to be true")
	}

	// Calling Stop multiple times must be safe and idempotent
	tm.Stop()
	tm.Stop()
}

func TestNotifyIconDataStructSize(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only struct size check")
	}
	var nid NOTIFYICONDATAW
	size := unsafe.Sizeof(nid)
	// On Windows amd64 NOTIFYICONDATAW is >= 500 bytes and <= 1024 bytes
	if size < 500 || size > 1024 {
		t.Fatalf("unexpected NOTIFYICONDATAW struct size: %d", size)
	}
}

func TestLoadAppIcon(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	hIcon := loadAppIcon()
	if hIcon == 0 {
		t.Log("loadAppIcon returned 0 (expected in headless test environment)")
	}
}

func TestTrayOpenUIAndExitAppHooks(t *testing.T) {
	tm := NewTrayManager("http://127.0.0.1:4040", "en", true)

	var openedURL string
	origOpenHook := openBrowserMaximizedHook
	openBrowserMaximizedHook = func(url string) error {
		openedURL = url
		return nil
	}
	defer func() { openBrowserMaximizedHook = origOpenHook }()

	tm.OpenUI()
	if openedURL != "http://127.0.0.1:4040" {
		t.Errorf("expected openBrowserMaximizedHook to receive %q, got %q", "http://127.0.0.1:4040", openedURL)
	}

	// Test error branch
	openBrowserMaximizedHook = func(url string) error {
		return errors.New("browser error")
	}
	tm.OpenUI()

	var exitCalled atomic.Bool
	origExitHook := exitAppHook
	exitAppHook = func(code int) {
		exitCalled.Store(true)
	}
	defer func() { exitAppHook = origExitHook }()

	tm.ExitApp()
	if !exitCalled.Load() {
		t.Errorf("expected exitAppHook to be called")
	}
}

func TestTrayWndProcMock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
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
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
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
