// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeAiderExecutable(t *testing.T, project string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aider")
	body := `for arg in "$@"; do
  if [ "$arg" = "--version" ]; then echo "aider 0.86.2"; exit 0; fi
  if [ "$arg" = "--show-repo-map" ]; then echo "Repository map: app.go -> main"; exit 0; fi
done
printf '\n// edited by fake aider\n' >> app.go
echo "Applied edit to app.go"
exit 0`
	if runtime.GOOS == "windows" {
		path += ".cmd"
		body = `for %%A in (%*) do if "%%~A"=="--version" (echo aider 0.86.2& exit /b 0)
for %%A in (%*) do if "%%~A"=="--show-repo-map" (echo Repository map: app.go -^> main& exit /b 0)
echo.>>app.go
echo // edited by fake aider>>app.go
echo Applied edit to app.go
exit /b 0`
	}
	writeTestExecutable(t, path, body)
	return path
}

func TestAiderStatusAndPinnedVersion(t *testing.T) {
	project := t.TempDir()
	fake := fakeAiderExecutable(t, project)
	cfg := defaultConfig()
	cfg.AiderExecutable = fake
	cfg.AiderVersion = aiderPinnedVersion
	status := aiderStatus(context.Background(), cfg)
	if !status.Installed {
		t.Fatalf("Aider was not verified: %#v", status)
	}
	if !strings.Contains(status.Version, aiderPinnedVersion) {
		t.Fatalf("unexpected version: %q", status.Version)
	}
}

func TestBuildAiderArgsUsesManagedSafeModeAndOllama(t *testing.T) {
	project := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":    "module example\n\ngo 1.23\n",
		"app.go":    "package main\nfunc main() {}\n",
		"README.md": "# Example\n",
		"AGENTS.md": "Do good work.\n",
		"STATE.md":  "Current state.\n",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig()
	cfg.AiderUseGit = false
	args, messageFile, err := buildAiderArgs(project, "change main function", "qwen2.5-coder:14b", "thread-1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(messageFile)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--model ollama_chat/qwen2.5-coder:14b",
		"--message-file",
		"--map-tokens 4096",
		"--max-chat-history-tokens 8192",
		"--config",
		"--env-file",
		"--no-suggest-shell-commands",
		"--disable-playwright",
		"--no-git",
		"--lint-cmd go vet ./...",
		"--test-cmd go test ./...",
		"--read README.md",
		"--read AGENTS.md",
		"--read STATE.md",
		"--file app.go",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in Aider args:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--file STATE.md") || strings.Contains(joined, "--file AGENTS.md") {
		t.Fatalf("managed documentation was made editable: %s", joined)
	}
}

func TestRunAiderDetectsChangesBacksUpAndCanRestore(t *testing.T) {
	project := t.TempDir()
	original := "package main\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(project, "app.go"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := fakeAiderExecutable(t, project)
	cfg := defaultConfig()
	cfg.AiderExecutable = fake
	cfg.AiderUseGit = false
	cfg.AiderAutoLint = false
	cfg.AiderAutoTest = false
	cfg.OllamaURL = "http://127.0.0.1:11434"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runAider(ctx, project, "edit app.go", "qwen2.5-coder:14b", "thread", cfg)
	if err != nil {
		t.Fatalf("runAider failed: %v\n%s", err, result.Output)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0] != "app.go" {
		t.Fatalf("unexpected changed files: %#v", result.ChangedFiles)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "LOCALCODE-AIDER-MANIFEST.json")); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}
	restored, err := restoreAiderBackup(project, result.BackupDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restored, "Restored files: 1") {
		t.Fatalf("unexpected restore report: %s", restored)
	}
	data, err := os.ReadFile(filepath.Join(project, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("restore did not recover original content:\n%s", data)
	}
}

func TestRestoreAiderBackupRefusesToOverwriteLaterUserChanges(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.go"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := fakeAiderExecutable(t, project)
	cfg := defaultConfig()
	cfg.AiderExecutable = fake
	cfg.AiderUseGit = false
	cfg.AiderAutoLint = false
	cfg.AiderAutoTest = false
	cfg.OllamaURL = "http://127.0.0.1:11434"
	result, err := runAider(context.Background(), project, "edit", "qwen2.5-coder:14b", "thread", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "app.go"), []byte("user changed after aider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreAiderBackup(project, result.BackupDir); err == nil || !strings.Contains(err.Error(), "changed after") {
		t.Fatalf("expected divergence protection, got %v", err)
	}
}

func TestAiderArchitectModePassesEditorSettings(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.AiderArchitectMode = true
	cfg.AiderArchitectModel = "gpt-oss:20b"
	cfg.AiderEditorModel = "qwen2.5-coder:14b"
	args, messageFile, err := buildAiderArgs(project, "implement feature", "qwen2.5-coder:14b", "thread", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(messageFile)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--model ollama_chat/gpt-oss:20b", "--architect", "--auto-accept-architect", "--editor-model ollama_chat/qwen2.5-coder:14b", "--editor-edit-format editor-diff"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestEmbeddedUIContainsAiderSettingsAndActions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("static", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, id := range []string{
		`id="setEditingEngine"`, `id="setAiderEnabled"`, `id="setAiderVersion"`,
		`id="setAiderArchitectMode"`, `id="setAiderMapTokens"`, `id="setAiderAutoLint"`,
		`id="setAiderAutoTest"`, `id="aiderStatusBtn"`, `id="aiderInstallBtn"`,
		`id="aiderTestBtn"`, `id="aiderUndoBtn"`, `id="aiderResult"`,
	} {
		if !strings.Contains(text, id) {
			t.Fatalf("embedded UI is missing %s", id)
		}
	}
	for _, fragment := range []string{"/api/aider/status", "/api/aider/setup", "/api/aider/undo"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("embedded UI is not wired to %s", fragment)
		}
	}
}

