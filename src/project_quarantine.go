// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const projectQuarantineDirName = ".localcode-quarantine"

type QuarantinedProject struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	OriginalPath  string    `json:"original_path"`
	QuarantinedAt time.Time `json:"quarantined_at"`
	Files         int       `json:"files"`
	Directories   int       `json:"directories"`
	Symlinks      int       `json:"symlinks"`
	Bytes         int64     `json:"bytes"`
}

func projectQuarantineRoot(root string) string {
	return filepath.Join(filepath.Clean(root), projectQuarantineDirName)
}

func ensureProjectQuarantineRoot(root string) (string, error) {
	root = filepath.Clean(root)
	quarantine := projectQuarantineRoot(root)
	info, err := os.Lstat(quarantine)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(quarantine, 0o700); err != nil {
			return "", err
		}
		info, err = os.Lstat(quarantine)
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("project quarantine path must be a real directory, not a symlink")
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedQuarantine, err := filepath.EvalSymlinks(quarantine)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedQuarantine)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("project quarantine escaped the configured project root")
	}
	return quarantine, nil
}

func validQuarantineID(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && id == filepath.Base(id) && !strings.ContainsAny(id, `/\\:`) && !strings.Contains(id, "..")
}

func newQuarantineID() string {
	return fmt.Sprintf("%d-%s", time.Now().UTC().UnixNano(), newID())
}

func quarantineMetadataPath(entryDir string) string {
	return filepath.Join(entryDir, "metadata.json")
}

func quarantinePayloadPath(entryDir string) string {
	return filepath.Join(entryDir, "project")
}

func writeQuarantineMetadata(path string, entry QuarantinedProject) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFileIfVersion(path, data, 0o600, fileVersion{})
}

