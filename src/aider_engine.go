// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const aiderPinnedVersion = "0.86.2"
const uvPinnedVersion = "0.11.16"
const uvWindowsDownloadURL = "https://releases.astral.sh/github/uv/releases/download/" + uvPinnedVersion + "/uv-x86_64-pc-windows-msvc.zip"
const uvWindowsSHA256 = "dd9d6d6554bfab265bfa98aa8e8a406c5c3a7b97582f93de1f4d48d9154a0395"
const ollamaInstallerDownloadURL = "https://ollama.com/download/OllamaSetup.exe"
const ollamaInstallerMaxBytes int64 = 4 << 30

type AiderStatus struct {
	Enabled          bool   `json:"enabled"`
	Installed        bool   `json:"installed"`
	Executable       string `json:"executable,omitempty"`
	Version          string `json:"version,omitempty"`
	ExpectedVersion  string `json:"expected_version"`
	UVExecutable     string `json:"uv_executable,omitempty"`
	InstallationRoot string `json:"installation_root"`
	Error            string `json:"error,omitempty"`
}

type AiderRunResult struct {
	Output       string
	ChangedFiles []string
	BackupDir    string
	Executable   string
	Arguments    []string
	Duration     time.Duration
	ExitCode     int
}

type AiderNotInstalledError struct {
	Status AiderStatus
}

func (e *AiderNotInstalledError) Error() string {
	if e.Status.Error != "" {
		return "Aider ist nicht einsatzbereit: " + e.Status.Error
	}
	return "Aider ist nicht installiert"
}

func aiderToolRoot() string {
	return filepath.Join(appDataDir(), "tools", "aider")
}

func aiderBinDir() string {
	return filepath.Join(aiderToolRoot(), "bin")
}

func aiderHistoryDir() string {
	return filepath.Join(appDataDir(), "aider-history")
}

func aiderExecutableName() string {
	if runtime.GOOS == "windows" {
		return "aider.exe"
	}
	return "aider"
}

func uvExecutableName() string {
	if runtime.GOOS == "windows" {
		return "uv.exe"
	}
	return "uv"
}

func executableVersion(ctx context.Context, path string, env []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty executable path")
	}
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = env
	hideCommandWindow(cmd)
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String() + "\n" + stderr.String()), err
	}
	return strings.TrimSpace(stdout.String() + "\n" + stderr.String()), nil
}

func findUVExecutable(cfg Config) string {
	candidates := []string{
		filepath.Join(appDataDir(), "tools", "uv", uvExecutableName()),
		filepath.Join(aiderToolRoot(), "uv", uvExecutableName()),
	}
	if p, err := exec.LookPath(uvExecutableName()); err == nil {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin", "uv.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "uv", "uv.exe"),
		)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func findAiderExecutable(cfg Config) string {
	candidates := []string{}
	if strings.TrimSpace(cfg.AiderExecutable) != "" {
		candidates = append(candidates, cfg.AiderExecutable)
	}
	candidates = append(candidates,
		filepath.Join(aiderBinDir(), aiderExecutableName()),
		filepath.Join(aiderToolRoot(), "tool-bin", aiderExecutableName()),
	)
	if p, err := exec.LookPath(aiderExecutableName()); err == nil {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin", "aider.exe"),
			filepath.Join(os.Getenv("APPDATA"), "Python", "Scripts", "aider.exe"),
		)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func aiderStatus(ctx context.Context, cfg Config) AiderStatus {
	status := AiderStatus{
		Enabled:          cfg.AiderEnabled,
		ExpectedVersion:  strings.TrimSpace(cfg.AiderVersion),
		InstallationRoot: aiderToolRoot(),
		UVExecutable:     findUVExecutable(cfg),
	}
	if status.ExpectedVersion == "" {
		status.ExpectedVersion = aiderPinnedVersion
	}
	status.Executable = findAiderExecutable(cfg)
	if status.Executable == "" {
		status.Error = "Aider executable not found"
		return status
	}
	versionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	version, err := executableVersion(versionCtx, status.Executable, commandEnvironment(cfg))
	if err != nil {
		status.Error = strings.TrimSpace(version + "\n" + err.Error())
		return status
	}
	status.Version = version
	status.Installed = strings.Contains(version, status.ExpectedVersion)
	if !status.Installed {
		status.Error = fmt.Sprintf("installed version does not match pinned version %s: %s", status.ExpectedVersion, version)
	}
	return status
}

type downloadProgressFunc func(written, total int64)
type ollamaInstallProgressFunc func(phase string, written, total int64)

type downloadProgressWriter struct {
	written  int64
	total    int64
	lastEmit time.Time
	callback downloadProgressFunc
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.written += int64(n)
	if w.callback != nil && (w.lastEmit.IsZero() || time.Since(w.lastEmit) >= 250*time.Millisecond || (w.total > 0 && w.written >= w.total)) {
		w.lastEmit = time.Now()
		w.callback(w.written, w.total)
	}
	return n, nil
}

func downloadFile(ctx context.Context, url, target string, maxBytes int64) error {
	return downloadFileWithProgress(ctx, url, target, maxBytes, nil)
}

func downloadFileWithProgress(ctx context.Context, url, target string, maxBytes int64, progress downloadProgressFunc) error {
	if maxBytes <= 0 {
		return fmt.Errorf("download size limit must be positive")
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("default HTTP transport has unexpected type %T", http.DefaultTransport)
	}
	transport := baseTransport.Clone()
	transport.ResponseHeaderTimeout = 45 * time.Second
	client := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "LocalCode/6.4.1 dependency installer")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes && resp.ContentLength >= 0 {
		return fmt.Errorf("download exceeds safe size limit: %d bytes received in headers, maximum is %d bytes", resp.ContentLength, maxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	part := target + ".part"
	if err := os.Remove(part); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if progress != nil {
		progress(0, resp.ContentLength)
	}
	counter := &downloadProgressWriter{total: resp.ContentLength, callback: progress}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	written, copyErr := io.Copy(out, io.TeeReader(limited, counter))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(part)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(part)
		return closeErr
	}
	if written > maxBytes {
		_ = os.Remove(part)
		return fmt.Errorf("download exceeds safe size limit of %d bytes", maxBytes)
	}
	if progress != nil {
		progress(written, resp.ContentLength)
	}
	// The destination is a managed download cache. Removing a stale target
	// first keeps repeated installation attempts reliable on Windows, where
	// os.Rename does not replace an existing file.
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(part)
		return err
	}
	if err := os.Rename(part, target); err != nil {
		_ = os.Remove(part)
		return err
	}
	return nil
}

