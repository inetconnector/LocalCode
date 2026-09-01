// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProjectPlanAndroidSDK(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "AndroidManifest.xml")
	if err := os.WriteFile(manifest, []byte("<manifest package=\"com.test\"/>"), 0o600); err != nil {
		t.Fatal(err)
	}

	plan := detectProjectPlan(dir)
	if plan.Kind != "android-sdk" {
		t.Fatalf("expected kind android-sdk, got %s", plan.Kind)
	}
	if plan.BuildTool != "javac" {
		t.Fatalf("expected build tool javac, got %s", plan.BuildTool)
	}
}

func TestAppendFileToZipArchive(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	dexPath := filepath.Join(dir, "classes.dex")

	if err := os.WriteFile(dexPath, []byte("DEX_CONTENT_BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create initial zip with one file
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("initial.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("initial content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Append classes.dex
	if err := appendFileToZipArchive(zipPath, dexPath, "classes.dex"); err != nil {
		t.Fatalf("appendFileToZipArchive failed: %v", err)
	}

	// Verify zip contents
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open updated zip: %v", err)
	}

	foundInitial := false
	foundDex := false
	for _, file := range zr.File {
		if file.Name == "initial.txt" {
			foundInitial = true
		}
		if file.Name == "classes.dex" {
			foundDex = true
			rc, err := file.Open()
			if err != nil {
				_ = zr.Close()
				t.Fatal(err)
			}
			content, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				_ = zr.Close()
				t.Fatal(err)
			}
			if string(content) != "DEX_CONTENT_BYTES" {
				_ = zr.Close()
				t.Fatalf("expected DEX_CONTENT_BYTES, got %s", string(content))
			}
		}
	}
	_ = zr.Close()

	if !foundInitial || !foundDex {
		t.Fatalf("expected both initial.txt and classes.dex in zip, found initial=%v, dex=%v", foundInitial, foundDex)
	}

	// Test replacing existing entry
	newDexPath := filepath.Join(dir, "classes_new.dex")
	if err := os.WriteFile(newDexPath, []byte("NEW_DEX_BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendFileToZipArchive(zipPath, newDexPath, "classes.dex"); err != nil {
		t.Fatalf("appendFileToZipArchive overwrite failed: %v", err)
	}

	zr2, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	countDex := 0
	for _, file := range zr2.File {
		if file.Name == "classes.dex" {
			countDex++
			rc, _ := file.Open()
			content, _ := io.ReadAll(rc)
			_ = rc.Close()
			if string(content) != "NEW_DEX_BYTES" {
				_ = zr2.Close()
				t.Fatalf("expected NEW_DEX_BYTES, got %s", string(content))
			}
		}
	}
	_ = zr2.Close()
	if countDex != 1 {
		t.Fatalf("expected exactly 1 classes.dex entry after overwrite, found %d", countDex)
	}
}

func TestBuildAndroidSDKProjectErrors(t *testing.T) {
	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	ctx := context.Background()

	// Missing manifest
	dir := t.TempDir()
	out, err := buildAndroidSDKProject(ctx, dir, cfg)
	if err == nil || !strings.Contains(err.Error(), "AndroidManifest.xml") {
		t.Fatalf("expected manifest missing error, got out=%q, err=%v", out, err)
	}

	// Manifest present, but missing android.jar
	manifest := filepath.Join(dir, "AndroidManifest.xml")
	_ = os.WriteFile(manifest, []byte("<manifest package=\"com.test\"/>"), 0o600)
	t.Setenv("ANDROID_SDK_ROOT", filepath.Join(dir, "nonexistent_sdk"))
	t.Setenv("ANDROID_HOME", filepath.Join(dir, "nonexistent_sdk"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "nonexistent_sdk"))

	out, err = buildAndroidSDKProject(ctx, dir, cfg)
	if err == nil || !strings.Contains(err.Error(), "android.jar") {
		t.Fatalf("expected android.jar missing error, got out=%q, err=%v", out, err)
	}
}

func TestBuildAndroidSDKProjectMockSuccess(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()
	sdkDir := t.TempDir()

	t.Setenv("PATH", filepath.Join(os.Getenv("SystemRoot"), "System32")+";"+tools)

	// Setup mock android.jar in platform
	platformDir := filepath.Join(sdkDir, "platforms", "android-34")
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(platformDir, "android.jar"), []byte("mock android.jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_SDK_ROOT", sdkDir)

	// Setup project structure
	manifest := filepath.Join(project, "app", "src", "main", "AndroidManifest.xml")
	_ = os.MkdirAll(filepath.Dir(manifest), 0o755)
	_ = os.WriteFile(manifest, []byte("<manifest package=\"com.test\"/>"), 0o600)

	javaFile := filepath.Join(project, "app", "src", "main", "java", "com", "test", "MainActivity.java")
	_ = os.MkdirAll(filepath.Dir(javaFile), 0o755)
	_ = os.WriteFile(javaFile, []byte("package com.test; public class MainActivity {}"), 0o600)

	resFile := filepath.Join(project, "app", "src", "main", "res", "values", "strings.xml")
	_ = os.MkdirAll(filepath.Dir(resFile), 0o755)
	_ = os.WriteFile(resFile, []byte("<resources><string name=\"app_name\">Test</string></resources>"), 0o600)

	// Setup mock tool scripts
	aapt2Tool := writeWindowsCmdFixture(t, tools, "aapt2", `
if "%1"=="version" (echo Android Asset Packaging Tool 2.34.0& exit /b 0)
if "%1"=="compile" (
  echo mock-res-zip > "%4"
  echo aapt2 compile ok
  exit /b 0
)
if "%1"=="link" (
  set OUTDIR=%CD%\build\outputs\apk\debug
  if not exist "%OUTDIR%" mkdir "%OUTDIR%"
  echo mock-unaligned-apk > "%OUTDIR%\app-unaligned.apk"
  echo aapt2 link ok
  exit /b 0
)
exit /b 0`)

	javacTool := writeWindowsCmdFixture(t, tools, "javac", `
if "%1"=="-version" (echo javac 17.0.2 1>&2& exit /b 0)
if exist "%CD%\build\classes" echo mock-class > "%CD%\build\classes\MainActivity.class"
echo mock javac ok
exit /b 0`)

	d8Tool := writeWindowsCmdFixture(t, tools, "d8", `
if "%1"=="--version" (echo D8 8.2.0& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-dex > "%OUTDIR%\classes.dex"
echo mock d8 ok
exit /b 0`)

	zipalignTool := writeWindowsCmdFixture(t, tools, "zipalign", `
if "%1"=="-h" (echo ZipAlign 1.0& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-aligned-apk > "%OUTDIR%\app-aligned.apk"
echo mock zipalign ok
exit /b 0`)

	apksignerTool := writeWindowsCmdFixture(t, tools, "apksigner", `
if "%1"=="version" (echo apksigner 0.9& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-signed-apk > "%OUTDIR%\app-debug.apk"
echo mock apksigner ok
exit /b 0`)

	keytoolTool := writeWindowsCmdFixture(t, tools, "keytool", `
if "%1"=="-help" (echo keytool help& exit /b 0)
echo mock keytool ok
exit /b 0`)

	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{
		"aapt2":     aapt2Tool,
		"javac":     javacTool,
		"d8":        d8Tool,
		"zipalign":  zipalignTool,
		"apksigner": apksignerTool,
		"keytool":   keytoolTool,
	}

	out, err := buildAndroidSDKProject(context.Background(), project, cfg)
	if err != nil {
		t.Fatalf("buildAndroidSDKProject failed: %v\nOutput: %s", err, out)
	}

	if !strings.Contains(out, "FINAL APK") || !strings.Contains(out, "app-debug.apk") {
		t.Fatalf("expected final apk path in output, got %s", out)
	}
}

func TestDeployAndroidSDKProject(t *testing.T) {
	project := t.TempDir()
	tools := t.TempDir()
	sdkDir := t.TempDir()

	t.Setenv("PATH", filepath.Join(os.Getenv("SystemRoot"), "System32")+";"+tools)

	platformDir := filepath.Join(sdkDir, "platforms", "android-34")
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(platformDir, "android.jar"), []byte("mock android.jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_SDK_ROOT", sdkDir)

	manifest := filepath.Join(project, "AndroidManifest.xml")
	_ = os.WriteFile(manifest, []byte("<manifest package=\"com.test\"/>"), 0o600)

	javaFile := filepath.Join(project, "src", "MainActivity.java")
	_ = os.MkdirAll(filepath.Dir(javaFile), 0o755)
	_ = os.WriteFile(javaFile, []byte("package com.test; public class MainActivity {}"), 0o600)

	aapt2Tool := writeWindowsCmdFixture(t, tools, "aapt2", `
if "%1"=="version" (echo Android Asset Packaging Tool 2.34.0& exit /b 0)
if "%1"=="compile" (echo mock-res > "%4"& exit /b 0)
if "%1"=="link" (
  set OUTDIR=%CD%\build\outputs\apk\debug
  if not exist "%OUTDIR%" mkdir "%OUTDIR%"
  echo mock-unaligned > "%OUTDIR%\app-unaligned.apk"
  exit /b 0
)
exit /b 0`)

	javacTool := writeWindowsCmdFixture(t, tools, "javac", `
if "%1"=="-version" (echo javac 17.0.2 1>&2& exit /b 0)
if exist "%CD%\build\classes" echo mock-class > "%CD%\build\classes\MainActivity.class"
exit /b 0`)

	d8Tool := writeWindowsCmdFixture(t, tools, "d8", `
if "%1"=="--version" (echo D8 8.2.0& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-dex > "%OUTDIR%\classes.dex"
exit /b 0`)

	zipalignTool := writeWindowsCmdFixture(t, tools, "zipalign", `
if "%1"=="-h" (echo ZipAlign 1.0& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-aligned > "%OUTDIR%\app-aligned.apk"
exit /b 0`)

	apksignerTool := writeWindowsCmdFixture(t, tools, "apksigner", `
if "%1"=="version" (echo apksigner 0.9& exit /b 0)
set OUTDIR=%CD%\build\outputs\apk\debug
echo mock-signed > "%OUTDIR%\app-debug.apk"
exit /b 0`)

	keytoolTool := writeWindowsCmdFixture(t, tools, "keytool", `
if "%1"=="-help" (echo keytool help& exit /b 0)
exit /b 0`)

	adbTool := writeWindowsCmdFixture(t, tools, "adb", `
if "%1"=="version" (echo Android Debug Bridge 1.0.41& exit /b 0)
if "%1"=="devices" (
  echo List of devices attached
  echo TEST_SERIAL_123 device product:mock model:MockDevice
  exit /b 0
)
if "%1"=="-s" (
  echo Success
  exit /b 0
)
exit /b 0`)

	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{
		"aapt2":     aapt2Tool,
		"javac":     javacTool,
		"d8":        d8Tool,
		"zipalign":  zipalignTool,
		"apksigner": apksignerTool,
		"keytool":   keytoolTool,
		"adb":       adbTool,
	}

	out, err := deployAndroid(context.Background(), project, cfg)
	if err != nil {
		t.Fatalf("deployAndroid for android-sdk failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "TEST_SERIAL_123") || !strings.Contains(out, "Success") {
		t.Fatalf("expected successful deploy output with serial, got %s", out)
	}
}
