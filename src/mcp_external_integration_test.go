// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests intentionally download and start the maintained upstream MCP
// packages. They are opt-in so ordinary offline builds stay deterministic.
func TestExternalFetchMCPToolsList(t *testing.T) {
	if os.Getenv("LOCALCODE_EXTERNAL_MCP_TESTS") != "1" {
		t.Skip("set LOCALCODE_EXTERNAL_MCP_TESTS=1 to test upstream MCP packages")
	}
	if _, ok := commandAvailable("uvx", ""); !ok {
		t.Skip("uvx is unavailable")
	}
	defaultMCPManager.Close()
	t.Cleanup(defaultMCPManager.Close)
	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{
		Name: "fetch-external", Enabled: true, Transport: "stdio", Command: "uvx",
		Args: []string{"mcp-server-fetch"}, TimeoutSec: 180,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	output, err := mcpCall(ctx, cfg, t.TempDir(), "fetch-external", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "fetch") {
		t.Fatalf("Fetch MCP tool missing from: %s", output)
	}
}

func TestExternalPlaywrightMCPToolsList(t *testing.T) {
	if os.Getenv("LOCALCODE_EXTERNAL_MCP_TESTS") != "1" {
		t.Skip("set LOCALCODE_EXTERNAL_MCP_TESTS=1 to test upstream MCP packages")
	}
	if _, ok := commandAvailable("npx", ""); !ok {
		t.Skip("npx is unavailable")
	}
	defaultMCPManager.Close()
	t.Cleanup(defaultMCPManager.Close)
	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{
		Name: "playwright-external", Enabled: true, Transport: "stdio", Command: "npx",
		Args: []string{"-y", "@playwright/mcp@latest", "--headless"}, TimeoutSec: 240,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	output, err := mcpCall(ctx, cfg, t.TempDir(), "playwright-external", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"browser_navigate", "browser_snapshot"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("Playwright MCP tool %s missing from: %s", expected, output)
		}
	}
}
