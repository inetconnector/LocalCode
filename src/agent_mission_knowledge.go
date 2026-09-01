// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MissionKnowledgeCategory string

const (
	MissionKnowledgeCategoryArchitecture MissionKnowledgeCategory = "architecture_decision"
	MissionKnowledgeCategoryContract     MissionKnowledgeCategory = "subsystem_contract"
	MissionKnowledgeCategoryKnownFailure MissionKnowledgeCategory = "known_failure"
	MissionKnowledgeCategoryTestEvidence MissionKnowledgeCategory = "test_evidence"
)

const (
	maxMissionKnowledgeItems      = 64
	maxMissionKnowledgeTitleLen   = 120
	maxMissionKnowledgeSummaryLen = 1200
	maxMissionKnowledgeTagLen     = 32
	maxMissionKnowledgeTags       = 8
	maxMissionKnowledgePromptLen  = 8000
)

var (
	errMissionKnowledgeInvalidCategory = errors.New("invalid mission knowledge category")
	errMissionKnowledgeMissingTitle    = errors.New("mission knowledge title is required")
	errMissionKnowledgeMissingSummary  = errors.New("mission knowledge summary is required")
	errMissionKnowledgeLimitExceeded   = errors.New("mission knowledge capacity limit reached (max 64 items)")
	errMissionKnowledgeNotFound        = errors.New("mission not found or inactive")
)

