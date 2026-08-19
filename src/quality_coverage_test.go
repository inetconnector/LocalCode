// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodeGraphCompatibilityWrappersMatchUntypedRelationPaths(t *testing.T) {
	files := []codeGraphFile{
		{Path: "service.go", Language: "go", Content: "package demo\nfunc Service() {}\n", Symbols: []string{"Service"}, BaseScore: 8, Outbound: 1},
		{Path: "helper.go", Language: "go", Content: "package demo\nfunc Helper() {}\n", Symbols: []string{"Helper"}, BaseScore: 2, Inbound: 1},
	}
	adjacency := []map[int]bool{{1: true}, {}}
	reverse := []map[int]bool{{}, {0: true}}

	wrapped := append([]codeGraphFile(nil), files...)
	direct := append([]codeGraphFile(nil), files...)
	applyCodeGraphRanks(wrapped, adjacency, reverse)
	applyCodeGraphRanksInternal(direct, nil, adjacency, reverse)
	if !reflect.DeepEqual(wrapped, direct) {
		t.Fatalf("compatibility rank wrapper diverged:\nwrapped=%#v\ndirect=%#v", wrapped, direct)
	}

	wrappedReport := formatCodeGraph(wrapped, adjacency, reverse, "Service")
	directReport := formatCodeGraphWithRelations(direct, nil, adjacency, reverse, "Service")
	if wrappedReport != directReport {
		t.Fatalf("compatibility formatter diverged:\nwrapped=%s\ndirect=%s", wrappedReport, directReport)
	}
	for _, want := range []string{"CODE INTELLIGENCE GRAPH", "service.go", "helper.go", "definition Service"} {
		if !strings.Contains(wrappedReport, want) {
			t.Fatalf("formatted graph missing %q:\n%s", want, wrappedReport)
		}
	}

	wrappedNeighbors := codeGraphTopNeighbors(0, wrapped, adjacency, reverse, 4)
	directNeighbors := codeGraphTopNeighborsWithRelations(0, wrapped, nil, adjacency, reverse, 4)
	if !reflect.DeepEqual(wrappedNeighbors, directNeighbors) || len(wrappedNeighbors) != 1 {
		t.Fatalf("compatibility neighbors diverged: wrapped=%#v direct=%#v", wrappedNeighbors, directNeighbors)
	}
}

func TestLSPPositionNormalizationResolutionAndFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	params := lspPositionParams(path, 0, -3)
	position, ok := params["position"].(map[string]any)
	if !ok || position["line"] != 0 || position["character"] != 0 {
		t.Fatalf("position=%#v, want zero-based origin", params["position"])
	}
	textDocument, ok := params["textDocument"].(map[string]any)
	if !ok || !strings.HasPrefix(textDocument["uri"].(string), "file:") {
		t.Fatalf("textDocument URI not normalized: %#v", params["textDocument"])
	}

	cases := map[string]string{
		" Go-To_Definition ": "gotodefinition",
		"workspace-symbol":   "workspacesymbol",
		"Incoming Calls":     "incomingcalls",
	}
	for input, want := range cases {
		if got := normalizeLSPOperation(input); got != want {
			t.Fatalf("normalizeLSPOperation(%q)=%q want %q", input, got, want)
		}
	}

	cfg := defaultConfig()
	if cfg.ToolOverrides == nil {
		cfg.ToolOverrides = map[string]string{}
	}
	cfg.ToolOverrides["gopls"] = os.Args[0]
	spec, err := resolveLSPServer(t.TempDir(), cfg, path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Tool != "gopls" || spec.Executable != os.Args[0] || spec.LanguageID != "go" || len(spec.Args) != 1 || spec.Args[0] != "serve" {
		t.Fatalf("unexpected resolved LSP spec: %#v", spec)
	}
	if _, err := resolveLSPServer(t.TempDir(), cfg, "README.unknown"); err == nil || !strings.Contains(err.Error(), "no LocalCode LSP profile") {
		t.Fatalf("expected unsupported-extension error, got %v", err)
	}

	result := json.RawMessage(`[{"uri":"file:///target.go","range":{"start":{"line":2,"character":1}}}]`)
	diagnostics := []json.RawMessage{json.RawMessage(`{"uri":"file:///sample.go","diagnostics":[{"message":"synthetic warning"}]}`)}
	formatted := formatLSPResult(spec, "textDocument/definition", result, diagnostics)
	for _, want := range []string{"LSP RESULT", "Server: gopls", "Method: textDocument/definition", "target.go", "Diagnostics observed while querying", "synthetic warning"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted LSP result missing %q:\n%s", want, formatted)
		}
	}

	fallback := formatLSPResult(spec, "textDocument/hover", json.RawMessage("not-json"), []json.RawMessage{json.RawMessage("not-json")})
	if !strings.Contains(fallback, "not-json") {
		t.Fatalf("invalid JSON fallback was lost: %s", fallback)
	}
	empty := formatLSPResult(spec, "textDocument/hover", nil, nil)
	if !strings.Contains(empty, "null") {
		t.Fatalf("empty LSP result should render null: %s", empty)
	}
}
