// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type ProjectSummary struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Pinned bool   `json:"pinned"`
}

type ProjectDeletePreview struct {
	Path                 string `json:"path"`
	Name                 string `json:"name"`
	Empty                bool   `json:"empty"`
	Files                int    `json:"files"`
	Directories          int    `json:"directories"`
	Symlinks             int    `json:"symlinks"`
	Bytes                 int64  `json:"bytes"`
	ConfirmationRequired bool   `json:"confirmation_required"`
	Confirmation         string `json:"confirmation"`
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

func projectListReplace(values []string, oldPath, newPath string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.EqualFold(filepath.Clean(value), filepath.Clean(oldPath)) {
			if !projectListContains(out, newPath) {
				out = append(out, newPath)
			}
			continue
		}
		out = append(out, value)
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

func validProjectFolderName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("folder name is required")
	}
	if !utf8.ValidString(name) || len([]rune(name)) > 120 {
		return errors.New("folder name is invalid or too long")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `<>:"/\\|?*`) {
		return errors.New("folder name contains invalid characters")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("folder name may not end with a dot or space")
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for i := 1; i <= 9; i++ {
		reserved[fmt.Sprintf("COM%d", i)] = true
		reserved[fmt.Sprintf("LPT%d", i)] = true
	}
	if reserved[base] {
		return errors.New("folder name is reserved by Windows")
	}
	return nil
}

func directProjectFolder(root, path string) (string, error) {
	root = filepath.Clean(root)
	full, err := ensureWithinRoot(root, path)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(full, root) || !strings.EqualFold(filepath.Dir(full), root) {
		return "", errors.New("folder operation is limited to direct project folders")
	}
	return full, nil
}

func inspectProjectDelete(root, path string) (ProjectDeletePreview, error) {
	full, err := directProjectFolder(root, path)
	if err != nil {
		return ProjectDeletePreview{}, err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return ProjectDeletePreview{}, errors.New("project directory not found")
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return ProjectDeletePreview{}, err
	}
	preview := ProjectDeletePreview{
		Path:                 full,
		Name:                 filepath.Base(full),
		Empty:                len(entries) == 0,
		ConfirmationRequired: len(entries) != 0,
		Confirmation:         filepath.Base(full),
	}
	if preview.Empty {
		return preview, nil
	}
	err = filepath.WalkDir(full, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.EqualFold(filepath.Clean(current), filepath.Clean(full)) {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			preview.Symlinks++
			return nil
		}
		if entry.IsDir() {
			preview.Directories++
			return nil
		}
		preview.Files++
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if entryInfo.Mode().IsRegular() {
			preview.Bytes += entryInfo.Size()
		}
		return nil
	})
	if err != nil {
		return ProjectDeletePreview{}, err
	}
	return preview, nil
}

func cloneThreadSnapshotLocked(threads map[string]*ChatThread) map[string]*ChatThread {
	snapshot := make(map[string]*ChatThread, len(threads))
	for id, thread := range threads {
		if thread == nil {
			continue
		}
		copy := *thread
		copy.Events = append([]UIEvent(nil), thread.Events...)
		snapshot[id] = &copy
	}
	return snapshot
}

func (s *AppState) updateProjectRuntimePath(oldPath, newPath string, archive bool) {
	s.mu.Lock()
	if strings.EqualFold(filepath.Clean(s.Project), filepath.Clean(oldPath)) {
		if archive {
			s.Project = ""
			s.CurrentThread = ""
			s.Events = nil
			s.Pending = nil
			s.Continuation = nil
		} else {
			s.Project = newPath
			if s.Continuation != nil && strings.EqualFold(filepath.Clean(s.Continuation.Project), filepath.Clean(oldPath)) {
				s.Continuation.Project = newPath
			}
		}
	}
	for _, thread := range s.Threads {
		if thread == nil || !strings.EqualFold(filepath.Clean(thread.Project), filepath.Clean(oldPath)) {
			continue
		}
		if archive {
			thread.Archived = true
		} else {
			thread.Project = newPath
		}
	}
	snapshot := cloneThreadSnapshotLocked(s.Threads)
	s.mu.Unlock()
	s.queueThreadSave(snapshot)
}

