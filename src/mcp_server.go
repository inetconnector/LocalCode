// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	localCodeMCPProtocolVersion = "2024-11-05"
	localCodeMCPMaxPayloadBytes = 10 * 1024 * 1024
)

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func mcpToolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "localcode_health",
			"description": "Return LocalCode engine version, runtime state, and advertised loopback capabilities.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			"name":        "localcode_providers_status",
			"description": "Return active inference provider, OS-Sandbox status, and boolean API key readiness (never exposes secrets).",
			"inputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			"name":        "localcode_list_pty_sessions",
			"description": "List active ConPTY/PTY terminal sessions managed by LocalCode.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			"name":        "localcode_verify_frame",
			"description": "Deterministically verify a PNG/JPEG UI screenshot (base64 or data URL) for black-screen or low-contrast regressions.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image_base64": map[string]any{
						"type":        "string",
						"description": "Base64 or data URL encoded PNG/JPEG frame.",
					},
				},
				"required":             []string{"image_base64"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "localcode_apk_update_info",
			"description": "Return version, version_code, and SHA-256 metadata for the locally signed Android release APK.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

func writeMCPRPCResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeMCPRPCError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

func (s *Server) dispatchMCPToolCall(name string, args map[string]any) (map[string]any, error) {
	switch name {
	case "localcode_health":
		s.state.mu.RLock()
		running := s.state.Running
		projectName := filepath.Base(strings.TrimSpace(s.state.Project))
		if projectName == "." || projectName == string(filepath.Separator) {
			projectName = ""
		}
		s.state.mu.RUnlock()
		return map[string]any{
			"app":          "LocalCode",
			"version":      version,
			"running":      running,
			"project_name": projectName,
			"capabilities": []string{"pty-v1", "mcp-server-v1", "apk-self-update-v1", "os-sandbox-v1"},
		}, nil
	case "localcode_providers_status":
		return buildProvidersStatusPayload(s.state), nil
	case "localcode_list_pty_sessions":
		sessions := s.state.EnsurePTYManager().ListSessions()
		return map[string]any{
			"count":    len(sessions),
			"sessions": sessions,
		}, nil
	case "localcode_verify_frame":
		encoded, _ := args["image_base64"].(string)
		raw, err := decodeBase64ImagePayload(encoded)
		if err != nil {
			return nil, err
		}
		return inspectUploadedImageFrame(raw)
	case "localcode_apk_update_info":
		return buildAPKUpdateMetadata(), nil
	default:
		return nil, fmt.Errorf("unknown MCP tool %q", name)
	}
}

// handleMCPServer exposes a first-class loopback MCP JSON-RPC 2.0 server on POST /mcp.
func (s *Server) handleMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, localCodeMCPMaxPayloadBytes+1))
	if err != nil {
		writeMCPRPCError(w, nil, -32700, "failed to read MCP request")
		return
	}
	if len(raw) > localCodeMCPMaxPayloadBytes {
		http.Error(w, "MCP payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req mcpRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeMCPRPCError(w, nil, -32700, "parse error: "+err.Error())
		return
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		writeMCPRPCError(w, req.ID, -32600, "invalid JSON-RPC 2.0 request")
		return
	}
	if req.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeMCPRPCResult(w, req.ID, map[string]any{
			"protocolVersion": localCodeMCPProtocolVersion,
			"serverInfo": map[string]any{
				"name":    "localcode-mcp-server",
				"version": version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
		})
	case "ping":
		writeMCPRPCResult(w, req.ID, map[string]any{"ok": true})
	case "tools/list":
		writeMCPRPCResult(w, req.ID, map[string]any{"tools": mcpToolDefinitions()})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			writeMCPRPCError(w, req.ID, -32602, "invalid tools/call params")
			return
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		res, callErr := s.dispatchMCPToolCall(strings.TrimSpace(params.Name), params.Arguments)
		if callErr != nil {
			errPayload := map[string]any{"error": callErr.Error(), "tool": params.Name}
			txt, _ := json.Marshal(errPayload)
			writeMCPRPCResult(w, req.ID, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": string(txt)}},
				"structuredContent": errPayload,
				"isError":           true,
			})
			return
		}
		txt, _ := json.Marshal(res)
		writeMCPRPCResult(w, req.ID, map[string]any{
			"content":           []map[string]any{{"type": "text", "text": string(txt)}},
			"structuredContent": res,
			"isError":           false,
		})
	default:
		writeMCPRPCError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}
