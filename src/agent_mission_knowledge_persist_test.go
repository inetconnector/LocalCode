// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistentMissionKnowledgePathAndEmptyProject(t *testing.T) {
	// Empty project
	_, _, err := persistentMissionKnowledgePath("")
	if !errors.Is(err, errPersistentKnowledgeEmptyProject) {
		t.Fatalf("expected errPersistentKnowledgeEmptyProject, got: %v", err)
	}

	// Valid project path
	root := filepath.Join(t.TempDir(), "MyProject")
	path, hash, err := persistentMissionKnowledgePath(root)
	if err != nil {
		t.Fatalf("persistentMissionKnowledgePath error: %v", err)
	}
	if hash == "" || len(hash) != 64 {
		t.Fatalf("unexpected sha256 hash: %q", hash)
	}
	if !strings.HasSuffix(path, hash+".json") {
		t.Fatalf("path does not end with hash.json: %s", path)
	}
}

func TestPersistentMissionKnowledgeSaveLoadRoundtrip(t *testing.T) {
	root := t.TempDir()
	now := time.Now().Truncate(time.Second)

	store := &PersistentMissionKnowledgeStore{
		SchemaVersion: PersistentMissionKnowledgeSchemaVersion,
		Items: []MissionKnowledgeItem{
			{
				ID:        "k1",
				Category:  MissionKnowledgeCategoryArchitecture,
				Title:     "Decoupled Scheduler Model",
				Summary:   "Inference parallelism is distinct from logical concurrency.",
				Tags:      []string{"architecture", "scheduler"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "k2",
				Category:  MissionKnowledgeCategoryKnownFailure,
				Title:     "Windows Lock Contention",
				Summary:   "Files must not be held open across child processes.",
				Tags:      []string{"windows", "locks"},
				CreatedAt: now.Add(time.Minute),
				UpdatedAt: now.Add(time.Minute),
			},
		},
	}

	if err := SavePersistentMissionKnowledge(root, store); err != nil {
		t.Fatalf("SavePersistentMissionKnowledge failed: %v", err)
	}

	loaded, err := LoadPersistentMissionKnowledge(root)
	if err != nil {
		t.Fatalf("LoadPersistentMissionKnowledge failed: %v", err)
	}

	if loaded.SchemaVersion != PersistentMissionKnowledgeSchemaVersion {
		t.Fatalf("loaded schema version=%d want %d", loaded.SchemaVersion, PersistentMissionKnowledgeSchemaVersion)
	}
	if len(loaded.Items) != 2 {
		t.Fatalf("loaded items count=%d want 2", len(loaded.Items))
	}
	if loaded.Items[0].Title != "Decoupled Scheduler Model" {
		t.Fatalf("unexpected first item title: %s", loaded.Items[0].Title)
	}
	if loaded.Items[1].Title != "Windows Lock Contention" {
		t.Fatalf("unexpected second item title: %s", loaded.Items[1].Title)
	}
}

func TestPersistentMissionKnowledgeSecretRedaction(t *testing.T) {
	root := t.TempDir()

	item := MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryContract,
		Title:    "API Authentication Contract",
		Summary:  "Authorization: Bearer sk-ant-secretkey1234567890; password=mySuperSecretPass123",
		Tags:     []string{"auth", "contract"},
	}

	recorded, err := RecordPersistentMissionKnowledge(root, item)
	if err != nil {
		t.Fatalf("RecordPersistentMissionKnowledge failed: %v", err)
	}

	if strings.Contains(recorded.Summary, "sk-ant-secretkey") || strings.Contains(recorded.Summary, "mySuperSecretPass123") {
		t.Fatalf("recorded summary contains unredacted secrets: %s", recorded.Summary)
	}

	loaded, err := LoadPersistentMissionKnowledge(root)
	if err != nil {
		t.Fatalf("LoadPersistentMissionKnowledge failed: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("loaded item count=%d want 1", len(loaded.Items))
	}
	if strings.Contains(loaded.Items[0].Summary, "sk-ant-secretkey") || strings.Contains(loaded.Items[0].Summary, "mySuperSecretPass123") {
		t.Fatalf("persisted summary contains unredacted secrets: %s", loaded.Items[0].Summary)
	}
}

func TestPersistentMissionKnowledgeFIFOEviction(t *testing.T) {
	root := t.TempDir()

	// Insert 70 items (max is 64)
	for i := 1; i <= 70; i++ {
		item := MissionKnowledgeItem{
			ID:       string(rune('A' + (i % 26))),
			Category: MissionKnowledgeCategoryTestEvidence,
			Title:    strings.Repeat("X", 10),
			Summary:  strings.Repeat("Y", 50),
		}
		item.Title = item.Title + string(rune('0'+(i%10)))
		if _, err := RecordPersistentMissionKnowledge(root, item); err != nil {
			t.Fatalf("inserting item %d failed: %v", i, err)
		}
	}

	loaded, err := LoadPersistentMissionKnowledge(root)
	if err != nil {
		t.Fatalf("LoadPersistentMissionKnowledge failed: %v", err)
	}

	if len(loaded.Items) > MaxPersistentMissionKnowledgeItems {
		t.Fatalf("item count %d exceeded max limit %d", len(loaded.Items), MaxPersistentMissionKnowledgeItems)
	}
}

func TestPersistentMissionKnowledgeByteSizeCompaction(t *testing.T) {
	root := t.TempDir()

	var bigItems []MissionKnowledgeItem
	for i := 0; i < 60; i++ {
		bigItems = append(bigItems, MissionKnowledgeItem{
			Category: MissionKnowledgeCategoryArchitecture,
			Title:    strings.Repeat("T", maxMissionKnowledgeTitleLen),
			Summary:  strings.Repeat("S", maxMissionKnowledgeSummaryLen),
			Tags:     []string{"big", "compaction", "test"},
		})
	}

	store := &PersistentMissionKnowledgeStore{
		SchemaVersion: PersistentMissionKnowledgeSchemaVersion,
		Items:         bigItems,
	}

	if err := SavePersistentMissionKnowledge(root, store); err != nil {
		t.Fatalf("SavePersistentMissionKnowledge failed: %v", err)
	}

	path, _, _ := persistentMissionKnowledgePath(root)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat on persistent store failed: %v", err)
	}

	if fi.Size() > MaxPersistentMissionKnowledgeBytes {
		t.Fatalf("file size %d exceeded max allowed %d", fi.Size(), MaxPersistentMissionKnowledgeBytes)
	}
}

