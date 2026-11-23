// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	turnCheckpointMaxFileBytes int64 = 16 << 20 // 16 MiB per file
	turnCheckpointMaxFiles           = 8000
)

var turnCheckpointMu sync.Mutex

var turnCheckpointIgnoredDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".mypy_cache":   true,
	".pytest_cache": true,
	"target":        true,
	".gradle":       true,
	".idea":         true,
	".vs":           true,
	".next":         true,
	".nuxt":         true,
	".cache":        true,
	".tools":        true,
}

type CheckpointFileEntry struct {
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Mode        uint32 `json:"mode"`
	ModTimeUnix int64  `json:"mod_time_unix"`
}

type CheckpointSkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Size   int64  `json:"size,omitempty"`
}

type TurnCheckpoint struct {
	ID                string                         `json:"id"`
	ThreadID          string                         `json:"thread_id"`
	Project           string                         `json:"project"`
	CreatedAt         time.Time                      `json:"created_at"`
	PromptPreview     string                         `json:"prompt_preview"`
	EventIndex        int                            `json:"event_index"`
	ThreadTitleBefore string                         `json:"thread_title_before"`
	Files             map[string]CheckpointFileEntry `json:"files"`
	Dirs              []string                       `json:"dirs"`
	SkippedFiles      []CheckpointSkippedFile        `json:"skipped_files,omitempty"`
	Truncated         bool                           `json:"truncated,omitempty"`
	GitHead           string                         `json:"git_head,omitempty"`
}

type RollbackPreview struct {
	CheckpointID   string   `json:"checkpoint_id"`
	ThreadID       string   `json:"thread_id"`
	Project        string   `json:"project"`
	Restorable     bool     `json:"restorable"`
	Reason         string   `json:"reason,omitempty"`
	PromptPreview  string   `json:"prompt_preview,omitempty"`
	TurnsRemoved   int      `json:"turns_removed"`
	EventsRemoved  int      `json:"events_removed"`
	FilesToRestore []string `json:"files_to_restore"`
	FilesToRevert  []string `json:"files_to_revert"`
	FilesToDelete  []string `json:"files_to_delete"`
	DirsToCreate   []string `json:"dirs_to_create"`
	DirsToRemove   []string `json:"dirs_to_remove"`
	Warnings       []string `json:"warnings,omitempty"`
}

type RollbackResult struct {
	OK            bool     `json:"ok"`
	CheckpointID  string   `json:"checkpoint_id"`
	ThreadID      string   `json:"thread_id"`
	TurnsRemoved  int      `json:"turns_removed"`
	EventsRemoved int      `json:"events_removed"`
	RestoredFiles int      `json:"restored_files"`
	RevertedFiles int      `json:"reverted_files"`
	DeletedFiles  int      `json:"deleted_files"`
	Warnings      []string `json:"warnings,omitempty"`
}

type ProjectSubdirItem struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	HasSubdirs bool   `json:"has_subdirs"`
}

func checkpointsRootDir() string {
	return filepath.Join(appDataDir(), "checkpoints")
}

func checkpointBlobsDir() string {
	return filepath.Join(checkpointsRootDir(), "blobs")
}

func checkpointManifestsDir() string {
	return filepath.Join(checkpointsRootDir(), "manifests")
}

func checkpointManifestPath(id string) string {
	return filepath.Join(checkpointManifestsDir(), id+".json")
}

func checkpointBlobPath(hash string) string {
	if len(hash) < 4 {
		return filepath.Join(checkpointBlobsDir(), hash)
	}
	return filepath.Join(checkpointBlobsDir(), hash[:2], hash)
}

