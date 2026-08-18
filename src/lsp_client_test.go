// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLSPCandidatesCoverPrimaryLanguages(t *testing.T) {
	cases := map[string]string{
		"main.go":      "gopls",
		"app.ts":       "typescript-language-server",
		"worker.py":    "basedpyright-langserver",
		"lib.rs":       "rust-analyzer",
		"native.cpp":   "clangd",
		"Program.cs":   "csharp-ls",
		"Service.java": "jdtls",
	}
	for path, want := range cases {
		candidates := lspCandidatesForPath(path)
		if len(candidates) == 0 || candidates[0].Tool != want {
			t.Fatalf("%s candidates=%#v want first %q", path, candidates, want)
		}
	}
}

func TestReadLSPEnvelopeParsesFraming(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`)
	wire := fmt.Sprintf("Content-Length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n%s", len(body), body)
	envelope, err := readLSPEnvelope(bufio.NewReader(strings.NewReader(wire)))
	if err != nil {
		t.Fatal(err)
	}
	if string(envelope.ID) != "7" || !bytes.Contains(envelope.Result, []byte(`"ok":true`)) {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestLSPClientRoundTripWithServerRequestsAndDiagnostics(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_HELPER") == "1" {
		return
	}
	cfg := defaultConfig()
	if cfg.EnvironmentVars == nil {
		cfg.EnvironmentVars = map[string]string{}
	}
	cfg.EnvironmentVars["LOCALCODE_LSP_HELPER"] = "1"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := startLSPClient(ctx, t.TempDir(), cfg, lspServerSpec{
		Tool:       "test-lsp",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestLocalCodeLSPHelperProcess", "--"},
		LanguageID: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if err := client.openDocument(t.TempDir()+"/sample.go", "go", "package sample\n"); err != nil {
		t.Fatal(err)
	}
	result, err := client.request(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": "file:///sample.go"},
		"position":     map[string]any{"line": 0, "character": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte(`target.go`)) {
		t.Fatalf("unexpected definition result: %s", result)
	}
	if len(client.diagnostics) != 1 || !bytes.Contains(client.diagnostics[0], []byte(`synthetic diagnostic`)) {
		t.Fatalf("diagnostics not captured: %#v", client.diagnostics)
	}
}

func TestLocalCodeLSPHelperProcess(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_HELPER") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	var pendingDefinitionID json.RawMessage
	for {
		envelope, err := readLSPEnvelope(reader)
		if err != nil {
			if err == io.EOF {
				os.Exit(0)
			}
			os.Exit(2)
		}
		if envelope.Method == "" {
			switch string(envelope.ID) {
			case "91":
				// After configuration, also prove that a server-originated edit is
				// rejected by LocalCode's read-only LSP boundary.
				_ = writeLSPTestMessage(map[string]any{
					"jsonrpc": "2.0",
					"id":      92,
					"method":  "workspace/applyEdit",
					"params":  map[string]any{"edit": map[string]any{"changes": map[string]any{}}},
				})
			case "92":
				if !bytes.Contains(envelope.Result, []byte(`"applied":false`)) {
					os.Exit(3)
				}
				if len(pendingDefinitionID) > 0 {
					writeLSPTestDefinitionResult(pendingDefinitionID)
					pendingDefinitionID = nil
				}
			}
			continue
		}
		switch envelope.Method {
		case "initialize":
			writeLSPTestResponse(envelope.ID, map[string]any{"capabilities": map[string]any{}})
			// Exercise LocalCode's server-request responder while the next request
			// is pending.
			_ = writeLSPTestMessage(map[string]any{
				"jsonrpc": "2.0",
				"id":      91,
				"method":  "workspace/configuration",
				"params":  map[string]any{"items": []any{map[string]any{"section": "test"}}},
			})
		case "initialized", "textDocument/didOpen":
			// Notifications do not receive responses.
		case "textDocument/definition":
			pendingDefinitionID = append(json.RawMessage(nil), envelope.ID...)
		case "shutdown":
			writeLSPTestResponse(envelope.ID, nil)
		case "exit":
			os.Exit(0)
		default:
			if len(envelope.ID) > 0 {
				writeLSPTestResponse(envelope.ID, nil)
			}
		}
	}
}

func writeLSPTestDefinitionResult(id json.RawMessage) {
	_ = writeLSPTestMessage(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri": "file:///sample.go",
			"diagnostics": []any{map[string]any{
				"message":  "synthetic diagnostic",
				"severity": 2,
			}},
		},
	})
	writeLSPTestResponse(id, []any{map[string]any{
		"uri": "file:///target.go",
		"range": map[string]any{
			"start": map[string]any{"line": 2, "character": 1},
			"end":   map[string]any{"line": 2, "character": 8},
		},
	}})
}

func writeLSPTestResponse(id json.RawMessage, result any) {
	var decoded any
	_ = json.Unmarshal(id, &decoded)
	_ = writeLSPTestMessage(map[string]any{"jsonrpc": "2.0", "id": decoded, "result": result})
}

func writeLSPTestMessage(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
