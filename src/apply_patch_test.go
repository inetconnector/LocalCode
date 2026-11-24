// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchMultiFileAddUpdateDelete(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()

	// Seed existing files: one to update (with CRLF) and one to delete.
	existingUpdate := filepath.Join(project, "pkg", "calc.go")
	if err := os.MkdirAll(filepath.Dir(existingUpdate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingUpdate, []byte("package pkg\r\n\r\nfunc Add(a, b int) int {\r\n\treturn a + b\r\n}\r\n\r\nfunc Mul(a, b int) int {\r\n\treturn a * b\r\n}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	existingDelete := filepath.Join(project, "obsolete.txt")
	if err := os.WriteFile(existingDelete, []byte("remove me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: pkg/calc.go
@@ func Add
 func Add(a, b int) int {
-	return a + b
+	sum := a + b
+	return sum
 }
@@ func Mul
 func Mul(a, b int) int {
-	return a * b
+	return a * b * 1
 }
*** Add File: pkg/sub.go
+package pkg
+
+func Sub(a, b int) int {
+	return a - b
+}
*** Delete File: obsolete.txt
*** End Patch`

	out, err := applyPatchToProject(project, cfg, patch)
	if err != nil {
		t.Fatalf("applyPatchToProject failed: %v", err)
	}
	if !strings.Contains(out, "PATCH APPLIED (3 file(s))") {
		t.Fatalf("unexpected patch output: %s", out)
	}

	// Verify multi-hunk update preserved CRLF line endings.
	updatedBytes, err := os.ReadFile(existingUpdate)
	if err != nil {
		t.Fatal(err)
	}
	updated := string(updatedBytes)
	if !strings.Contains(updated, "sum := a + b\r\n\treturn sum\r\n") || !strings.Contains(updated, "return a * b * 1\r\n") {
		t.Fatalf("unexpected updated calc.go content: %q", updated)
	}

	// Verify added file exists with exact content.
	addedBytes, err := os.ReadFile(filepath.Join(project, "pkg", "sub.go"))
	if err != nil {
		t.Fatalf("added file missing: %v", err)
	}
	if string(addedBytes) != "package pkg\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n" {
		t.Fatalf("unexpected added file content: %q", string(addedBytes))
	}

	// Verify deleted file is gone.
	if _, err := os.Stat(existingDelete); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected obsolete.txt to be deleted, stat err=%v", err)
	}
}

func TestApplyPatchFuzzyWhitespaceMatching(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()

	target := filepath.Join(project, "hello.go")
	// File has extra trailing spaces and tab indentation that differ from the patch context.
	orig := "package main\n\nfunc greet() string {   \n\t   msg := \"hello\"  \n\treturn msg\n}\n"
	if err := os.WriteFile(target, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: hello.go
@@
 func greet() string {
-  msg := "hello"
+  msg := "hi there"
   return msg
 }
*** End Patch`

	if _, err := applyPatchToProject(project, cfg, patch); err != nil {
		t.Fatalf("expected fuzzy whitespace patch to succeed, got error: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `msg := "hi there"`) {
		t.Fatalf("expected fuzzy-patched line in file, got: %q", string(got))
	}
}

func TestApplyPatchAtomicRollbackOnSecondFileFailure(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()

	file1 := filepath.Join(project, "one.txt")
	file2 := filepath.Join(project, "two.txt")
	file3 := filepath.Join(project, "three.txt")
	orig1 := "alpha\nbeta\n"
	orig2 := "gamma\ndelta\n"
	orig3 := "epsilon\nzeta\n"
	for p, body := range map[string]string{file1: orig1, file2: orig2, file3: orig3} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Case A: Mid-commit failure on file 2 of a 3-file patch (after file 1 AND newly added file 0 were already written).
	// Inject a real concurrent edit conflict on file 2 right before op #2 commits!
	prevHook := applyPatchBeforeCommitOpHook
	defer func() { applyPatchBeforeCommitOpHook = prevHook }()

	applyPatchBeforeCommitOpHook = func(opIndex int, relPath string) error {
		if opIndex == 2 && relPath == "two.txt" {
			// Simulate external editor modifying two.txt mid-transaction -> triggers real EditConflictError!
			return os.WriteFile(file2, []byte("concurrent external change\n"), 0o644)
		}
		return nil
	}

	patch := `*** Begin Patch
*** Add File: created_first.txt
+should be rolled back
*** Update File: one.txt
@@
 alpha
-beta
+BETA_MODIFIED
*** Update File: two.txt
@@
 gamma
-delta
+DELTA_MODIFIED
*** Update File: three.txt
@@
 epsilon
-zeta
+ZETA_MODIFIED
*** End Patch`

	_, err := applyPatchToProject(project, cfg, patch)
	if err == nil {
		t.Fatal("expected applyPatchToProject to fail when file 2 conflicts mid-commit")
	}
	if !strings.Contains(err.Error(), "rolled back 2 previously applied file(s)") {
		t.Fatalf("expected rollback message for 2 prior files, got: %v", err)
	}

	// File 0 (created_first.txt) must have been removed by rollback!
	if _, statErr := os.Stat(filepath.Join(project, "created_first.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("created_first.txt should have been rolled back and removed, stat err=%v", statErr)
	}
	// File 1 (one.txt) must be restored to its exact original bytes!
	got1, _ := os.ReadFile(file1)
	if string(got1) != orig1 {
		t.Fatalf("one.txt was not rolled back! got=%q want=%q", string(got1), orig1)
	}
	// File 3 (three.txt) must never have been modified!
	got3, _ := os.ReadFile(file3)
	if string(got3) != orig3 {
		t.Fatalf("three.txt should remain untouched, got=%q want=%q", string(got3), orig3)
	}

	// Case B: Hunk mismatch in file 2 of 3 during planning also leaves file 1 untouched.
	applyPatchBeforeCommitOpHook = nil
	if err := os.WriteFile(file2, []byte(orig2), 0o644); err != nil {
		t.Fatal(err)
	}
	badHunkPatch := `*** Begin Patch
*** Update File: one.txt
@@
 alpha
-beta
+SHOULD_NOT_WRITE
*** Update File: two.txt
@@
 nonexistent_line
-still_missing
+nope
*** End Patch`
	if _, err := applyPatchToProject(project, cfg, badHunkPatch); err == nil {
		t.Fatal("expected hunk mismatch error")
	}
	got1After, _ := os.ReadFile(file1)
	if string(got1After) != orig1 {
		t.Fatalf("one.txt should not have been touched on planning failure, got=%q", string(got1After))
	}
}

func TestApplyPatchPathTraversalAndEscapeBlocked(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("top-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	insideFile := filepath.Join(project, "inside.txt")
	if err := os.WriteFile(insideFile, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()

	traversalPatches := []struct {
		name  string
		patch string
	}{
		{
			name: "relative_parent_add",
			patch: `*** Begin Patch
*** Add File: ../pwned.txt
+bad
*** End Patch`,
		},
		{
			name: "relative_parent_update",
			patch: `*** Begin Patch
*** Update File: ../secret.txt
@@
-top-secret
+hacked
*** End Patch`,
		},
		{
			name:  "absolute_outside_delete",
			patch: "*** Begin Patch\n*** Delete File: " + outsideFile + "\n*** End Patch",
		},
		{
			name: "move_to_outside",
			patch: `*** Begin Patch
*** Update File: inside.txt
*** Move to: ../escaped.txt
@@
-ok
+moved
*** End Patch`,
		},
	}

	for _, tc := range traversalPatches {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := applyPatchToProject(project, cfg, tc.patch); err == nil {
				t.Fatalf("expected path traversal %s to be blocked", tc.name)
			}
		})
	}

	// Ensure outside file is untouched and pwned.txt was never created.
	gotOutside, _ := os.ReadFile(outsideFile)
	if string(gotOutside) != "top-secret\n" {
		t.Fatalf("outside file was modified: %q", string(gotOutside))
	}
	if _, err := os.Stat(filepath.Join(parent, "pwned.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pwned.txt should never exist outside root")
	}

	// Symlink escape check (when OS permits symlink creation).
	linkPath := filepath.Join(project, "escape_link.txt")
	if symErr := os.Symlink(outsideFile, linkPath); symErr == nil {
		symPatch := `*** Begin Patch
*** Update File: escape_link.txt
@@
-top-secret
+symlink-pwned
*** End Patch`
		if _, err := applyPatchToProject(project, cfg, symPatch); err == nil {
			t.Fatal("expected symlink escape patch to be blocked by ensureWithinRoot")
		}
		gotOutside2, _ := os.ReadFile(outsideFile)
		if string(gotOutside2) != "top-secret\n" {
			t.Fatalf("outside file was modified via symlink: %q", string(gotOutside2))
		}
	}
}

func TestApplyPatchUnifiedDiffCompatibility(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()

	mainPath := filepath.Join(project, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"v1\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(project, "legacy.txt")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	unified := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -4,4 +4,4 @@ import "fmt"
 
 func main() {
-	fmt.Println("v1")
+	fmt.Println("v2")
 }
diff --git a/notes.txt b/notes.txt
new file mode 100644
--- /dev/null
+++ b/notes.txt
@@ -0,0 +1,2 @@
+line one
+line two
diff --git a/legacy.txt b/legacy.txt
deleted file mode 100644
--- a/legacy.txt
+++ /dev/null
@@ -1 +0,0 @@
-old
`

	if _, err := applyPatchToProject(project, cfg, unified); err != nil {
		t.Fatalf("unified diff apply failed: %v", err)
	}
	gotMain, _ := os.ReadFile(mainPath)
	if !strings.Contains(string(gotMain), `fmt.Println("v2")`) {
		t.Fatalf("unexpected main.go after unified diff: %q", string(gotMain))
	}
	gotNotes, err := os.ReadFile(filepath.Join(project, "notes.txt"))
	if err != nil || string(gotNotes) != "line one\nline two\n" {
		t.Fatalf("unexpected notes.txt: %q err=%v", string(gotNotes), err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy.txt to be deleted via unified diff, stat err=%v", err)
	}
}

func TestApplyPatchAgentRegistrationAndApproval(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify actionSchema contains apply_patch.
	props, ok := actionSchema["properties"].(orderedActionSchemaProperties)
	if !ok {
		t.Fatal("actionSchema properties not orderedActionSchemaProperties")
	}
	enumVals, _ := props.Action["enum"].([]string)
	found := false
	for _, v := range enumVals {
		if v == "apply_patch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("apply_patch missing from actionSchema enum")
	}

	patch := "*** Begin Patch\n*** Update File: app.txt\n@@\n-hello\n+world\n*** End Patch"
	act := fillAgentActionFromArguments(AgentAction{
		Action:  "apply_patch",
		Message: "Update app.txt",
		Arguments: map[string]any{
			"patch": "```patch\n" + patch + "\n```",
		},
	})
	if err := validateAgentAction(act); err != nil {
		t.Fatalf("validateAgentAction failed: %v", err)
	}
	if !actionMutatesProject(act) {
		t.Fatal("apply_patch must be classified as mutating")
	}
	paths := mutatedActionPaths(act)
	if len(paths) != 1 || paths[0] != "app.txt" {
		t.Fatalf("unexpected mutatedActionPaths: %v", paths)
	}

	balancedCfg := defaultConfig()
	balancedCfg.ApprovalMode = "balanced"
	if !actionNeedsApproval(balancedCfg, project, act) {
		t.Fatal("apply_patch must require approval in balanced mode")
	}
	autoCfg := defaultConfig()
	autoCfg.ApprovalMode = "auto"
	if actionNeedsApproval(autoCfg, project, act) {
		t.Fatal("non-deleting apply_patch should be auto-approved in auto mode like replace_text/write_file")
	}
	deleteAct := AgentAction{
		Action:  "apply_patch",
		Message: "Delete app.txt",
		Patch:   "*** Begin Patch\n*** Delete File: app.txt\n*** End Patch",
	}
	if !actionNeedsApproval(autoCfg, project, deleteAct) {
		t.Fatal("deleting apply_patch must still require approval in auto mode")
	}

	preview, err := previewAction(project, balancedCfg, act)
	if err != nil || !strings.Contains(preview, "UPDATE app.txt") {
		t.Fatalf("previewAction failed: %v preview=%q", err, preview)
	}
	res, err := executeAction(context.Background(), project, balancedCfg, act)
	if err != nil || !strings.Contains(res, "PATCH APPLIED") {
		t.Fatalf("executeAction failed: %v res=%q", err, res)
	}
	got, _ := os.ReadFile(filepath.Join(project, "app.txt"))
	if string(got) != "world\n" {
		t.Fatalf("expected app.txt to contain world, got %q", string(got))
	}
}