func TestInferAiderQualityCommandsIsConservative(t *testing.T) {
	node := t.TempDir()
	if err := os.WriteFile(filepath.Join(node, "package.json"), []byte(`{"scripts":{"lint":"eslint .","test":"vitest run"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lint, test := inferAiderQualityCommands(node)
	if lint != "npm run lint" || test != "npm test" {
		t.Fatalf("unexpected node commands: lint=%q test=%q", lint, test)
	}

	pythonWithoutPytest := t.TempDir()
	if err := os.WriteFile(filepath.Join(pythonWithoutPytest, "requirements.txt"), []byte("requests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lint, test = inferAiderQualityCommands(pythonWithoutPytest)
	if lint != "python -m compileall ." || test != "" {
		t.Fatalf("pytest should not be assumed: lint=%q test=%q", lint, test)
	}

	pythonWithPytest := t.TempDir()
	if err := os.WriteFile(filepath.Join(pythonWithPytest, "pyproject.toml"), []byte("[project]\nname='sample'\nversion='0.1.0'\n[tool.pytest.ini_options]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pythonWithPytest, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pythonWithPytest, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lint, test = inferAiderQualityCommands(pythonWithPytest)
	if lint != "python -m compileall ." || test != "python -m pytest" {
		t.Fatalf("pytest configuration was not detected: lint=%q test=%q", lint, test)
	}
}

func TestAiderDocumentationAndLicenseFilesArePresent(t *testing.T) {
	root := filepath.Clean("..")
	for _, name := range []string{
		"README.md", "STATE.md", "COMMIT_MESSAGE.txt", "NOTICE-AIDER.md",
		filepath.Join("docs", "AIDER-INTEGRATION.md"), filepath.Join("licenses", "aider-LICENSE.txt"),
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("required Aider integration file %s is missing: %v", name, err)
		}
		if len(strings.TrimSpace(string(data))) < 40 {
			t.Fatalf("required Aider integration file %s is unexpectedly empty", name)
		}
	}
}

func TestAiderModelNameAlwaysUsesOllamaChatForLocalModels(t *testing.T) {
	cases := map[string]string{
		"qwen2.5-coder:14b":               "ollama_chat/qwen2.5-coder:14b",
		"hf.co/example/Model-GGUF:Q5_K_M": "ollama_chat/hf.co/example/Model-GGUF:Q5_K_M",
		"ollama/qwen3-coder:30b":          "ollama_chat/qwen3-coder:30b",
		"ollama_chat/qwen2.5-coder:14b":   "ollama_chat/qwen2.5-coder:14b",
	}
	for input, want := range cases {
		if got := aiderModelName(input); got != want {
			t.Fatalf("aiderModelName(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestBuildAiderUtilityArgsUsesManagedIsolationAndInferredCommands(t *testing.T) {
	project := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example\n\ngo 1.23\n",
		"app.go": "package main\nfunc main() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig()
	cfg.AiderUseGit = false
	cfg.AiderLintCommand = ""
	cfg.AiderTestCommand = ""
	for _, tc := range []struct {
		mode string
		want []string
	}{
		{mode: "repo-map", want: []string{"--show-repo-map"}},
		{mode: "lint", want: []string{"--lint", "--lint-cmd go vet ./..."}},
		{mode: "test", want: []string{"--test", "--test-cmd go test ./..."}},
	} {
		args, err := buildAiderUtilityArgs(project, tc.mode, "hf.co/example/coder:Q5_K_M", "thread/1", cfg)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{
			"--model ollama_chat/hf.co/example/coder:Q5_K_M",
			"--config",
			"--env-file",
			"--aiderignore",
			"--max-chat-history-tokens 8192",
			"--no-suggest-shell-commands",
			"--disable-playwright",
			"--no-auto-lint",
			"--no-auto-test",
			"--no-git",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s utility args missing %q:\n%s", tc.mode, want, joined)
			}
		}
		for _, want := range tc.want {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s utility args missing %q:\n%s", tc.mode, want, joined)
			}
		}
	}
}

func TestRunAiderHonorsCancellationAndReturnsControl(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "aider")
	body := `if [ "$1" = "--version" ]; then echo "aider 0.86.2"; exit 0; fi
sleep 20
exit 0`
	if runtime.GOOS == "windows" {
		path += ".cmd"
		body = `@echo off
if "%1"=="--version" (echo aider 0.86.2& exit /b 0)
ping 127.0.0.1 -n 20 >nul
exit /b 0`
	}
	writeTestExecutable(t, path, body)
	cfg := defaultConfig()
	cfg.AiderExecutable = path
	cfg.AiderUseGit = false
	cfg.AiderAutoLint = false
	cfg.AiderAutoTest = false
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runAider(ctx, project, "edit app.go", "qwen2.5-coder:14b", "thread", cfg)
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected a controlled deadline error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cancelled Aider process took too long to return: %s", elapsed)
	}
}
