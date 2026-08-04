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
	if strings.TrimSpace(target) == "" {
		return absRoot, nil
	}
	var absTarget string
	if filepath.IsAbs(target) {
		absTarget, err = filepath.Abs(target)
	} else {
		absTarget, err = filepath.Abs(filepath.Join(absRoot, target))
	}
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
