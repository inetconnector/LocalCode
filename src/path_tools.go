// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func sandboxRoots(cfg Config, project string) []string {
	roots := []string{}
	switch cfg.SandboxMode {
	case "project":
		roots = append(roots, project)
	case "workspace":
		roots = append(roots, cfg.RootProjectDir)
	case "unrestricted":
		return nil
	}
	roots = append(roots, cfg.AllowedRoots...)
	return roots
}

func canonicalSandboxPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	// EvalSymlinks requires the complete path to exist. For a new file, walk
	// upward to the nearest existing ancestor, resolve all symlinks/junctions
	// there, and append the still-missing suffix. This closes the common escape
	// where a directory inside the project is an NTFS junction to another drive.
	current := abs
	var suffix []string
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(evalErr, os.ErrNotExist) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", evalErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func resolveSandboxPath(cfg Config, project, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("path is empty")
	}
	var full string
	if filepath.IsAbs(raw) {
		full = filepath.Clean(raw)
	} else {
		full = filepath.Join(project, raw)
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	roots := sandboxRoots(cfg, project)
	if roots == nil {
		return abs, nil
	}
	canonicalCandidate, err := canonicalSandboxPath(abs)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		canonicalRoot, rootErr := canonicalSandboxPath(root)
		if rootErr == nil && pathWithin(canonicalRoot, canonicalCandidate) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path is outside configured sandbox roots: %s", abs)
}

func copyPath(cfg Config, project, source, destination string) (string, error) {
	src, err := resolveSandboxPath(cfg, project, source)
	if err != nil {
		return "", err
	}
	dst, err := resolveSandboxPath(cfg, project, destination)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return "", err
		}
	} else {
		if err := copyFile(src, dst, info.Mode()); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Copied %s -> %s\n\nPOSTCONDITION:\n%s\n%s", src, dst, describePathState("source", src), describePathState("destination", dst)), nil
}

func movePath(cfg Config, project, source, destination string) (string, error) {
	src, err := resolveSandboxPath(cfg, project, source)
	if err != nil {
		return "", err
	}
	dst, err := resolveSandboxPath(cfg, project, destination)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dst); err != nil {
		if _, copyErr := copyPath(cfg, project, src, dst); copyErr != nil {
			return "", err
		}
		if removeErr := os.RemoveAll(src); removeErr != nil {
			return "", removeErr
		}
	}
	return fmt.Sprintf("Moved %s -> %s\n\nPOSTCONDITION:\n%s\n%s", src, dst, describePathState("source", src), describePathState("destination", dst)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, _ := d.Info()
			mode := os.FileMode(0o755)
			if info != nil {
				mode = info.Mode()
			}
			return os.MkdirAll(target, mode.Perm())
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func describePathState(label, path string) string {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Sprintf("%s: exists=false path=%s", label, strconv.Quote(path))
		}
		return fmt.Sprintf("%s: error=%s path=%s", label, strconv.Quote(err.Error()), strconv.Quote(path))
	}
	if info.IsDir() {
		files, dirs, bytes, walkErr := summarizeDir(path)
		if walkErr != nil {
			return fmt.Sprintf("%s: exists=true type=dir path=%s error=%s", label, strconv.Quote(path), strconv.Quote(walkErr.Error()))
		}
		return fmt.Sprintf("%s: exists=true type=dir path=%s files=%d dirs=%d bytes=%d", label, strconv.Quote(path), files, dirs, bytes)
	}
	hash, hashErr := sha256File(path)
	if hashErr != nil {
		return fmt.Sprintf("%s: exists=true type=file path=%s size=%d error=%s", label, strconv.Quote(path), info.Size(), strconv.Quote(hashErr.Error()))
	}
	return fmt.Sprintf("%s: exists=true type=file path=%s size=%d sha256=%s", label, strconv.Quote(path), info.Size(), hash)
}

func summarizeDir(root string) (files, dirs int, bytes int64, err error) {
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if d.IsDir() {
			dirs++
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, dirs, bytes, err
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
