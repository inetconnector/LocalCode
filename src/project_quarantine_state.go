// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"path/filepath"
	"strings"
)

func (s *AppState) ListProjectQuarantine() ([]QuarantinedProject, error) {
	s.mu.RLock()
	root := s.Config.RootProjectDir
	s.mu.RUnlock()
	return listQuarantinedProjects(root)
}

func (s *AppState) ProjectQuarantineAction(action, id, confirmation string) (QuarantinedProject, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	id = strings.TrimSpace(id)
	if !validQuarantineID(id) {
		return QuarantinedProject{}, errors.New("invalid quarantine id")
	}

	s.mu.RLock()
	root := s.Config.RootProjectDir
	running := s.Running
	s.mu.RUnlock()
	if running {
		return QuarantinedProject{}, errors.New("agent is running")
	}

	switch action {
	case "restore":
		entry, err := restoreQuarantinedProject(root, id)
		if err != nil {
			return QuarantinedProject{}, err
		}
		s.restoreProjectThreadReferences(entry.OriginalPath)
		return entry, nil
	case "purge":
		return purgeQuarantinedProject(root, id, confirmation)
	default:
		return QuarantinedProject{}, errors.New("quarantine action is not allowed")
	}
}

func (s *AppState) restoreProjectThreadReferences(project string) {
	project = filepath.Clean(project)
	changed := false
	var snapshot map[string]*ChatThread

	s.mu.Lock()
	for _, thread := range s.Threads {
		if thread == nil || !strings.EqualFold(filepath.Clean(thread.Project), project) {
			continue
		}
		if thread.Archived {
			thread.Archived = false
			changed = true
		}
	}
	if changed {
		snapshot = cloneThreadSnapshotLocked(s.Threads)
	}
	s.mu.Unlock()

	if changed {
		s.queueThreadSave(snapshot)
	}
}