func validCheckpointID(id string) bool {
	if len(id) < 4 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func isTurnCheckpointIgnoredDir(name string) bool {
	return turnCheckpointIgnoredDirs[strings.ToLower(strings.TrimSpace(name))]
}

func readGitHeadFast(projectDir string) string {
	headBytes, err := os.ReadFile(filepath.Join(projectDir, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(headBytes))
	if strings.HasPrefix(head, "ref: ") {
		refRel := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
		refPath := filepath.Join(projectDir, ".git", filepath.FromSlash(refRel))
		if refBytes, err := os.ReadFile(refPath); err == nil {
			return strings.TrimSpace(string(refBytes))
		}
		return refRel
	}
	return head
}

func hashAndStoreBlob(srcPath string) (string, int64, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, turnCheckpointMaxFileBytes+1))
	if err != nil {
		return "", 0, err
	}
	if n > turnCheckpointMaxFileBytes {
		return "", n, errors.New("size_limit")
	}
	sum := hex.EncodeToString(h.Sum(nil))
	dst := checkpointBlobPath(sum)
	if st, statErr := os.Stat(dst); statErr == nil && !st.IsDir() && st.Size() == n {
		return sum, n, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", 0, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".blob-*")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, f); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		// Another concurrent writer may have raced to write the identical immutable blob.
		if st, statErr := os.Stat(dst); statErr == nil && st.Size() == n {
			_ = os.Remove(tmpName)
			return sum, n, nil
		}
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	return sum, n, nil
}

func hashFileOnly(srcPath string) (string, int64, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, turnCheckpointMaxFileBytes+1))
	if err != nil {
		return "", 0, err
	}
	if n > turnCheckpointMaxFileBytes {
		return "", n, errors.New("size_limit")
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func verifyBlobSHA256(hash string) error {
	p := checkpointBlobPath(hash)
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, hash) {
		return fmt.Errorf("blob checksum mismatch for %s", hash)
	}
	return nil
}

func copyBlobToWorkspaceFile(hash, dstPath string, mode uint32) error {
	src := checkpointBlobPath(hash)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dstPath), ".lc-undo-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	perm := fs.FileMode(mode) & fs.ModePerm
	if perm == 0 {
		perm = 0o644
	}
	_ = os.Chmod(tmpName, perm)
	if err := os.Rename(tmpName, dstPath); err != nil {
		// On Windows renaming over an existing file can occasionally fail if read-only; retry safely.
		_ = os.Chmod(dstPath, 0o666)
		_ = os.Remove(dstPath)
		if err2 := os.Rename(tmpName, dstPath); err2 != nil {
			_ = os.Remove(tmpName)
			return err2
		}
	}
	return nil
}

