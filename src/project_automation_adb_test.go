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

func TestExtractManifestPackageAndLauncherEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// 1. Missing file
	_, _, err := extractManifestPackageAndLauncher(filepath.Join(dir, "missing.xml"))
	if err == nil {
		t.Error("expected error for missing file")
	}

	// 2. Invalid XML
	badPath := filepath.Join(dir, "bad.xml")
	_ = os.WriteFile(badPath, []byte("not xml"), 0o600)
	_, _, err = extractManifestPackageAndLauncher(badPath)
	if err == nil {
		t.Error("expected error for invalid XML")
	}

	// 3. No package
	noPkgPath := filepath.Join(dir, "nopkg.xml")
	_ = os.WriteFile(noPkgPath, []byte(`<manifest></manifest>`), 0o600)
	_, _, err = extractManifestPackageAndLauncher(noPkgPath)
	if err == nil {
		t.Error("expected error for manifest without package")
	}

	// 4. Manifest without launcher
	noLauncherPath := filepath.Join(dir, "nolauncher.xml")
	_ = os.WriteFile(noLauncherPath, []byte(`<manifest package="com.test"><application><activity android:name=".OnlyActivity"/></application></manifest>`), 0o600)
	pkg, launcher, err := extractManifestPackageAndLauncher(noLauncherPath)
	if err != nil || pkg != "com.test" || launcher != ".OnlyActivity" {
		t.Errorf("expected fallback to first activity, got: pkg=%q launcher=%q err=%v", pkg, launcher, err)
	}

	// 5. findProjectManifest subdirectories
	subApp := filepath.Join(dir, "app", "src", "main")
	_ = os.MkdirAll(subApp, 0o755)
	subManifest := filepath.Join(subApp, "AndroidManifest.xml")
	_ = os.WriteFile(subManifest, []byte(`<manifest package="com.sub"></manifest>`), 0o600)
	if found := findProjectManifest(dir); found != subManifest {
		t.Errorf("expected found %q, got %q", subManifest, found)
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
if "%1"=="tcpip" (
  echo restarting in TCP mode port: %2
  exit /b 0
)
if "%1"=="install" (
  echo Success
  exit /b 0
)
if "%1"=="logcat" (
  echo --------- beginning of main
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

	// 7. Action: tcpip
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "tcpip",
		Target: "5555",
	}, cfg)
	if err != nil || !strings.Contains(out, "restarting in TCP mode") {
		t.Fatalf("tcpip action failed: %v, out: %s", err, out)
	}

	// 8. Action: logcat
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "logcat",
	}, cfg)
	if err != nil || !strings.Contains(out, "beginning of main") {
		t.Fatalf("logcat action failed: %v, out: %s", err, out)
	}

	// 9. Action: install
	apkPath := filepath.Join(project, "app.apk")
	_ = os.WriteFile(apkPath, []byte("fake-apk"), 0o600)
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "install",
		Target: apkPath,
	}, cfg)
	if err != nil || !strings.Contains(out, "Success") {
		t.Fatalf("install action failed: %v, out: %s", err, out)
	}

	// 10. Action: install without target should find APK in project
	out, err = runADBAction(ctx, project, adbActionRequest{
		Action: "install",
	}, cfg)
	if err != nil || !strings.Contains(out, "Success") {
		t.Fatalf("auto-install action failed: %v, out: %s", err, out)
	}

	// 11. Unsupported action
	_, err = runADBAction(ctx, project, adbActionRequest{Action: "unknown_action"}, cfg)
	if err == nil {
		t.Error("expected error for unknown action")
	}

	// 12. Connect without target
	_, err = runADBAction(ctx, project, adbActionRequest{Action: "connect"}, cfg)
	if err == nil {
		t.Error("expected error for connect without target")
	}

	// 13. Launch without target
	_, err = runADBAction(ctx, project, adbActionRequest{Action: "launch"}, cfg)
	if err == nil {
		t.Error("expected error for launch without target")
	}

	// 14. Stop without target
	_, err = runADBAction(ctx, project, adbActionRequest{Action: "stop"}, cfg)
	if err == nil {
		t.Error("expected error for stop without target")
	}
}

