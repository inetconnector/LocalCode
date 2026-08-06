// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	if cfg.SchemaVersion != 10 || !cfg.OllamaAutoInstall || !cfg.OllamaAutoPull || !cfg.AiderAutoInstall {
		t.Fatalf("runtime bootstrap migration incomplete: %#v", cfg)
	}
	if cfg.OllamaDefaultModel != defaultCodingModel {
		t.Fatalf("default model = %q", cfg.OllamaDefaultModel)
	}
}

func TestRequiredModelsOnlyIncludesSelectedEngineModels(t *testing.T) {
	installed := []ModelInfo{{Name: "qwen2.5-coder:14b"}}

	claude := defaultConfig()
	claude.EditingEngine = editingEngineClaude
	claude.LastModel = "qwen2.5-coder:14b"
	claude.AiderArchitectModel = "large-aider-only:latest"
	claude.OpenCodeModel = "ollama/large-opencode-only:latest"
	if missing := requiredOllamaModels(claude, installed); len(missing) != 0 {
		t.Fatalf("inactive engine models must not be downloaded: %#v", missing)
	}

	opencode := claude
	opencode.EditingEngine = editingEngineOpenCode
	if missing := requiredOllamaModels(opencode, installed); len(missing) != 1 || missing[0] != "large-opencode-only:latest" {
		t.Fatalf("selected OpenCode Ollama model missing=%#v", missing)
	}

	aider := claude
	aider.EditingEngine = editingEngineAider
	aider.AiderMainModel = "ollama_chat/qwen2.5-coder:14b"
	if missing := requiredOllamaModels(aider, installed); len(missing) != 1 || missing[0] != "large-aider-only:latest" {
		t.Fatalf("selected Aider models missing=%#v", missing)
	}
}

func TestDisabledSelectedEngineFallsBackToNative(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "de"
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeEnabled = false
	var progress []BootstrapProgress
	updated, detail, err := ensureSelectedEditingEngineRuntimeWithProgress(context.Background(), cfg, func(p BootstrapProgress) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.EditingEngine != editingEngineNative {
		t.Fatalf("engine=%q", updated.EditingEngine)
	}
	if !strings.Contains(detail, "native Engine") {
		t.Fatalf("detail=%q", detail)
	}
	if len(progress) < 2 || progress[len(progress)-1].Percent != 94 {
		t.Fatalf("progress=%#v", progress)
	}
}

func TestBootstrapProgressClampsAndLocalizes(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "de"
	var got []BootstrapProgress
	reporter := func(p BootstrapProgress) { got = append(got, p) }
	reportBootstrap(cfg, reporter, -7, "Deutsch", "English", " detail ")
	reportBootstrap(cfg, reporter, 107, "Fertig", "Done", "")
	reportBootstrap(cfg, nil, 50, "ignoriert", "ignored", "")
	if len(got) != 2 || got[0].Percent != 0 || got[0].Stage != "Deutsch" || got[0].Detail != "detail" || got[1].Percent != 100 {
		t.Fatalf("progress=%#v", got)
	}
}

func TestGermanSetupDownloadErrorsAreLocalized(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "de"
	cfg.OllamaAutoPull = true
	cfg.SetupDownloadsEnabled = false
	_, _, err := ensureConfiguredModels(context.Background(), cfg, NewOllamaClient(), nil)
	if err == nil || !strings.Contains(err.Error(), "Downloads für die automatische Einrichtung") {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeOllamaBaseURLPreservesSecureSchemeAndIPv6(t *testing.T) {
	cases := map[string]string{
		"localhost":                   "http://localhost:11434",
		"https://ollama.example":      "https://ollama.example:11434",
		"http://[::1]:11435/api/tags": "http://[::1]:11435",
		"http://0.0.0.0":              "http://127.0.0.1:11434",
		"ftp://ollama.example:11434":  "",
		"http://":                     "",
		"   ":                         "",
	}
	for input, want := range cases {
		if got := normalizeOllamaBaseURL(input); got != want {
			t.Errorf("normalizeOllamaBaseURL(%q)=%q want %q", input, got, want)
		}
	}
}

func TestPlatformHelpersAndEnvironmentBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.EnvironmentVars = map[string]string{"LOCALCODE_TEST_VALUE": "ok", "   ": "ignored"}
	env := commandEnvironment(cfg)
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "LOCALCODE_TEST_VALUE=ok") || strings.Contains(joined, "=ignored") {
		t.Fatalf("environment=%q", joined)
	}
	if runtime.GOOS != "windows" {
		showFatal("LocalCode test", "expected console fallback")
		if detail := androidHostDeviceDiagnostic(); detail != "" {
			t.Fatalf("unexpected host diagnostic=%q", detail)
		}
		if _, err := installOllama(context.Background(), nil); err == nil {
			t.Fatal("non-Windows automatic Ollama installation must fail explicitly")
		}
	}
}

func TestRequiredModelsFallbackAndEmptyModelChecks(t *testing.T) {
	if modelInstalled([]ModelInfo{{Name: "qwen:latest"}}, "") {
		t.Fatal("empty model name must never be installed")
	}
	cfg := Config{EditingEngine: editingEngineNative}
	missing := requiredOllamaModels(cfg, nil)
	if len(missing) != 1 || missing[0] != defaultCodingModel {
		t.Fatalf("fallback models=%#v", missing)
	}
}

func TestNativeEngineAndAlreadyInstalledModelProgress(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	cfg.EditingEngine = editingEngineNative
	var progress []BootstrapProgress
	updated, detail, err := ensureSelectedEditingEngineRuntimeWithProgress(context.Background(), cfg, func(p BootstrapProgress) {
		progress = append(progress, p)
	})
	if err != nil || updated.EditingEngine != editingEngineNative || !strings.Contains(detail, "native editing engine") {
		t.Fatalf("updated=%+v detail=%q err=%v", updated, detail, err)
	}
	if len(progress) < 2 || progress[len(progress)-1].Percent != 94 {
		t.Fatalf("engine progress=%#v", progress)
	}

	models := []ModelInfo{{Name: cfg.OllamaDefaultModel}}
	progress = nil
	got, details, err := ensureConfiguredModelsWithProgress(context.Background(), cfg, NewOllamaClient(), models, func(p BootstrapProgress) {
		progress = append(progress, p)
	})
	if err != nil || len(details) != 0 || len(got) != 1 {
		t.Fatalf("models=%#v details=%#v err=%v", got, details, err)
	}
	if len(progress) < 2 || progress[len(progress)-1].Percent != 68 {
		t.Fatalf("model progress=%#v", progress)
	}
}

func TestAutomaticModelPullCanBeExplicitlyDisabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	cfg.OllamaAutoPull = false
	_, _, err := ensureConfiguredModels(context.Background(), cfg, NewOllamaClient(), nil)
	if err == nil || !strings.Contains(err.Error(), "automatic model download is disabled") {
		t.Fatalf("err=%v", err)
	}
}