func scanWorkspaceState(projectDir string, storeBlobs bool) (map[string]CheckpointFileEntry, []string, []CheckpointSkippedFile, bool, error) {
	cleanRoot := filepath.Clean(projectDir)
	info, err := os.Stat(cleanRoot)
	if err != nil || !info.IsDir() {
		return nil, nil, nil, false, fmt.Errorf("project directory not accessible: %s", projectDir)
	}
	files := make(map[string]CheckpointFileEntry)
	dirs := make([]string, 0, 32)
	skipped := make([]CheckpointSkippedFile, 0)
	truncated := false

	walkErr := filepath.WalkDir(cleanRoot, func(curr string, d fs.DirEntry, err error) error {
		if err != nil {
			if curr == cleanRoot {
				return err
			}
			rel, _ := filepath.Rel(cleanRoot, curr)
			skipped = append(skipped, CheckpointSkippedFile{Path: filepath.ToSlash(rel), Reason: "unreadable"})
			return nil
		}
		if curr == cleanRoot {
			return nil
		}
		name := d.Name()
		rel, relErr := filepath.Rel(cleanRoot, curr)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		if d.Type()&os.ModeSymlink != 0 {
			skipped = append(skipped, CheckpointSkippedFile{Path: relSlash, Reason: "symlink"})
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if isTurnCheckpointIgnoredDir(name) {
				return filepath.SkipDir
			}
			dirs = append(dirs, relSlash)
			return nil
		}
		if len(files) >= turnCheckpointMaxFiles {
			truncated = true
			return nil
		}
		fi, infoErr := d.Info()
		if infoErr != nil {
			skipped = append(skipped, CheckpointSkippedFile{Path: relSlash, Reason: "unreadable"})
			return nil
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		if fi.Size() > turnCheckpointMaxFileBytes {
			skipped = append(skipped, CheckpointSkippedFile{Path: relSlash, Reason: "size_limit", Size: fi.Size()})
			return nil
		}
		var hash string
		var sz int64
		var hErr error
		if storeBlobs {
			hash, sz, hErr = hashAndStoreBlob(curr)
		} else {
			hash, sz, hErr = hashFileOnly(curr)
		}
		if hErr != nil {
			skipped = append(skipped, CheckpointSkippedFile{Path: relSlash, Reason: hErr.Error(), Size: fi.Size()})
			return nil
		}
		files[relSlash] = CheckpointFileEntry{
			SHA256:      hash,
			Size:        sz,
			Mode:        uint32(fi.Mode().Perm()),
			ModTimeUnix: fi.ModTime().Unix(),
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, nil, false, walkErr
	}
	sort.Strings(dirs)
	return files, dirs, skipped, truncated, nil
}

func saveTurnCheckpointManifest(cp *TurnCheckpoint) error {
	dir := checkpointManifestsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	dst := checkpointManifestPath(cp.ID)
	tmp, err := os.CreateTemp(dir, ".cp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(dst)
		if err2 := os.Rename(tmpName, dst); err2 != nil {
			_ = os.Remove(tmpName)
			return err2
		}
	}
	return nil
}

func loadTurnCheckpointManifest(id string) (*TurnCheckpoint, error) {
	if !validCheckpointID(id) {
		return nil, errors.New("invalid checkpoint id")
	}
	var cp TurnCheckpoint
	data, err := os.ReadFile(checkpointManifestPath(id))
	if err != nil {
		return nil, err
	}
	if err := readJSON(strings.NewReader(string(data)), &cp); err != nil {
		return nil, err
	}
	if cp.Files == nil {
		cp.Files = map[string]CheckpointFileEntry{}
	}
	return &cp, nil
}

func gcUnreferencedCheckpointBlobs() {
	entries, err := os.ReadDir(checkpointManifestsDir())
	if err != nil {
		return
	}
	used := make(map[string]struct{}, 256)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if cp, err := loadTurnCheckpointManifest(id); err == nil {
			for _, f := range cp.Files {
				if f.SHA256 != "" {
					used[f.SHA256] = struct{}{}
				}
			}
		}
	}
	_ = filepath.WalkDir(checkpointBlobsDir(), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if _, ok := used[name]; !ok {
			_ = os.Remove(path)
		}
		return nil
	})
}

func (s *AppState) CreateTurnCheckpoint(threadID, project, prompt string) (*TurnCheckpoint, error) {
	turnCheckpointMu.Lock()
	defer turnCheckpointMu.Unlock()

	s.mu.RLock()
	t := s.Threads[threadID]
	eventIdx := 0
	titleBefore := "Neuer Chat"
	if t != nil {
		eventIdx = len(t.Events)
		if t.Title != "" {
			titleBefore = t.Title
		}
	}
	s.mu.RUnlock()

	files, dirs, skipped, truncated, err := scanWorkspaceState(project, true)
	if err != nil {
		return nil, err
	}
	preview := strings.TrimSpace(prompt)
	if runes := []rune(preview); len(runes) > 120 {
		preview = strings.TrimSpace(string(runes[:120])) + "…"
	}
	cp := &TurnCheckpoint{
		ID:                "cp-" + newID(),
		ThreadID:          threadID,
		Project:           filepath.Clean(project),
		CreatedAt:         time.Now(),
		PromptPreview:     preview,
		EventIndex:        eventIdx,
		ThreadTitleBefore: titleBefore,
		Files:             files,
		Dirs:              dirs,
		SkippedFiles:      skipped,
		Truncated:         truncated,
		GitHead:           readGitHeadFast(project),
	}
	if err := saveTurnCheckpointManifest(cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func findCheckpointEventIndex(events []UIEvent, checkpointID string, fallbackIdx int) int {
	for i, ev := range events {
		if ev.Type == "user" && ev.CheckpointID == checkpointID {
			return i
		}
	}
	if fallbackIdx >= 0 && fallbackIdx <= len(events) {
		return fallbackIdx
	}
	return -1
}

func countUserTurnsFrom(events []UIEvent, startIdx int) int {
	if startIdx < 0 || startIdx >= len(events) {
		return 0
	}
	n := 0
	for i := startIdx; i < len(events); i++ {
		if events[i].Type == "user" {
			n++
		}
	}
	return n
}

func computeWorkspaceRollbackPlan(cp *TurnCheckpoint) (
	toRestore []string,
	toRevert []string,
	toDelete []string,
	dirsToCreate []string,
	dirsToRemove []string,
	warnings []string,
	restorable bool,
	reason string,
) {
	liveFiles, liveDirs, liveSkipped, liveTruncated, err := scanWorkspaceState(cp.Project, false)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, false, err.Error()
	}

	cpDirSet := make(map[string]bool, len(cp.Dirs))
	for _, d := range cp.Dirs {
		cpDirSet[d] = true
	}
	liveDirSet := make(map[string]bool, len(liveDirs))
	for _, d := range liveDirs {
		liveDirSet[d] = true
	}
	skippedSet := make(map[string]bool, len(cp.SkippedFiles)+len(liveSkipped))
	for _, sf := range cp.SkippedFiles {
		skippedSet[sf.Path] = true
	}
	for _, sf := range liveSkipped {
		skippedSet[sf.Path] = true
	}

	for rel, entry := range cp.Files {
		live, exists := liveFiles[rel]
		if !exists {
			if err := verifyBlobSHA256(entry.SHA256); err != nil {
				return nil, nil, nil, nil, nil, nil, false, fmt.Sprintf("missing or corrupt backup blob for %s", rel)
			}
			toRestore = append(toRestore, rel)
		} else if !strings.EqualFold(live.SHA256, entry.SHA256) {
			if err := verifyBlobSHA256(entry.SHA256); err != nil {
				return nil, nil, nil, nil, nil, nil, false, fmt.Sprintf("missing or corrupt backup blob for %s", rel)
			}
			toRevert = append(toRevert, rel)
		}
	}
	for rel := range liveFiles {
		if _, existed := cp.Files[rel]; !existed && !skippedSet[rel] {
			toDelete = append(toDelete, rel)
		}
	}
	for _, d := range cp.Dirs {
		if !liveDirSet[d] {
			dirsToCreate = append(dirsToCreate, d)
		}
	}
	for _, d := range liveDirs {
		if !cpDirSet[d] {
			dirsToRemove = append(dirsToRemove, d)
		}
	}

	sort.Strings(toRestore)
	sort.Strings(toRevert)
	sort.Strings(toDelete)
	sort.Strings(dirsToCreate)
	// Remove deepest directories first
	sort.Slice(dirsToRemove, func(i, j int) bool {
		di := strings.Count(dirsToRemove[i], "/")
		dj := strings.Count(dirsToRemove[j], "/")
		if di != dj {
			return di > dj
		}
		return dirsToRemove[i] > dirsToRemove[j]
	})

	if cp.Truncated || liveTruncated {
		warnings = append(warnings, "Workspace überschreitet das Snapshot-Limit (8000 Dateien); nur erfasste Dateien werden zurückgerollt.")
	}
	if len(cp.SkippedFiles) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d übersprungene Dateien (>16 MiB oder Symlinks) waren nicht im Snapshot enthalten.", len(cp.SkippedFiles)))
	}
	if cp.GitHead != "" {
		if curHead := readGitHeadFast(cp.Project); curHead != "" && curHead != cp.GitHead {
			warnings = append(warnings, "Git HEAD hat sich seit diesem Schritt geändert; Arbeitsverzeichnis wird wiederhergestellt, Git-Commits bleiben unberührt.")
		}
	}
	return toRestore, toRevert, toDelete, dirsToCreate, dirsToRemove, warnings, true, ""
}

func (s *AppState) PreviewRollbackToCheckpoint(threadID, checkpointID string) (RollbackPreview, error) {
	turnCheckpointMu.Lock()
	defer turnCheckpointMu.Unlock()

	cp, err := loadTurnCheckpointManifest(checkpointID)
	if err != nil {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Restorable: false, Reason: "Checkpoint nicht gefunden oder abgelaufen."}, nil
	}
	s.mu.RLock()
	if threadID == "" {
		threadID = cp.ThreadID
	}
	t := s.Threads[threadID]
	running := s.Running || s.isThreadRunningLocked(threadID)
	rootDir := s.Config.RootProjectDir
	var events []UIEvent
	if t != nil {
		events = append([]UIEvent(nil), t.Events...)
	}
	s.mu.RUnlock()

	if t == nil {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Restorable: false, Reason: "Chat nicht gefunden."}, nil
	}
	if cp.ThreadID != "" && cp.ThreadID != threadID {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Restorable: false, Reason: "Checkpoint gehört zu einem anderen Chat."}, nil
	}
	if _, err := ensureWithinRoot(rootDir, cp.Project); err != nil {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Project: cp.Project, Restorable: false, Reason: "Projekt liegt außerhalb der aktiven Projektwurzel."}, nil
	}
	if running {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Project: cp.Project, Restorable: false, Reason: "Agent läuft gerade. Bitte Vorgang zuerst stoppen."}, nil
	}
	cutIdx := findCheckpointEventIndex(events, checkpointID, cp.EventIndex)
	if cutIdx < 0 {
		return RollbackPreview{CheckpointID: checkpointID, ThreadID: threadID, Project: cp.Project, Restorable: false, Reason: "Schritt im Chatverlauf nicht mehr vorhanden."}, nil
	}

	toRestore, toRevert, toDelete, dirsToCreate, dirsToRemove, warnings, restorable, reason := computeWorkspaceRollbackPlan(cp)
	return RollbackPreview{
		CheckpointID:   cp.ID,
		ThreadID:       threadID,
		Project:        cp.Project,
		Restorable:     restorable,
		Reason:         reason,
		PromptPreview:  cp.PromptPreview,
		TurnsRemoved:   countUserTurnsFrom(events, cutIdx),
		EventsRemoved:  len(events) - cutIdx,
		FilesToRestore: toRestore,
		FilesToRevert:  toRevert,
		FilesToDelete:  toDelete,
		DirsToCreate:   dirsToCreate,
		DirsToRemove:   dirsToRemove,
		Warnings:       warnings,
	}, nil
}

