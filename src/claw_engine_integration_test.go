// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestClawEngineIsFirstClassSelection(t *testing.T) {
	if got := normalizeEditingEngine(" ClAw "); got != editingEngineClaw {
		t.Fatalf("normalizeEditingEngine(claw) = %q", got)
	}
	if got := codingEngineDisplayName(editingEngineClaw); got != "Claw Code" {
		t.Fatalf("display name = %q", got)
	}
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = true
	if !codingEngineEnabled(cfg, editingEngineClaw) {
		t.Fatal("Claw should be selectable without a separate hidden enable flag")
	}
	if !codingEngineAutoInstall(cfg, editingEngineClaw) {
		t.Fatal("Claw automatic setup should follow SetupDownloadsEnabled")
	}
}

func TestClawRustProfilesUseManagedRustupOnWindows(t *testing.T) {
	for _, name := range []string{"cargo", "rustc"} {
		profile := profileForTool(name)
		if profile.InstallKind != "winget" || profile.WingetID != "Rustlang.Rustup" {
			t.Fatalf("%s install profile = kind %q id %q", name, profile.InstallKind, profile.WingetID)
		}
		if runtime.GOOS == "windows" && !toolInstallSupported(name) {
			t.Fatalf("%s should be managed-installable on Windows", name)
		}
	}
}

func TestClawEngineUIIsIntegratedIntoPolishLayer(t *testing.T) {
	loader, err := os.ReadFile("static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	loaderText := string(loader)
	if !strings.Contains(loaderText, "/ui_polish.js") {
		t.Fatalf("base UI polish script is not loaded: %s", loaderText)
	}
	if strings.Contains(loaderText, "claw_engine_ui.js") {
		t.Fatalf("Claw UI must not depend on a second patch script: %s", loaderText)
	}

	polish, err := os.ReadFile("static/ui_polish.js")
	if err != nil {
		t.Fatal(err)
	}
	polishText := string(polish)
	for _, required := range []string{
		"function installClawEngineUI()",
		"option.value = 'claw'",
		"return 'Claw Code'",
		"engineLoginBtn",
		"installClawEngineUI();",
	} {
		if !strings.Contains(polishText, required) {
			t.Fatalf("integrated Claw UI missing %q", required)
		}
	}
}