func (s *AppState) ProjectAction(path, action, value string) (ProjectSummary, error) {
	path = strings.TrimSpace(path)
	action = strings.ToLower(strings.TrimSpace(action))
	value = strings.TrimSpace(value)
	if path == "" {
		return ProjectSummary{}, errors.New("project path is required")
	}

	s.mu.RLock()
	running := s.Running
	root := filepath.Clean(s.Config.RootProjectDir)
	s.mu.RUnlock()
	if running {
		return ProjectSummary{}, errors.New("Agent läuft gerade")
	}

	if action == "create_folder" || action == "create_project" {
		if !strings.EqualFold(filepath.Clean(path), root) {
			return ProjectSummary{}, errors.New("new project folders can only be created in the project root")
		}
		if err := validProjectFolderName(value); err != nil {
			return ProjectSummary{}, err
		}
		created := filepath.Join(root, value)
		if _, err := os.Stat(created); err == nil {
			return ProjectSummary{}, errors.New("a folder with this name already exists")
		} else if !os.IsNotExist(err) {
			return ProjectSummary{}, err
		}
		if err := os.Mkdir(created, 0o755); err != nil {
			return ProjectSummary{}, err
		}
		s.mu.RLock()
		cfg := s.Config
		s.mu.RUnlock()
		if action == "create_project" && cfg.CreateProjectDocs {
			if err := ensureProjectDocs(created, cfg); err != nil {
				_ = os.RemoveAll(created)
				return ProjectSummary{}, err
			}
			note := localizeConfigText(cfg, "Projekt sicher angelegt; README.md, AGENTS.md und STATE.md sind für die Übergabe vorbereitet.", "Project created safely; README.md, AGENTS.md and STATE.md are prepared for handoff.")
			if err := updateStateDocument(created, cfg, false, "", "", "", nil, note); err != nil {
				_ = os.RemoveAll(created)
				return ProjectSummary{}, err
			}
		}
		return ProjectSummary{Path: created, Name: projectDisplayName(cfg, created), Pinned: false}, nil
	}

	full, err := directProjectFolder(root, path)
	if err != nil {
		return ProjectSummary{}, err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return ProjectSummary{}, errors.New("project directory not found")
	}
	if action == "rename" && len([]rune(value)) > 120 {
		return ProjectSummary{}, errors.New("project name is too long")
	}

	switch action {
	case "rename_folder":
		if err := validProjectFolderName(value); err != nil {
			return ProjectSummary{}, err
		}
		newPath := filepath.Join(root, value)
		if strings.EqualFold(newPath, full) {
			return ProjectSummary{Path: full, Name: filepath.Base(full), Pinned: false}, nil
		}
		if _, err := os.Stat(newPath); err == nil {
			return ProjectSummary{}, errors.New("a folder with this name already exists")
		} else if !os.IsNotExist(err) {
			return ProjectSummary{}, err
		}
		if err := os.Rename(full, newPath); err != nil {
			return ProjectSummary{}, err
		}
		cfg, cfgErr := s.mutateConfig(func(cfg *Config) error {
			if alias, ok := cfg.ProjectAliases[full]; ok {
				delete(cfg.ProjectAliases, full)
				cfg.ProjectAliases[newPath] = alias
			}
			cfg.PinnedProjects = projectListReplace(cfg.PinnedProjects, full, newPath)
			cfg.HiddenProjects = projectListReplace(cfg.HiddenProjects, full, newPath)
			if strings.EqualFold(filepath.Clean(cfg.LastProject), filepath.Clean(full)) {
				cfg.LastProject = newPath
			}
			return nil
		})
		if cfgErr != nil {
			_ = os.Rename(newPath, full)
			return ProjectSummary{}, cfgErr
		}
		s.updateProjectRuntimePath(full, newPath, false)
		return ProjectSummary{Path: newPath, Name: projectDisplayName(cfg, newPath), Pinned: projectListContains(cfg.PinnedProjects, newPath)}, nil

	case "delete_empty":
		preview, err := inspectProjectDelete(root, full)
		if err != nil {
			return ProjectSummary{}, err
		}
		if !preview.Empty {
			return ProjectSummary{}, errors.New("folder is not empty; use recursive delete with confirmation")
		}
		if err := os.Remove(full); err != nil {
			return ProjectSummary{}, err
		}
		_, cfgErr := s.mutateConfig(func(cfg *Config) error {
			delete(cfg.ProjectAliases, full)
			cfg.PinnedProjects = projectListWithout(cfg.PinnedProjects, full)
			cfg.HiddenProjects = projectListWithout(cfg.HiddenProjects, full)
			if strings.EqualFold(filepath.Clean(cfg.LastProject), filepath.Clean(full)) {
				cfg.LastProject = ""
			}
			return nil
		})
		if cfgErr != nil {
			return ProjectSummary{}, cfgErr
		}
		s.updateProjectRuntimePath(full, "", true)
		return ProjectSummary{}, nil

	case "delete_recursive":
		preview, err := inspectProjectDelete(root, full)
		if err != nil {
			return ProjectSummary{}, err
		}
		if preview.Empty {
			return ProjectSummary{}, errors.New("folder is empty; use delete_empty")
		}
		if value == "" || !strings.EqualFold(value, preview.Confirmation) {
			return ProjectSummary{}, errors.New("recursive delete confirmation must match the folder name")
		}
		if err := os.RemoveAll(full); err != nil {
			return ProjectSummary{}, err
		}
		_, cfgErr := s.mutateConfig(func(cfg *Config) error {
			delete(cfg.ProjectAliases, full)
			cfg.PinnedProjects = projectListWithout(cfg.PinnedProjects, full)
			cfg.HiddenProjects = projectListWithout(cfg.HiddenProjects, full)
			if strings.EqualFold(filepath.Clean(cfg.LastProject), filepath.Clean(full)) {
				cfg.LastProject = ""
			}
			return nil
		})
		if cfgErr != nil {
			return ProjectSummary{}, cfgErr
		}
		s.updateProjectRuntimePath(full, "", true)
		return ProjectSummary{}, nil
	}

	removeActive := false
	cfg, err := s.mutateConfig(func(cfg *Config) error {
		switch action {
		case "rename":
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
			removeActive = strings.EqualFold(filepath.Clean(s.Project), filepath.Clean(full))
			if removeActive {
				cfg.LastProject = ""
			}
		case "restore":
			cfg.HiddenProjects = projectListWithout(cfg.HiddenProjects, full)
		default:
			return errors.New("unsupported project action")
		}
		return nil
	})
	if err != nil {
		return ProjectSummary{}, err
	}

	if removeActive {
		s.mu.Lock()
		if strings.EqualFold(filepath.Clean(s.Project), filepath.Clean(full)) {
			s.Project = ""
			s.CurrentThread = ""
			s.Events = nil
			s.Pending = nil
			s.Continuation = nil
		}
		s.mu.Unlock()
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
