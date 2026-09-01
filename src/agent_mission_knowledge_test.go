// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMissionKnowledgeNormalizationAndValidation(t *testing.T) {
	// Category normalization
	cases := []struct {
		input string
		want  MissionKnowledgeCategory
	}{
		{"architecture", MissionKnowledgeCategoryArchitecture},
		{"architecture_decision", MissionKnowledgeCategoryArchitecture},
		{"arch", MissionKnowledgeCategoryArchitecture},
		{"decision", MissionKnowledgeCategoryArchitecture},
		{"contract", MissionKnowledgeCategoryContract},
		{"subsystem_contract", MissionKnowledgeCategoryContract},
		{"interface", MissionKnowledgeCategoryContract},
		{"spec", MissionKnowledgeCategoryContract},
		{"failure", MissionKnowledgeCategoryKnownFailure},
		{"known_failure", MissionKnowledgeCategoryKnownFailure},
		{"bug", MissionKnowledgeCategoryKnownFailure},
		{"gotcha", MissionKnowledgeCategoryKnownFailure},
		{"test", MissionKnowledgeCategoryTestEvidence},
		{"test_evidence", MissionKnowledgeCategoryTestEvidence},
		{"evidence", MissionKnowledgeCategoryTestEvidence},
		{"verification", MissionKnowledgeCategoryTestEvidence},
	}

	for _, tc := range cases {
		got, err := normalizeMissionKnowledgeCategory(tc.input)
		if err != nil {
			t.Fatalf("normalizeCategory(%q) error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeCategory(%q)=%q want %q", tc.input, got, tc.want)
		}
	}

	// Invalid category
	if _, err := normalizeMissionKnowledgeCategory("unknown_category"); !errors.Is(err, errMissionKnowledgeInvalidCategory) {
		t.Fatalf("expected errMissionKnowledgeInvalidCategory, got: %v", err)
	}

	// Missing title
	itemNoTitle := MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryArchitecture,
		Summary:  "Some summary",
	}
	if err := validateMissionKnowledgeItem(itemNoTitle); !errors.Is(err, errMissionKnowledgeMissingTitle) {
		t.Fatalf("expected errMissionKnowledgeMissingTitle, got: %v", err)
	}

	// Missing summary
	itemNoSummary := MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryArchitecture,
		Title:    "Some title",
	}
	if err := validateMissionKnowledgeItem(itemNoSummary); !errors.Is(err, errMissionKnowledgeMissingSummary) {
		t.Fatalf("expected errMissionKnowledgeMissingSummary, got: %v", err)
	}
}

func TestMissionKnowledgeSanitizationAndBounds(t *testing.T) {
	longTitle := strings.Repeat("A", 200)
	longSummary := "api_key: supersecret123; " + strings.Repeat("B", 2000)
	tags := []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6", "tag7", "tag8", "tag9", "tag10"}

	item := MissionKnowledgeItem{
		Title:      longTitle,
		Summary:    longSummary,
		Tags:       tags,
		SourcePath: `src\subsystem\contract.go`,
	}

	sanitized := sanitizeMissionKnowledgeItem(item)

	if len(sanitized.Title) > maxMissionKnowledgeTitleLen+3 || !strings.HasSuffix(sanitized.Title, "…") {
		t.Fatalf("title length=%d want <= %d with ellipsis", len(sanitized.Title), maxMissionKnowledgeTitleLen+3)
	}
	if len(sanitized.Summary) > maxMissionKnowledgeSummaryLen+3 || !strings.HasSuffix(sanitized.Summary, "…") {
		t.Fatalf("summary length=%d want <= %d with ellipsis", len(sanitized.Summary), maxMissionKnowledgeSummaryLen+3)
	}
	if strings.Contains(sanitized.Summary, "supersecret123") {
		t.Fatalf("secret was not redacted in summary: %s", sanitized.Summary)
	}
	if len(sanitized.Tags) > maxMissionKnowledgeTags {
		t.Fatalf("tags count=%d > %d", len(sanitized.Tags), maxMissionKnowledgeTags)
	}
	if sanitized.SourcePath != "src/subsystem/contract.go" {
		t.Fatalf("source path=%q want src/subsystem/contract.go", sanitized.SourcePath)
	}
}

