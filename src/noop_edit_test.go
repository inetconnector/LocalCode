// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteProjectFileRejectsNoOp(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "same.txt")
	const content = "already correct\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeProjectFile(project, "same.txt", content)
	if err == nil {
		t.Fatalf("expected no-op write to fail, got result %q", result)
	}
	if !strings.Contains(err.Error(), "no observable project changes") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != content {
		t.Fatalf("no-op rejection changed the file: %q", got)
	}
}

func TestReplaceTextRejectsNoOp(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "same.txt")
	const content = "alpha beta gamma\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := replaceText(project, "same.txt", "beta", "beta")
	if err == nil {
		t.Fatalf("expected no-op replacement to fail, got result %q", result)
	}
	if !strings.Contains(err.Error(), "no observable project changes") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != content {
		t.Fatalf("no-op rejection changed the file: %q", got)
	}
}

func TestWriteProjectFileStillAppliesRealChange(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "change.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := writeProjectFile(project, "change.txt", "after\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "+after") {
		t.Fatalf("expected diff to describe the change, got %q", result)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "after\n" {
		t.Fatalf("real write was not applied: %q", got)
	}
}

func TestReplaceTextStillAppliesRealChange(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "change.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := replaceText(project, "change.txt", "beta", "delta")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "+alpha delta gamma") {
		t.Fatalf("expected diff to describe the change, got %q", result)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "alpha delta gamma\n" {
		t.Fatalf("real replacement was not applied: %q", got)
	}
}