func verifyFileSHA256(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", filepath.Base(path), expected, actual)
	}
	return nil
}

func extractZipFile(zipPath, destination string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, file := range reader.File {
		clean := filepath.Clean(file.Name)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe zip entry: %s", file.Name)
		}
		target := filepath.Join(destination, clean)
		if !pathWithin(destination, target) {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
	}
	return nil
}

func ensureUVInstalled(ctx context.Context, cfg Config) (string, string, error) {
	if path := findUVExecutable(cfg); path != "" {
		version, err := executableVersion(ctx, path, commandEnvironment(cfg))
		return path, version, err
	}
	if runtime.GOOS != "windows" {
		return "", "", errors.New("uv is not installed; install uv and retry")
	}
	uvDir := filepath.Join(appDataDir(), "tools", "uv")
	zipPath := filepath.Join(uvDir, "uv-windows.zip")
	if err := downloadFile(ctx, uvWindowsDownloadURL, zipPath, 100<<20); err != nil {
		return "", "", err
	}
	if err := verifyFileSHA256(zipPath, uvWindowsSHA256); err != nil {
		_ = os.Remove(zipPath)
		return "", "", err
	}
	extractDir := filepath.Join(uvDir, "extract")
	_ = os.RemoveAll(extractDir)
	if err := extractZipFile(zipPath, extractDir); err != nil {
		return "", "", err
	}
	var found string
	_ = filepath.WalkDir(extractDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() && strings.EqualFold(d.Name(), "uv.exe") {
			found = path
			return io.EOF
		}
		return nil
	})
	if found == "" {
		return "", "", errors.New("downloaded uv archive did not contain uv.exe")
	}
	target := filepath.Join(uvDir, "uv.exe")
	data, err := os.ReadFile(found)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return "", "", err
	}
	version, err := executableVersion(ctx, target, commandEnvironment(cfg))
	return target, version, err
}

func runCapturedCommand(ctx context.Context, executable string, args, env []string, dir string) (string, int, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(executable))
		if ext == ".cmd" || ext == ".bat" {
			cmd = exec.Command("cmd.exe", "/D", "/S", "/C", buildWindowsCommandLine(executable, args))
		} else {
			cmd = exec.Command(executable, args...)
		}
	} else {
		cmd = exec.Command(executable, args...)
	}
	cmd.Dir = dir
	cmd.Env = env
	hideCommandWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", -1, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		// Do not use exec.CommandContext here: on Windows it can terminate the
		// wrapper process before taskkill /T observes its child tree, leaving a
		// ping/python/aider child alive with open handles. Kill the complete tree
		// first and then wait for Cmd.Wait to reap it.
		killProcessTree(cmd)
		select {
		case <-done:
			err = ctx.Err()
		case <-time.After(4 * time.Second):
			return "", -1, fmt.Errorf("%w: subprocess did not terminate within the cancellation grace period", ctx.Err())
		}
	}
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString("STDOUT:\n" + stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n" + stderr.String())
	}
	return strings.TrimSpace(result.String()), exitCode, err
}

func installAider(ctx context.Context, cfg Config) (AiderStatus, string, error) {
	if !cfg.SetupDownloadsEnabled {
		return aiderStatus(ctx, cfg), "", errors.New("downloads for automatic setup are disabled")
	}
	uv, uvVersion, err := ensureUVInstalled(ctx, cfg)
	if err != nil {
		return aiderStatus(ctx, cfg), uvVersion, err
	}
	version := strings.TrimSpace(cfg.AiderVersion)
	if version == "" {
		version = aiderPinnedVersion
	}
	if err := os.MkdirAll(aiderBinDir(), 0o755); err != nil {
		return aiderStatus(ctx, cfg), "", err
	}
	toolDir := filepath.Join(aiderToolRoot(), "uv-tools")
	env := append(commandEnvironment(cfg),
		"UV_TOOL_DIR="+toolDir,
		"UV_TOOL_BIN_DIR="+aiderBinDir(),
		"UV_PYTHON_INSTALL_DIR="+filepath.Join(aiderToolRoot(), "python"),
		"UV_CACHE_DIR="+filepath.Join(aiderToolRoot(), "uv-cache"),
		"UV_NO_CONFIG=1",
		"UV_NO_SYSTEM_CONFIG=1",
		"UV_NO_ENV_FILE=1",
	)
	args := []string{"tool", "install", "--force", "--python", "3.12", "--exclude-newer", "2026-02-13", "aider-chat==" + version}
	output, exitCode, runErr := runCapturedCommand(ctx, uv, args, env, aiderToolRoot())
	if runErr != nil {
		return aiderStatus(ctx, cfg), output, fmt.Errorf("uv tool install failed with exit code %d: %w", exitCode, runErr)
	}
	status := aiderStatus(ctx, cfg)
	if !status.Installed {
		return status, output, errors.New("Aider installation could not be verified")
	}
	return status, strings.TrimSpace("uv: " + uvVersion + "\n\n" + output), nil
}

func aiderModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "ollama_chat/") {
		return model
	}
	if strings.HasPrefix(lower, "ollama/") {
		return "ollama_chat/" + strings.TrimSpace(model[len("ollama/"):])
	}
	// Ollama model names may themselves contain slashes, for example
	// hf.co/owner/model:quant. A slash therefore does not mean that a
	// LiteLLM provider prefix is already present. LocalCode's Aider engine is
	// deliberately bound to the configured local Ollama endpoint, so every
	// unqualified model is routed through Aider's ollama_chat provider.
	return "ollama_chat/" + model
}

func safeThreadFileName(threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		threadID = "default"
	}
	var b strings.Builder
	for _, r := range threadID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func ensureAiderIgnore(project string) (string, error) {
	cacheDir := filepath.Join(appDataDir(), "aider-config")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(project))))
	path := filepath.Join(cacheDir, hex.EncodeToString(sum[:8])+".aiderignore")
	content := `.git/
.vs/
.idea/
.vscode/
.gradle/
node_modules/
vendor/
bin/
obj/
build/
dist/
target/
coverage/
.localcode/
*.exe
*.dll
*.so
*.dylib
*.class
*.jar
*.apk
*.aab
*.zip
*.7z
*.rar
*.png
*.jpg
*.jpeg
*.gif
*.webp
*.pdf
*.mp4
*.mp3
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

type fileScore struct {
	Path  string
	Score int
}

func taskKeywords(task string) []string {
	words := strings.FieldsFunc(strings.ToLower(task), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-'
	})
	stop := map[string]bool{"der": true, "die": true, "das": true, "und": true, "oder": true, "ein": true, "eine": true, "einen": true, "mit": true, "für": true, "von": true, "im": true, "in": true, "zu": true, "auf": true, "the": true, "and": true, "or": true, "a": true, "an": true, "to": true, "of": true, "with": true, "for": true, "please": true, "bitte": true, "mach": true, "mache": true, "fix": true, "fixe": true}
	seen := map[string]bool{}
	out := []string{}
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) < 3 || stop[word] || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

func relevantFilesForAider(project, task string, maxFiles int) []string {
	if maxFiles <= 0 {
		maxFiles = 12
	}
	keywords := taskKeywords(task)
	scores := []fileScore{}
	_ = filepath.WalkDir(project, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != project && ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2<<20 {
			return nil
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		lowerPath := strings.ToLower(relSlash)
		score := 0
		for _, keyword := range keywords {
			if strings.Contains(lowerPath, keyword) {
				score += 12
			}
		}
		base := strings.ToLower(filepath.Base(path))
		if base == "readme.md" || base == "agents.md" || base == "state.md" {
			score += 1
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".go" || ext == ".cs" || ext == ".kt" || ext == ".java" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".tsx" || ext == ".cpp" || ext == ".h" || ext == ".rs" {
			score += 2
		}
		if score > 0 {
			if data, err := os.ReadFile(path); err == nil {
				text := strings.ToLower(string(data[:min(len(data), 65536)]))
				for _, keyword := range keywords {
					if strings.Contains(text, keyword) {
						score += 3
					}
				}
			}
			scores = append(scores, fileScore{Path: relSlash, Score: score})
		}
		return nil
	})
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Path < scores[j].Path
	})
	if len(scores) > maxFiles {
		scores = scores[:maxFiles]
	}
	out := make([]string, 0, len(scores))
	for _, score := range scores {
		out = append(out, score.Path)
	}
	return out
}

type projectFileFingerprint struct {
	Hash string
	Size int64
}

type aiderBackupEntry struct {
	Path         string `json:"path"`
	BeforeExists bool   `json:"before_exists"`
	BeforeHash   string `json:"before_hash,omitempty"`
	AfterHash    string `json:"after_hash,omitempty"`
}

type aiderBackupManifest struct {
	Project   string             `json:"project"`
	CreatedAt time.Time          `json:"created_at"`
	Entries   []aiderBackupEntry `json:"entries"`
}

func snapshotProjectFingerprints(project string) map[string]projectFileFingerprint {
	out := map[string]projectFileFingerprint{}
	_ = filepath.WalkDir(project, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != project && ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 8<<20 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = projectFileFingerprint{Hash: hex.EncodeToString(sum[:]), Size: info.Size()}
		return nil
	})
	return out
}

func changedFingerprintPaths(before, after map[string]projectFileFingerprint) []string {
	set := map[string]bool{}
	for path, old := range before {
		if current, ok := after[path]; !ok || current != old {
			set[path] = true
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			set[path] = true
		}
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func writeAiderBackupManifest(backupDir, project string, before, after map[string]projectFileFingerprint, changed []string) error {
	manifest := aiderBackupManifest{Project: filepath.Clean(project), CreatedAt: time.Now(), Entries: make([]aiderBackupEntry, 0, len(changed))}
	for _, path := range changed {
		entry := aiderBackupEntry{Path: filepath.ToSlash(path)}
		if fp, ok := before[path]; ok {
			entry.BeforeExists = true
			entry.BeforeHash = fp.Hash
		}
		if fp, ok := after[path]; ok {
			entry.AfterHash = fp.Hash
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backupDir, "LOCALCODE-AIDER-MANIFEST.json"), data, 0o600)
}

func fileSHA256(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true
}

func latestAiderBackup(project string) string {
	cache := userCacheBaseDir()
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(project))))
	root := filepath.Join(cache, productDirName, "aider-backups", hex.EncodeToString(sum[:8]))
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if !entries[i].IsDir() {
			continue
		}
		candidate := filepath.Join(root, entries[i].Name())
		if _, err := os.Stat(filepath.Join(candidate, "LOCALCODE-AIDER-MANIFEST.json")); err == nil {
			return candidate
		}
	}
	return ""
}

func restoreAiderBackup(project, backupDir string) (string, error) {
	project = filepath.Clean(project)
	backupDir = filepath.Clean(backupDir)
	if !pathWithin(filepath.Join(userCacheBaseDir(), productDirName, "aider-backups"), backupDir) {
		return "", errors.New("backup directory is outside the managed Aider backup root")
	}
	data, err := os.ReadFile(filepath.Join(backupDir, "LOCALCODE-AIDER-MANIFEST.json"))
	if err != nil {
		return "", fmt.Errorf("Aider backup manifest is missing: %w", err)
	}
	var manifest aiderBackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("invalid Aider backup manifest: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(manifest.Project), project) {
		return "", errors.New("backup belongs to a different project")
	}
	var restored, removed []string
	for _, entry := range manifest.Entries {
		target, err := ensureWithinRoot(project, entry.Path)
		if err != nil {
			return "", err
		}
		currentHash, exists := fileSHA256(target)
		if entry.AfterHash != "" {
			if !exists || currentHash != entry.AfterHash {
				return "", fmt.Errorf("refusing to overwrite %s because it changed after the Aider run", entry.Path)
			}
		} else if exists {
			return "", fmt.Errorf("refusing to remove %s because it was recreated after the Aider run", entry.Path)
		}
		if entry.BeforeExists {
			source, err := ensureWithinRoot(backupDir, entry.Path)
			if err != nil {
				return "", err
			}
			original, err := os.ReadFile(source)
			if err != nil {
				return "", fmt.Errorf("backup content missing for %s: %w", entry.Path, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			mode := os.FileMode(0o644)
			if info, statErr := os.Stat(target); statErr == nil {
				mode = info.Mode().Perm()
			}
			// Direct replacement is deliberate here: the manifest hash above
			// proves the target still matches the exact post-Aider state. This
			// also works on Windows, where os.Rename cannot replace an existing file.
			if err := os.WriteFile(target, original, mode); err != nil {
				return "", err
			}
			restored = append(restored, entry.Path)
		} else if exists {
			if err := os.Remove(target); err != nil {
				return "", err
			}
			removed = append(removed, entry.Path)
		}
	}
	return fmt.Sprintf("Restored files: %d\nRemoved newly created files: %d\nBackup: %s\n%s", len(restored), len(removed), backupDir, strings.Join(append(restored, removed...), "\n")), nil
}

func createAiderBackup(project string) (string, error) {
	cache := userCacheBaseDir()
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(project))))
	dir := filepath.Join(cache, productDirName, "aider-backups", hex.EncodeToString(sum[:8]), time.Now().Format("20060102-150405.000"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var total int64
	const maxTotal = int64(256 << 20)
	err := filepath.WalkDir(project, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if path != project && ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 4<<20 || total+info.Size() > maxTotal {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !isProbablyText(data) {
			return nil
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return nil
		}
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return "", err
	}
	manifest := fmt.Sprintf("Project: %s\nCreated: %s\nBytes: %d\n", project, time.Now().Format(time.RFC3339), total)
	_ = os.WriteFile(filepath.Join(dir, "LOCALCODE-AIDER-BACKUP.txt"), []byte(manifest), 0o600)
	return dir, nil
}

func localizedAiderLanguage(cfg Config) string {
	if resolvedLanguage(cfg) == "de" {
		return "German"
	}
	return "English"
}

func ensureManagedAiderConfig() (configFile, envFile string, err error) {
	dir := filepath.Join(appDataDir(), "aider-config")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	configFile = filepath.Join(dir, "managed.aider.conf.yml")
	// Passing an explicit minimal config prevents project- or home-level Aider
	// configuration from silently changing LocalCode's controlled execution.
	config := "# Managed by LocalCode. Runtime options are supplied explicitly.\n"
	if err = os.WriteFile(configFile, []byte(config), 0o600); err != nil {
		return "", "", err
	}
	envFile = filepath.Join(dir, "empty.env")
	// Aider normally auto-loads .env from the repository. LocalCode deliberately
	// uses an empty managed env file so project secrets are not ingested unless
	// the user explicitly exposes them through LocalCode environment settings.
	if err = os.WriteFile(envFile, []byte("# Intentionally empty; managed by LocalCode.\n"), 0o600); err != nil {
		return "", "", err
	}
	return configFile, envFile, nil
}

func inferAiderQualityCommands(project string) (lintCommand, testCommand string) {
	plan := detectProjectPlan(project)
	switch plan.Kind {
	case "go":
		return "go vet ./...", "go test ./..."
	case "rust":
		return "cargo clippy --all-targets --all-features", "cargo test"
	case "android-gradle":
		if runtime.GOOS == "windows" {
			return "gradlew.bat lint", "gradlew.bat test"
		}
		return "./gradlew lint", "./gradlew test"
	case "gradle":
		if runtime.GOOS == "windows" {
			return "gradlew.bat check", "gradlew.bat test"
		}
		return "./gradlew check", "./gradlew test"
	case "dotnet":
		return "dotnet format --verify-no-changes", "dotnet test"
	case "visual-studio":
		return "", ""
	case "python":
		lintCommand = "python -m compileall ."
		for _, name := range []string{"pytest.ini", "pyproject.toml", "tox.ini", "setup.cfg", "requirements.txt", "requirements-dev.txt"} {
			data, readErr := os.ReadFile(filepath.Join(project, name))
			if readErr == nil && strings.Contains(strings.ToLower(string(data)), "pytest") {
				testCommand = "python -m pytest"
				break
			}
		}
		return lintCommand, testCommand
	case "node":
		data, err := os.ReadFile(filepath.Join(project, "package.json"))
		if err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(data, &pkg) == nil {
				if strings.TrimSpace(pkg.Scripts["lint"]) != "" {
					lintCommand = "npm run lint"
				}
				if strings.TrimSpace(pkg.Scripts["test"]) != "" {
					testCommand = "npm test"
				} else if strings.TrimSpace(pkg.Scripts["build"]) != "" {
					testCommand = "npm run build"
				}
			}
		}
	}
	return lintCommand, testCommand
}

func buildAiderArgs(project, task, model, threadID string, cfg Config) ([]string, string, error) {
	if strings.TrimSpace(task) == "" {
		return nil, "", errors.New("Aider task is empty")
	}
	if strings.TrimSpace(model) == "" {
		model = cfg.AiderMainModel
	}
	if strings.TrimSpace(cfg.AiderMainModel) != "" {
		model = cfg.AiderMainModel
	}
	model = aiderModelName(model)
	if model == "" {
		return nil, "", errors.New("Aider model is empty")
	}
	if err := os.MkdirAll(aiderHistoryDir(), 0o755); err != nil {
		return nil, "", err
	}
	messageDir := filepath.Join(appDataDir(), "aider-messages")
	if err := os.MkdirAll(messageDir, 0o755); err != nil {
		return nil, "", err
	}
	messageFile := filepath.Join(messageDir, newID()+".txt")
	if err := os.WriteFile(messageFile, []byte(task), 0o600); err != nil {
		return nil, "", err
	}
	thread := safeThreadFileName(threadID)
	ignoreFile, err := ensureAiderIgnore(project)
	if err != nil {
		return nil, "", err
	}
	managedConfig, managedEnv, err := ensureManagedAiderConfig()
	if err != nil {
		return nil, "", err
	}
	args := []string{
		"--model", model,
		"--message-file", messageFile,
		"--yes-always",
		"--no-pretty",
		"--no-stream",
		"--no-check-update",
		"--no-show-release-notes",
		"--no-show-model-warnings",
		"--no-check-model-accepts-settings",
		"--analytics-disable",
		"--config", managedConfig,
		"--env-file", managedEnv,
		"--encoding", "utf-8",
		"--line-endings", "platform",
		"--no-suggest-shell-commands",
		"--no-detect-urls",
		"--disable-playwright",
		"--aiderignore", ignoreFile,
		"--map-tokens", strconv.Itoa(cfg.AiderMapTokens),
		"--map-refresh", "auto",
		"--max-chat-history-tokens", strconv.Itoa(cfg.AiderMaxChatHistoryTokens),
		"--chat-history-file", filepath.Join(aiderHistoryDir(), thread+".chat.md"),
		"--input-history-file", filepath.Join(aiderHistoryDir(), thread+".input.history"),
		"--llm-history-file", filepath.Join(aiderHistoryDir(), thread+".llm.history"),
		"--restore-chat-history",
		"--chat-language", localizedAiderLanguage(cfg),
		"--commit-language", localizedAiderLanguage(cfg),
		"--timeout", strconv.Itoa(max(30, cfg.ModelTimeout)),
		"--show-diffs",
		"--no-gitignore",
		"--no-attribute-author",
		"--no-attribute-committer",
		"--no-attribute-co-authored-by",
		"--no-dirty-commits",
		"--no-watch-files",
		"--no-notifications",
		"--no-fancy-input",
		"--no-cache-prompts",
	}
	if cfg.AiderUseGit && isGitRepository(project, cfg) {
		args = append(args, "--git")
		if cfg.AiderAutoCommits {
			args = append(args, "--auto-commits")
		} else {
			args = append(args, "--no-auto-commits")
		}
	} else {
		args = append(args, "--no-git", "--no-auto-commits")
	}
	inferredLint, inferredTest := inferAiderQualityCommands(project)
	lintCommand := strings.TrimSpace(cfg.AiderLintCommand)
	if lintCommand == "" {
		lintCommand = inferredLint
	}
	testCommand := strings.TrimSpace(cfg.AiderTestCommand)
	if testCommand == "" {
		testCommand = inferredTest
	}
	if cfg.AiderAutoLint {
		args = append(args, "--auto-lint")
	} else {
		args = append(args, "--no-auto-lint")
	}
	if cfg.AiderAutoTest {
		args = append(args, "--auto-test")
	} else {
		args = append(args, "--no-auto-test")
	}
	if lintCommand != "" {
		args = append(args, "--lint-cmd", lintCommand)
	}
	if testCommand != "" {
		args = append(args, "--test-cmd", testCommand)
	}
	if cfg.AiderArchitectMode {
		args = append(args, "--architect", "--auto-accept-architect")
		architectModel := strings.TrimSpace(cfg.AiderArchitectModel)
		if architectModel != "" {
			args[1] = aiderModelName(architectModel)
		}
		editorModel := strings.TrimSpace(cfg.AiderEditorModel)
		if editorModel == "" {
			editorModel = strings.TrimSpace(cfg.AiderMainModel)
		}
		if editorModel == "" {
			editorModel = strings.TrimPrefix(model, "ollama_chat/")
		}
		args = append(args, "--editor-model", aiderModelName(editorModel))
		if strings.TrimSpace(cfg.AiderEditorEditFormat) != "" && cfg.AiderEditorEditFormat != "auto" {
			args = append(args, "--editor-edit-format", cfg.AiderEditorEditFormat)
		}
	} else if strings.TrimSpace(cfg.AiderEditFormat) != "" && cfg.AiderEditFormat != "auto" {
		args = append(args, "--edit-format", cfg.AiderEditFormat)
	}
	for _, name := range []string{"AGENTS.md", "README.md", "STATE.md"} {
		if info, err := os.Stat(filepath.Join(project, name)); err == nil && !info.IsDir() {
			args = append(args, "--read", name)
		}
	}
	for _, file := range relevantFilesForAider(project, task, 12) {
		base := strings.ToLower(filepath.Base(file))
		if base == "agents.md" || base == "readme.md" || base == "state.md" {
			continue
		}
		args = append(args, "--file", file)
	}
	return args, messageFile, nil
}

func runAider(ctx context.Context, project, task, model, threadID string, cfg Config) (AiderRunResult, error) {
	status := aiderStatus(ctx, cfg)
	if !status.Installed {
		return AiderRunResult{}, &AiderNotInstalledError{Status: status}
	}
	args, messageFile, err := buildAiderArgs(project, task, model, threadID, cfg)
	if err != nil {
		return AiderRunResult{}, err
	}
	defer os.Remove(messageFile)
	before := snapshotProjectFingerprints(project)
	backupDir, err := createAiderBackup(project)
	if err != nil {
		return AiderRunResult{}, fmt.Errorf("could not create pre-edit backup: %w", err)
	}
	env := append(commandEnvironment(cfg),
		"OLLAMA_API_BASE="+strings.TrimRight(cfg.OllamaURL, "/"),
		"AIDER_ANALYTICS_DISABLE=true",
		"PYTHONUTF8=1",
		"PYTHONIOENCODING=utf-8",
	)
	started := time.Now()
	output, exitCode, runErr := runCapturedCommand(ctx, status.Executable, args, env, project)
	after := snapshotProjectFingerprints(project)
	changed := changedFingerprintPaths(before, after)
	if manifestErr := writeAiderBackupManifest(backupDir, project, before, after, changed); manifestErr != nil && runErr == nil {
		runErr = fmt.Errorf("Aider edits completed but backup manifest could not be written: %w", manifestErr)
	}
	result := AiderRunResult{
		Output:       output,
		ChangedFiles: changed,
		BackupDir:    backupDir,
		Executable:   status.Executable,
		Arguments:    args,
		Duration:     time.Since(started),
		ExitCode:     exitCode,
	}
	if runErr != nil {
		return result, fmt.Errorf("Aider failed with exit code %d: %w", exitCode, runErr)
	}
	return result, nil
}

func formatAiderRunResult(result AiderRunResult, cfg Config) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s\n%s: %d\n%s: %s\n%s: %s\n",
		localizeConfigText(cfg, "Aider-Programmdatei", "Aider executable"), result.Executable,
		localizeConfigText(cfg, "Exitcode", "Exit code"), result.ExitCode,
		localizeConfigText(cfg, "Dauer", "Duration"), result.Duration.Round(time.Millisecond),
		localizeConfigText(cfg, "Backup", "Backup"), result.BackupDir)
	if len(result.ChangedFiles) == 0 {
		out.WriteString(localizeConfigText(cfg, "Geänderte Dateien: keine erkannt\n", "Changed files: none detected\n"))
	} else {
		out.WriteString(localizeConfigText(cfg, "Geänderte Dateien:\n", "Changed files:\n"))
		for _, path := range result.ChangedFiles {
			out.WriteString("- " + path + "\n")
		}
	}
	if strings.TrimSpace(result.Output) != "" {
		out.WriteString("\nAIDER OUTPUT:\n" + result.Output)
	}
	return truncateText(out.String(), 160000)
}

func (s *AppState) offerInstallAider(ctx context.Context, project string, cfg Config) (Config, string, bool, error) {
	status := aiderStatus(ctx, cfg)
	if status.Installed {
		return cfg, "Aider ist bereits installiert: " + status.Version, true, nil
	}
	if !cfg.AiderAutoInstall {
		return cfg, status.Error, false, &AiderNotInstalledError{Status: status}
	}
	action := AgentAction{
		Action:  "install_aider",
		Message: localizeConfigText(cfg, "Aider Editing Engine installieren", "Install Aider Editing Engine"),
		Task:    "aider-chat==" + cfg.AiderVersion,
	}
	preview := localizeConfigText(cfg,
		"LocalCode installiert die fest angeheftete Aider-Version "+cfg.AiderVersion+" benutzerlokal. Falls uv fehlt, wird die offizielle Windows-Ausgabe von astral-sh/uv geladen. Es werden keine globalen Python-Pakete verändert. Installationsordner: "+aiderToolRoot(),
		"LocalCode installs pinned Aider version "+cfg.AiderVersion+" for the current user. If uv is missing, the official Windows build from astral-sh/uv is downloaded. No global Python packages are modified. Installation directory: "+aiderToolRoot())
	approved, err := s.requestApprovalWithPreview(ctx, project, action, preview)
	if err != nil {
		return cfg, "", false, err
	}
	if !approved {
		return cfg, localizeConfigText(cfg, "Aider-Installation wurde abgelehnt.", "Aider installation was declined."), false, nil
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: action.Message, Action: action.Action, Preview: preview})
	installCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	status, output, installErr := installAider(installCtx, cfg)
	cancel()
	if installErr != nil {
		detail := strings.TrimSpace(output + "\n\nERROR: " + installErr.Error())
		s.AddEvent(UIEvent{Type: "tool_error", Message: localizeConfigText(cfg, "Aider konnte nicht installiert werden", "Aider could not be installed"), Detail: detail, Action: action.Action})
		return cfg, detail, false, installErr
	}
	cfg.AiderExecutable = status.Executable
	cfg.AiderVersion = status.ExpectedVersion
	cfg = normalizeConfig(cfg)
	s.mu.Lock()
	s.Config = cfg
	s.mu.Unlock()
	if err := saveConfig(cfg); err != nil {
		return cfg, output, false, fmt.Errorf("Aider installed but configuration could not be saved: %w", err)
	}
	detail := strings.TrimSpace(output + "\n\nVerified: " + status.Executable + "\n" + status.Version)
	s.AddEvent(UIEvent{Type: "action_done", Message: localizeConfigText(cfg, "Aider installiert und verifiziert", "Aider installed and verified"), Detail: truncateText(detail, 30000), Action: action.Action})
	return cfg, detail, true, nil
}

func (s *AppState) currentAiderThreadAndModel(cfg Config) (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	model := s.Model
	if strings.TrimSpace(cfg.AiderMainModel) != "" {
		model = cfg.AiderMainModel
	}
	return s.CurrentThread, model
}

func buildAiderUtilityArgs(project, mode, model, threadID string, cfg Config) ([]string, error) {
	if strings.TrimSpace(model) == "" {
		model = cfg.AiderMainModel
	}
	if strings.TrimSpace(cfg.AiderMainModel) != "" {
		model = cfg.AiderMainModel
	}
	model = aiderModelName(model)
	if model == "" {
		return nil, errors.New("Aider model is empty")
	}
	if err := os.MkdirAll(aiderHistoryDir(), 0o755); err != nil {
		return nil, err
	}
	managedConfig, managedEnv, err := ensureManagedAiderConfig()
	if err != nil {
		return nil, err
	}
	ignoreFile, err := ensureAiderIgnore(project)
	if err != nil {
		return nil, err
	}
	thread := safeThreadFileName(threadID)
	args := []string{
		"--model", model,
		"--yes-always",
		"--no-pretty",
		"--no-stream",
		"--no-check-update",
		"--no-show-release-notes",
		"--no-show-model-warnings",
		"--no-check-model-accepts-settings",
		"--analytics-disable",
		"--config", managedConfig,
		"--env-file", managedEnv,
		"--encoding", "utf-8",
		"--line-endings", "platform",
		"--no-suggest-shell-commands",
		"--no-detect-urls",
		"--disable-playwright",
		"--aiderignore", ignoreFile,
		"--map-tokens", strconv.Itoa(cfg.AiderMapTokens),
		"--map-refresh", "auto",
		"--max-chat-history-tokens", strconv.Itoa(cfg.AiderMaxChatHistoryTokens),
		"--chat-history-file", filepath.Join(aiderHistoryDir(), thread+".chat.md"),
		"--input-history-file", filepath.Join(aiderHistoryDir(), thread+".input.history"),
		"--llm-history-file", filepath.Join(aiderHistoryDir(), thread+".llm.history"),
		"--restore-chat-history",
		"--chat-language", localizedAiderLanguage(cfg),
		"--commit-language", localizedAiderLanguage(cfg),
		"--timeout", strconv.Itoa(max(30, cfg.ModelTimeout)),
		"--no-auto-commits",
		"--no-auto-lint",
		"--no-auto-test",
		"--no-gitignore",
		"--no-attribute-author",
		"--no-attribute-committer",
		"--no-attribute-co-authored-by",
		"--no-dirty-commits",
		"--no-watch-files",
		"--no-notifications",
		"--no-fancy-input",
		"--no-cache-prompts",
	}
	if cfg.AiderUseGit && isGitRepository(project, cfg) {
		args = append(args, "--git")
	} else {
		args = append(args, "--no-git")
	}
	inferredLint, inferredTest := inferAiderQualityCommands(project)
	lintCommand := strings.TrimSpace(cfg.AiderLintCommand)
	if lintCommand == "" {
		lintCommand = inferredLint
	}
	testCommand := strings.TrimSpace(cfg.AiderTestCommand)
	if testCommand == "" {
		testCommand = inferredTest
	}
	switch mode {
	case "repo-map":
		args = append(args, "--show-repo-map")
	case "lint":
		args = append(args, "--lint")
		if lintCommand != "" {
			args = append(args, "--lint-cmd", lintCommand)
		}
	case "test":
		args = append(args, "--test")
		if testCommand != "" {
			args = append(args, "--test-cmd", testCommand)
		}
	default:
		return nil, fmt.Errorf("unsupported Aider utility mode: %s", mode)
	}
	return args, nil
}

func runAiderUtility(ctx context.Context, project, mode, model, threadID string, cfg Config) (string, error) {
	status := aiderStatus(ctx, cfg)
	if !status.Installed {
		return "", &AiderNotInstalledError{Status: status}
	}
	args, err := buildAiderUtilityArgs(project, mode, model, threadID, cfg)
	if err != nil {
		return "", err
	}
	env := append(commandEnvironment(cfg),
		"OLLAMA_API_BASE="+strings.TrimRight(cfg.OllamaURL, "/"),
		"AIDER_ANALYTICS_DISABLE=true",
		"PYTHONUTF8=1",
		"PYTHONIOENCODING=utf-8",
	)
	var before map[string]projectFileFingerprint
	var backupDir string
	if mode == "lint" || mode == "test" {
		before = snapshotProjectFingerprints(project)
		var backupErr error
		backupDir, backupErr = createAiderBackup(project)
		if backupErr != nil {
			return "", fmt.Errorf("could not create pre-%s backup: %w", mode, backupErr)
		}
	}
	output, exitCode, runErr := runCapturedCommand(ctx, status.Executable, args, env, project)
	if backupDir != "" {
		after := snapshotProjectFingerprints(project)
		changed := changedFingerprintPaths(before, after)
		if manifestErr := writeAiderBackupManifest(backupDir, project, before, after, changed); manifestErr != nil && runErr == nil {
			runErr = fmt.Errorf("Aider %s completed but backup manifest could not be written: %w", mode, manifestErr)
		}
		var summary strings.Builder
		fmt.Fprintf(&summary, "%s: %s\n", localizeConfigText(cfg, "Backup", "Backup"), backupDir)
		if len(changed) == 0 {
			summary.WriteString(localizeConfigText(cfg, "Geänderte Dateien: keine erkannt\n", "Changed files: none detected\n"))
		} else {
			summary.WriteString(localizeConfigText(cfg, "Geänderte Dateien:\n", "Changed files:\n"))
			for _, path := range changed {
				summary.WriteString("- " + path + "\n")
			}
		}
		if strings.TrimSpace(output) != "" {
			summary.WriteString("\nAIDER OUTPUT:\n" + output)
		}
		output = summary.String()
	}
	if runErr != nil {
		return output, fmt.Errorf("Aider %s failed with exit code %d: %w", mode, exitCode, runErr)
	}
	return output, nil
}

func (s *AppState) executeAiderAction(ctx context.Context, project string, cfg Config, action AgentAction) (string, error) {
	// Aider is a subprocess and must never outlive the configured command
	// budget. The parent context still supports immediate user cancellation;
	// this additional deadline guarantees controlled termination when a model,
	// linter, test runner, or child process stops making progress.
	aiderCtx, cancel := context.WithTimeout(ctx, time.Duration(max(60, cfg.CommandTimeout))*time.Second)
	defer cancel()
	threadID, model := s.currentAiderThreadAndModel(cfg)
	switch action.Action {
	case "aider_edit":
		task := strings.TrimSpace(action.Task)
		if task == "" {
			task = strings.TrimSpace(action.Message)
		}
		result, err := runAider(aiderCtx, project, task, model, threadID, cfg)
		if result.BackupDir != "" {
			s.mu.Lock()
			s.LastAiderBackup = result.BackupDir
			s.mu.Unlock()
		}
		formatted := formatAiderRunResult(result, cfg)
		if err != nil {
			return formatted, err
		}
		return formatted, nil
	case "aider_repo_map":
		return runAiderUtility(aiderCtx, project, "repo-map", model, threadID, cfg)
	case "aider_lint":
		output, err := runAiderUtility(aiderCtx, project, "lint", model, threadID, cfg)
		if backup := latestAiderBackup(project); backup != "" {
			s.mu.Lock()
			s.LastAiderBackup = backup
			s.mu.Unlock()
		}
		return output, err
	case "aider_test":
		output, err := runAiderUtility(aiderCtx, project, "test", model, threadID, cfg)
		if backup := latestAiderBackup(project); backup != "" {
			s.mu.Lock()
			s.LastAiderBackup = backup
			s.mu.Unlock()
		}
		return output, err
	case "aider_undo":
		s.mu.RLock()
		backup := s.LastAiderBackup
		s.mu.RUnlock()
		if strings.TrimSpace(backup) == "" {
			backup = latestAiderBackup(project)
		}
		if strings.TrimSpace(backup) == "" {
			return "", errors.New("no Aider backup is available for this project")
		}
		return restoreAiderBackup(project, backup)
	default:
		return "", fmt.Errorf("unsupported Aider action: %s", action.Action)
	}
}