func TestOllamaInstallerDownloadLimitCoversCurrentLargeWindowsInstaller(t *testing.T) {
	const observedInstallerSize = int64(1563278432)
	if ollamaInstallerMaxBytes <= observedInstallerSize {
		t.Fatalf("Ollama installer limit %d does not cover observed installer size %d", ollamaInstallerMaxBytes, observedInstallerSize)
	}
	if ollamaInstallerMaxBytes < 2<<30 {
		t.Fatalf("Ollama installer limit is unexpectedly small: %d", ollamaInstallerMaxBytes)
	}
}

func TestDownloadFileReportsProgressAndCleansPartFile(t *testing.T) {
	payload := strings.Repeat("0123456789abcdef", 8192)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "download.bin")
	var calls int
	var finalWritten, finalTotal int64
	err := downloadFileWithProgress(context.Background(), server.URL, target, int64(len(payload))+1, func(written, total int64) {
		calls++
		finalWritten = written
		finalTotal = total
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 || finalWritten != int64(len(payload)) || finalTotal != int64(len(payload)) {
		t.Fatalf("progress calls=%d written=%d total=%d", calls, finalWritten, finalTotal)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != payload {
		t.Fatalf("downloaded payload mismatch: len=%d err=%v", len(data), err)
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
}

func TestDownloadFileRejectsNonPositiveLimit(t *testing.T) {
	target := filepath.Join(t.TempDir(), "download.bin")
	if err := downloadFileWithProgress(context.Background(), "https://example.invalid/file", target, 0, nil); err == nil {
		t.Fatal("expected non-positive size limit to be rejected before network access")
	}
}

func TestFormatDownloadAmount(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		1024:    "1.0 KiB",
		1 << 20: "1.0 MiB",
		1 << 30: "1.00 GiB",
	}
	for input, want := range cases {
		if got := formatDownloadAmount(input); got != want {
			t.Fatalf("formatDownloadAmount(%d)=%q want %q", input, got, want)
		}
	}
}

func TestReportOllamaInstallProgressCoversAllPhases(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "de"
	var got []BootstrapProgress
	reporter := func(progress BootstrapProgress) {
		got = append(got, progress)
	}
	reportOllamaInstallProgress(cfg, reporter, "download", 50, 100)
	reportOllamaInstallProgress(cfg, reporter, "download", 200, 100)
	reportOllamaInstallProgress(cfg, reporter, "verify", 0, 0)
	reportOllamaInstallProgress(cfg, reporter, "install", 0, 0)
	reportOllamaInstallProgress(cfg, reporter, "locate", 0, 0)
	reportOllamaInstallProgress(cfg, reporter, "unknown", 0, 0)
	if len(got) != 5 {
		t.Fatalf("progress events=%#v", got)
	}
	if got[0].Percent != 15 || got[1].Percent != 17 {
		t.Fatalf("download progress=%#v", got[:2])
	}
	if got[2].Percent != 18 || got[3].Percent != 19 || got[4].Percent != 20 {
		t.Fatalf("phase progress=%#v", got[2:])
	}
	if !strings.Contains(got[3].Detail, "offizielle Installer") {
		t.Fatalf("localized install detail=%q", got[3].Detail)
	}
}

func TestLoadConfigAcceptsUTF8BOM(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", base)
	t.Setenv("LOCALCODE_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("LOCALCODE_USER_HOME", filepath.Join(base, "home"))
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"schema_version":10,"port":33333,"language":"de"}`)...)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig()
	if cfg.Port != 33333 || cfg.Language != "de" {
		t.Fatalf("config with BOM was not loaded: %+v", cfg)
	}
}