func (s *AppState) RollbackToCheckpoint(threadID, checkpointID string) (RollbackResult, error) {
	turnCheckpointMu.Lock()
	defer turnCheckpointMu.Unlock()

	cp, err := loadTurnCheckpointManifest(checkpointID)
	if err != nil {
		return RollbackResult{}, errors.New("checkpoint not found")
	}
	s.mu.RLock()
	if threadID == "" {
		threadID = cp.ThreadID
	}
	t := s.Threads[threadID]
	running := s.Running || s.isThreadRunningLocked(threadID)
	rootDir := s.Config.RootProjectDir
	var events []UIEvent
	if t != nil {
		events = append([]UIEvent(nil), t.Events...)
	}
	s.mu.RUnlock()

	if t == nil {
		return RollbackResult{}, errors.New("chat thread not found")
	}
	if cp.ThreadID != "" && cp.ThreadID != threadID {
		return RollbackResult{}, errors.New("checkpoint does not belong to thread")
	}
	if running {
		return RollbackResult{}, errors.New("agent is running")
	}
	if _, err := ensureWithinRoot(rootDir, cp.Project); err != nil {
		return RollbackResult{}, err
	}
	cutIdx := findCheckpointEventIndex(events, checkpointID, cp.EventIndex)
	if cutIdx < 0 {
		return RollbackResult{}, errors.New("checkpoint event no longer in thread")
	}

	toRestore, toRevert, toDelete, dirsToCreate, dirsToRemove, warnings, restorable, reason := computeWorkspaceRollbackPlan(cp)
	if !restorable {
		return RollbackResult{}, errors.New(reason)
	}

	// 1. Recreate any deleted directories first
	for _, relDir := range dirsToCreate {
		target, err := ensureWithinRoot(cp.Project, filepath.Join(cp.Project, filepath.FromSlash(relDir)))
		if err != nil {
			return RollbackResult{}, err
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return RollbackResult{}, err
		}
	}

	// 2. Restore deleted files & revert modified files atomically from verified SHA-256 blobs
	for _, rel := range toRestore {
		entry := cp.Files[rel]
		target, err := ensureWithinRoot(cp.Project, filepath.Join(cp.Project, filepath.FromSlash(rel)))
		if err != nil {
			return RollbackResult{}, err
		}
		if err := copyBlobToWorkspaceFile(entry.SHA256, target, entry.Mode); err != nil {
			return RollbackResult{}, fmt.Errorf("restore %s: %w", rel, err)
		}
	}
	for _, rel := range toRevert {
		entry := cp.Files[rel]
		target, err := ensureWithinRoot(cp.Project, filepath.Join(cp.Project, filepath.FromSlash(rel)))
		if err != nil {
			return RollbackResult{}, err
		}
		if err := copyBlobToWorkspaceFile(entry.SHA256, target, entry.Mode); err != nil {
			return RollbackResult{}, fmt.Errorf("revert %s: %w", rel, err)
		}
	}

	// 3. Delete files created after the checkpoint
	for _, rel := range toDelete {
		target, err := ensureWithinRoot(cp.Project, filepath.Join(cp.Project, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		_ = os.Chmod(target, 0o666)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("Konnte neue Datei nicht entfernen: %s (%v)", rel, err))
		}
	}

	// 4. Clean up newly created empty directories deepest-first
	for _, relDir := range dirsToRemove {
		target, err := ensureWithinRoot(cp.Project, filepath.Join(cp.Project, filepath.FromSlash(relDir)))
		if err != nil {
			continue
		}
		_ = os.Remove(target) // os.Remove only removes empty directories, never deletes skipped files
	}

	// 5. Collect obsolete checkpoint IDs from removed events & truncate thread completely
	obsoleteCPs := []string{cp.ID}
	turnsRemoved := 0
	eventsRemoved := 0

	var threadSnapshot map[string]*ChatThread
	s.mu.Lock()
	liveThread := s.Threads[threadID]
	if liveThread != nil {
		if cutIdx > len(liveThread.Events) {
			cutIdx = len(liveThread.Events)
		}
		eventsRemoved = len(liveThread.Events) - cutIdx
		for i := cutIdx; i < len(liveThread.Events); i++ {
			if liveThread.Events[i].Type == "user" {
				turnsRemoved++
			}
			if liveThread.Events[i].CheckpointID != "" {
				obsoleteCPs = append(obsoleteCPs, liveThread.Events[i].CheckpointID)
			}
		}
		liveThread.Events = append([]UIEvent(nil), liveThread.Events[:cutIdx]...)
		if cutIdx == 0 {
			liveThread.Title = cp.ThreadTitleBefore
			if strings.TrimSpace(liveThread.Title) == "" {
				liveThread.Title = "Neuer Chat"
			}
		}
		liveThread.UpdatedAt = time.Now()
		if s.CurrentThread == threadID {
			s.Events = append([]UIEvent(nil), liveThread.Events...)
			s.Continuation = nil
			s.Pending = nil
			s.LastSummary = ""
			s.ActionLog = nil
		}
		s.Recovery = nil
		threadSnapshot = cloneThreads(s.Threads)
	}
	s.mu.Unlock()
	if threadSnapshot != nil {
		_ = saveThreads(threadSnapshot)
	}
	_ = os.Remove(runJournalPath())

	// 6. Remove obsolete checkpoint manifests and GC unreferenced blobs
	for _, id := range obsoleteCPs {
		if validCheckpointID(id) {
			_ = os.Remove(checkpointManifestPath(id))
		}
	}
	gcUnreferencedCheckpointBlobs()

	s.Broadcast(UIEvent{
		Type:      "thread_rolled_back",
		ThreadID:  threadID,
		Action:    cp.ID,
		Timestamp: time.Now(),
	})

	return RollbackResult{
		OK:            true,
		CheckpointID:  cp.ID,
		ThreadID:      threadID,
		TurnsRemoved:  turnsRemoved,
		EventsRemoved: eventsRemoved,
		RestoredFiles: len(toRestore),
		RevertedFiles: len(toRevert),
		DeletedFiles:  len(toDelete),
		Warnings:      warnings,
	}, nil
}

