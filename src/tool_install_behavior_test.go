// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func isolateToolInstallGlobals(t *testing.T) {
	t.Helper()
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("LOCALCODE_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	oldClient := toolHTTPClient
	oldAndroid := androidPlatformToolsURL
	oldGitRelease := minGitReleaseURL
	oldNodeIndex := nodeIndexURL
	oldNodeBase := nodeDistBaseURL
	oldGHRelease := githubCLIReleaseURL
	oldDotnet := dotnetInstallURL
	oldVS := vsBuildToolsURL
	oldGOOS := toolRuntimeGOOS
	t.Cleanup(func() {
		toolHTTPClient = oldClient
		androidPlatformToolsURL = oldAndroid
		minGitReleaseURL = oldGitRelease
		nodeIndexURL = oldNodeIndex
		nodeDistBaseURL = oldNodeBase
		githubCLIReleaseURL = oldGHRelease
		dotnetInstallURL = oldDotnet
		vsBuildToolsURL = oldVS
		toolRuntimeGOOS = oldGOOS
	})
	toolRuntimeGOOS = "windows"
}

func TestManagedPortableToolInstallersUseVerifiedLocalFixtures(t *testing.T) {
	isolateToolInstallGlobals(t)

	androidZip := zipFixture(t, map[string]string{
		"platform-tools/adb.exe":      "adb",
		"platform-tools/fastboot.exe": "fastboot",
	})
	minGitZip := zipFixture(t, map[string]string{
		"cmd/git.exe": "git",
	})
	nodeZip := zipFixture(t, map[string]string{
		"node-v22.1.0-win-x64/node.exe": "node",
		"node-v22.1.0-win-x64/npm.cmd":  "npm",
		"node-v22.1.0-win-x64/npx.cmd":  "npx",
	})
	ghZip := zipFixture(t, map[string]string{
		"gh_9.9.9_windows_amd64/bin/gh.exe": "gh",
	})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/platform.zip":
			_, _ = w.Write(androidZip)
		case "/mingit-release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]any{
				{"name": "MinGit-2.0-64-bit.zip", "browser_download_url": server.URL + "/mingit.zip"},
				{"name": "MinGit-busybox-64-bit.zip", "browser_download_url": server.URL + "/ignored.zip"},
			}})
		case "/mingit.zip":
			_, _ = w.Write(minGitZip)
		case "/node-index":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"version": "v23.0.0", "lts": false, "files": []string{"win-x64-zip"}},
				{"version": "v22.1.0", "lts": "Jod", "files": []string{"win-x64-zip"}},
			})
		case "/dist/v22.1.0/node-v22.1.0-win-x64.zip":
			_, _ = w.Write(nodeZip)
		case "/gh-release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]any{
				{"name": "gh_9.9.9_linux_amd64.tar.gz", "browser_download_url": server.URL + "/wrong"},
				{"name": "gh_9.9.9_windows_amd64.zip", "browser_download_url": server.URL + "/gh.zip"},
			}})
		case "/gh.zip":
			_, _ = w.Write(ghZip)
		case "/download.txt":
			w.Header().Set("X-Seen-User-Agent", r.Header.Get("User-Agent"))
			_, _ = io.WriteString(w, "downloaded")
		case "/fail":
			http.Error(w, "fixture failure", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	toolHTTPClient = func(time.Duration) *http.Client { return server.Client() }
	androidPlatformToolsURL = server.URL + "/platform.zip"
	minGitReleaseURL = server.URL + "/mingit-release"
	nodeIndexURL = server.URL + "/node-index"
	nodeDistBaseURL = server.URL + "/dist"
	githubCLIReleaseURL = server.URL + "/gh-release"

	ctx := context.Background()
	root, output, err := installAndroidPlatformTools(ctx)
	if err != nil || !strings.Contains(output, "Google") {
		t.Fatalf("Android installer root=%q output=%q err=%v", root, output, err)
	}
	for _, name := range []string{"adb.exe", "fastboot.exe"} {
		if st, err := os.Stat(filepath.Join(root, "platform-tools", name)); err != nil || st.IsDir() {
			t.Fatalf("Android fixture missing %s: %v", name, err)
		}
	}

	gitPath, output, err := installPortableGit(ctx)
	if err != nil || !strings.HasSuffix(strings.ToLower(gitPath), filepath.Join("cmd", "git.exe")) || !strings.Contains(output, "Git-for-Windows") {
		t.Fatalf("MinGit path=%q output=%q err=%v", gitPath, output, err)
	}

	nodeRoot, output, err := installPortableNode(ctx)
	if err != nil || !strings.Contains(output, "Node.js LTS") {
		t.Fatalf("Node root=%q output=%q err=%v", nodeRoot, output, err)
	}
	for _, name := range []string{"node.exe", "npm.cmd", "npx.cmd"} {
		if st, err := os.Stat(filepath.Join(nodeRoot, name)); err != nil || st.IsDir() {
			t.Fatalf("Node fixture missing %s: %v", name, err)
		}
	}

	ghPath, output, err := installPortableGitHubCLI(ctx)
	if err != nil || !strings.EqualFold(filepath.Base(ghPath), "gh.exe") || !strings.Contains(output, "GitHub release") {
		t.Fatalf("GitHub CLI path=%q output=%q err=%v", ghPath, output, err)
	}

	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = true
	cfg.ToolOverrides = map[string]string{}
	updated, output, err := installKnownTool(ctx, t.TempDir(), "adb", cfg)
	if err != nil || updated.ToolOverrides["adb"] == "" || !strings.Contains(output, "ADB") {
		t.Fatalf("installKnownTool adb overrides=%#v output=%q err=%v", updated.ToolOverrides, output, err)
	}
}

