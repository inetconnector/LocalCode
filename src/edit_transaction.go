// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// fileVersion is an optimistic-concurrency token for a project file. It is
// intentionally content-based instead of timestamp-based so editors which keep
// mtimes or filesystems with coarse timestamp resolution cannot hide a change.
type fileVersion struct {
	Exists bool
	SHA256 string
}

// EditConflictError means the file changed between the version LocalCode read
// and the point at which it was ready to commit the replacement. Callers must
// re-read/rebase instead of silently overwriting the newer file.
type EditConflictError struct {
	Path     string
	Expected fileVersion
	Actual   fileVersion
}

func (e *EditConflictError) Error() string {
	return fmt.Sprintf("EDIT_CONFLICT: %s changed while the edit was being prepared; re-read the file and rebase the patch before retrying", e.Path)
}

func isEditConflict(err error) bool {
	var conflict *EditConflictError
	return errors.As(err, &conflict)
}

func versionForBytes(data []byte) fileVersion {
	sum := sha256.Sum256(data)
	return fileVersion{Exists: true, SHA256: hex.EncodeToString(sum[:])}
}

func readFileVersion(fullPath string) (fileVersion, error) {
	data, err := os.ReadFile(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return fileVersion{}, nil
	}
	if err != nil {
		return fileVersion{}, err
	}
	return versionForBytes(data), nil
}

func verifyFileVersion(fullPath string, expected fileVersion) error {
	actual, err := readFileVersion(fullPath)
	if err != nil {
		return err
	}
	if actual != expected {
		return &EditConflictError{Path: filepath.ToSlash(fullPath), Expected: expected, Actual: actual}
	}
	return nil
}

type editPathMutex struct {
	mu   sync.Mutex
	refs int
}

var editPathLocks = struct {
	sync.Mutex
	items map[string]*editPathMutex
}{items: make(map[string]*editPathMutex)}

func editLockKey(fullPath string) string {
	key := filepath.Clean(fullPath)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

// acquireEditPathLock serializes LocalCode mutations to one path while still
// allowing unrelated files to be edited in parallel. Reference counting keeps
// the registry bounded over long sessions.
func acquireEditPathLock(fullPath string) func() {
	key := editLockKey(fullPath)
	editPathLocks.Lock()
	item := editPathLocks.items[key]
	if item == nil {
		item = &editPathMutex{}
		editPathLocks.items[key] = item
	}
	item.refs++
	editPathLocks.Unlock()

	item.mu.Lock()
	return func() {
		item.mu.Unlock()
		editPathLocks.Lock()
		item.refs--
		if item.refs == 0 {
			delete(editPathLocks.items, key)
		}
		editPathLocks.Unlock()
	}
}

// atomicWriteFileIfVersion stages bytes in the destination directory, fsyncs
// the staged file, revalidates the optimistic-concurrency token, and only then
// atomically swaps it into place. A same-directory temporary file keeps the
// operation on one filesystem/volume.
func atomicWriteFileIfVersion(fullPath string, data []byte, mode os.FileMode, expected fileVersion) error {
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".localcode-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(mode.Perm()); err != nil {
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

	// Re-check after the potentially slow staging/sync step. If an editor,
	// formatter, watcher or another LocalCode action changed the destination,
	// preserve that newer content and force a rebase instead of overwriting it.
	if err := verifyFileVersion(fullPath, expected); err != nil {
		return err
	}
	if err := replaceFileAtomic(tmpName, fullPath, expected.Exists); err != nil {
		return err
	}
	keepTemp = false
	return nil
}