func quarantineProject(root, projectPath string) (QuarantinedProject, error) {
	full, err := directProjectFolder(root, projectPath)
	if err != nil {
		return QuarantinedProject{}, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return QuarantinedProject{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return QuarantinedProject{}, errors.New("only a real project directory can be quarantined")
	}
	preview, err := inspectProjectDelete(root, full)
	if err != nil {
		return QuarantinedProject{}, err
	}
	if preview.Empty {
		return QuarantinedProject{}, errors.New("empty project folders should be removed with delete_empty")
	}
	quarantineRoot, err := ensureProjectQuarantineRoot(root)
	if err != nil {
		return QuarantinedProject{}, err
	}

	var entryDir, id string
	for attempt := 0; attempt < 8; attempt++ {
		id = newQuarantineID()
		entryDir = filepath.Join(quarantineRoot, id)
		if err := os.Mkdir(entryDir, 0o700); err == nil {
			break
		} else if !errors.Is(err, os.ErrExist) {
			return QuarantinedProject{}, err
		}
		entryDir = ""
	}
	if entryDir == "" {
		return QuarantinedProject{}, errors.New("could not allocate a unique quarantine entry")
	}

	payload := quarantinePayloadPath(entryDir)
	if err := os.Rename(full, payload); err != nil {
		_ = os.Remove(entryDir)
		return QuarantinedProject{}, fmt.Errorf("move project into same-volume quarantine: %w", err)
	}
	entry := QuarantinedProject{
		ID:            id,
		Name:          preview.Name,
		OriginalPath:  full,
		QuarantinedAt: time.Now().UTC(),
		Files:         preview.Files,
		Directories:   preview.Directories,
		Symlinks:      preview.Symlinks,
		Bytes:         preview.Bytes,
	}
	if err := writeQuarantineMetadata(quarantineMetadataPath(entryDir), entry); err != nil {
		if rollbackErr := os.Rename(payload, full); rollbackErr != nil {
			return QuarantinedProject{}, fmt.Errorf("write quarantine metadata: %v; rollback failed: %w", err, rollbackErr)
		}
		_ = os.RemoveAll(entryDir)
		return QuarantinedProject{}, err
	}
	return entry, nil
}

func loadQuarantinedProject(root, id string) (QuarantinedProject, string, error) {
	if !validQuarantineID(id) {
		return QuarantinedProject{}, "", errors.New("invalid quarantine id")
	}
	quarantineRoot, err := ensureProjectQuarantineRoot(root)
	if err != nil {
		return QuarantinedProject{}, "", err
	}
	entryDir := filepath.Join(quarantineRoot, id)
	info, err := os.Lstat(entryDir)
	if err != nil {
		return QuarantinedProject{}, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return QuarantinedProject{}, "", errors.New("invalid quarantine entry directory")
	}
	data, err := os.ReadFile(quarantineMetadataPath(entryDir))
	if err != nil {
		return QuarantinedProject{}, "", err
	}
	var entry QuarantinedProject
	if err := json.Unmarshal(data, &entry); err != nil {
		return QuarantinedProject{}, "", err
	}
	if entry.ID != id || entry.Name == "" {
		return QuarantinedProject{}, "", errors.New("quarantine metadata identity mismatch")
	}
	original, err := directProjectFolder(root, entry.OriginalPath)
	if err != nil || !strings.EqualFold(original, filepath.Clean(entry.OriginalPath)) || filepath.Base(original) != entry.Name {
		return QuarantinedProject{}, "", errors.New("quarantine metadata contains an invalid original project path")
	}
	payload := quarantinePayloadPath(entryDir)
	payloadInfo, err := os.Lstat(payload)
	if err != nil {
		return QuarantinedProject{}, "", err
	}
	if payloadInfo.Mode()&os.ModeSymlink != 0 || !payloadInfo.IsDir() {
		return QuarantinedProject{}, "", errors.New("quarantine payload is not a real directory")
	}
	return entry, entryDir, nil
}

func listQuarantinedProjects(root string) ([]QuarantinedProject, error) {
	quarantineRoot := projectQuarantineRoot(root)
	if _, err := os.Lstat(quarantineRoot); errors.Is(err, os.ErrNotExist) {
		return []QuarantinedProject{}, nil
	}
	if _, err := ensureProjectQuarantineRoot(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil {
		return nil, err
	}
	out := make([]QuarantinedProject, 0, len(entries))
	for _, dirEntry := range entries {
		if !dirEntry.IsDir() || !validQuarantineID(dirEntry.Name()) {
			continue
		}
		entry, _, err := loadQuarantinedProject(root, dirEntry.Name())
		if err != nil {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QuarantinedAt.After(out[j].QuarantinedAt) })
	return out, nil
}

func restoreQuarantinedProject(root, id string) (QuarantinedProject, error) {
	entry, entryDir, err := loadQuarantinedProject(root, id)
	if err != nil {
		return QuarantinedProject{}, err
	}
	if _, err := os.Lstat(entry.OriginalPath); err == nil {
		return QuarantinedProject{}, errors.New("cannot restore project because the original destination is occupied")
	} else if !errors.Is(err, os.ErrNotExist) {
		return QuarantinedProject{}, err
	}
	payload := quarantinePayloadPath(entryDir)
	if err := os.Rename(payload, entry.OriginalPath); err != nil {
		return QuarantinedProject{}, fmt.Errorf("restore quarantined project: %w", err)
	}
	if err := os.RemoveAll(entryDir); err != nil {
		return entry, fmt.Errorf("project restored but quarantine metadata cleanup failed: %w", err)
	}
	return entry, nil
}

func purgeQuarantinedProject(root, id, confirmation string) (QuarantinedProject, error) {
	entry, entryDir, err := loadQuarantinedProject(root, id)
	if err != nil {
		return QuarantinedProject{}, err
	}
	expected := "PURGE " + entry.Name
	if confirmation != expected {
		return QuarantinedProject{}, fmt.Errorf("permanent purge confirmation must exactly match %q", expected)
	}
	if err := os.RemoveAll(entryDir); err != nil {
		return QuarantinedProject{}, err
	}
	return entry, nil
}
