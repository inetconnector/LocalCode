// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// approvedFilePrecondition binds an approval preview to the exact file bytes
// that were present before the preview was rendered. It survives the user wait
// and is carried into the final atomic mutation as an optimistic-concurrency
// precondition.
type approvedFilePrecondition struct {
	FullPath string
	Version  fileVersion
}

func captureApprovedFilePrecondition(project string, action AgentAction) (*approvedFilePrecondition, error) {
	switch action.Action {
	case "replace_text", "write_file", "delete_file":
		full, err := ensureWithinRoot(project, action.Path)
		if err != nil {
			return nil, err
		}
		version, err := readFileVersion(full)
		if err != nil {
			return nil, err
		}
		return &approvedFilePrecondition{FullPath: full, Version: version}, nil
	default:
		return nil, nil
	}
}

func verifyApprovedFilePrecondition(precondition approvedFilePrecondition) error {
	return verifyFileVersion(precondition.FullPath, precondition.Version)
}

func replaceTextAtVersion(projectRoot, path, oldText, newText string, expected fileVersion) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	release := acquireEditPathLock(full)
	defer release()

	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	actual := versionForBytes(data)
	if actual != expected || !expected.Exists {
		return "", &EditConflictError{Path: filepath.ToSlash(full), Expected: expected, Actual: actual}
	}
	original := string(data)
	count := strings.Count(original, oldText)
	if count != 1 {
		return "", fmt.Errorf("old_text must occur exactly once; found %d occurrences", count)
	}
	updated := strings.Replace(original, oldText, newText, 1)
	if updated == original {
		return "", noObservableProjectChanges("replacement leaves the file unchanged")
	}
	if err := backupFile(projectRoot, full); err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(full); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := atomicWriteFileIfVersion(full, []byte(updated), mode, expected); err != nil {
		return "", err
	}
	return simpleDiff(original, updated), nil
}

func writeProjectFileAtVersion(projectRoot, path, content string, expected fileVersion) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	release := acquireEditPathLock(full)
	defer release()

	actual, err := readFileVersion(full)
	if err != nil {
		return "", err
	}
	if actual != expected {
		return "", &EditConflictError{Path: filepath.ToSlash(full), Expected: expected, Actual: actual}
	}

	old := ""
	mode := os.FileMode(0o644)
	if expected.Exists {
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			return "", readErr
		}
		actual = versionForBytes(data)
		if actual != expected {
			return "", &EditConflictError{Path: filepath.ToSlash(full), Expected: expected, Actual: actual}
		}
		if !isProbablyText(data) {
			return "", fmt.Errorf("refusing to overwrite binary or non-UTF-8 file: %s", path)
		}
		old = string(data)
		if old == content {
			return "", noObservableProjectChanges("file already has the requested content")
		}
		if info, statErr := os.Stat(full); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := backupFile(projectRoot, full); err != nil {
			return "", err
		}
	}
	if err := atomicWriteFileIfVersion(full, []byte(content), mode, expected); err != nil {
		return "", err
	}
	return simpleDiff(old, content) + "\n\nPOSTCONDITION:\n" + describePathState("target", full), nil
}

func deleteProjectFileAtVersion(projectRoot, path string, expected fileVersion) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	release := acquireEditPathLock(full)
	defer release()

	if !expected.Exists {
		actual, versionErr := readFileVersion(full)
		if versionErr != nil {
			return "", versionErr
		}
		return "", &EditConflictError{Path: filepath.ToSlash(full), Expected: expected, Actual: actual}
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("directory deletion is not allowed")
	}
	old, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	actual := versionForBytes(old)
	if actual != expected {
		return "", &EditConflictError{Path: filepath.ToSlash(full), Expected: expected, Actual: actual}
	}
	if err := backupFile(projectRoot, full); err != nil {
		return "", err
	}
	// Revalidate immediately before deletion. Portable file APIs do not expose a
	// content-hash compare-and-delete primitive, so this minimizes the external
	// race while still rejecting stale approvals in normal editor workflows.
	if err := verifyFileVersion(full, expected); err != nil {
		return "", err
	}
	if err := os.Remove(full); err != nil {
		return "", err
	}
	return simpleDiff(string(old), "") + "\n\nPOSTCONDITION:\n" + describePathState("target", full), nil
}