func TestPersistentMissionKnowledgeCorruptSchema(t *testing.T) {
	root := t.TempDir()
	path, _, err := persistentMissionKnowledgePath(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	// Write invalid JSON
	if err := os.WriteFile(path, []byte("{ invalid json ..."), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = LoadPersistentMissionKnowledge(root)
	if !errors.Is(err, errPersistentKnowledgeCorruptSchema) {
		t.Fatalf("expected errPersistentKnowledgeCorruptSchema, got: %v", err)
	}

	// Write unsupported schema version
	unsupported := `{"schema_version": 999, "items": []}`
	if err := os.WriteFile(path, []byte(unsupported), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = LoadPersistentMissionKnowledge(root)
	if !errors.Is(err, errPersistentKnowledgeCorruptSchema) {
		t.Fatalf("expected errPersistentKnowledgeCorruptSchema for unsupported version, got: %v", err)
	}
}

func TestPersistentMissionKnowledgeSyncFromAppState(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	app := &AppState{}
	item := MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryArchitecture,
		Title:    "Sync Test Decision",
		Summary:  "This should sync to persistent storage cleanly.",
		Tags:     []string{"sync"},
	}

	_, err := app.RecordMissionKnowledge("admission-mission", item)
	if err != nil {
		t.Fatalf("RecordMissionKnowledge failed: %v", err)
	}

	projectRoot := t.TempDir()
	synced, err := app.SyncMissionKnowledgeToPersistence(projectRoot, "admission-mission")
	if err != nil {
		t.Fatalf("SyncMissionKnowledgeToPersistence failed: %v", err)
	}
	if synced != 1 {
		t.Fatalf("synced count=%d want 1", synced)
	}

	loaded, err := LoadPersistentMissionKnowledge(projectRoot)
	if err != nil {
		t.Fatalf("LoadPersistentMissionKnowledge failed: %v", err)
	}
	if len(loaded.Items) != 1 || loaded.Items[0].Title != "Sync Test Decision" {
		t.Fatalf("unexpected synced item in store: %#v", loaded.Items)
	}
}

func TestPersistentMissionKnowledgeHTTPTransport(t *testing.T) {
	appState := &AppState{}
	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	projectRoot := t.TempDir()

	// 1. POST /api/mission/knowledge with persistent: true
	payload := `{"project":"` + filepath.ToSlash(projectRoot) + `","persistent":true,"category":"architecture_decision","title":"Persistent REST Decision","summary":"Summary stored via HTTP endpoint","tags":["http","persistent"]}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/mission/knowledge", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server.URL)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST persistent knowledge status=%d body=%s", resp.StatusCode, body)
	}

	// 2. GET /api/mission/knowledge?project=...&scope=persistent
	getResp, err := server.Client().Get(server.URL + "/api/mission/knowledge?project=" + filepath.ToSlash(projectRoot) + "&scope=persistent")
	if err != nil {
		t.Fatal(err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET persistent knowledge status=%d body=%s", getResp.StatusCode, getBody)
	}

	var getResult struct {
		OK        bool                   `json:"ok"`
		Knowledge []MissionKnowledgeItem `json:"knowledge"`
		Count     int                    `json:"count"`
	}
	if err := json.Unmarshal(getBody, &getResult); err != nil {
		t.Fatalf("unmarshal get result failed: %v", err)
	}
	if !getResult.OK || getResult.Count != 1 || getResult.Knowledge[0].Title != "Persistent REST Decision" {
		t.Fatalf("unexpected persistent get result: %#v", getResult)
	}
}

func TestFilterMissionKnowledge(t *testing.T) {
	items := []MissionKnowledgeItem{
		{
			ID:       "1",
			Category: MissionKnowledgeCategoryArchitecture,
			Title:    "DB Layout",
			Summary:  "Uses SQLite WAL mode",
			Tags:     []string{"db", "storage"},
		},
		{
			ID:       "2",
			Category: MissionKnowledgeCategoryContract,
			Title:    "API Protocol",
			Summary:  "REST JSON endpoints",
			Tags:     []string{"api", "http"},
		},
		{
			ID:       "3",
			Category: MissionKnowledgeCategoryKnownFailure,
			Title:    "Port Conflict",
			Summary:  "Port 8080 might be taken by another service",
			Tags:     []string{"network"},
		},
	}

	// Filter by Category
	arch := filterMissionKnowledge(items, MissionKnowledgeCategoryArchitecture, "", "")
	if len(arch) != 1 || arch[0].ID != "1" {
		t.Fatalf("category filter failed: %#v", arch)
	}

	// Filter by Tag
	apiItems := filterMissionKnowledge(items, "", "api", "")
	if len(apiItems) != 1 || apiItems[0].ID != "2" {
		t.Fatalf("tag filter failed: %#v", apiItems)
	}

	// Filter by Query (matching title or summary)
	q1 := filterMissionKnowledge(items, "", "", "sqlite")
	if len(q1) != 1 || q1[0].ID != "1" {
		t.Fatalf("query filter for summary failed: %#v", q1)
	}

	q2 := filterMissionKnowledge(items, "", "", "conflict")
	if len(q2) != 1 || q2[0].ID != "3" {
		t.Fatalf("query filter for title failed: %#v", q2)
	}

	// Filter returning empty
	none := filterMissionKnowledge(items, "", "", "nonexistent")
	if len(none) != 0 {
		t.Fatalf("expected empty filter result, got %#v", none)
	}
}

func TestFormatMissionKnowledgeForPrompt(t *testing.T) {
	if got := formatMissionKnowledgeForPrompt(nil, 1000); got != "" {
		t.Fatalf("expected empty prompt for nil items, got %q", got)
	}

	items := []MissionKnowledgeItem{
		{
			Category:   MissionKnowledgeCategoryArchitecture,
			Title:      "Architecture Choice",
			Summary:    "We chose Go and vanilla JS.",
			SourcePath: "src/main.go",
			Tags:       []string{"go", "js"},
			CreatedAt:  time.Now(),
		},
		{
			Category:  MissionKnowledgeCategoryKnownFailure,
			Title:     "Deadlock issue",
			Summary:   "Never hold mutex across network calls.",
			CreatedAt: time.Now().Add(time.Second),
		},
	}

	prompt := formatMissionKnowledgeForPrompt(items, 5000)
	if !strings.Contains(prompt, "# Mission Knowledge & Memory") {
		t.Errorf("missing header in prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "Architecture Decisions") || !strings.Contains(prompt, "We chose Go and vanilla JS.") {
		t.Errorf("missing architecture section: %s", prompt)
	}
	if !strings.Contains(prompt, "Known Failures & Gotchas") || !strings.Contains(prompt, "Never hold mutex across network calls.") {
		t.Errorf("missing known failure section: %s", prompt)
	}
}

func TestRecordPersistentMissionKnowledgeUpdateExisting(t *testing.T) {
	root := t.TempDir()

	item1 := MissionKnowledgeItem{
		ID:       "fixed-id-1",
		Category: MissionKnowledgeCategoryArchitecture,
		Title:    "Initial Title",
		Summary:  "Initial Summary",
	}

	if _, err := RecordPersistentMissionKnowledge(root, item1); err != nil {
		t.Fatal(err)
	}

	// Update the same item
	item2 := MissionKnowledgeItem{
		ID:       "fixed-id-1",
		Category: MissionKnowledgeCategoryArchitecture,
		Title:    "Updated Title",
		Summary:  "Updated Summary",
	}

	if _, err := RecordPersistentMissionKnowledge(root, item2); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPersistentMissionKnowledge(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(loaded.Items))
	}
	if loaded.Items[0].Title != "Updated Title" || loaded.Items[0].Summary != "Updated Summary" {
		t.Fatalf("item was not updated: %#v", loaded.Items[0])
	}
}

func TestSavePersistentMissionKnowledgeNilStore(t *testing.T) {
	root := t.TempDir()
	if err := SavePersistentMissionKnowledge(root, nil); err == nil {
		t.Fatal("expected error saving nil store")
	}
}

func TestNormalizeMissionKnowledgeCategoryAliases(t *testing.T) {
	aliases := map[string]MissionKnowledgeCategory{
		"decision":     MissionKnowledgeCategoryArchitecture,
		"arch":         MissionKnowledgeCategoryArchitecture,
		"interface":    MissionKnowledgeCategoryContract,
		"spec":         MissionKnowledgeCategoryContract,
		"bug":          MissionKnowledgeCategoryKnownFailure,
		"gotcha":       MissionKnowledgeCategoryKnownFailure,
		"verification": MissionKnowledgeCategoryTestEvidence,
		"evidence":     MissionKnowledgeCategoryTestEvidence,
	}

	for alias, expected := range aliases {
		cat, err := normalizeMissionKnowledgeCategory(alias)
		if err != nil {
			t.Errorf("alias %q failed: %v", alias, err)
		}
		if cat != expected {
			t.Errorf("alias %q returned %s, want %s", alias, cat, expected)
		}
	}

	if _, err := normalizeMissionKnowledgeCategory("invalid_unknown"); err == nil {
		t.Error("expected error for invalid category")
	}
}
