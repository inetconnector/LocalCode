// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLanguageSettingsAndResponseLanguage(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 4, Language: "de", PreferredLanguage: "Deutsch"})
	if cfg.SchemaVersion != 7 {
		t.Fatalf("schema = %d, want 7", cfg.SchemaVersion)
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
		_, err := state.requestApprovalWithPreview(ctx, t.TempDir(), AgentAction{Action: "run_command", Message: "Run tests", Command: "go test ./..."}, "preview")
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
		Root     string           `json:"root"`
		Projects []ProjectSummary `json:"projects"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Root != root || len(response.Projects) != 2 {
		t.Fatalf("unexpected projects response: %#v", response)
	}
	if response.Projects[0].Name != "Alpha" || response.Projects[1].Name != "Beta" {
		t.Fatalf("unexpected project ordering: %#v", response.Projects)
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
		`id="contextMenu"`, `id="actionModal"`, `showProjectMenu(`, `showThreadMenu(`,
		`Neue Aufgabe starten`, `project-new-task`, `thread-more`,
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

func TestProjectCatalogActionsAndOrdering(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	alpha := filepath.Join(root, "Alpha")
	beta := filepath.Join(root, "Beta")
	for _, path := range []string{alpha, beta} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	if _, err := state.ProjectAction(beta, "rename", "My Beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(beta, "pin", ""); err != nil {
		t.Fatal(err)
	}
	state.mu.RLock()
	cfg = state.Config
	state.mu.RUnlock()
	projects, err := listProjects(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].Path != beta || projects[0].Name != "My Beta" || !projects[0].Pinned {
		t.Fatalf("unexpected pinned catalog: %#v", projects)
	}
	if _, err := state.ProjectAction(beta, "remove", ""); err != nil {
		t.Fatal(err)
	}
	state.mu.RLock()
	cfg = state.Config
	state.mu.RUnlock()
	projects, err = listProjects(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Path != alpha {
		t.Fatalf("removed project still visible: %#v", projects)
	}
}

func TestThreadContextActionsPersist(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	thread, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RenameChat(thread.ID, "Renamed task"); err != nil {
		t.Fatal(err)
	}
	duplicate, err := state.DuplicateChat(thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(duplicate.Title, "Kopie") && !strings.Contains(duplicate.Title, "Copy") {
		t.Fatalf("unexpected duplicate title: %q", duplicate.Title)
	}
	if err := state.DeleteChat(thread.ID); err != nil {
		t.Fatal(err)
	}
	state.mu.RLock()
	_, exists := state.Threads[thread.ID]
	_, duplicateExists := state.Threads[duplicate.ID]
	state.mu.RUnlock()
	if exists || !duplicateExists {
		t.Fatalf("thread actions were not applied: deleted=%v duplicate=%v", exists, duplicateExists)
	}
}

func TestProjectAndThreadContextAPIs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	project := filepath.Join(root, "Demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	server := NewServer(state)

	post := func(path string, body any) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145"+path, strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		return rr
	}

	if rr := post("/api/project-action", map[string]any{"path": project, "action": "rename", "value": "Demo Alias"}); rr.Code != http.StatusOK {
		t.Fatalf("project rename failed: %d %s", rr.Code, rr.Body.String())
	}
	projectsReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/projects", nil)
	projectsRR := httptest.NewRecorder()
	server.ServeHTTP(projectsRR, projectsReq)
	if !strings.Contains(projectsRR.Body.String(), "Demo Alias") {
		t.Fatalf("project alias missing from list: %s", projectsRR.Body.String())
	}

	thread, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	if rr := post("/api/rename-chat", map[string]any{"id": thread.ID, "title": "New title"}); rr.Code != http.StatusOK {
		t.Fatalf("thread rename failed: %d %s", rr.Code, rr.Body.String())
	}
	duplicateRR := post("/api/duplicate-chat", map[string]any{"id": thread.ID})
	if duplicateRR.Code != http.StatusOK || !strings.Contains(duplicateRR.Body.String(), "thread") {
		t.Fatalf("thread duplicate failed: %d %s", duplicateRR.Code, duplicateRR.Body.String())
	}
	if rr := post("/api/delete-chat", map[string]any{"id": thread.ID}); rr.Code != http.StatusOK {
		t.Fatalf("thread delete failed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestApprovalDockProvidesProjectAndGlobalPersistentScopes(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		`id="approvalAlwaysBtn"`, `approveAction('project')`,
		`id="approvalGlobalBtn"`, `approveAction('global')`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("persistent approval scope missing from client: %s", fragment)
		}
	}
}

func TestSnapshotCanTargetIndependentThread(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	first, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	state.AddEvent(UIEvent{Type: "final", Message: "first task"})
	second, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	state.AddEvent(UIEvent{Type: "final", Message: "second task"})
	server := NewServer(state)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/snapshot?thread_id="+url.QueryEscape(first.ID), nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("targeted snapshot failed: %d %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Events        []UIEvent `json:"events"`
		CurrentThread string    `json:"current_thread"`
		Running       bool      `json:"running"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CurrentThread != first.ID || len(payload.Events) != 1 || payload.Events[0].Message != "first task" {
		t.Fatalf("wrong task snapshot: %#v", payload)
	}
	if payload.Running {
		t.Fatal("inactive task window must not report another task as running")
	}
	if payload.Events[0].ThreadID != first.ID {
		t.Fatalf("event was not tagged with its task: %#v", payload.Events[0])
	}
	_ = second
}

func TestContextMenusExposeCodexStyleProjectAndTaskActions(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		`class="new-task-row"`, `project-new-task`, `project-rename`, `project-open-visualstudio`,
		`project-open-explorer`, `project-pin`, `project-remove`, `thread-rename`,
		`thread-open-window`, `thread-archive`, `/api/open-chat-window`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("context action missing from client: %s", fragment)
		}
	}
}

func TestOpenChatWindowAPIUsesTaskURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	thread, err := state.NewChat(project)
	if err != nil {
		t.Fatal(err)
	}
	oldLauncher := launchTaskWindow
	defer func() { launchTaskWindow = oldLauncher }()
	var launched string
	launchTaskWindow = func(value string) error {
		launched = value
		return nil
	}
	server := NewServer(state)
	body, _ := json.Marshal(map[string]string{"id": thread.ID})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/open-chat-window", strings.NewReader(string(body)))
	req.Host = "127.0.0.1:32145"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("open task window failed: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(launched, "?thread="+url.QueryEscape(thread.ID)) {
		t.Fatalf("task ID missing from launched URL: %q", launched)
	}
}