func TestMissionKnowledgePromptFormatting(t *testing.T) {
	now := time.Now()
	items := []MissionKnowledgeItem{
		{
			Category:  MissionKnowledgeCategoryArchitecture,
			Title:     "Decouple Scheduler from Child Execution",
			Summary:   "Children run detached copies outside the scheduler lock.",
			Tags:      []string{"scheduler", "concurrency"},
			CreatedAt: now,
		},
		{
			Category:   MissionKnowledgeCategoryContract,
			Title:      "Child Agent Result Schema",
			Summary:    "Child agents emit structured JSON AgentResult.",
			SourcePath: "src/agent_team_types.go",
			CreatedAt:  now.Add(time.Minute),
		},
		{
			Category:  MissionKnowledgeCategoryKnownFailure,
			Title:     "Windows Lock Contention",
			Summary:   "Do not hold file handles open across child execution.",
			CreatedAt: now.Add(2 * time.Minute),
		},
		{
			Category:  MissionKnowledgeCategoryTestEvidence,
			Title:     "Scheduler Fairness 14-Task DAG",
			Summary:   "Verified drain without starvation or memory leaks.",
			CreatedAt: now.Add(3 * time.Minute),
		},
	}

	formatted := formatMissionKnowledgeForPrompt(items, 8000)
	if !strings.Contains(formatted, "# Mission Knowledge & Memory") {
		t.Fatal("header missing in prompt formatting")
	}
	if !strings.Contains(formatted, "## Architecture Decisions") {
		t.Fatal("Architecture category missing")
	}
	if !strings.Contains(formatted, "## Subsystem Contracts & Interfaces") {
		t.Fatal("Contract category missing")
	}
	if !strings.Contains(formatted, "## Known Failures & Gotchas") {
		t.Fatal("Known failure category missing")
	}
	if !strings.Contains(formatted, "## Verified Test Evidence") {
		t.Fatal("Test evidence category missing")
	}
	if !strings.Contains(formatted, "Decouple Scheduler from Child Execution") {
		t.Fatal("title missing")
	}

	// Empty list
	if formatMissionKnowledgeForPrompt(nil, 8000) != "" {
		t.Fatal("expected empty string for nil items")
	}
}

func TestMissionKnowledgeFilter(t *testing.T) {
	items := []MissionKnowledgeItem{
		{
			Category: MissionKnowledgeCategoryArchitecture,
			Title:    "Memory System Design",
			Summary:  "Bounded memory for missions.",
			Tags:     []string{"memory", "architecture"},
		},
		{
			Category: MissionKnowledgeCategoryContract,
			Title:    "Storage Interface",
			Summary:  "Filesystem contract.",
			Tags:     []string{"storage"},
		},
	}

	// Filter by category
	arch := filterMissionKnowledge(items, MissionKnowledgeCategoryArchitecture, "", "")
	if len(arch) != 1 || arch[0].Title != "Memory System Design" {
		t.Fatalf("filtered by category=%#v", arch)
	}

	// Filter by tag
	storage := filterMissionKnowledge(items, "", "storage", "")
	if len(storage) != 1 || storage[0].Title != "Storage Interface" {
		t.Fatalf("filtered by tag=%#v", storage)
	}

	// Filter by query
	queryResult := filterMissionKnowledge(items, "", "", "bounded")
	if len(queryResult) != 1 || queryResult[0].Title != "Memory System Design" {
		t.Fatalf("filtered by query=%#v", queryResult)
	}
}

