// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractManifestPackageAndLauncher(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "AndroidManifest.xml")
	manifestContent := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.pacman.nativeapp">
    <application android:label="Pacman">
        <activity android:name=".PacmanActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
        <activity android:name="SettingsActivity" />
    </application>
</manifest>`

	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o600); err != nil {
		t.Fatal(err)
	}

	pkg, launcher, err := extractManifestPackageAndLauncher(manifestPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != "com.pacman.nativeapp" {
		t.Errorf("expected package com.pacman.nativeapp, got %q", pkg)
	}
	if launcher != ".PacmanActivity" {
		t.Errorf("expected launcher .PacmanActivity, got %q", launcher)
	}

	// Test findProjectManifest
	found := findProjectManifest(dir)
	if found != manifestPath {
		t.Errorf("expected found manifest %q, got %q", manifestPath, found)
	}
}

func TestRunADBActionMock(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()

	adbTool := writeWindowsCmdFixture(t, tools, "adb", `
if "%1"=="version" (echo Android Debug Bridge 1.0.41& exit /b 0)
if "%1"=="devices" (
  echo List of devices attached
  echo TEST_DEV_123 device product:phone model:GalaxyS24
  exit /b 0
)
if "%1"=="-s" (
  echo ADB_CMD_MOCK_SUCCESS: %*
  exit /b 0
)
if "%1"=="connect" (
  echo connected to %2
  exit /b 0
)
exit /b 0`)

	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{
		"adb": adbTool,
	}

	ctx := context.Background()

	// 1. Devices
	devs, err := listConnectedADBDevices(ctx, project, cfg)
	if err != nil {
		t.Fatalf("listConnectedADBDevices failed: %v", err)
	}
	if len(devs) != 1 || devs[0].Serial != "TEST_DEV_123" {
		t.Fatalf("expected 1 device with TEST_DEV_123, got %#v", devs)
	}

	// 2. Action: devices
	out, err := runADBAction(ctx, project, adbActionRequest{Action: "devices"}, cfg)
	if err != nil || !strings.Contains(out, "TEST_DEV_123") {
		t.Fatalf("devices action failed: %v, out: %s", err, out)
	}

	// 3. Action: launch
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "launch",
		Serial: "TEST_DEV_123",
		Target: "com.pacman.nativeapp/.PacmanActivity",
	}, cfg)
	if err != nil || !strings.Contains(out, "ADB_CMD_MOCK_SUCCESS") {
		t.Fatalf("launch action failed: %v, out: %s", err, out)
	}

	// 4. Action: stop
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "stop",
		Serial: "TEST_DEV_123",
		Target: "com.pacman.nativeapp",
	}, cfg)
	if err != nil || !strings.Contains(out, "ADB_CMD_MOCK_SUCCESS") {
		t.Fatalf("stop action failed: %v, out: %s", err, out)
	}

	// 5. Action: reverse
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "reverse",
		Serial: "TEST_DEV_123",
		Target: "32145",
	}, cfg)
	if err != nil || !strings.Contains(out, "ADB_CMD_MOCK_SUCCESS") {
		t.Fatalf("reverse action failed: %v, out: %s", err, out)
	}

	// 6. Action: connect
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "connect",
		Target: "192.168.1.90:5555",
	}, cfg)
	if err != nil || !strings.Contains(out, "connected to") {
		t.Fatalf("connect action failed: %v, out: %s", err, out)
	}
}

func TestServerADBRoutes(t *testing.T) {
	appState := &AppState{
		Config: defaultConfig(),
	}
	server := NewServer(appState)

	// GET /api/adb/devices
	req := httptest.NewRequest(http.MethodGet, "/api/adb/devices", nil)
	rr := httptest.NewRecorder()
	server.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// POST /api/adb/action with unsupported action
	req2 := httptest.NewRequest(http.MethodPost, "/api/adb/action", strings.NewReader(`{"action":"invalid_action"}`))
	rr2 := httptest.NewRecorder()
	server.mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK && !strings.Contains(rr2.Body.String(), "unsupported") {
		t.Fatalf("expected unsupported error json, got %d: %s", rr2.Code, rr2.Body.String())
	}
}
