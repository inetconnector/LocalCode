// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClawOllamaHostRemovesOpenAIPath(t *testing.T) {
	cases := map[string]string{
		"":                               "http://127.0.0.1:11434",
		"http://127.0.0.1:11434/":        "http://127.0.0.1:11434",
		"http://127.0.0.1:11434/v1":      "http://127.0.0.1:11434",
		"https://local.example.test/v1/": "https://local.example.test",
	}
	for input, want := range cases {
		if got := clawOllamaHost(input); got != want {
			t.Fatalf("clawOllamaHost(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestClawCommandEnvironmentForcesLocalOllama(t *testing.T) {
	cfg := defaultConfig()
	cfg.OllamaURL = "http://127.0.0.1:11434/v1"
	cfg.EnvironmentVars = map[string]string{
		"OPENAI_BASE_URL":      "https://example.invalid/v1",
		"OPENAI_API_KEY":       "secret-openai",
		"ANTHROPIC_API_KEY":    "secret-anthropic",
		"ANTHROPIC_AUTH_TOKEN": "secret-token",
		"KEEP_ME":              "yes",
	}
	env := clawCommandEnvironment(cfg)
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, forbidden := range []string{"OPENAI_BASE_URL=", "OPENAI_API_KEY=", "ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("Claw environment leaked provider variable %s: %s", forbidden, joined)
		}
	}
	for _, required := range []string{"\nOLLAMA_HOST=http://127.0.0.1:11434\n", "\nCLAW_OUTPUT_FORMAT=json\n", "\nKEEP_ME=yes\n"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Claw environment missing %q: %s", required, joined)
		}
	}
}

func TestBuildClawArgsUsesBoundedPermissions(t *testing.T) {
	readOnly := buildClawArgs("inspect", "qwen2.5-coder:14b", "repo-map")
	readText := strings.Join(readOnly, " ")
	if !strings.Contains(readText, "--permission-mode read-only") {
		t.Fatalf("repo map must be read-only: %v", readOnly)
	}
	write := buildClawArgs("edit", "qwen2.5-coder:14b", "edit")
	writeText := strings.Join(write, " ")
	if !strings.Contains(writeText, "--permission-mode workspace-write") {
		t.Fatalf("edit must use workspace-write: %v", write)
	}
	for _, args := range [][]string{readOnly, write} {
		text := strings.ToLower(strings.Join(args, " "))
		if strings.Contains(text, "danger-full-access") || strings.Contains(text, "skip-permissions") {
			t.Fatalf("unsafe Claw permission flag exposed: %v", args)
		}
		if !strings.Contains(text, "--output-format json") || !strings.Contains(text, " prompt ") {
			t.Fatalf("Claw invocation must be machine-readable one-shot prompt: %v", args)
		}
	}
}

func TestClawSelectedModelUsesLocalCodeModel(t *testing.T) {
	cfg := defaultConfig()
	cfg.OllamaDefaultModel = "qwen2.5-coder:7b"
	if got := clawSelectedModel(cfg, "qwen2.5-coder:14b"); got != "qwen2.5-coder:14b" {
		t.Fatalf("selected model = %q", got)
	}
	if got := clawSelectedModel(cfg, ""); got != "qwen2.5-coder:7b" {
		t.Fatalf("default model = %q", got)
	}
}

func TestFindClawExecutableDetectsStandaloneLocalSetup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("standalone claw-code-local install path is Windows-specific")
	}
	localApp := t.TempDir()
	t.Setenv("LOCALAPPDATA", localApp)
	t.Setenv("PATH", "")
	path := filepath.Join(localApp, "Programs", "ClawCode", "bin", "claw.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findClawExecutable(); got != path {
		t.Fatalf("findClawExecutable() = %q; want %q", got, path)
	}
}
