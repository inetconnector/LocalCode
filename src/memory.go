// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	memoryScopeProject = "project"
	memoryScopeGlobal  = "global"
)

var sensitiveMemoryPattern = regexp.MustCompile(`(?i)((password|passwort|passwd|api[_ -]?key|secret|token|private[_ -]?key)\b.{0,24}(:|=|\bis\b|\bist\b|\blautet\b|\bheisst\b|\bheißt\b)|ssh-rsa|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func normalizeMemoryEntries(entries []MemoryEntry) []MemoryEntry {
	out := make([]MemoryEntry, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Scope = normalizeMemoryScope(entry.Scope)
		entry.Project = normalizeMemoryProject(entry.Project)
		entry.Content = strings.TrimSpace(entry.Content)
		if entry.ID == "" || entry.Content == "" || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		if entry.Scope == memoryScopeGlobal {
			entry.Project = ""
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = entry.UpdatedAt
		}
		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = entry.CreatedAt
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}

func normalizeMemoryScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case memoryScopeGlobal, "user", "profile":
		return memoryScopeGlobal
	default:
		return memoryScopeProject
	}
}

func normalizeMemoryProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	if abs, err := filepath.Abs(project); err == nil {
		project = abs
	}
	return filepath.Clean(project)
}

func memoryLooksSensitive(content string) bool {
	return sensitiveMemoryPattern.MatchString(content)
}

func (s *AppState) executeMemoryAction(project string, a AgentAction) (string, error) {
	s.mu.Lock()
	cfg := s.Config
	var result string
	var err error
	switch a.Action {
	case "memory_remember":
		result, err = rememberInConfig(&cfg, project, a.Content, a.Scope)
	case "memory_list":
		result = formatMemoryEntries(filterMemories(cfg, project, a.Scope, a.Query))
	case "memory_forget":
		result, err = forgetMemoryInConfig(&cfg, a.MemoryID)
	default:
		err = fmt.Errorf("unsupported memory action %s", a.Action)
	}
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	if a.Action == "memory_remember" || a.Action == "memory_forget" {
		cfg.Memories = normalizeMemoryEntries(cfg.Memories)
		if saveErr := saveConfig(cfg); saveErr != nil {
			s.mu.Unlock()
			return "", saveErr
		}
		s.Config = cfg
	}
	s.mu.Unlock()
	return result, nil
}

func rememberInConfig(cfg *Config, project, content, scope string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("memory content is empty")
	}
	if memoryLooksSensitive(content) {
		return "", errors.New("memory content looks like a secret; passwords, tokens, private keys, and API keys must not be stored")
	}
	scope = normalizeMemoryScope(scope)
	projectKey := ""
	if scope == memoryScopeProject {
		projectKey = normalizeMemoryProject(project)
	}
	now := time.Now().UTC()
	for i := range cfg.Memories {
		if cfg.Memories[i].Scope == scope && cfg.Memories[i].Project == projectKey && strings.EqualFold(cfg.Memories[i].Content, content) {
			cfg.Memories[i].Content = content
			cfg.Memories[i].UpdatedAt = now
			return "MEMORY UPDATED\n" + memoryEntryLine(cfg.Memories[i]), nil
		}
	}
	entry := MemoryEntry{ID: newID(), Scope: scope, Project: projectKey, Content: content, CreatedAt: now, UpdatedAt: now}
	cfg.Memories = append(cfg.Memories, entry)
	return "MEMORY STORED\n" + memoryEntryLine(entry), nil
}

func forgetMemoryInConfig(cfg *Config, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("memory_id is empty")
	}
	for i, entry := range cfg.Memories {
		if entry.ID == id {
			cfg.Memories = append(cfg.Memories[:i], cfg.Memories[i+1:]...)
			return "MEMORY DELETED\n" + memoryEntryLine(entry), nil
		}
	}
	return "", fmt.Errorf("memory not found: %s", id)
}

func filterMemories(cfg Config, project, scope, query string) []MemoryEntry {
	projectKey := normalizeMemoryProject(project)
	scope = strings.TrimSpace(scope)
	query = strings.ToLower(strings.TrimSpace(query))
	var out []MemoryEntry
	for _, entry := range normalizeMemoryEntries(cfg.Memories) {
		if scope != "" && entry.Scope != normalizeMemoryScope(scope) {
			continue
		}
		if entry.Scope == memoryScopeProject && entry.Project != projectKey {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Content), query) && !strings.Contains(strings.ToLower(entry.ID), query) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func memoryContextForAgent(cfg Config, project string) string {
	entries := filterMemories(cfg, project, "", "")
	if len(entries) == 0 {
		return "Keine gespeicherten Erinnerungen."
	}
	lines := make([]string, 0, len(entries)+2)
	lines = append(lines, "GESPEICHERTE ERINNERUNGEN:")
	for _, entry := range entries {
		lines = append(lines, "- "+memoryEntryLine(entry))
	}
	lines = append(lines, "Nutze diese Fakten nur, wenn sie zur aktuellen Aufgabe passen. Speichere keine Geheimnisse; lösche Erinnerungen per memory_forget, wenn der Nutzer es verlangt.")
	return truncateText(strings.Join(lines, "\n"), 12000)
}

func formatMemoryEntries(entries []MemoryEntry) string {
	if len(entries) == 0 {
		return "NO MEMORIES"
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, memoryEntryLine(entry))
	}
	return strings.Join(lines, "\n")
}

func memoryEntryLine(entry MemoryEntry) string {
	project := ""
	if entry.Project != "" {
		project = " project=" + entry.Project
	}
	return fmt.Sprintf("%s scope=%s%s updated=%s content=%s", entry.ID, entry.Scope, project, entry.UpdatedAt.Format(time.RFC3339), entry.Content)
}
