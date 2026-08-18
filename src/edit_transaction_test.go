// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAtomicWriteRejectsStaleVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := os.WriteFile(path, []byte("version-a\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	expected, err := readFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external-change\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	err = atomicWriteFileIfVersion(path, []byte("localcode-change\n"), 0o640, expected)
	if err == nil || !isEditConflict(err) {
		t.Fatalf("expected EDIT_CONFLICT, got %v", err)
	}
	var conflict *EditConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected typed conflict, got %T", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "external-change\n" {
		t.Fatalf("stale write overwrote newer content: %q", data)
	}
	matches, globErr := filepath.Glob(filepath.Join(root, ".localcode-write-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("staged temp file leaked after conflict: %#v", matches)
	}
}

func TestWriteProjectFileUsesAtomicReplacementAndPreservesMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := writeProjectFile(root, "config.txt", "after\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after\n" {
		t.Fatalf("unexpected content: %q", data)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: got %o want 640", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(root, ".localcode-write-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("staged temp file leaked after success: %#v", matches)
	}
}

func TestReplaceTextKeepsExactMatchContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nconst value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := replaceText(root, "main.go", "const value = 1", "const value = 2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "-const value = 1") || !strings.Contains(result, "+const value = 2") {
		t.Fatalf("diff missing expected edit:\n%s", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\n\nconst value = 2\n" {
		t.Fatalf("unexpected updated file: %q", data)
	}
}

func TestPathEditLockSerializesConditionalWriters(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(path, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := readFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, content := range []string{"writer-one\n", "writer-two\n"} {
		content := content
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release := acquireEditPathLock(path)
			defer release()
			results <- atomicWriteFileIfVersion(path, []byte(content), 0o644, expected)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case isEditConflict(err):
			conflicts++
		default:
			t.Fatalf("unexpected writer error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("got successes=%d conflicts=%d; want 1/1", successes, conflicts)
	}
}
