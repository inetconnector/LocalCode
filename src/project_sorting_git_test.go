// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProjectCatalogSortingByLastModifiedDescending(t *testing.T) {
	root := t.TempDir()
	p1 := filepath.Join(root, "project-old")
	p2 := filepath.Join(root, "project-new")
	p3 := filepath.Join(root, "project-pinned-old")

	for _, p := range []string{p1, p2, p3} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()
	// Set distinct mtimes
	_ = os.Chtimes(p1, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	_ = os.Chtimes(p2, now.Add(-10*time.Minute), now.Add(-10*time.Minute))
	_ = os.Chtimes(p3, now.Add(-5*time.Hour), now.Add(-5*time.Hour))

	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.PinnedProjects = []string{p3}

	projects, err := listProjects(cfg)
	if err != nil {
		t.Fatalf("listProjects failed: %v", err)
	}

	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}

	// 1st: pinned project (p3)
	if projects[0].Path != p3 || !projects[0].Pinned {
		t.Errorf("expected projects[0] to be pinned p3, got %+v", projects[0])
	}
	// 2nd: more recently modified unpinned project (p2)
	if projects[1].Path != p2 {
		t.Errorf("expected projects[1] to be p2 (newest unpinned), got %+v", projects[1])
	}
	// 3rd: older modified unpinned project (p1)
	if projects[2].Path != p1 {
		t.Errorf("expected projects[2] to be p1 (oldest unpinned), got %+v", projects[2])
	}
}

func TestGitToolCandidateDiscoveryIncludesLocalAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific tool candidate test")
	}

	cfg := defaultConfig()
	profile := toolProfile{Name: "git", DisplayName: "Git", Aliases: []string{"git", "git.exe"}}
	candidates := toolCandidatePaths(t.TempDir(), profile, cfg)

	foundLocalAppData := false
	foundMinGit := false
	for _, c := range candidates {
		lower := strings.ToLower(c.path)
		if strings.Contains(lower, "appdata") && strings.Contains(lower, "git") {
			foundLocalAppData = true
		}
		if strings.Contains(lower, "mingit") {
			foundMinGit = true
		}
	}

	if !foundLocalAppData {
		t.Errorf("expected toolCandidatePaths for git to include LocalAppData path, got: %+v", candidates)
	}
	if !foundMinGit {
		t.Errorf("expected toolCandidatePaths for git to include MinGit path, got: %+v", candidates)
	}
}

func TestHandleGitOverviewGracefulNonRepo(t *testing.T) {
	root := t.TempDir()
	nonRepoProject := filepath.Join(root, "my-empty-project")
	if err := os.MkdirAll(nonRepoProject, 0o755); err != nil {
		t.Fatal(err)
	}

	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	state.Project = nonRepoProject
	state.Config.GitEnabled = true

	// If git binary is not installed in the environment, this test checks gitAvailable branch
	if !gitAvailable(nonRepoProject, state.Config) {
		t.Skip("Git binary not available in test environment")
	}

	server := NewServer(state)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/git-overview", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for non-repo git-overview, got %d: %s", rr.Code, rr.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if isRepo, ok := res["is_repo"].(bool); !ok || isRepo {
		t.Errorf("expected is_repo to be false, got %v", res["is_repo"])
	}
}
