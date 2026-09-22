// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"sync/atomic"
	"testing"
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