func TestManagedToolInstallerFailureAndDownloadBranches(t *testing.T) {
	isolateToolInstallGlobals(t)
	seenUserAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			seenUserAgent = r.Header.Get("User-Agent")
			_, _ = io.WriteString(w, "payload")
			return
		}
		http.Error(w, "fixture failure", http.StatusBadGateway)
	}))
	defer server.Close()
	toolHTTPClient = func(time.Duration) *http.Client { return server.Client() }

	target := filepath.Join(t.TempDir(), "nested", "payload.bin")
	if err := downloadToFile(context.Background(), server.URL+"/ok", target); err != nil {
		t.Fatal(err)
	}
	if seenUserAgent != localCodeUserAgent()+" tool-installer" {
		t.Fatalf("installer User-Agent=%q want %q", seenUserAgent, localCodeUserAgent()+" tool-installer")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "payload" {
		t.Fatalf("downloaded data=%q err=%v", data, err)
	}
	if err := downloadToFile(context.Background(), server.URL+"/fail", target); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected HTTP failure, got %v", err)
	}

	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	if _, _, err := installKnownTool(context.Background(), t.TempDir(), "adb", cfg); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("downloads-disabled install error=%v", err)
	}
	cfg.SetupDownloadsEnabled = true
	oldGOOS := toolRuntimeGOOS
	toolRuntimeGOOS = "linux"
	if _, _, err := installKnownTool(context.Background(), t.TempDir(), "adb", cfg); err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("non-Windows install error=%v", err)
	}
	toolRuntimeGOOS = oldGOOS
	if _, _, err := installKnownTool(context.Background(), t.TempDir(), "unknown-tool", cfg); err == nil || !strings.Contains(err.Error(), "no automatic installer") {
		t.Fatalf("unsupported installer error=%v", err)
	}

	dotnetInstallURL = server.URL + "/fail"
	if _, output, err := installDotnetSDK(context.Background(), t.TempDir()); err == nil || !strings.Contains(output, "Download") {
		t.Fatalf("dotnet failure output=%q err=%v", output, err)
	}
	vsBuildToolsURL = server.URL + "/fail"
	if _, output, err := installVisualStudioBuildTools(context.Background()); err == nil || !strings.Contains(output, "Download") {
		t.Fatalf("VS failure output=%q err=%v", output, err)
	}

	t.Setenv("PATH", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("ProgramFiles", t.TempDir())
	if got := findWingetExecutable(); got != "" {
		t.Fatalf("winget unexpectedly found: %q", got)
	}
	if _, err := installWithWinget(context.Background(), "Example.Package"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "winget") {
		t.Fatalf("winget missing error=%v", err)
	}
}
