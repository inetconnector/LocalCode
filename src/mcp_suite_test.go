// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedMCPDefaultsPresent(t *testing.T) {
	cfg := defaultConfig()
	for _, name := range []string{"filesystem", "powershell", "git", "fetch", "github", "playwright"} {
		if findMCPServerIndex(cfg, name) < 0 {
			t.Fatalf("managed MCP server %s is missing", name)
		}
	}
	for _, name := range []string{"powershell", "git", "fetch", "playwright"} {
		server := cfg.MCPServers[findMCPServerIndex(cfg, name)]
		if !server.AutoInstall {
			t.Fatalf("managed MCP dependency %s must offer approved automatic installation", name)
		}
	}
	github := cfg.MCPServers[findMCPServerIndex(cfg, "github")]
	if github.URL != "https://api.githubcopilot.com/mcp/x/all" {
		t.Fatalf("unexpected GitHub MCP all-tool endpoint: %s", github.URL)
	}
}

func TestBuiltinFilesystemMCPReadWriteAndList(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	writeParams := map[string]any{"name": "write_file", "arguments": map[string]any{"path": "docs/test.txt", "content": "hello MCP"}}
	output, err := mcpCall(context.Background(), cfg, root, "filesystem", "tools/call", writeParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "hello MCP") {
		t.Fatalf("unexpected write output: %s", output)
	}
	readParams := map[string]any{"name": "read_text_file", "arguments": map[string]any{"path": "docs/test.txt"}}
	output, err = mcpCall(context.Background(), cfg, root, "filesystem", "tools/call", readParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "hello MCP") {
		t.Fatalf("unexpected read output: %s", output)
	}
	output, err = mcpCall(context.Background(), cfg, root, "filesystem", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"read_text_file", "write_file", "search_files", "copy_path"} {
		if !strings.Contains(output, tool) {
			t.Fatalf("tool %s missing from %s", tool, output)
		}
	}
}

func TestBuiltinFilesystemMCPRejectsEscape(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	_, err := mcpCall(context.Background(), cfg, root, "filesystem", "tools/call", map[string]any{
		"name": "write_file", "arguments": map[string]any{"path": filepath.Join("..", "escape.txt"), "content": "no"},
	})
	if err == nil {
		t.Fatal("expected project escape to be rejected")
	}
}

func TestBuiltinGitMCPListsTools(t *testing.T) {
	cfg := defaultConfig()
	output, err := mcpCall(context.Background(), cfg, t.TempDir(), "git", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"git_status", "git_init", "git_commit", "git_push"} {
		if !strings.Contains(output, tool) {
			t.Fatalf("tool %s missing from %s", tool, output)
		}
	}
}

func TestMCPPersistentHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_PERSISTENT_HELPER") != "1" {
		return
	}
	if counter := os.Getenv("MCP_PROCESS_COUNTER"); counter != "" {
		data, _ := os.ReadFile(counter)
		count := 0
		if len(data) > 0 {
			_ = json.Unmarshal(data, &count)
		}
		count++
		encoded, _ := json.Marshal(count)
		_ = os.WriteFile(counter, encoded, 0o644)
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	initialized := false
	rootsSeen := false
	var pendingToolIDs []any
	emitTools := func(id any) {
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
			"tools": []map[string]any{{"name": "persistent_echo", "description": "test", "inputSchema": map[string]any{"type": "object"}}},
		}})
	}
	for scanner.Scan() {
		var request map[string]any
		_ = json.Unmarshal(scanner.Bytes(), &request)
		method, _ := request["method"].(string)
		id, hasID := request["id"]
		if !hasID {
			continue
		}
		if method == "" {
			if numeric, ok := id.(float64); ok && int(numeric) == 9001 {
				rootsSeen = true
				for _, pendingID := range pendingToolIDs {
					emitTools(pendingID)
				}
				pendingToolIDs = nil
			}
			continue
		}
		switch method {
		case "initialize":
			params, _ := request["params"].(map[string]any)
			protocol, _ := params["protocolVersion"].(string)
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": protocol,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "persistent-helper", "version": "1"},
			}})
			initialized = true
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 9001, "method": "roots/list", "params": map[string]any{}})
		case "tools/list":
			if !initialized {
				os.Exit(2)
			}
			if !rootsSeen {
				pendingToolIDs = append(pendingToolIDs, id)
				continue
			}
			emitTools(id)
		}
	}
	os.Exit(0)
}