type MissionKnowledgeItem struct {
	ID              string                   `json:"id"`
	MissionID       string                   `json:"mission_id"`
	Category        MissionKnowledgeCategory `json:"category"`
	Title           string                   `json:"title"`
	Summary         string                   `json:"summary"`
	Tags            []string                 `json:"tags,omitempty"`
	CreatedByTaskID string                   `json:"created_by_task_id,omitempty"`
	SourcePath      string                   `json:"source_path,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
}

func normalizeMissionKnowledgeCategory(raw string) (MissionKnowledgeCategory, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "architecture", "architecture_decision", "decision", "arch":
		return MissionKnowledgeCategoryArchitecture, nil
	case "contract", "subsystem_contract", "interface", "spec":
		return MissionKnowledgeCategoryContract, nil
	case "failure", "known_failure", "bug", "issue", "gotcha":
		return MissionKnowledgeCategoryKnownFailure, nil
	case "test", "test_evidence", "evidence", "test_result", "verification":
		return MissionKnowledgeCategoryTestEvidence, nil
	default:
		return "", fmt.Errorf("%w: %q (allowed: architecture_decision, subsystem_contract, known_failure, test_evidence)", errMissionKnowledgeInvalidCategory, raw)
	}
}

func validateMissionKnowledgeItem(item MissionKnowledgeItem) error {
	if _, err := normalizeMissionKnowledgeCategory(string(item.Category)); err != nil {
		return err
	}
	if strings.TrimSpace(item.Title) == "" {
		return errMissionKnowledgeMissingTitle
	}
	if strings.TrimSpace(item.Summary) == "" {
		return errMissionKnowledgeMissingSummary
	}
	return nil
}

func sanitizeMissionKnowledgeItem(item MissionKnowledgeItem) MissionKnowledgeItem {
	item.Title = sanitizeRunJournalText(item.Title, maxMissionKnowledgeTitleLen)
	item.Summary = sanitizeRunJournalText(item.Summary, maxMissionKnowledgeSummaryLen)
	item.CreatedByTaskID = sanitizeRunJournalText(item.CreatedByTaskID, 160)
	if item.SourcePath != "" {
		item.SourcePath = filepath.ToSlash(filepath.Clean(sanitizeRunJournalText(item.SourcePath, 260)))
	}
	var cleanTags []string
	seen := map[string]bool{}
	for _, tag := range item.Tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		if clean == "" || seen[clean] {
			continue
		}
		if len(clean) > maxMissionKnowledgeTagLen {
			clean = clean[:maxMissionKnowledgeTagLen]
		}
		seen[clean] = true
		cleanTags = append(cleanTags, clean)
		if len(cleanTags) >= maxMissionKnowledgeTags {
			break
		}
	}
	item.Tags = cleanTags
	return item
}

func filterMissionKnowledge(items []MissionKnowledgeItem, category MissionKnowledgeCategory, tag string, query string) []MissionKnowledgeItem {
	var out []MissionKnowledgeItem
	query = strings.ToLower(strings.TrimSpace(query))
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, item := range items {
		if category != "" && item.Category != category {
			continue
		}
		if tag != "" {
			hasTag := false
			for _, t := range item.Tags {
				if t == tag {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}
		if query != "" {
			title := strings.ToLower(item.Title)
			summary := strings.ToLower(item.Summary)
			if !strings.Contains(title, query) && !strings.Contains(summary, query) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func formatMissionKnowledgeForPrompt(items []MissionKnowledgeItem, maxBytes int) string {
	if len(items) == 0 {
		return ""
	}
	if maxBytes <= 0 || maxBytes > maxMissionKnowledgePromptLen {
		maxBytes = maxMissionKnowledgePromptLen
	}
	var b strings.Builder
	b.WriteString("# Mission Knowledge & Memory\n\n")

	// Group by category
	byCategory := map[MissionKnowledgeCategory][]MissionKnowledgeItem{}
	for _, item := range items {
		byCategory[item.Category] = append(byCategory[item.Category], item)
	}

	categories := []MissionKnowledgeCategory{
		MissionKnowledgeCategoryArchitecture,
		MissionKnowledgeCategoryContract,
		MissionKnowledgeCategoryKnownFailure,
		MissionKnowledgeCategoryTestEvidence,
	}

	categoryTitles := map[MissionKnowledgeCategory]string{
		MissionKnowledgeCategoryArchitecture: "## Architecture Decisions",
		MissionKnowledgeCategoryContract:     "## Subsystem Contracts & Interfaces",
		MissionKnowledgeCategoryKnownFailure: "## Known Failures & Gotchas",
		MissionKnowledgeCategoryTestEvidence: "## Verified Test Evidence",
	}

	for _, cat := range categories {
		group := byCategory[cat]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].CreatedAt.Before(group[j].CreatedAt)
		})
		b.WriteString(categoryTitles[cat] + "\n\n")
		for _, item := range group {
			var line strings.Builder
			line.WriteString(fmt.Sprintf("- **%s**", item.Title))
			if item.SourcePath != "" {
				line.WriteString(fmt.Sprintf(" (`%s`)", item.SourcePath))
			}
			line.WriteString(fmt.Sprintf(": %s\n", item.Summary))
			if len(item.Tags) > 0 {
				line.WriteString(fmt.Sprintf("  *Tags: %s*\n", strings.Join(item.Tags, ", ")))
			}
			if b.Len()+line.Len() > maxBytes {
				break
			}
			b.WriteString(line.String())
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func (s *AppState) RecordMissionKnowledge(missionID string, item MissionKnowledgeItem) (MissionKnowledgeItem, error) {
	missionID = strings.TrimSpace(missionID)
	cat, err := normalizeMissionKnowledgeCategory(string(item.Category))
	if err != nil {
		return MissionKnowledgeItem{}, err
	}
	item.Category = cat
	if err := validateMissionKnowledgeItem(item); err != nil {
		return MissionKnowledgeItem{}, err
	}
	item = sanitizeMissionKnowledgeItem(item)

	now := time.Now()
	if item.ID == "" {
		item.ID = newID()
	}
	item.MissionID = missionID
	item.CreatedAt = now
	item.UpdatedAt = now

	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()

	state, err := loadRunJournal()
	if err != nil {
		return MissionKnowledgeItem{}, err
	}
	if state == nil || state.Terminal || state.Mission == nil || (missionID != "" && state.Mission.MissionID != missionID) {
		return MissionKnowledgeItem{}, errMissionKnowledgeNotFound
	}
	if item.MissionID == "" {
		item.MissionID = state.Mission.MissionID
	}

	if len(state.Mission.Knowledge) >= maxMissionKnowledgeItems {
		return MissionKnowledgeItem{}, errMissionKnowledgeLimitExceeded
	}

	state.Mission.Knowledge = append(state.Mission.Knowledge, item)
	state.Mission.UpdatedAt = now
	state.UpdatedAt = now
	state.Events = append(state.Events, RunJournalEvent{
		At:      now,
		Type:    "mission_knowledge_recorded",
		Action:  string(item.Category),
		Message: sanitizeRunJournalText(item.Title, 160),
	})
	if len(state.Events) > 64 {
		state.Events = append([]RunJournalEvent(nil), state.Events[len(state.Events)-64:]...)
	}

	if err := writeRunJournalUnlocked(*state); err != nil {
		return MissionKnowledgeItem{}, err
	}
	return item, nil
}

func (s *AppState) ListMissionKnowledge(missionID string) ([]MissionKnowledgeItem, error) {
	missionID = strings.TrimSpace(missionID)

	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()

	state, err := loadRunJournal()
	if err != nil {
		return nil, err
	}
	if state == nil || state.Mission == nil || (missionID != "" && state.Mission.MissionID != missionID) {
		return []MissionKnowledgeItem{}, nil
	}
	out := make([]MissionKnowledgeItem, len(state.Mission.Knowledge))
	copy(out, state.Mission.Knowledge)
	return out, nil
}
