// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ProjectSummary struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Pinned bool   `json:"pinned"`
}

func projectListContains(values []string, path string) bool {
	path = filepath.Clean(path)
	for _, value := range values {
		if strings.EqualFold(filepath.Clean(value), path) {
			return true
		}
	}
	return false
}

func projectListWithout(values []string, path string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !strings.EqualFold(filepath.Clean(value), filepath.Clean(path)) {
			out = append(out, value)
		}
	}
	return out
}

func projectDisplayName(cfg Config, path string) string {
	for key, alias := range cfg.ProjectAliases {
		if strings.EqualFold(filepath.Clean(key), filepath.Clean(path)) && strings.TrimSpace(alias) != "" {
			return strings.TrimSpace(alias)
		}
	}
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return filepath.Clean(path)
	}
	return name
}

func listProjects(cfg Config) ([]ProjectSummary, error) {
	entries, err := os.ReadDir(cfg.RootProjectDir)
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(cfg.RootProjectDir, entry.Name())
		if projectListContains(cfg.HiddenProjects, path) {
			continue
		}
		projects = append(projects, ProjectSummary{
			Path:   path,
			Name:   projectDisplayName(cfg, path),
			Pinned: projectListContains(cfg.PinnedProjects, path),
		})
	}
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].Pinned != projects[j].Pinned {
			return projects[i].Pinned
		}
		left, right := strings.ToLower(projects[i].Name), strings.ToLower(projects[j].Name)
		if left == right {
			return strings.ToLower(projects[i].Path) < strings.ToLower(projects[j].Path)
		}
		return left < right
	})
	return projects, nil
}

func (s *AppState) ProjectAction(path, action, value string) (ProjectSummary, error) {
	path = strings.TrimSpace(path)
	action = strings.ToLower(strings.TrimSpace(action))
	value = strings.TrimSpace(value)
	if path == "" {
		return ProjectSummary{}, errors.New("project path is required")
	}

	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return ProjectSummary{}, errors.New("Agent läuft gerade")
	}
	cfg := s.Config
	s.mu.Unlock()

	full, err := ensureWithinRoot(cfg.RootProjectDir, path)
	if err != nil {
		return ProjectSummary{}, err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return ProjectSummary{}, errors.New("project directory not found")
	}

	switch action {
	case "rename":
		runes := []rune(value)
		if len(runes) > 120 {
			return ProjectSummary{}, errors.New("project name is too long")
		}
		if cfg.ProjectAliases == nil {
			cfg.ProjectAliases = map[string]string{}
		}
		for key := range cfg.ProjectAliases {
			if strings.EqualFold(filepath.Clean(key), filepath.Clean(full)) {
				delete(cfg.ProjectAliases, key)
			}
		}
		if value != "" && !strings.EqualFold(value, filepath.Base(full)) {
			cfg.ProjectAliases[full] = value
		}
	case "pin":
		cfg.PinnedProjects = projectListWithout(cfg.PinnedProjects, full)
		cfg.PinnedProjects = append(cfg.PinnedProjects, full)
		cfg.HiddenProjects = projectListWithout(cfg.HiddenProjects, full)
	case "unpin":
		cfg.PinnedProjects = projectListWithout(cfg.PinnedProjects, full)
	case "remove":
		cfg.PinnedProjects = projectListWithout(cfg.PinnedProjects, full)
		if !projectListContains(cfg.HiddenProjects, full) {
			cfg.HiddenProjects = append(cfg.HiddenProjects, full)
		}
	case "restore":
		cfg.HiddenProjects = projectListWithout(cfg.HiddenProjects, full)
	default:
		return ProjectSummary{}, errors.New("unsupported project action")
	}
	cfg = normalizeConfig(cfg)
	if err := saveConfig(cfg); err != nil {
		return ProjectSummary{}, err
	}

	s.mu.Lock()
	s.Config = cfg
	if action == "remove" && strings.EqualFold(filepath.Clean(s.Project), filepath.Clean(full)) {
		s.Project = ""
		s.CurrentThread = ""
		s.Events = nil
		s.Pending = nil
		s.Continuation = nil
		s.Config.LastProject = ""
		cfg = s.Config
	}
	s.mu.Unlock()
	if action == "remove" && cfg.LastProject == "" {
		_ = saveConfig(cfg)
	}

	return ProjectSummary{Path: full, Name: projectDisplayName(cfg, full), Pinned: projectListContains(cfg.PinnedProjects, full)}, nil
}

func listHiddenProjects(cfg Config) []ProjectSummary {
	projects := make([]ProjectSummary, 0, len(cfg.HiddenProjects))
	for _, path := range cfg.HiddenProjects {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || !pathWithin(cfg.RootProjectDir, path) {
			continue
		}
		projects = append(projects, ProjectSummary{Path: path, Name: projectDisplayName(cfg, path), Pinned: false})
	}
	sort.SliceStable(projects, func(i, j int) bool { return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name) })
	return projects
}