func listProjectSubdirs(root, reqPath string) ([]ProjectSubdirItem, error) {
	full, err := ensureWithinRoot(root, reqPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	items := make([]ProjectSubdirItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || isTurnCheckpointIgnoredDir(name) {
			continue
		}
		childPath := filepath.Join(full, name)
		hasSub := false
		if subEntries, subErr := os.ReadDir(childPath); subErr == nil {
			for _, se := range subEntries {
				if se.IsDir() && !strings.HasPrefix(se.Name(), ".") && !isTurnCheckpointIgnoredDir(se.Name()) {
					hasSub = true
					break
				}
			}
		}
		items = append(items, ProjectSubdirItem{
			Path:       childPath,
			Name:       name,
			HasSubdirs: hasSub,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (s *Server) handleProjectSubdirs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	reqPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if reqPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	root := s.state.Config.RootProjectDir
	s.state.mu.RUnlock()
	subdirs, err := listProjectSubdirs(root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"path": filepath.Clean(reqPath), "subdirs": subdirs})
}

func (s *Server) handleCheckpointPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	checkpointID := strings.TrimSpace(r.URL.Query().Get("checkpoint_id"))
	if checkpointID == "" {
		http.Error(w, "checkpoint_id is required", http.StatusBadRequest)
		return
	}
	preview, err := s.state.PreviewRollbackToCheckpoint(threadID, checkpointID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, preview)
}

func (s *Server) handleCheckpointRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadID     string `json:"thread_id"`
		CheckpointID string `json:"checkpoint_id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.state.RollbackToCheckpoint(strings.TrimSpace(req.ThreadID), strings.TrimSpace(req.CheckpointID))
	if err != nil {
		status := http.StatusConflict
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, res)
}
