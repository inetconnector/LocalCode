// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureWithinRoot(t *testing.T) {
	root := t.TempDir()
	got, err := ensureWithinRoot(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("got %q want %q", got, root)
	}

	got, err = ensureWithinRoot(root, filepath.Join("a", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, root) {
		t.Fatalf("path escaped root: %q", got)
	}

	if _, err := ensureWithinRoot(root, filepath.Join("..", "outside.txt")); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestWriteAndReplaceFile(t *testing.T) {
	root := t.TempDir()
	if _, err := writeProjectFile(root, "x.txt", "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	diff, err := replaceText(root, "x.txt", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+three") {
		t.Fatalf("unexpected diff: %s", diff)
	}
	data, err := os.ReadFile(filepath.Join(root, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nthree\n" {
		t.Fatalf("unexpected file: %q", data)
	}
}

func TestProjectTreeAndSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n// needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "x", "skip.js"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := projectTree(root, "", 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tree, "src/main.go") {
		t.Fatalf("missing file: %s", tree)
	}
	if strings.Contains(tree, "node_modules") {
		t.Fatalf("ignored directory included: %s", tree)
	}

	hits, err := searchProject(root, "needle", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hits, "src/main.go:2") {
		t.Fatalf("missing hit: %s", hits)
	}
	if strings.Contains(hits, "skip.js") {
		t.Fatalf("ignored hit included: %s", hits)
	}
}

func TestParseAgentAction(t *testing.T) {
	a, err := parseAgentAction(`{"action":"read_file","message":"Lese Datei","path":"README.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Action != "read_file" || a.Path != "README.md" {
		t.Fatalf("unexpected action: %#v", a)
	}
}

func TestNormalizeOllamaBaseURL(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:11434":         "http://127.0.0.1:11434",
		"http://localhost:11434/": "http://localhost:11434",
		"0.0.0.0:11434":           "http://127.0.0.1:11434",
		"[::]:11434":              "http://127.0.0.1:11434",
	}
	for in, want := range cases {
		if got := normalizeOllamaBaseURL(in); got != want {
			t.Fatalf("normalizeOllamaBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentLoopExecutesStructuredActions(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		calls++
		content := `{"action":"list_files","message":"Lese Projektstruktur","path":"","max_depth":2}`
		if calls > 1 {
			content = `{"action":"finish","message":"Projekt analysiert; README.md wurde gefunden; keine Änderungen vorgenommen."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": content},
			"done":    true,
		})
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model"}, client)
	if err := state.StartAgent("Analysiere das Projekt", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Projekt analysiert") {
					return
				}
			}
			t.Fatalf("agent ended without final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish in time")
}

func TestChatAcceptsStructuredActionFromThinking(t *testing.T) {
	var thinkValue any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		thinkValue = req["think"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"role":     "assistant",
				"content":  "",
				"thinking": `interne Verarbeitung\n{"action":"finish","message":"Fertig"}`,
			},
			"done":        true,
			"done_reason": "stop",
		})
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	content, err := client.Chat(context.Background(), "gpt-oss:20b", []OllamaMessage{{Role: "user", Content: "test"}}, actionSchema)
	if err != nil {
		t.Fatal(err)
	}
	if content != `{"action":"finish","message":"Fertig"}` {
		t.Fatalf("unexpected content: %q", content)
	}
	if thinkValue != "low" {
		t.Fatalf("think = %#v, want low", thinkValue)
	}
}

func TestAgentFallsBackFromEmptyGPTOSSResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "gpt-oss:20b", "size": 1, "modified_at": time.Now()},
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/chat":
			var req struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Model == "gpt-oss:20b" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message":     map[string]any{"role": "assistant", "content": "", "thinking": ""},
					"done":        true,
					"done_reason": "length",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{"role": "assistant", "content": `{"action":"finish","message":"Fallback erfolgreich"}`},
				"done":    true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "gpt-oss:20b"}, client)
	if err := state.StartAgent("Analysiere das Projekt", "gpt-oss:20b", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		model := state.Model
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if model != "qwen2.5-coder:14b" {
				t.Fatalf("model = %q, want fallback", model)
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Fallback erfolgreich") {
					return
				}
			}
			t.Fatalf("agent ended without fallback final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish in time")
}