func TestMCPStdioSessionPersistsAcrossCalls(t *testing.T) {
	defaultMCPManager.Close()
	counter := filepath.Join(t.TempDir(), "counter.json")
	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{
		Name: "persistent", Enabled: true, Transport: "stdio", Command: os.Args[0],
		Args:       []string{"-test.run=TestMCPPersistentHelperProcess"},
		Env:        map[string]string{"GO_WANT_MCP_PERSISTENT_HELPER": "1", "MCP_PROCESS_COUNTER": counter},
		TimeoutSec: 10, ProjectScoped: true,
	}}
	project := t.TempDir()
	// Register this cleanup after both TempDir calls. Go executes test cleanups
	// in LIFO order, so the persistent child is stopped and reaped before
	// Windows tries to remove its working directory and counter directory.
	t.Cleanup(defaultMCPManager.Close)
	for i := 0; i < 2; i++ {
		output, err := mcpCall(context.Background(), cfg, project, "persistent", "tools/list", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, "persistent_echo") {
			t.Fatalf("unexpected output: %s", output)
		}
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := json.Unmarshal(data, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one persistent MCP process, got %d", count)
	}
	defaultMCPManager.Close()
	if err := os.RemoveAll(project); err != nil {
		t.Fatalf("persistent MCP process still holds its project directory: %v", err)
	}
}

func TestMCPStatusForBuiltins(t *testing.T) {
	cfg := defaultConfig()
	statuses := allMCPStatuses(context.Background(), cfg, t.TempDir(), false)
	found := map[string]MCPServerStatus{}
	for _, status := range statuses {
		found[status.Name] = status
	}
	for _, name := range []string{"filesystem", "powershell", "git"} {
		status, ok := found[name]
		if !ok || status.ToolCount == 0 {
			t.Fatalf("unexpected status for %s: %#v", name, status)
		}
	}
	if !found["filesystem"].Installed || !found["filesystem"].Connected {
		t.Fatalf("filesystem MCP must be immediately available: %#v", found["filesystem"])
	}
	if !found["powershell"].Installed && found["powershell"].Error == "" {
		t.Fatalf("missing PowerShell must include a concrete diagnostic: %#v", found["powershell"])
	}
	if !found["git"].Installed && found["git"].Error == "" {
		t.Fatalf("missing Git must include a concrete diagnostic: %#v", found["git"])
	}
}

func TestBuiltinPowerShellMCPListsTools(t *testing.T) {
	cfg := defaultConfig()
	output, err := mcpCall(context.Background(), cfg, t.TempDir(), "powershell", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"powershell_run", "powershell_get_command", "powershell_get_help"} {
		if !strings.Contains(output, tool) {
			t.Fatalf("tool %s missing from %s", tool, output)
		}
	}
}

func TestBuiltinGitMCPInitStageCommitAndStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed in the test environment")
	}
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.ToolOverrides["git"] = "git"
	if _, err := mcpCall(context.Background(), cfg, project, "git", "tools/call", map[string]any{
		"name": "git_init", "arguments": map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "user.name", "LocalCode Test"}, {"config", "user.email", "localcode-test@example.invalid"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# MCP Git test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := mcpCall(context.Background(), cfg, project, "git", "tools/call", map[string]any{
		"name": "git_commit", "arguments": map[string]any{"message": "test: verify Git MCP", "stage_all": true},
	})
	if err != nil {
		t.Fatalf("commit failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "VERIFICATION") {
		t.Fatalf("commit verification missing: %s", output)
	}
	output, err = mcpCall(context.Background(), cfg, project, "git", "tools/call", map[string]any{
		"name": "git_status", "arguments": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "##") {
		t.Fatalf("unexpected git status output: %s", output)
	}
}
