// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestSecureDefaultsKeepRemoteOptIn(t *testing.T) {
	cfg := defaultConfig()
	if cfg.RemoteEnabled {
		t.Fatal("remote access must be disabled by default")
	}
	if cfg.RemoteBindHost != "127.0.0.1" {
		t.Fatalf("default remote bind host = %q, want loopback", cfg.RemoteBindHost)
	}
	if cfg.ApprovalMode != "strict" {
		t.Fatalf("default approval mode = %q, want strict", cfg.ApprovalMode)
	}
	if cfg.SandboxMode != "project" {
		t.Fatalf("default sandbox mode = %q, want project", cfg.SandboxMode)
	}
}

func TestExternalMCPDefaultsAreDisabledAndVersionPinned(t *testing.T) {
	for _, server := range defaultMCPServers() {
		if strings.EqualFold(server.Transport, "builtin") {
			continue
		}
		if server.Enabled {
			t.Fatalf("external MCP server %q must be opt-in", server.Name)
		}
		joined := strings.ToLower(strings.Join(server.Args, " "))
		if strings.Contains(joined, "@latest") || strings.Contains(joined, "==latest") {
			t.Fatalf("external MCP server %q uses an unpinned latest dependency: %q", server.Name, joined)
		}
	}
}

func TestExternalCodingEngineDefaultsUsePinnedVersionWhereConfigured(t *testing.T) {
	cfg := defaultConfig()
	if strings.EqualFold(strings.TrimSpace(cfg.OpenCodeVersion), "latest") || strings.TrimSpace(cfg.OpenCodeVersion) == "" {
		t.Fatalf("OpenCode default version must be pinned, got %q", cfg.OpenCodeVersion)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.AiderVersion), "latest") || strings.TrimSpace(cfg.AiderVersion) == "" {
		t.Fatalf("Aider default version must be pinned, got %q", cfg.AiderVersion)
	}
}