func TestChooseDefaultModelMigratesFromGPTOSS(t *testing.T) {
	models := []ModelInfo{{Name: "gpt-oss:20b"}, {Name: "qwen2.5-coder:14b"}}
	if got := chooseDefaultModel(models, "gpt-oss:20b"); got != "qwen2.5-coder:14b" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeConfigRepairsAppDataProjectRoot(t *testing.T) {
	profile := t.TempDir()
	projects := filepath.Join(profile, "Projekte")
	if err := os.MkdirAll(filepath.Join(projects, "Geschichten"), 0o755); err != nil {
		t.Fatal(err)
	}
	localAppData := filepath.Join(profile, "AppData", "Local")
	broken := filepath.Join(localAppData, "LocalCode")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", profile)
	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("APPDATA", filepath.Join(profile, "AppData", "Roaming"))

	cfg := normalizeConfig(Config{RootProjectDir: broken, LastProject: broken, Port: 32145})
	if cfg.RootProjectDir != projects {
		t.Fatalf("root = %q, want %q", cfg.RootProjectDir, projects)
	}
	if cfg.LastProject != "" {
		t.Fatalf("last project should be cleared, got %q", cfg.LastProject)
	}
}

func TestNormalizeConfigKeepsValidCustomProjectRoot(t *testing.T) {
	profile := t.TempDir()
	projects := filepath.Join(profile, "Projekte")
	custom := filepath.Join(profile, "Code")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", profile)
	t.Setenv("LOCALAPPDATA", filepath.Join(profile, "AppData", "Local"))
	t.Setenv("APPDATA", filepath.Join(profile, "AppData", "Roaming"))

	cfg := normalizeConfig(Config{RootProjectDir: custom, Port: 32145})
	if cfg.RootProjectDir != custom {
		t.Fatalf("root = %q, want %q", cfg.RootProjectDir, custom)
	}
}

func TestProjectsEndpointListsSubdirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Geschichten", "FritzShare"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	state := NewAppState(Config{RootProjectDir: root, Port: 32145}, NewOllamaClient())
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rr := httptest.NewRecorder()
	NewServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Geschichten") || !strings.Contains(rr.Body.String(), "FritzShare") {
		t.Fatalf("missing projects: %s", rr.Body.String())
	}
}

func TestProjectPickerControlsAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{"chooseRootBtn", "defaultRootBtn", "/api/browse-root", "/api/reset-root"} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing project picker control %q", required)
		}
	}
	if strings.Contains(html, `id="setRootBtn"`) || strings.Contains(html, `$('#setRootBtn')`) {
		t.Fatal("obsolete free-text root setter is still present")
	}
}

func TestValidateImages(t *testing.T) {
	image := ImageAttachment{Name: "shot.png", MIME: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("png-data"))}
	got, err := validateImages([]ImageAttachment{image})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "shot.png" {
		t.Fatalf("unexpected images: %#v", got)
	}
	if _, err := validateImages([]ImageAttachment{{Name: "bad.txt", MIME: "text/plain", Data: image.Data}}); err == nil {
		t.Fatal("expected unsupported MIME error")
	}
}

func TestFindVisionModelUsesCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
				{"name": "gemma4:e2b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/show":
			var req struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			caps := []string{"completion"}
			if req.Model == "gemma4:e2b" {
				caps = append(caps, "vision")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	model, err := client.FindVisionModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if model != "gemma4:e2b" {
		t.Fatalf("model = %q", model)
	}
}

func TestDescribeImagesSendsBase64Images(t *testing.T) {
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req OllamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("messages = %d", len(req.Messages))
		}
		received = req.Messages[0].Images
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "Das Bild zeigt eine Fehlermeldung."},
			"done":    true,
		})
	}))
	defer server.Close()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	data := base64.StdEncoding.EncodeToString([]byte("image"))
	result, err := client.DescribeImages(context.Background(), "vision-model", "Analysiere", []ImageAttachment{{Name: "shot.png", MIME: "image/png", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" || len(received) != 1 || received[0] != data {
		t.Fatalf("result=%q images=%#v", result, received)
	}
}

func TestAgentUsesImageAnalysisBeforeCoding(t *testing.T) {
	var visionCall bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "qwen2.5-coder:14b", "size": 1, "modified_at": time.Now()},
				{"name": "gemma4:e2b", "size": 1, "modified_at": time.Now()},
			}})
		case "/api/show":
			var req struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			caps := []string{"completion"}
			if req.Model == "gemma4:e2b" {
				caps = append(caps, "vision")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
		case "/api/chat":
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if len(req.Messages) > 0 && len(req.Messages[0].Images) > 0 {
				visionCall = true
				_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": "Screenshot: roter Fehlerdialog."}, "done": true})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"finish","message":"Bild und Projekt analysiert"}`}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "qwen2.5-coder:14b"}, client)
	image := ImageAttachment{Name: "shot.png", MIME: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))}
	if err := state.StartAgent("Was zeigt das Bild?", "qwen2.5-coder:14b", []ImageAttachment{image}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if !visionCall {
				t.Fatal("vision call was not made")
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Bild und Projekt") {
					return
				}
			}
			t.Fatalf("missing final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish")
}

func TestComposerSupportsEnterAndGeneralAttachments(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{
		`id="fileInput"`,
		`id="attachBtn"`,
		`e.key==='Enter'&&!e.shiftKey`,
		`onpaste=`,
		`focusPrompt(true)`,
		`const attachments=state.attachments`,
		`Dateien hinzufügen`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing composer feature %q", required)
		}
	}
}

