// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolatedAiderConfig(t *testing.T, project string) Config {
	t.Helper()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("LOCALCODE_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.CreateProjectDocs = false
	cfg.AiderExecutable = fakeAiderExecutable(t, project)
	cfg.AiderEnabled = true
	cfg.AiderAutoInstall = false
	cfg.AiderUseGit = false
	cfg.AiderAutoLint = false
	cfg.AiderAutoTest = false
	cfg.AiderMainModel = "test-model"
	cfg.CommandTimeout = 60
	return cfg
}

func TestAiderUtilityAndActionDispatchUseControlledExecutable(t *testing.T) {
	project := t.TempDir()
	appPath := filepath.Join(project, "app.go")
	original := "package main\nfunc main() {}\n"
	if err := os.WriteFile(appPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := isolatedAiderConfig(t, project)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repoMap, err := runAiderUtility(ctx, project, "repo-map", "test-model", "utility-thread", cfg)
	if err != nil || !strings.Contains(repoMap, "Repository map") {
		t.Fatalf("repo-map output=%q err=%v", repoMap, err)
	}
	if _, err := buildAiderUtilityArgs(project, "unsupported", "test-model", "thread", cfg); err == nil {
		t.Fatal("unsupported Aider utility mode must fail")
	}

	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Project = project
	state.Model = "test-model"
	state.CurrentThread = "thread-1"
	state.mu.Unlock()

	lintOutput, err := state.executeAiderAction(ctx, project, cfg, AgentAction{Action: "aider_lint", Message: "lint"})
	if err != nil || !strings.Contains(lintOutput, "Backup:") || !strings.Contains(lintOutput, "app.go") {
		t.Fatalf("aider_lint output=%q err=%v", lintOutput, err)
	}
	state.mu.RLock()
	backup := state.LastAiderBackup
	state.mu.RUnlock()
	if backup == "" {
		t.Fatal("aider_lint did not publish latest backup")
	}
	if _, err := os.Stat(filepath.Join(backup, "LOCALCODE-AIDER-MANIFEST.json")); err != nil {
		t.Fatalf("Aider utility backup manifest missing: %v", err)
	}

	undoOutput, err := state.executeAiderAction(ctx, project, cfg, AgentAction{Action: "aider_undo", Message: "undo"})
	if err != nil || !strings.Contains(undoOutput, "Restored files") {
		t.Fatalf("aider_undo output=%q err=%v", undoOutput, err)
	}
	data, err := os.ReadFile(appPath)
	if err != nil || string(data) != original {
		t.Fatalf("undo content=%q err=%v", data, err)
	}

	mapOutput, err := state.executeAiderAction(ctx, project, cfg, AgentAction{Action: "aider_repo_map", Message: "map"})
	if err != nil || !strings.Contains(mapOutput, "Repository map") {
		t.Fatalf("dispatch repo-map output=%q err=%v", mapOutput, err)
	}
	if _, err := state.executeAiderAction(ctx, project, cfg, AgentAction{Action: "aider_unknown", Message: "bad"}); err == nil {
		t.Fatal("unsupported Aider action must fail")
	}
}

func TestAiderHTTPHandlersUseConfiguredFakeExecutable(t *testing.T) {
	project := t.TempDir()
	appPath := filepath.Join(project, "app.go")
	original := "package main\nfunc main() {}\n"
	if err := os.WriteFile(appPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := isolatedAiderConfig(t, project)
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	state.mu.Lock()
	state.Project = project
	state.Model = "test-model"
	state.CurrentThread = "http-thread"
	state.mu.Unlock()
	server := NewServer(state)

	status := serveHTTP(server, http.MethodGet, "/api/aider/status", "", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"installed":true`) {
		t.Fatalf("Aider status=%d body=%s", status.Code, status.Body.String())
	}
	setup := serveHTTP(server, http.MethodPost, "/api/aider/setup", `{"action":"test"}`, "")
	if setup.Code != http.StatusOK || !strings.Contains(setup.Body.String(), "Repository map") {
		t.Fatalf("Aider setup test=%d body=%s", setup.Code, setup.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	editOutput, err := state.executeAiderAction(ctx, project, cfg, AgentAction{Action: "aider_edit", Task: "edit app.go", Message: "edit"})
	if err != nil || !strings.Contains(editOutput, "app.go") {
		t.Fatalf("Aider edit output=%q err=%v", editOutput, err)
	}
	undo := serveHTTP(server, http.MethodPost, "/api/aider/undo", `{}`, "")
	if undo.Code != http.StatusOK || !strings.Contains(undo.Body.String(), "Restored files") {
		t.Fatalf("Aider undo=%d body=%s", undo.Code, undo.Body.String())
	}
	data, err := os.ReadFile(appPath)
	if err != nil || string(data) != original {
		t.Fatalf("HTTP undo content=%q err=%v", data, err)
	}
}
