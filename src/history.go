// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChatThread struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Title     string    `json:"title"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Events    []UIEvent `json:"events"`
	Archived  bool      `json:"archived,omitempty"`
}

type ChatThreadSummary struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Title     string    `json:"title"`
	Model     string    `json:"model,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Archived  bool      `json:"archived,omitempty"`
}

var threadFileMu sync.Mutex

type threadFile struct {
	Version int          `json:"version"`
	Threads []ChatThread `json:"threads"`
}

func threadsPath() string {
	return filepath.Join(appDataDir(), "threads.json")
}

func loadThreads() map[string]*ChatThread {
	out := map[string]*ChatThread{}
	data, err := os.ReadFile(threadsPath())
	if err != nil {
		return out
	}
	var file threadFile
	if json.Unmarshal(data, &file) != nil {
		return out
	}
	for i := range file.Threads {
		t := file.Threads[i]
		if t.ID == "" || t.Project == "" {
			continue
		}
		copy := t
		if copy.Title == "" {
			copy.Title = "Neuer Chat"
		}
		out[copy.ID] = &copy
	}
	return out
}

func saveThreads(threads map[string]*ChatThread) error {
	threadFileMu.Lock()
	defer threadFileMu.Unlock()
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		return err
	}
	list := make([]ChatThread, 0, len(threads))
	for _, t := range threads {
		if t == nil {
			continue
		}
		copy := *t
		if len(copy.Events) > 1000 {
			copy.Events = append([]UIEvent(nil), copy.Events[len(copy.Events)-1000:]...)
		}
		list = append(list, copy)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	if len(list) > 250 {
		list = list[:250]
	}
	data, err := json.MarshalIndent(threadFile{Version: 1, Threads: list}, "", "  ")
	if err != nil {
		return err
	}
	path := threadsPath()
	tmp, err := os.CreateTemp(filepath.Dir(path), "threads-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func threadTitle(prompt string) string {
	prompt = strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if prompt == "" {
		return "Dateien analysieren"
	}
	const max = 72
	if len([]rune(prompt)) > max {
		r := []rune(prompt)
		prompt = strings.TrimSpace(string(r[:max])) + "…"
	}
	return prompt
}

func newThread(project, model string) *ChatThread {
	now := time.Now()
	return &ChatThread{ID: newID(), Project: project, Title: "Neuer Chat", Model: model, CreatedAt: now, UpdatedAt: now, Events: []UIEvent{}}
}

func summariesForThreads(threads map[string]*ChatThread) []ChatThreadSummary {
	list := make([]ChatThreadSummary, 0, len(threads))
	for _, t := range threads {
		if t == nil {
			continue
		}
		list = append(list, ChatThreadSummary{ID: t.ID, Project: t.Project, Title: t.Title, Model: t.Model, UpdatedAt: t.UpdatedAt, Archived: t.Archived})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list
}

func (s *AppState) selectProjectThread(project string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *ChatThread
	for _, t := range s.Threads {
		if t.Project == project && !t.Archived && (latest == nil || t.UpdatedAt.After(latest.UpdatedAt)) {
			latest = t
		}
	}
	if latest == nil {
		latest = newThread(project, s.Model)
		s.Threads[latest.ID] = latest
	}
	s.Project = project
	s.CurrentThread = latest.ID
	s.Events = append([]UIEvent(nil), latest.Events...)
	s.Continuation = nil
	if latest.Model != "" {
		s.Model = latest.Model
	}
}

func (s *AppState) NewChat(project string) (ChatThreadSummary, error) {
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return ChatThreadSummary{}, fmt.Errorf("Agent läuft gerade")
	}
	if project == "" {
		project = s.Project
	}
	if project == "" {
		s.mu.Unlock()
		return ChatThreadSummary{}, fmt.Errorf("Kein Projekt ausgewählt")
	}
	t := newThread(project, s.Model)
	s.Threads[t.ID] = t
	s.CurrentThread = t.ID
	s.Project = project
	s.Events = nil
	s.Pending = nil
	s.Continuation = nil
	s.Config.LastProject = project
	cfg := s.Config
	threads := make(map[string]*ChatThread, len(s.Threads))
	for id, item := range s.Threads {
		copy := *item
		copy.Events = append([]UIEvent(nil), item.Events...)
		threads[id] = &copy
	}
	s.mu.Unlock()
	_ = saveConfig(cfg)
	_ = saveThreads(threads)
	return ChatThreadSummary{ID: t.ID, Project: t.Project, Title: t.Title, Model: t.Model, UpdatedAt: t.UpdatedAt}, nil
}

func (s *AppState) SelectChat(id string) error {
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return fmt.Errorf("Agent läuft gerade")
	}
	t := s.Threads[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("Chat nicht gefunden")
	}
	s.CurrentThread = id
	s.Project = t.Project
	s.Events = append([]UIEvent(nil), t.Events...)
	s.Pending = nil
	s.Continuation = nil
	if t.Model != "" {
		s.Model = t.Model
	}
	s.Config.LastProject = t.Project
	s.Config.LastModel = s.Model
	cfg := s.Config
	s.mu.Unlock()
	return saveConfig(cfg)
}

func (s *AppState) threadSummaries() []ChatThreadSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return summariesForThreads(s.Threads)
}

func (s *AppState) ArchiveChat(id string, archived bool) error {
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return fmt.Errorf("Agent läuft gerade")
	}
	t := s.Threads[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("Chat nicht gefunden")
	}
	t.Archived = archived
	t.UpdatedAt = time.Now()
	if archived && s.CurrentThread == id {
		s.CurrentThread = ""
		s.Events = nil
		s.Continuation = nil
		var latest *ChatThread
		for _, candidate := range s.Threads {
			if candidate.Project == t.Project && !candidate.Archived && (latest == nil || candidate.UpdatedAt.After(latest.UpdatedAt)) {
				latest = candidate
			}
		}
		if latest != nil {
			s.CurrentThread = latest.ID
			s.Events = append([]UIEvent(nil), latest.Events...)
		}
	}
	threads := make(map[string]*ChatThread, len(s.Threads))
	for key, item := range s.Threads {
		copy := *item
		copy.Events = append([]UIEvent(nil), item.Events...)
		threads[key] = &copy
	}
	s.mu.Unlock()
	return saveThreads(threads)
}

func (s *AppState) RenameChat(id, title string) error {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return fmt.Errorf("Chatname fehlt")
	}
	if runes := []rune(title); len(runes) > 120 {
		title = strings.TrimSpace(string(runes[:120]))
	}
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return fmt.Errorf("Agent läuft gerade")
	}
	t := s.Threads[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("Chat nicht gefunden")
	}
	t.Title = title
	t.UpdatedAt = time.Now()
	threads := cloneThreads(s.Threads)
	s.mu.Unlock()
	return saveThreads(threads)
}

func (s *AppState) DuplicateChat(id string) (ChatThreadSummary, error) {
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return ChatThreadSummary{}, fmt.Errorf("Agent läuft gerade")
	}
	original := s.Threads[id]
	if original == nil {
		s.mu.Unlock()
		return ChatThreadSummary{}, fmt.Errorf("Chat nicht gefunden")
	}
	now := time.Now()
	copyThread := &ChatThread{
		ID:        newID(),
		Project:   original.Project,
		Title:     original.Title + localizeConfigText(s.Config, " – Kopie", " – Copy"),
		Model:     original.Model,
		CreatedAt: now,
		UpdatedAt: now,
		Events:    append([]UIEvent(nil), original.Events...),
	}
	s.Threads[copyThread.ID] = copyThread
	s.Project = copyThread.Project
	s.CurrentThread = copyThread.ID
	s.Events = append([]UIEvent(nil), copyThread.Events...)
	s.Pending = nil
	s.Continuation = nil
	threads := cloneThreads(s.Threads)
	s.mu.Unlock()
	if err := saveThreads(threads); err != nil {
		return ChatThreadSummary{}, err
	}
	return ChatThreadSummary{ID: copyThread.ID, Project: copyThread.Project, Title: copyThread.Title, Model: copyThread.Model, UpdatedAt: copyThread.UpdatedAt}, nil
}

func (s *AppState) DeleteChat(id string) error {
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return fmt.Errorf("Agent läuft gerade")
	}
	removed := s.Threads[id]
	if removed == nil {
		s.mu.Unlock()
		return fmt.Errorf("Chat nicht gefunden")
	}
	delete(s.Threads, id)
	if s.CurrentThread == id {
		s.CurrentThread = ""
		s.Events = nil
		s.Pending = nil
		s.Continuation = nil
		var latest *ChatThread
		for _, candidate := range s.Threads {
			if candidate.Project == removed.Project && !candidate.Archived && (latest == nil || candidate.UpdatedAt.After(latest.UpdatedAt)) {
				latest = candidate
			}
		}
		if latest == nil {
			latest = newThread(removed.Project, s.Model)
			s.Threads[latest.ID] = latest
		}
		s.CurrentThread = latest.ID
		s.Project = latest.Project
		s.Events = append([]UIEvent(nil), latest.Events...)
	}
	threads := cloneThreads(s.Threads)
	s.mu.Unlock()
	return saveThreads(threads)
}

func cloneThreads(source map[string]*ChatThread) map[string]*ChatThread {
	threads := make(map[string]*ChatThread, len(source))
	for id, item := range source {
		if item == nil {
			continue
		}
		copy := *item
		copy.Events = append([]UIEvent(nil), item.Events...)
		threads[id] = &copy
	}
	return threads
}
