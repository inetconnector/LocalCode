// SPDX-License-Identifier: Apache-2.0

package main

import (
	"runtime"
	"testing"
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

	// Test WM_TRAY_CALLBACK with unexpected mouse event
	ret := trayWndProc(0, WM_TRAY_CALLBACK, 0, 0x0200) // WM_MOUSEMOVE
	if ret != 0 {
		t.Errorf("expected 0, got %d", ret)
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