func TestServerADBRoutes(t *testing.T) {
	project := t.TempDir()
	appState := &AppState{
		Config:  defaultConfig(),
		Project: project,
	}
	server := NewServer(appState)

	// 1. GET /api/adb/devices
	req := httptest.NewRequest(http.MethodGet, "/api/adb/devices", nil)
	rr := httptest.NewRecorder()
	server.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 2. GET /api/adb/devices method not allowed
	reqPostDev := httptest.NewRequest(http.MethodPost, "/api/adb/devices", nil)
	rrPostDev := httptest.NewRecorder()
	server.mux.ServeHTTP(rrPostDev, reqPostDev)
	if rrPostDev.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rrPostDev.Code)
	}

	// 3. POST /api/adb/action with unsupported action
	req2 := httptest.NewRequest(http.MethodPost, "/api/adb/action", strings.NewReader(`{"action":"invalid_action"}`))
	rr2 := httptest.NewRecorder()
	server.mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK && !strings.Contains(rr2.Body.String(), "unsupported") {
		t.Fatalf("expected unsupported error json, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// 4. POST /api/adb/action bad body
	reqBad := httptest.NewRequest(http.MethodPost, "/api/adb/action", strings.NewReader(`{invalid_json`))
	rrBad := httptest.NewRecorder()
	server.mux.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", rrBad.Code)
	}

	// 5. POST /api/adb/action method not allowed
	reqGetAct := httptest.NewRequest(http.MethodGet, "/api/adb/action", nil)
	rrGetAct := httptest.NewRecorder()
	server.mux.ServeHTTP(rrGetAct, reqGetAct)
	if rrGetAct.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rrGetAct.Code)
	}

	// 6. POST /api/adb/deploy without project
	appStateEmpty := &AppState{Config: defaultConfig()}
	serverEmpty := NewServer(appStateEmpty)
	reqDeployNoProj := httptest.NewRequest(http.MethodPost, "/api/adb/deploy", nil)
	rrDeployNoProj := httptest.NewRecorder()
	serverEmpty.mux.ServeHTTP(rrDeployNoProj, reqDeployNoProj)
	if rrDeployNoProj.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for deploy without project, got %d", rrDeployNoProj.Code)
	}

	// 7. POST /api/adb/deploy method not allowed
	reqGetDeploy := httptest.NewRequest(http.MethodGet, "/api/adb/deploy", nil)
	rrGetDeploy := httptest.NewRecorder()
	server.mux.ServeHTTP(rrGetDeploy, reqGetDeploy)
	if rrGetDeploy.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rrGetDeploy.Code)
	}
}

func TestADBAdditionalCoverage(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()

	adbTool := writeWindowsCmdFixture(t, tools, "adb", `
if "%1"=="devices" (
  echo List of devices attached
  echo DEV_ONLINE device
  echo DEV_OFFLINE offline
  echo DEV_UNAUTH unauthorized
  exit /b 0
)
if "%1"=="-s" (
  echo DEV_COMMAND_OK
  exit /b 0
)
exit /b 0`)

	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{
		"adb": adbTool,
	}

	ctx := context.Background()

	// 1. Devices with mixed states
	devs, err := listConnectedADBDevices(ctx, project, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devs) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devs))
	}

	// 2. runADBAction with target serial
	out, err := runADBAction(ctx, project, adbActionRequest{
		Action: "launch",
		Serial: "DEV_ONLINE",
		Target: "com.example/.MainActivity",
	}, cfg)
	if err != nil || !strings.Contains(out, "DEV_COMMAND_OK") {
		t.Fatalf("launch failed: %v, out: %s", err, out)
	}

	// 3. findProjectManifest fallback when missing
	emptyDir := t.TempDir()
	if found := findProjectManifest(emptyDir); found != "" {
		t.Errorf("expected empty string for missing manifest, got %q", found)
	}
}
