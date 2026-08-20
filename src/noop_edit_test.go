// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNoOpFixture(t *testing.T, name, content string) (string, string) {
	t.Helper()
	project := t.TempDir()
	full := filepath.Join(project, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return project, full
}

func assertNoOpEditError(t *testing.T, result string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected no-op edit to fail, got result %q", result)
	}
	if !errors.Is(err, errNoObservableProjectChanges) {
		t.Fatalf("expected no-observable-change sentinel, got %v", err)
	}
}

func TestWriteProjectFileRejectsNoOp(t *testing.T) {
	const content = "already correct\n"
	project, full := writeNoOpFixture(t, "same.txt", content)
	result, err := writeProjectFile(project, "same.txt", content)
	assertNoOpEditError(t, result, err)
	got, readErr := os.ReadFile(full)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != content {
		t.Fatalf("no-op rejection changed the file: %q", got)
	}
}

func TestReplaceTextRejectsNoOp(t *testing.T) {
	const content = "alpha beta gamma\n"
	project, full := writeNoOpFixture(t, "same.txt", content)
	result, err := replaceText(project, "same.txt", "beta", "beta")
	assertNoOpEditError(t, result, err)
	got, readErr := os.ReadFile(full)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != content {
		t.Fatalf("no-op rejection changed the file: %q", got)
	}
}

func TestApprovedWriteProjectFileRejectsNoOp(t *testing.T) {
	const content = "approved content\n"
	project, full := writeNoOpFixture(t, "same.txt", content)
	expected, err := readFileVersion(full)
	if err != nil {
		t.Fatal(err)
	}
	result, err := writeProjectFileAtVersion(project, "same.txt", content, expected)
	assertNoOpEditError(t, result, err)
	actual, err := readFileVersion(full)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("approved no-op write changed file version: got %+v want %+v", actual, expected)
	}
}

func TestApprovedReplaceTextRejectsNoOp(t *testing.T) {
	const content = "alpha beta gamma\n"
	project, full := writeNoOpFixture(t, "same.txt", content)
	expected, err := readFileVersion(full)
	if err != nil {
		t.Fatal(err)
	}
	result, err := replaceTextAtVersion(project, "same.txt", "beta", "beta", expected)
	assertNoOpEditError(t, result, err)
	actual, err := readFileVersion(full)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("approved no-op replacement changed file version: got %+v want %+v", actual, expected)
	}
}

func TestWriteProjectFileStillAppliesRealChange(t *testing.T) {
	project, full := writeNoOpFixture(t, "change.txt", "before\n")
	result, err := writeProjectFile(project, "change.txt", "after\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "+after") {
		t.Fatalf("expected diff to describe the change, got %q", result)
	}
	got, readErr := os.ReadFile(full)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "after\n" {
		t.Fatalf("real write was not applied: %q", got)
	}
}

func TestReplaceTextStillAppliesRealChange(t *testing.T) {
	project, full := writeNoOpFixture(t, "change.txt", "alpha beta gamma\n")
	result, err := replaceText(project, "change.txt", "beta", "delta")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "+alpha delta gamma") {
		t.Fatalf("expected diff to describe the change, got %q", result)
	}
	got, readErr := os.ReadFile(full)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "alpha delta gamma\n" {
		t.Fatalf("real replacement was not applied: %q", got)
	}
}

func TestApprovedWriteProjectFileStillAppliesRealChange(t *testing.T) {
	project, full := writeNoOpFixture(t, "approved.txt", "before\n")
	expected, err := readFileVersion(full)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeProjectFileAtVersion(project, "approved.txt", "after\n", expected); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("approved real write was not applied: %q", got)
	}
}