func TestStateDocumentIsCreatedAndPreservesManualNotes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "STATE.md"), []byte("## Manuelle Notiz\nNicht löschen.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	cfg.AutoStateUpdate = true
	cfg.StateFile = "STATE.md"
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = root
	state.Model = "qwen2.5-coder:14b"
	state.LastTask = "Test"
	state.recordAction("Datei gelesen")
	state.UpdateProjectState("Testlauf")
	data, err := os.ReadFile(filepath.Join(root, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{stateBegin, stateEnd, "Manuelle Notiz", "Testlauf", "Datei gelesen"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestProjectDocsCreatedOnlyWhenMissing(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.CreateProjectDocs = true
	if err := ensureProjectDocs(root, cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectDocs(root, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(data) != "custom" {
		t.Fatal("existing README was overwritten")
	}
}

func TestSettingsEndpointRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	server := NewServer(state)
	payload := cfg
	payload.ApprovalMode = "auto"
	payload.WebSearchProvider = "disabled"
	payload.MCPServers = []MCPServerConfig{{Name: "demo", Enabled: true, Transport: "stdio", Command: "demo"}}
	body, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.RLock()
	got := state.Config
	state.mu.RUnlock()
	if got.ApprovalMode != "auto" || len(got.MCPServers) != 1 {
		t.Fatalf("unexpected settings: %#v", got)
	}
}

func TestValidatePublicURLBlocksLocalhost(t *testing.T) {
	if _, err := validatePublicURL("http://127.0.0.1:1234/private"); err == nil {
		t.Fatal("expected local URL to be blocked")
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req map[string]any
		_ = json.Unmarshal(scanner.Bytes(), &req)
		id, hasID := req["id"]
		if !hasID {
			continue
		}
		method, _ := req["method"].(string)
		result := map[string]any{}
		if method == "initialize" {
			result = map[string]any{"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
		}
		if method == "tools/list" {
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "description": "test", "inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	os.Exit(0)
}

func TestMCPStdioToolsList(t *testing.T) {
	cfg := defaultConfig()
	cfg.MCPServers = []MCPServerConfig{{Name: "test", Enabled: true, Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, Env: map[string]string{"GO_WANT_MCP_HELPER": "1"}, TimeoutSec: 10}}
	out, err := mcpCall(context.Background(), cfg, "test", "tools/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "echo") {
		t.Fatalf("unexpected MCP output: %s", out)
	}
}

func TestSettingsUIIsEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"settingsBtn", "settingsScreen", "settings-nav-btn", "setMCP", "/api/settings", "terminalBtn"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing UI element %q", want)
		}
	}
}

func TestResizablePanelsAndCodexStyleSettingsAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`id="leftSplitter"`, `id="rightSplitter"`, `id="terminalSplitter"`,
		`data-settings="general"`, `data-settings="appearance"`, `data-settings="plugins"`,
		`data-settings="hooks"`, `data-settings="worktrees"`, `data-settings="archived"`,
		`setupSplitters()`, `ui_left_width`, `ui_right_width`, `ui_terminal_height`,
		`terminal_dock`, `terminal.parentElement!==rightPane`, `terminal.parentElement!==chatWrap`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing resizable/settings feature %q", want)
		}
	}
}

func TestValidateGeneralAttachments(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("package main\n"))
	items, err := validateAttachments([]Attachment{{Name: "main.go", MIME: "text/plain", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "main.go" || items[0].Size == 0 {
		t.Fatalf("unexpected attachments: %#v", items)
	}
	if _, err := validateAttachments([]Attachment{{Name: "empty.txt", MIME: "text/plain", Data: ""}}); err == nil {
		t.Fatal("expected empty attachment error")
	}
}

func TestExtractTextAndZipAttachments(t *testing.T) {
	text, kind := extractAttachmentText(context.Background(), "README.md", "text/markdown", []byte("# Hallo\nInhalt"), "")
	if kind != "Text" || !strings.Contains(text, "Hallo") {
		t.Fatalf("text=%q kind=%q", text, kind)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("package main"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	listing, kind := extractAttachmentText(context.Background(), "project.zip", "application/zip", buf.Bytes(), "")
	if kind != "Archivinhalt" || !strings.Contains(listing, "src/main.go") {
		t.Fatalf("listing=%q kind=%q", listing, kind)
	}
}

func TestCodexLikeLayoutHasNoPlaceholderNavigation(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, required := range []string{`id="newChatBtn"`, `id="gitBtn"`, `id="terminalBtn"`, `id="settingsBtn"`, `id="projectTree"`, `data-tab="outputs"`, `data-tab="sources"`} {
		if !strings.Contains(html, required) {
			t.Fatalf("missing functional UI element %q", required)
		}
	}
	for _, forbidden := range []string{">Pull Requests<", ">Websites<", ">Geplant<"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("placeholder navigation found: %q", forbidden)
		}
	}
}

func TestAgentContinuesAfterAskUserAnswer(t *testing.T) {
	var calls int
	var sawAnswer bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			var req struct {
				Messages []OllamaMessage `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			content := `{"action":"ask_user","message":"Soll Git initialisiert werden?"}`
			if calls > 1 {
				for _, m := range req.Messages {
					if strings.Contains(m.Content, "Antwort: ja") && strings.Contains(m.Content, "Soll Git initialisiert werden?") {
						sawAnswer = true
					}
				}
				content = `{"action":"finish","message":"Git-Entscheidung übernommen; Aufgabe fortgesetzt."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model", MaxAgentSteps: 10}, client)
	if err := state.StartAgent("Git im Projekt einrichten", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		continuation := state.Continuation
		state.mu.RUnlock()
		if !running && continuation != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	state.mu.RLock()
	continuation := state.Continuation
	state.mu.RUnlock()
	if continuation == nil {
		t.Fatal("expected pending agent continuation")
	}

	if err := state.StartAgent("ja", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			if !sawAnswer {
				t.Fatal("continuation request did not include the user's answer and original question")
			}
			for _, event := range events {
				if event.Type == "final" && strings.Contains(event.Message, "Aufgabe fortgesetzt") {
					return
				}
			}
			t.Fatalf("continuation ended without final event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("continuation did not finish in time")
}

func TestReadOnlyToolResultIsVisibleAsEvent(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("sichtbarer Inhalt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, GitEnabled: true}, NewOllamaClient())
	result, done := state.handleAgentAction(context.Background(), project, AgentAction{Action: "read_file", Path: "README.md", Message: "Lese README.md"})
	if done {
		t.Fatal("read_file unexpectedly finished agent")
	}
	if !strings.Contains(result, "sichtbarer Inhalt") {
		t.Fatalf("unexpected result: %q", result)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	for _, event := range events {
		if event.Type == "tool_result" && event.Action == "read_file" && strings.Contains(event.Detail, "sichtbarer Inhalt") {
			return
		}
	}
	t.Fatalf("tool result event missing: %#v", events)
}

func TestAskUserProducesOnlyOneVisibleQuestion(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"ask_user","message":"Soll Git initialisiert werden?"}`}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	state := NewAppState(Config{RootProjectDir: project, LastProject: project, LastModel: "test-model", MaxAgentSteps: 4}, client)
	if err := state.StartAgent("Git im Projekt einrichten", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			questions := 0
			for _, event := range events {
				if event.Type == "question" && event.Message == "Soll Git initialisiert werden?" {
					questions++
				}
				if event.Type == "agent_step" && event.Action == "ask_user" {
					t.Fatalf("ask_user leaked as duplicate agent_step: %#v", events)
				}
			}
			if questions != 1 {
				t.Fatalf("expected one question event, got %d: %#v", questions, events)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not reach question state")
}

func TestSettingsEndpointAcceptsForwardCompatibleFieldsWhileAgentRuns(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	state.Running = true
	server := NewServer(state)
	body := `{"approval_mode":"balanced","model_timeout_seconds":240,"future_ui_option":"ignored"}`
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.RLock()
	got := state.Config
	state.mu.RUnlock()
	if got.ApprovalMode != "balanced" || got.ModelTimeout != 240 || !got.NetworkEnabled || !got.GitEnabled {
		t.Fatalf("settings were not merged correctly: %#v", got)
	}
}

func TestForceStopAlwaysReleasesUIState(t *testing.T) {
	state := NewAppState(defaultConfig(), NewOllamaClient())
	ctx, cancel := context.WithCancel(context.Background())
	state.Running = true
	state.Cancel = cancel
	state.RunID = "active-run"
	state.RunPhase = "model"
	pending := &PendingAction{ID: "pending", Result: make(chan bool, 1)}
	state.Pending = pending
	if !state.ForceStopAgent() {
		t.Fatal("expected running agent to be force-stopped")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("agent context was not cancelled")
	}
	state.mu.RLock()
	running, phase, currentPending := state.Running, state.RunPhase, state.Pending
	state.mu.RUnlock()
	if running || phase != "idle" || currentPending != nil {
		t.Fatalf("UI state not released: running=%v phase=%q pending=%#v", running, phase, currentPending)
	}
}

func TestControlAndOutputUIAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`id="runControl"`, `id="headerStopBtn"`, `cancelRun()`, `/api/force-stop`,
		`tool_result`, `action_done`, `tool_error`, `output-list`, `model_timeout_seconds`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing control/output UI feature %q", want)
		}
	}
}

func TestBackgroundWindowsCommandsUseNoWindowFlag(t *testing.T) {
	data, err := os.ReadFile("platform_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{"CREATE_NO_WINDOW", "hideCommandWindow(cmd)", "runProjectCommand"} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing hidden-window implementation %q", want)
		}
	}
}

func TestModelCallTimeoutReturnsControl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "slow-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer server.CloseClientConnections()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "slow-model"
	cfg.ModelTimeout = 1
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	if err := state.StartAgent("Analysiere das Projekt", "slow-model", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if !running {
			for _, ev := range events {
				if ev.Type == "error" && strings.Contains(ev.Message, "Zeitüberschreitung") {
					return
				}
			}
			t.Fatalf("agent stopped without timeout event: %#v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not return control after model timeout")
}

func TestStopAgentCancelsBlockedModelCall(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "blocked-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer server.CloseClientConnections()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "blocked-model"
	cfg.ModelTimeout = 60
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	if err := state.StartAgent("Analysiere das Projekt", "blocked-model", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("model call did not start")
	}
	if !state.StopAgent() {
		t.Fatal("StopAgent did not report an active run")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent remained running after cancellation")
}

func TestLegacyProductDataMigrationCopiesUserState(t *testing.T) {
	base := t.TempDir()
	configBase := filepath.Join(base, "config")
	cacheBase := filepath.Join(base, "cache")
	t.Setenv("XDG_CONFIG_HOME", configBase)
	t.Setenv("XDG_CACHE_HOME", cacheBase)

	legacyDir := filepath.Join(configBase, legacyProductDirName)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configData := []byte(`{"port":32145,"root_project_dir":"C:\\\\Users\\\\frede\\\\Projekte"}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), configData, 0o600); err != nil {
		t.Fatal(err)
	}
	threadsData := []byte(`{"version":1,"threads":[]}`)
	if err := os.WriteFile(filepath.Join(legacyDir, "threads.json"), threadsData, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyBackups := filepath.Join(cacheBase, legacyProductDirName, "backups", "demo")
	if err := os.MkdirAll(legacyBackups, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyBackups, "file.txt"), []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacyProductData()

	for _, item := range []struct {
		path string
		want string
	}{
		{filepath.Join(configBase, productDirName, "config.json"), string(configData)},
		{filepath.Join(configBase, productDirName, "threads.json"), string(threadsData)},
		{filepath.Join(cacheBase, productDirName, "backups", "demo", "file.txt"), "backup"},
	} {
		data, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatalf("read migrated %s: %v", item.path, err)
		}
		if string(data) != item.want {
			t.Fatalf("migrated %s = %q, want %q", item.path, data, item.want)
		}
	}
}

func TestStateDocumentMigratesLegacyManagedMarkers(t *testing.T) {
	root := t.TempDir()
	old := "# Handbuch\n\n" + legacyStateBegin + "\nalt\n" + legacyStateEnd + "\n\n## Manuell\nBehalten.\n"
	if err := os.WriteFile(filepath.Join(root, "STATE.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.StateFile = "STATE.md"
	if err := updateStateDocument(root, cfg, false, "qwen2.5-coder:14b", "Umbenennung", "fertig", []string{"Migration"}, "Markertest"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, stateBegin) || !strings.Contains(text, stateEnd) {
		t.Fatalf("new markers missing: %s", text)
	}
	if strings.Contains(text, legacyStateBegin) || strings.Contains(text, legacyStateEnd) {
		t.Fatalf("legacy markers still present: %s", text)
	}
	for _, want := range []string{"# Handbuch", "## Manuell", "Behalten.", "Markertest"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestLocalCodeBrandAndLicenseAreEmbedded(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"<title>LocalCode</title>", ">LocalCode<", "Apache-2.0"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing brand or license marker %q", want)
		}
	}
	legacyVisibleName := "Local" + "Codex"
	if strings.Contains(html, legacyVisibleName) {
		t.Fatalf("legacy product name remains visible in UI")
	}
	license, err := os.ReadFile(filepath.Join("..", "LICENSE"))
	if err != nil {
		t.Fatalf("LICENSE missing: %v", err)
	}
	if !strings.Contains(string(license), "Apache License") || !strings.Contains(string(license), "Version 2.0") {
		t.Fatal("LICENSE is not the complete Apache License 2.0 text")
	}
	if _, err := os.Stat(filepath.Join("..", "NOTICE")); err != nil {
		t.Fatalf("NOTICE missing: %v", err)
	}
}

func TestPingReportsLocalCodeAndLicense(t *testing.T) {
	state := NewAppState(defaultConfig(), NewOllamaClient())
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	rr := httptest.NewRecorder()
	NewServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"app":"LocalCode"`, `"license":"Apache-2.0"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
}

func TestAnalysisTaskDoesNotRequireGitOrMutateProject(t *testing.T) {
	var modelCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			modelCalls++
			content := `{"action":"ask_user","message":"Es scheint, dass kein Git-Repository initialisiert wurde. Möchten Sie ein neues Git-Repository erstellen?"}`
			if modelCalls == 2 {
				content = `{"action":"replace_text","message":"Ändere einen Platzhalter","path":"README.md","old_text":"demo","new_text":"changed"}`
			}
			if modelCalls >= 3 {
				content = `{"action":"finish","message":"Analyse abgeschlossen. Git war dafür nicht erforderlich; keine Dateien wurden geändert."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	readme := filepath.Join(project, "README.md")
	if err := os.WriteFile(readme, []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.MaxAgentSteps = 10
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	if err := state.StartAgent("analysiere das projekt", "test-model", nil); err != nil {
		t.Fatal(err)
	}

	waitForAgentStop(t, state, 4*time.Second)
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "demo\n" {
		t.Fatalf("analysis mutated README: %q", data)
	}
	if _, err := os.Stat(filepath.Join(project, ".git")); !os.IsNotExist(err) {
		t.Fatalf("analysis created .git unexpectedly: %v", err)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	for _, event := range events {
		if event.Type == "question" {
			t.Fatalf("analysis leaked unnecessary question: %#v", event)
		}
	}
	if !eventContains(events, "final", "Projektanalyse kontrolliert abgeschlossen") {
		t.Fatalf("analysis did not reach final report: %#v", events)
	}
}

func TestAffirmativeGitQuestionInitializesRepositoryAndContinues(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			content := `{"action":"ask_user","message":"Soll Git in diesem Projekt initialisiert werden?"}`
			if calls > 1 {
				content = `{"action":"finish","message":"Git wurde initialisiert und verifiziert."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.MaxAgentSteps = 10
	cfg.CreateProjectDocs = false
	cfg.ApprovalMode = "balanced"
	state := NewAppState(cfg, client)
	if err := state.StartAgent("Richte die Git-Versionierung ein", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 4*time.Second)
	state.mu.RLock()
	continuation := state.Continuation
	state.mu.RUnlock()
	if continuation == nil || continuation.SuggestedAction == nil {
		t.Fatalf("expected executable Git continuation: %#v", continuation)
	}
	if err := state.StartAgent("ja", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 6*time.Second)
	if !isGitRepository(project, cfg) {
		t.Fatal("confirmed Git initialization did not create repository")
	}
	if _, err := os.Stat(filepath.Join(project, ".gitignore")); err != nil {
		t.Fatalf("Git initialization did not create .gitignore: %v", err)
	}
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	questions := 0
	for _, event := range events {
		if event.Type == "question" {
			questions++
		}
	}
	if questions != 1 {
		t.Fatalf("Git question repeated instead of executing confirmation: %d %#v", questions, events)
	}
	if !eventContains(events, "final", "initialisiert") {
		t.Fatalf("Git continuation did not finish: %#v", events)
	}
}

func TestAgentCompactsLargeContextAndContinues(t *testing.T) {
	var actionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			var req OllamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			isCompaction := false
			for _, message := range req.Messages {
				if strings.Contains(message.Content, "Verdichte den folgenden Verlauf") {
					isCompaction = true
					break
				}
			}
			content := ""
			if isCompaction {
				content = `{"summary":"Große Datei wurde gelesen.","original_task":"prüfe die große datei","decisions":[],"project_facts":["big.txt ist groß"],"files_read":["big.txt"],"files_changed":[],"commands":[],"failures":[],"open_items":[],"next_recommended_action":"Bericht abschließen"}`
			} else {
				actionCalls++
				if actionCalls == 1 {
					content = `{"action":"read_file","message":"Lese große Datei","path":"big.txt"}`
				} else {
					content = `{"action":"finish","message":"Große Datei geprüft; Kontext wurde kontrolliert weitergeführt."}`
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "big.txt"), []byte(strings.Repeat("large-context-line\n", 9000)), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.ContextLength = 4096
	cfg.ContextCompactionEnabled = true
	cfg.ContextCompactionThresholdPercent = 45
	cfg.ContextCompactionKeepRecent = 6
	cfg.MaxAgentSteps = 8
	cfg.CreateProjectDocs = false
	state := NewAppState(cfg, client)
	if err := state.StartAgent("prüfe die große datei", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitForAgentStop(t, state, 6*time.Second)
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	if !eventContains(events, "context_compacted", "Kontext komprimiert") {
		t.Fatalf("large context was not compacted: %#v", events)
	}
	if !eventContains(events, "final", "kontrolliert weitergeführt") {
		t.Fatalf("agent did not continue after compaction: %#v", events)
	}
}

func waitForAgentStop(t *testing.T, state *AppState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not stop before timeout")
}

func eventContains(events []UIEvent, eventType, fragment string) bool {
	for _, event := range events {
		if event.Type == eventType && strings.Contains(event.Message+"\n"+event.Detail, fragment) {
			return true
		}
	}
	return false
}
