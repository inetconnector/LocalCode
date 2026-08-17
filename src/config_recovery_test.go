// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func isolateConfigTestHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(filepath.Join(profile, "Projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("LOCALCODE_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("LOCALCODE_USER_HOME", profile)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	return root
}

func TestConfigKeepsLastKnownGoodBackup(t *testing.T) {
	isolateConfigTestHome(t)
	cfg := defaultConfig()
	cfg.ProfileName = "backup-version"
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("first save: %v", err)
	}
	cfg.ProfileName = "primary-version"
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if _, err := os.Stat(configPath() + ".bak"); err != nil {
		t.Fatalf("expected persistent config backup: %v", err)
	}
}

func TestLoadConfigRecoversCorruptPrimaryFromBackup(t *testing.T) {
	isolateConfigTestHome(t)
	cfg := defaultConfig()
	cfg.ProfileName = "last-known-good"
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("first save: %v", err)
	}
	cfg.ProfileName = "new-primary"
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("second save: %v", err)
	}

	if err := os.WriteFile(configPath(), []byte(`{"schema_version":11,"profile_name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadConfig()
	if got.ProfileName != "last-known-good" {
		t.Fatalf("ProfileName = %q, want backup value", got.ProfileName)
	}
	if _, err := os.Stat(configPath() + ".corrupt"); err != nil {
		t.Fatalf("corrupt primary should be preserved for diagnosis: %v", err)
	}
	if _, _, err := readConfigFile(configPath()); err != nil {
		t.Fatalf("primary should be restored to valid JSON after recovery: %v", err)
	}
}

func TestLoadConfigCorruptWithoutBackupFallsBackSafely(t *testing.T) {
	isolateConfigTestHome(t)
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(), []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadConfig()
	want := defaultConfig()
	if got.ApprovalMode != want.ApprovalMode || got.SandboxMode != want.SandboxMode || got.RemoteEnabled != want.RemoteEnabled {
		t.Fatalf("unsafe fallback: approval=%q sandbox=%q remote=%v", got.ApprovalMode, got.SandboxMode, got.RemoteEnabled)
	}
}
