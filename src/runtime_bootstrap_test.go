// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComparableModelNameNormalizesProvidersAndLatest(t *testing.T) {
	cases := map[string]string{
		"qwen2.5-coder:14b":             "qwen2.5-coder:14b",
		"ollama/qwen2.5-coder:14b":      "qwen2.5-coder:14b",
		"ollama_chat/QWEN2.5-CODER:14B": "qwen2.5-coder:14b",
		"gemma3":                        "gemma3:latest",
		"hf.co/owner/model:Q5_K_M":      "hf.co/owner/model:q5_k_m",
	}
	for input, want := range cases {
		if got := comparableModelName(input); got != want {
			t.Fatalf("comparableModelName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRequiredModelsBootstrapsOnlyConfiguredDefaultOnFreshMachine(t *testing.T) {
	cfg := defaultConfig()
	cfg.LastModel = "gpt-oss:20b"
	cfg.AiderMainModel = ""
	cfg.OllamaDefaultModel = "qwen2.5-coder:14b"
	missing := requiredOllamaModels(cfg, nil)
	if len(missing) != 1 || missing[0] != "qwen2.5-coder:14b" {
		t.Fatalf("fresh-machine models = %#v", missing)
	}
}

func TestRequiredModelsIncludesMissingExplicitAiderModels(t *testing.T) {
	cfg := defaultConfig()
	cfg.LastModel = "qwen2.5-coder:14b"
	cfg.AiderMainModel = "ollama_chat/qwen2.5-coder:14b"
	cfg.AiderArchitectModel = "gpt-oss:20b"
	models := []ModelInfo{{Name: "qwen2.5-coder:14b"}}
	missing := requiredOllamaModels(cfg, models)
	if len(missing) != 1 || missing[0] != "gpt-oss:20b" {
		t.Fatalf("missing models = %#v", missing)
	}
}

func TestResolveSandboxPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable on this Windows host: %v", err)
		}
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.SandboxMode = "project"
	if _, err := resolveSandboxPath(cfg, root, filepath.Join("linked", "new.txt")); err == nil {
		t.Fatal("sandbox accepted a path through a symlink/junction outside the project")
	}
}

func TestPublicOnlyDialContextRejectsLoopbackLiteral(t *testing.T) {
	_, err := publicOnlyDialContext(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", "80"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "blocked") {
		t.Fatalf("loopback dial was not blocked: %v", err)
	}
}

func TestPortableDirectoryOverridesAreCrossPlatform(t *testing.T) {
	base := t.TempDir()
	configHome := filepath.Join(base, "config")
	cacheHome := filepath.Join(base, "cache")
	userHome := filepath.Join(base, "home")
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("LOCALCODE_CACHE_HOME", cacheHome)
	t.Setenv("LOCALCODE_USER_HOME", userHome)
	if got := userConfigBaseDir(); got != configHome {
		t.Fatalf("config home = %q", got)
	}
	if got := userCacheBaseDir(); got != cacheHome {
		t.Fatalf("cache home = %q", got)
	}
	if got := userProfileDir(); got != userHome {
		t.Fatalf("user home = %q", got)
	}
}

func TestEnsureConfiguredModelsPullsAndRefreshes(t *testing.T) {
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousLogOutput) })

	installed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pull":
			var request ollamaPullRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Model != defaultCodingModel || request.Stream {
				t.Fatalf("unexpected pull request: %#v", request)
			}
			installed = true
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		case "/api/tags":
			models := []map[string]any{}
			if installed {
				models = append(models, map[string]any{"name": defaultCodingModel, "size": 1})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := defaultConfig()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	models, details, err := ensureConfiguredModels(context.Background(), cfg, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !installed || len(models) != 1 || models[0].Name != defaultCodingModel {
		t.Fatalf("model was not installed and refreshed: installed=%v models=%#v", installed, models)
	}
	if len(details) != 1 || !strings.Contains(details[0], defaultCodingModel) {
		t.Fatalf("missing installation detail: %#v", details)
	}
}

func TestSchemaEightEnablesRuntimeAutoSetupDuringMigration(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 6})
	if cfg.SchemaVersion != 9 || !cfg.OllamaAutoInstall || !cfg.OllamaAutoPull || !cfg.AiderAutoInstall {
		t.Fatalf("runtime bootstrap migration incomplete: %#v", cfg)
	}
	if cfg.OllamaDefaultModel != defaultCodingModel {
		t.Fatalf("default model = %q", cfg.OllamaDefaultModel)
	}
}
