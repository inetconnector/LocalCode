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
			absTarget, err = filepath.Abs(filepath.Join(absRoot, target))
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

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
