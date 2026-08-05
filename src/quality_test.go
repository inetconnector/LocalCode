// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLanguageSettingsAndResponseLanguage(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 4, Language: "de", PreferredLanguage: "Deutsch"})
	if cfg.SchemaVersion != 5 {
		t.Fatalf("schema = %d, want 5", cfg.SchemaVersion)
	}
	if cfg.Language != "auto" || cfg.PreferredLanguage != "auto" {
		t.Fatalf("legacy implicit German default was not migrated: %#v", cfg)
	}
	cfg.Language = "en"
	cfg.PreferredLanguage = "auto"
	if got := responseLanguage(cfg); got != "English" {
		t.Fatalf("responseLanguage = %q, want English", got)
	}
	cfg.Language = "de"
	if got := responseLanguage(cfg); got != "Deutsch" {
		t.Fatalf("responseLanguage = %q, want Deutsch", got)
	}
}

func TestApprovalPendingIsClearedWhenContextIsCancelled(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := state.requestApprovalWithPreview(ctx, AgentAction{Action: "run_command", Message: "Run tests", Command: "go test ./..."}, "preview")
		result <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		pending := state.Pending
		state.mu.RUnlock()
		if pending != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("approval request did not return after cancellation")
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.Pending != nil {
		t.Fatalf("stale pending approval remained after cancellation: %#v", state.Pending)
	}
	if !eventContains(state.Events, "approval", "Approval cancelled") {
		t.Fatalf("missing cancellation event: %#v", state.Events)
	}
}

func TestProjectRootCanBeAppliedAndEnumerated(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Alpha", "Beta"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	server := NewServer(state)
	if got, err := server.applyRoot(root); err != nil || got != root {
		t.Fatalf("applyRoot = %q, %v", got, err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/projects", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("projects status = %d: %s", rr.Code, rr.Body.String())
	}
	var response struct {
		Root     string   `json:"root"`
		Projects []string `json:"projects"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Root != root || len(response.Projects) != 2 {
		t.Fatalf("unexpected projects response: %#v", response)
	}
}

func TestEmbeddedUIContainsPersistentApprovalAndRootControls(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		`id="approvalDock"`, `id="approvalApproveBtn"`, `id="approvalRejectBtn"`,
		`id="rootModal"`, `id="rootModalInput"`, `value="auto">Automatisch (Windows)`,
		`<script src="/i18n.js"></script>`, `renderApprovalDock()`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("embedded UI is missing %q", fragment)
		}
	}
}

func TestTranslationCatalogsHaveIdenticalKeys(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	prefix := "const dictionaries = "
	start := strings.Index(text, prefix)
	if start < 0 {
		t.Fatal("translation catalog not found")
	}
	start += len(prefix)
	end := strings.Index(text[start:], ";\n  let current")
	if end < 0 {
		t.Fatal("translation catalog terminator not found")
	}
	var catalogs map[string]map[string]string
	if err := json.Unmarshal([]byte(text[start:start+end]), &catalogs); err != nil {
		t.Fatalf("invalid translation catalog: %v", err)
	}
	de, en := catalogs["de"], catalogs["en"]
	if len(de) == 0 || len(en) == 0 || len(de) != len(en) {
		t.Fatalf("catalog sizes differ: de=%d en=%d", len(de), len(en))
	}
	for key := range de {
		if _, ok := en[key]; !ok {
			t.Fatalf("English catalog is missing %q", key)
		}
	}
}

func TestGeneratedProjectDocsAreBilingual(t *testing.T) {
	project := filepath.Join(t.TempDir(), "Demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.CreateProjectDocs = true
	if err := ensureProjectDocs(project, cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(project, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "## Deutsch") || !strings.Contains(text, "## English") {
			t.Fatalf("%s is not bilingual:\n%s", name, text)
		}
	}
}

func TestLiteralUIEventMessagesHaveEnglishTranslations(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	prefix := "const dictionaries = "
	start := strings.Index(text, prefix)
	if start < 0 {
		t.Fatal("translation catalog not found")
	}
	start += len(prefix)
	end := strings.Index(text[start:], ";\n  let current")
	if end < 0 {
		t.Fatal("translation catalog terminator not found")
	}
	var catalogs map[string]map[string]string
	if err := json.Unmarshal([]byte(text[start:start+end]), &catalogs); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)AddEvent\s*\(\s*UIEvent\s*\{.*?Message:\s*"((?:\\.|[^"\\])*)"`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range re.FindAllSubmatch(source, -1) {
			message := string(match[1])
			if message == "" {
				continue
			}
			if _, ok := catalogs["en"][message]; !ok {
				t.Errorf("%s contains untranslated UI event message %q", entry.Name(), message)
			}
		}
	}
}

func TestClientRoundsIntegerSettingsBeforeSaving(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		"c.ui_left_width=Math.round",
		"c.ui_right_width=Math.round",
		"c.ui_terminal_height=Math.round",
		"c.context_length=Math.round",
		"c.command_timeout_seconds=Math.round",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("integer normalization missing from client: %s", fragment)
		}
	}
}

