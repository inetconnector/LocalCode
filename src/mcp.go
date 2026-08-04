// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const mcpProtocolVersion = "2025-11-25"

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func findMCPServer(cfg Config, name string) (MCPServerConfig, error) {
	for _, server := range cfg.MCPServers {
		if server.Enabled && strings.EqualFold(server.Name, strings.TrimSpace(name)) {
			return server, nil
		}
	}
	return MCPServerConfig{}, fmt.Errorf("enabled MCP server not found: %s", name)
}

func mcpServersSummary(cfg Config) string {
	var lines []string
	for _, s := range cfg.MCPServers {
		if s.Enabled {
			lines = append(lines, fmt.Sprintf("- %s (%s)", s.Name, s.Transport))
		}
	}
	if len(lines) == 0 {
		return "Keine MCP-Server aktiviert."
	}
	return strings.Join(lines, "\n")
}

func expandConfigValue(s string) string {
	return os.Expand(s, func(k string) string { return os.Getenv(k) })
}

func mcpCall(ctx context.Context, cfg Config, serverName, method string, params any) (string, error) {
	server, err := findMCPServer(cfg, serverName)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(server.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch server.Transport {
	case "stdio":
		return mcpCallStdio(callCtx, server, method, params)
	case "http", "streamable-http":
		return mcpCallHTTP(callCtx, server, method, params)
	default:
		return "", fmt.Errorf("unsupported MCP transport %q", server.Transport)
	}
}

func mcpInitializeRequest(id int) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "initialize", "params": map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": false}},
		"clientInfo":      mcpClientInfo{Name: "LocalCode", Version: version},
	}}
}

func mcpRequest(id int, method string, params any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

func mcpNotification(method string, params any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
}

func mcpCallStdio(ctx context.Context, server MCPServerConfig, method string, params any) (string, error) {
	if strings.TrimSpace(server.Command) == "" {
		return "", errors.New("MCP stdio command is empty")
	}
	cmd := exec.CommandContext(ctx, expandConfigValue(server.Command), expandArgs(server.Args)...)
	hideCommandWindow(cmd)
	env := os.Environ()
	for k, v := range server.Env {
		env = append(env, k+"="+expandConfigValue(v))
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	enc.SetEscapeHTML(false)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	if err := enc.Encode(mcpInitializeRequest(1)); err != nil {
		return "", err
	}
	if _, err := readMCPResponse(scanner, 1); err != nil {
		return "", fmt.Errorf("MCP initialize failed: %w; stderr=%s", err, truncateText(stderr.String(), 2000))
	}
	if err := enc.Encode(mcpNotification("notifications/initialized", map[string]any{})); err != nil {
		return "", err
	}
	if err := enc.Encode(mcpRequest(2, method, params)); err != nil {
		return "", err
	}
	resp, err := readMCPResponse(scanner, 2)
	if err != nil {
		return "", fmt.Errorf("MCP %s failed: %w; stderr=%s", method, err, truncateText(stderr.String(), 2000))
	}
	return prettyJSON(resp.Result), nil
}

func expandArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = expandConfigValue(a)
	}
	return out
}

func readMCPResponse(scanner *bufio.Scanner, wantedID int) (mcpRPCResponse, error) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var resp mcpRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if idMatches(resp.ID, wantedID) {
			if resp.Error != nil {
				return resp, fmt.Errorf("JSON-RPC %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return mcpRPCResponse{}, err
	}
	return mcpRPCResponse{}, errors.New("MCP server closed stdout before response")
}

func idMatches(v any, wanted int) bool {
	switch x := v.(type) {
	case float64:
		return int(x) == wanted
	case string:
		n, _ := strconv.Atoi(x)
		return n == wanted
	case json.Number:
		n, _ := x.Int64()
		return int(n) == wanted
	default:
		return false
	}
}

func mcpCallHTTP(ctx context.Context, server MCPServerConfig, method string, params any) (string, error) {
	endpoint := strings.TrimSpace(expandConfigValue(server.URL))
	if endpoint == "" {
		return "", errors.New("MCP HTTP URL is empty")
	}
	client := &http.Client{Timeout: time.Duration(server.TimeoutSec) * time.Second}
	if client.Timeout <= 0 {
		client.Timeout = 60 * time.Second
	}
	initResp, session, err := mcpHTTPPost(ctx, client, server, endpoint, "", mcpInitializeRequest(1))
	if err != nil {
		return "", err
	}
	if initResp.Error != nil {
		return "", fmt.Errorf("MCP initialize: %s", initResp.Error.Message)
	}
	_, _, _ = mcpHTTPPost(ctx, client, server, endpoint, session, mcpNotification("notifications/initialized", map[string]any{}))
	resp, _, err := mcpHTTPPost(ctx, client, server, endpoint, session, mcpRequest(2, method, params))
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("MCP %s: %s", method, resp.Error.Message)
	}
	return prettyJSON(resp.Result), nil
}

func mcpHTTPPost(ctx context.Context, client *http.Client, server MCPServerConfig, endpoint, session string, payload any) (mcpRPCResponse, string, error) {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocolVersion)
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	for k, v := range server.Headers {
		req.Header.Set(k, expandConfigValue(v))
	}
	resp, err := client.Do(req)
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	defer resp.Body.Close()
	if h := resp.Header.Get("Mcp-Session-Id"); h != "" {
		session = h
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	if resp.StatusCode == http.StatusAccepted && len(bytes.TrimSpace(body)) == 0 {
		return mcpRPCResponse{}, session, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mcpRPCResponse{}, session, fmt.Errorf("MCP HTTP %d: %s", resp.StatusCode, truncateText(string(body), 2000))
	}
	var rpc mcpRPCResponse
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		rpc, err = parseMCPSSE(body)
	} else {
		err = json.Unmarshal(body, &rpc)
	}
	return rpc, session, err
}

func parseMCPSSE(body []byte) (mcpRPCResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var resp mcpRPCResponse
			if json.Unmarshal([]byte(data), &resp) == nil && resp.ID != nil {
				return resp, nil
			}
		}
	}
	return mcpRPCResponse{}, errors.New("no JSON-RPC response found in SSE")
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if json.Indent(&out, raw, "", "  ") == nil {
		return out.String()
	}
	return string(raw)
}
