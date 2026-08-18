// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var ignoredDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "vendor": true,
	"bin": true, "obj": true, "dist": true, "build": true, "target": true,
	".idea": true, ".vs": true, ".vscode": false,
	".gradle": true, ".next": true, ".nuxt": true, "coverage": true,
	".localcode": true, ".local" + "codex": true,
}

var ignoredExt = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true,
	".zip": true, ".7z": true, ".rar": true, ".tar": true, ".gz": true,
	".pdf": true, ".mp3": true, ".mp4": true, ".mov": true, ".avi": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
	".class": true, ".jar": true, ".pyc": true, ".o": true, ".a": true,
}

func projectTree(root, sub string, maxDepth, maxEntries int) (string, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > 6 {
		maxDepth = 6
	}
	if maxEntries <= 0 {
		maxEntries = 600
	}
	start, err := ensureWithinRoot(root, sub)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(start)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", sub)
	}

	type entry struct {
		rel   string
		isDir bool
	}
	entries := make([]entry, 0, maxEntries)
	baseDepth := strings.Count(filepath.Clean(start), string(filepath.Separator))
	err = filepath.WalkDir(start, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == start {
			return nil
		}
		name := d.Name()
		if d.IsDir() && ignoredDirs[name] {
			return filepath.SkipDir
		}
		depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - baseDepth
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		entries = append(entries, entry{filepath.ToSlash(rel), d.IsDir()})
		if len(entries) >= maxEntries {
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	var b strings.Builder
	for _, e := range entries {
		if e.isDir {
			b.WriteString("[D] ")
		} else {
			b.WriteString("[F] ")
		}
		b.WriteString(e.rel)
		b.WriteByte('\n')
	}
	if len(entries) >= maxEntries {
		b.WriteString("...[entry limit reached]\n")
	}
	return b.String(), nil
}

func isProbablyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func readProjectFile(root, path string) (string, error) {
	full, err := ensureWithinRoot(root, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", path)
	}
	if info.Size() > 1024*1024 {
		return "", fmt.Errorf("file too large (%d bytes): %s", info.Size(), path)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if !isProbablyText(data) {
		return "", fmt.Errorf("binary or non-UTF-8 file: %s", path)
	}
	return truncateText(string(data), 100000), nil
}

func searchProject(root, query, sub string, maxHits int) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("search query is empty")
	}
	if maxHits <= 0 {
		maxHits = 100
	}
	start, err := ensureWithinRoot(root, sub)
	if err != nil {
		return "", err
	}
	q := strings.ToLower(query)
	hits := 0
	var out strings.Builder
	err = filepath.WalkDir(start, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != start && ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1024*1024 {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), q) {
				rel, _ := filepath.Rel(root, path)
				fmt.Fprintf(&out, "%s:%d: %s\n", filepath.ToSlash(rel), lineNo, truncateText(strings.TrimSpace(line), 500))
				hits++
				if hits >= maxHits {
					_ = f.Close()
					return io.EOF
				}
			}
		}
		_ = f.Close()
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if hits == 0 {
		return "No matches.", nil
	}
	if hits >= maxHits {
		out.WriteString("...[hit limit reached]\n")
	}
	return out.String(), nil
}

func backupFile(projectRoot, fullPath string) error {
	info, err := os.Stat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("cannot backup directory")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(projectRoot))
	rel, _ := filepath.Rel(projectRoot, fullPath)
	dir := filepath.Join(cache, "LocalCode", "backups", hex.EncodeToString(sum[:8]), time.Now().Format("20060102-150405.000"))
	target := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, info.Mode().Perm())
}

func replaceText(projectRoot, path, oldText, newText string) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	original := string(data)
	count := strings.Count(original, oldText)
	if count != 1 {
		return "", fmt.Errorf("old_text must occur exactly once; found %d occurrences", count)
	}
	updated := strings.Replace(original, oldText, newText, 1)
	if updated == original {
		return "", errors.New("no observable project changes: replacement leaves file unchanged")
	}
	if err := backupFile(projectRoot, full); err != nil {
		return "", err
	}
	info, _ := os.Stat(full)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(full, []byte(updated), mode); err != nil {
		return "", err
	}
	return simpleDiff(original, updated), nil
}

func writeProjectFile(projectRoot, path, content string) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	old := ""
	mode := os.FileMode(0o644)
	existed := false
	if data, err := os.ReadFile(full); err == nil {
		if !isProbablyText(data) {
			return "", fmt.Errorf("refusing to overwrite binary or non-UTF-8 file: %s", path)
		}
		existed = true
		old = string(data)
		if old == content {
			return "", errors.New("no observable project changes: file already has the requested content")
		}
		if info, err := os.Stat(full); err == nil {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if existed {
		if err := backupFile(projectRoot, full); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		return "", err
	}
	return simpleDiff(old, content) + "\n\nPOSTCONDITION:\n" + describePathState("target", full), nil
}

func deleteProjectFile(projectRoot, path string) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("directory deletion is not allowed")
	}
	old, _ := os.ReadFile(full)
	if err := backupFile(projectRoot, full); err != nil {
		return "", err
	}
	if err := os.Remove(full); err != nil {
		return "", err
	}
	return simpleDiff(string(old), "") + "\n\nPOSTCONDITION:\n" + describePathState("target", full), nil
}

func simpleDiff(oldText, newText string) string {
	oldLines := strings.Split(strings.ReplaceAll(oldText, "\r\n", "\n"), "\n")
	newLines := strings.Split(strings.ReplaceAll(newText, "\r\n", "\n"), "\n")
	// Fast, readable prefix/suffix diff; intentionally bounded for large files.
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix && oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	start := prefix
	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix
	contextStart := start - 3
	if contextStart < 0 {
		contextStart = 0
	}
	contextOldEnd := oldEnd + 3
	if contextOldEnd > len(oldLines) {
		contextOldEnd = len(oldLines)
	}
	contextNewEnd := newEnd + 3
	if contextNewEnd > len(newLines) {
		contextNewEnd = len(newLines)
	}
	var b strings.Builder
	b.WriteString("--- before\n+++ after\n")
	for i := contextStart; i < start && i < len(oldLines); i++ {
		b.WriteString(" ")
		b.WriteString(oldLines[i])
		b.WriteByte('\n')
	}
	for i := start; i < oldEnd; i++ {
		b.WriteString("-")
		b.WriteString(oldLines[i])
		b.WriteByte('\n')
	}
	for i := start; i < newEnd; i++ {
		b.WriteString("+")
		b.WriteString(newLines[i])
		b.WriteByte('\n')
	}
	tailStartOld := oldEnd
	tailStartNew := newEnd
	tailCount := contextOldEnd - tailStartOld
	if n := contextNewEnd - tailStartNew; n < tailCount {
		tailCount = n
	}
	for i := 0; i < tailCount; i++ {
		b.WriteString(" ")
		b.WriteString(newLines[tailStartNew+i])
		b.WriteByte('\n')
	}
	return truncateText(b.String(), 80000)
}

func executeCommand(parent context.Context, projectRoot, command string, cfg Config) (string, error) {
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return runProjectCommand(ctx, projectRoot, command, cfg)
}
