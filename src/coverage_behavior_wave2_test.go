// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func pngFixtureBase64(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{G: 255, A: 255})
	img.Set(0, 1, color.NRGBA{B: 255, A: 255})
	img.Set(1, 1, color.NRGBA{R: 255, G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func officeZipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMCPHTTPInitializationSummaryHeadersAndReset(t *testing.T) {
	project := t.TempDir()
	t.Setenv("MCP_FIXTURE_TOKEN", "fixture-secret")
	var mu sync.Mutex
	initializeCount := 0
	seenSession := false
	seenProjectHeader := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := payload["method"].(string)
		switch method {
		case "initialize":
			mu.Lock()
			initializeCount++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "fixture-session")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{"protocolVersion": mcpProtocolVersion, "instructions": "fixture instructions"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			mu.Lock()
			seenSession = r.Header.Get("Mcp-Session-Id") == "fixture-session"
			seenProjectHeader = r.Header.Get("X-Project") == project
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 2,
				"result": map[string]any{"tools": []map[string]any{{"name": "alpha"}}},
			})
		default:
			http.Error(w, "unexpected method: "+method, http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{
		Name: "fixture", DisplayName: "Fixture MCP", Enabled: true, Transport: "streamable-http", URL: server.URL,
		Headers: map[string]string{"X-Project": "${PROJECT_ROOT}", "X-Token": "${MCP_FIXTURE_TOKEN}"}, TimeoutSec: 5,
	}}

	summary := mcpServersSummary(cfg)
	if !strings.Contains(summary, "Fixture MCP") || !strings.Contains(summary, "streamable-http") {
		t.Fatalf("unexpected MCP summary: %q", summary)
	}
	mu.Lock()
	if initializeCount != 0 {
		t.Fatalf("summary unexpectedly initialized external MCP %d times", initializeCount)
	}
	mu.Unlock()

	headers := resolveMCPHeaders(context.Background(), cfg, project, cfg.MCPServers[0])
	if headers["X-Project"] != project || headers["X-Token"] != "fixture-secret" {
		t.Fatalf("resolved MCP headers=%#v", headers)
	}

	manager := newMCPManager()
	defer manager.Close()
	session, err := manager.httpSession(context.Background(), project, cfg.MCPServers[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ensureInitialized(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if session.protocol == "" || session.session != "fixture-session" || session.instructions != "fixture instructions" {
		t.Fatalf("unexpected initialized session: protocol=%q session=%q instructions=%q", session.protocol, session.session, session.instructions)
	}
	instructions, err := manager.serverInstructions(context.Background(), cfg, project, cfg.MCPServers[0])
	if err != nil || instructions != "fixture instructions" {
		t.Fatalf("server instructions=%q err=%v", instructions, err)
	}
	result, err := manager.callHTTP(context.Background(), cfg, project, cfg.MCPServers[0], "tools/list", map[string]any{})
	if err != nil || !strings.Contains(result, "alpha") {
		t.Fatalf("MCP tools/list result=%q err=%v", result, err)
	}
	mu.Lock()
	if initializeCount != 1 || !seenSession || !seenProjectHeader {
		t.Fatalf("MCP lifecycle initialize=%d sessionHeader=%v projectHeader=%v", initializeCount, seenSession, seenProjectHeader)
	}
	mu.Unlock()

	manager.ResetServer("fixture")
	replacement, err := manager.httpSession(context.Background(), project, cfg.MCPServers[0])
	if err != nil {
		t.Fatal(err)
	}
	if replacement == session {
		t.Fatal("ResetServer must evict cached HTTP session")
	}
	if _, err := manager.serverInstructions(context.Background(), cfg, project, MCPServerConfig{Name: "bad", Transport: "unknown"}); err == nil {
		t.Fatal("unsupported MCP transport must fail")
	}

	builtin := MCPServerConfig{Name: "filesystem", Enabled: true, Transport: "builtin", Preset: "filesystem"}
	if text, err := manager.serverInstructions(context.Background(), cfg, project, builtin); err != nil || strings.TrimSpace(text) == "" {
		t.Fatalf("builtin MCP instructions=%q err=%v", text, err)
	}
}

func TestAttachmentExtractionDispatchCoversStructuredFormats(t *testing.T) {
	ctx := context.Background()
	text, kind := extractAttachmentText(ctx, "notes.txt", "text/plain", []byte(" hello\r\nworld "), "")
	if kind != "Text" || text != "hello\nworld" {
		t.Fatalf("text extraction kind=%q text=%q", kind, text)
	}

	docx := officeZipFixture(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="urn:w"><w:p><w:t>Hello DOCX</w:t></w:p></w:document>`,
	})
	text, kind = extractAttachmentText(ctx, "sample.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx, "")
	if kind != "DOCX-Text" || !strings.Contains(text, "Hello DOCX") {
		t.Fatalf("DOCX extraction kind=%q text=%q", kind, text)
	}

	pptx := officeZipFixture(t, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="urn:p"><a:t xmlns:a="urn:a">Hello PPTX</a:t></p:sld>`,
	})
	text, kind = extractAttachmentText(ctx, "sample.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", pptx, "")
	if kind != "PPTX-Text" || !strings.Contains(text, "Hello PPTX") {
		t.Fatalf("PPTX extraction kind=%q text=%q", kind, text)
	}

	xlsx := officeZipFixture(t, map[string]string{
		"xl/sharedStrings.xml": `<sst><si><t>Hello XLSX</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><c><v>42</v></c></worksheet>`,
	})
	text, kind = extractAttachmentText(ctx, "sample.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsx, "")
	if kind != "XLSX-Text" || !strings.Contains(text, "Hello XLSX") || !strings.Contains(text, "42") {
		t.Fatalf("XLSX extraction kind=%q text=%q", kind, text)
	}

	archive := officeZipFixture(t, map[string]string{"a.txt": "one", "dir/b.txt": "two"})
	text, kind = extractAttachmentText(ctx, "sample.zip", "application/zip", archive, "")
	if kind != "Archivinhalt" || !strings.Contains(text, "a.txt") || !strings.Contains(text, "dir/b.txt") {
		t.Fatalf("ZIP extraction kind=%q text=%q", kind, text)
	}

	pdfRaw := []byte("%PDF-1.7\n" + strings.Repeat("Printable fallback PDF text content. ", 8))
	pdfPath := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(pdfPath, pdfRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	text, kind = extractAttachmentText(ctx, "sample.pdf", "application/pdf", pdfRaw, pdfPath)
	if kind == "" || strings.TrimSpace(text) == "" {
		t.Fatalf("PDF extraction unexpectedly empty kind=%q text=%q", kind, text)
	}
	if text, kind = extractAttachmentText(ctx, "unknown.bin", "application/octet-stream", []byte{0, 1, 2, 3}, ""); text != "" || kind != "" {
		t.Fatalf("binary extraction should be empty kind=%q text=%q", kind, text)
	}
}

func TestPreviewActionBehaviorAcrossMutatingAndToolCapabilities(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CreateProjectDocs = false
	cfg.ImageGeneratorProvider = "automatic1111"
	cfg.ImageGeneratorURL = "http://127.0.0.1:7860"
	cfg.EditingEngine = editingEngineAider
	cfg.AiderMainModel = "test-model"

	if err := os.WriteFile(filepath.Join(project, "text.txt"), []byte("old value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "source.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><rect width="20" height="10"/></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}
	png64 := pngFixtureBase64(t)
	pngBytes, err := base64.StdEncoding.DecodeString(png64)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "source.png"), pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	actions := []AgentAction{
		{Action: "replace_text", Path: "text.txt", OldText: "old", NewText: "new"},
		{Action: "write_file", Path: "created.txt", Content: "created\n"},
		{Action: "create_svg_asset", Path: "created.svg", Content: `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><circle cx="4" cy="4" r="3"/></svg>`},
		{Action: "create_image_asset", Path: "created.png", Content: png64},
		{Action: "generate_image_asset", Path: "generated.png", Content: "a small test image", Width: 128, Height: 96},
		{Action: "render_asset", Source: "source.svg", Destination: "rendered.png", Width: 200, Height: 100},
		{Action: "convert_image_asset", Source: "source.png", Destination: "converted.jpg", Width: 4, Height: 4},
		{Action: "delete_file", Path: "text.txt"},
		{Action: "run_tool", Tool: "go", Args: []string{"version"}},
		{Action: "run_command", Command: "echo fixture"},
		{Action: "open_terminal", Command: "go test ./..."},
		{Action: "copy_path", Source: "source.png", Destination: "copy.png"},
		{Action: "move_path", Source: "source.png", Destination: "moved.png"},
		{Action: "git", Args: []string{"status", "--short"}},
		{Action: "git_commit", Message: "add fixture tests"},
		{Action: "web_search", Query: "fixture search"},
		{Action: "web_fetch", URL: "https://example.invalid"},
		{Action: "engine_edit", Task: "update source.png metadata"},
		{Action: "engine_repo_map"},
		{Action: "engine_lint"},
		{Action: "engine_test"},
		{Action: "build_project"},
		{Action: "deploy_android"},
		{Action: "mcp_call_tool", Server: "filesystem", Tool: "read_file", Arguments: map[string]any{"path": "README.md"}},
	}
	for _, action := range actions {
		preview, err := previewAction(project, cfg, action)
		if err != nil {
			t.Fatalf("previewAction(%s) failed: %v", action.Action, err)
		}
		if strings.TrimSpace(preview) == "" {
			t.Fatalf("previewAction(%s) returned empty preview", action.Action)
		}
	}
	if _, err := previewAction(project, cfg, AgentAction{Action: "run_command", Command: ""}); err == nil {
		t.Fatal("empty command preview must fail")
	}
	if _, err := previewAction(project, cfg, AgentAction{Action: "unknown"}); err == nil {
		t.Fatal("unknown approval action must fail")
	}
}

func TestCommitMessageFallbackAndConfigMutationUnwrap(t *testing.T) {
	project := t.TempDir()
	message, useFile := commitMessageFromProject(project, "Implement feature with tests")
	if useFile || !strings.HasPrefix(message, "feat: ") {
		t.Fatalf("fallback commit message=%q useFile=%v", message, useFile)
	}
	if err := os.WriteFile(filepath.Join(project, "COMMIT_MESSAGE.txt"), []byte("fix: exact fixture message\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	message, useFile = commitMessageFromProject(project, "ignored")
	if !useFile || message != "fix: exact fixture message" {
		t.Fatalf("file commit message=%q useFile=%v", message, useFile)
	}

	rootErr := errors.New("root config error")
	wrapped := &configMutationError{err: rootErr}
	if !errors.Is(wrapped, rootErr) || wrapped.Unwrap() != rootErr {
		t.Fatal("configMutationError must unwrap its cause")
	}
	var nilWrapped *configMutationError
	if nilWrapped.Unwrap() != nil || nilWrapped.Error() != "invalid config mutation" {
		t.Fatal("nil configMutationError behavior changed")
	}
}
