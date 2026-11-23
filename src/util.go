// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func readJSON(r io.Reader, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r, 192<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func readJSONPermissive(r io.Reader, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r, 192<<20))
	return dec.Decode(dst)
}

func ensureWithinRoot(root, target string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absRoot = filepath.Clean(absRoot)

	absTarget := absRoot
	if strings.TrimSpace(target) != "" {
		if filepath.IsAbs(target) {
			absTarget, err = filepath.Abs(target)
		} else {
			relTarget := normalizeProjectRelativeTarget(absRoot, target)
			absTarget, err = filepath.Abs(filepath.Join(absRoot, relTarget))
		}
		if err != nil {
			return "", err
		}
		absTarget = filepath.Clean(absTarget)
	}

	canonicalRoot, err := canonicalSandboxPath(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	canonicalTarget, err := canonicalSandboxPath(absTarget)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	if !pathWithin(canonicalRoot, canonicalTarget) {
		return "", fmt.Errorf("path escapes project root: %s", target)
	}
	return absTarget, nil
}

// normalizeProjectRelativeTarget strips a redundant leading project-folder
// segment (e.g. "product/src/engine.js" when absRoot ends in "product")
// provided no actual subdirectory with that name exists inside absRoot.
func normalizeProjectRelativeTarget(absRoot, target string) string {
	trimmed := strings.TrimSpace(target)
	slashed := filepath.ToSlash(trimmed)
	for strings.HasPrefix(slashed, "./") {
		slashed = strings.TrimPrefix(slashed, "./")
	}
	idx := strings.IndexByte(slashed, '/')
	if idx <= 0 {
		return target
	}
	firstSeg := slashed[:idx]
	rest := strings.TrimLeft(slashed[idx+1:], "/")
	if rest == "" || firstSeg == "." || firstSeg == ".." {
		return target
	}
	rootBase := filepath.Base(absRoot)
	if !strings.EqualFold(firstSeg, rootBase) {
		return target
	}
	if _, err := os.Stat(filepath.Join(absRoot, firstSeg)); err == nil {
		return target
	}
	return filepath.FromSlash(rest)
}

func normalizeProjectRelativeArgs(project string, args []string) []string {
	if len(args) == 0 {
		return args
	}
	absRoot, err := filepath.Abs(project)
	if err != nil {
		return args
	}
	absRoot = filepath.Clean(absRoot)
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = normalizeProjectRelativeTarget(absRoot, arg)
	}
	return out
}

func normalizeCommandProjectRelativePaths(project, command string) string {
	absRoot, err := filepath.Abs(project)
	if err != nil {
		return command
	}
	absRoot = filepath.Clean(absRoot)
	rootBase := filepath.Base(absRoot)
	if rootBase == "" || rootBase == "." {
		return command
	}
	if _, err := os.Stat(filepath.Join(absRoot, rootBase)); err == nil {
		return command
	}
	fields := strings.Fields(command)
	changed := false
	for i, token := range fields {
		quote := ""
		inner := token
		if len(token) >= 2 && ((strings.HasPrefix(token, `"`) && strings.HasSuffix(token, `"`)) || (strings.HasPrefix(token, `'`) && strings.HasSuffix(token, `'`))) {
			quote = token[:1]
			inner = token[1 : len(token)-1]
		}
		norm := normalizeProjectRelativeTarget(absRoot, inner)
		if norm != inner {
			fields[i] = quote + filepath.ToSlash(norm) + quote
			changed = true
		}
	}
	if !changed {
		return command
	}
	return strings.Join(fields, " ")
}

// ensureExistingProjectDir validates both the canonical project boundary and
// the concrete directory before a caller uses it as a process working folder.
// A containment check alone intentionally permits missing paths for file
// creation, but a project/working-directory boundary must not do so.
func ensureExistingProjectDir(root, target string) (string, error) {
	full, err := ensureWithinRoot(root, target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("project directory not found: %s", full)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", full)
	}
	return full, nil
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