func TestMissionKnowledgeAppStateRecordAndList(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{}

	// Record item
	item := MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryArchitecture,
		Title:    "Use Single Run Journal",
		Summary:  "active-run.json is the single durable recovery authority.",
		Tags:     []string{"recovery", "journal"},
	}
	recorded, err := appState.RecordMissionKnowledge("admission-mission", item)
	if err != nil {
		t.Fatalf("RecordMissionKnowledge error: %v", err)
	}
	if recorded.ID == "" || recorded.MissionID != "admission-mission" {
		t.Fatalf("unexpected recorded item: %#v", recorded)
	}

	// List items
	list, err := appState.ListMissionKnowledge("admission-mission")
	if err != nil {
		t.Fatalf("ListMissionKnowledge error: %v", err)
	}
	if len(list) != 1 || list[0].Title != "Use Single Run Journal" {
		t.Fatalf("unexpected list: %#v", list)
	}

	// Check on-disk persistence
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Mission.Knowledge) != 1 || loaded.Mission.Knowledge[0].Title != "Use Single Run Journal" {
		t.Fatalf("knowledge not persisted in journal: %#v", loaded.Mission.Knowledge)
	}

	// Rejection on non-matching mission ID
	_, err = appState.RecordMissionKnowledge("non-existent-mission", item)
	if !errors.Is(err, errMissionKnowledgeNotFound) {
		t.Fatalf("expected errMissionKnowledgeNotFound, got: %v", err)
	}

	// Fill to max capacity and test limit rejection
	for i := 1; i < maxMissionKnowledgeItems; i++ {
		_, err := appState.RecordMissionKnowledge("admission-mission", MissionKnowledgeItem{
			Category: MissionKnowledgeCategoryContract,
			Title:    fmt.Sprintf("Contract %d", i),
			Summary:  "Description",
		})
		if err != nil {
			t.Fatalf("recording item %d failed: %v", i, err)
		}
	}

	// 65th item must fail
	_, err = appState.RecordMissionKnowledge("admission-mission", MissionKnowledgeItem{
		Category: MissionKnowledgeCategoryContract,
		Title:    "Overflow Item",
		Summary:  "Should be rejected",
	})
	if !errors.Is(err, errMissionKnowledgeLimitExceeded) {
		t.Fatalf("expected errMissionKnowledgeLimitExceeded, got: %v", err)
	}
}

func TestMissionKnowledgeHTTPTransport(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{}
	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	// 1. POST /api/mission/knowledge
	payload := `{"mission_id":"admission-mission","category":"architecture_decision","title":"Test Arch Decision","summary":"Summary of architecture decision","tags":["arch","test"]}`
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
		t.Fatalf("POST knowledge status=%d body=%s", resp.StatusCode, body)
	}

	var postResult struct {
		OK   bool                 `json:"ok"`
		Item MissionKnowledgeItem `json:"item"`
	}
	if err := json.Unmarshal(body, &postResult); err != nil {
		t.Fatalf("unmarshal post result failed: %v", err)
	}
	if !postResult.OK || postResult.Item.Title != "Test Arch Decision" {
		t.Fatalf("unexpected post result: %#v", postResult)
	}

	// 2. GET /api/mission/knowledge
	getResp, err := server.Client().Get(server.URL + "/api/mission/knowledge?mission_id=admission-mission&category=architecture")
	if err != nil {
		t.Fatal(err)
	}
	getBody, _ := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET knowledge status=%d body=%s", getResp.StatusCode, getBody)
	}

	var getResult struct {
		OK        bool                   `json:"ok"`
		Knowledge []MissionKnowledgeItem `json:"knowledge"`
		Count     int                    `json:"count"`
	}
	if err := json.Unmarshal(getBody, &getResult); err != nil {
		t.Fatalf("unmarshal get result failed: %v", err)
	}
	if !getResult.OK || getResult.Count != 1 || getResult.Knowledge[0].Title != "Test Arch Decision" {
		t.Fatalf("unexpected get result: %#v", getResult)
	}
}