func TestThreadEventPersistenceIsQueued(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	thread := newThread(project, "test-model")
	state.mu.Lock()
	state.Project = project
	state.CurrentThread = thread.ID
	state.Threads[thread.ID] = thread
	state.mu.Unlock()
	started := time.Now()
	for i := 0; i < 100; i++ {
		state.AddEvent(UIEvent{Type: "progress", Message: "Agent arbeitet"})
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("AddEvent blocked on persistence for %v", elapsed)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(threadsPath()); err == nil && info.Size() > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("queued thread state was not persisted")
}

func TestToolInstallationMetadataIsLocalized(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 5, Language: "en", ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	info := discoverTool(t.TempDir(), "adb", cfg, false)
	if !strings.Contains(info.InstallHint, "Install Android SDK Platform-Tools") {
		t.Fatalf("English install hint missing: %q", info.InstallHint)
	}
	if !strings.Contains(info.InstallPreview, "Installs the official Android SDK Platform-Tools") {
		t.Fatalf("English install preview missing: %q", info.InstallPreview)
	}
}

func TestCloseFlushesLatestQueuedThreadSnapshot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := NewAppState(defaultConfig(), NewOllamaClient())
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	thread := newThread(project, "test-model")
	state.mu.Lock()
	state.Project = project
	state.CurrentThread = thread.ID
	state.Threads[thread.ID] = thread
	state.mu.Unlock()
	state.AddEvent(UIEvent{Type: "progress", Message: "latest event"})
	state.Close()
	data, err := os.ReadFile(threadsPath())
	if err != nil {
		t.Fatalf("thread file missing after Close: %v", err)
	}
	if !strings.Contains(string(data), "latest event") {
		t.Fatalf("latest queued snapshot was not flushed: %s", data)
	}
}

func TestServerRejectsNonLoopbackHostAndCrossOriginMutation(t *testing.T) {
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	server := NewServer(state)

	badHost := httptest.NewRequest(http.MethodGet, "http://evil.example/api/ping", nil)
	badHost.Host = "evil.example"
	badHostResult := httptest.NewRecorder()
	server.ServeHTTP(badHostResult, badHost)
	if badHostResult.Code != http.StatusForbidden {
		t.Fatalf("non-loopback Host must be rejected, got %d", badHostResult.Code)
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/shutdown", nil)
	crossOrigin.Host = "127.0.0.1:32145"
	crossOrigin.Header.Set("Origin", "https://evil.example")
	crossOriginResult := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResult, crossOrigin)
	if crossOriginResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin mutation must be rejected, got %d", crossOriginResult.Code)
	}

	crossSiteForm := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/shutdown", nil)
	crossSiteForm.Host = "127.0.0.1:32145"
	crossSiteForm.Header.Set("Sec-Fetch-Site", "cross-site")
	crossSiteResult := httptest.NewRecorder()
	server.ServeHTTP(crossSiteResult, crossSiteForm)
	if crossSiteResult.Code != http.StatusForbidden {
		t.Fatalf("cross-site form mutation must be rejected, got %d", crossSiteResult.Code)
	}

	trusted := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/ping", nil)
	trusted.Host = "127.0.0.1:32145"
	trustedResult := httptest.NewRecorder()
	server.ServeHTTP(trustedResult, trusted)
	if trustedResult.Code != http.StatusOK {
		t.Fatalf("loopback request should succeed, got %d: %s", trustedResult.Code, trustedResult.Body.String())
	}
}
