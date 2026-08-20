// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	clawUpstreamRepository = "https://github.com/ultraworkers/claw-code.git"
	// Pin automatic builds to a reviewed upstream revision. Updating Claw is an
	// explicit LocalCode source change instead of an implicit pull from main.
	clawPinnedCommit = "08106b0c3771ef5b4a5aa176acccd460e88b7325"
)

func clawToolRoot() string {
	return filepath.Join(appDataDir(), "tools", "claw-code")
}

func clawManagedSourceRoot() string {
	return filepath.Join(clawToolRoot(), "source")
}

func clawManagedBinary() string {
	name := "claw"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(clawToolRoot(), "bin", name)
}

func findClawExecutable() string {
	name := "claw"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	pathCandidate, _ := exec.LookPath(name)
	return firstExecutable(
		pathCandidate,
		clawManagedBinary(),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "ClawCode", "bin", name),
		filepath.Join(userProfileDir(), ".cargo", "bin", name),
	)
}

func clawOllamaHost(raw string) string {
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	if host == "" {
		host = "http://127.0.0.1:11434"
	}
	if strings.HasSuffix(strings.ToLower(host), "/v1") {
		host = strings.TrimRight(host[:len(host)-3], "/")
	}
	return host
}

func removeEnvironmentKeys(env []string, keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[strings.ToUpper(strings.TrimSpace(key))] = true
	}
	out := make([]string, 0, len(env))
	for _, item := range env {
		key := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if blocked[strings.ToUpper(strings.TrimSpace(key))] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func clawCommandEnvironment(cfg Config) []string {
	// The LocalCode Claw profile is deliberately local-first. Do not allow
	// ambient cloud-provider credentials, provider overrides, or a stale
	// OpenAI-compatible endpoint to silently route a supposedly local run away
	// from Ollama.
	env := removeEnvironmentKeys(commandEnvironment(cfg),
		"OLLAMA_HOST",
		"OPENAI_BASE_URL", "OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"XAI_BASE_URL", "XAI_API_KEY",
		"DASHSCOPE_BASE_URL", "DASHSCOPE_API_KEY",
		"CLAUDE_CODE_PROVIDER",
	)
	return append(env,
		"OLLAMA_HOST="+clawOllamaHost(cfg.OllamaURL),
		"CLAW_OUTPUT_FORMAT=json",
	)
}

func clawSelectedModel(cfg Config, fallback string) string {
	model := normalizeConfiguredOllamaModel(fallback)
	if model == "" {
		model = normalizeConfiguredOllamaModel(cfg.OllamaDefaultModel)
	}
	return model
}

func buildClawArgs(task, model, mode string) []string {
	permission := "workspace-write"
	if mode == "repo-map" {
		permission = "read-only"
	}
	args := []string{"--output-format", "json", "--permission-mode", permission}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	args = append(args, "prompt", task)
	return args
}

func ensureClawBuildDependency(ctx context.Context, project, name string, cfg Config) (Config, string, error) {
	info := discoverTool(project, name, cfg, true)
	if info.Available {
		return cfg, info.Path, nil
	}
	if !toolInstallSupported(name) {
		return cfg, "", fmt.Errorf("%s is required to build Claw Code and has no managed installer", name)
	}
	updated, output, err := installKnownTool(ctx, project, name, cfg)
	if err != nil {
		return cfg, output, err
	}
	info = discoverTool(project, name, updated, true)
	if !info.Available {
		return updated, output, fmt.Errorf("%s is unavailable after managed installation", name)
	}
	return updated, info.Path, nil
}

func clawCapturedStdout(raw string) string {
	value := strings.TrimSpace(raw)
	for _, prefix := range []string{"STDOUT:\r\n", "STDOUT:\n"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
			break
		}
	}
	// runCapturedCommand labels stdout and stderr separately. A successful
	// Claw command can still emit a warning on stderr; version JSON must be
	// parsed only from the stdout channel rather than from the combined text.
	for _, marker := range []string{"\r\nSTDERR:\r\n", "\nSTDERR:\n", "\r\nSTDERR:", "\nSTDERR:"} {
		if index := strings.Index(value, marker); index >= 0 {
			value = strings.TrimSpace(value[:index])
			break
		}
	}
	return value
}

type clawVersionReport struct {
	GitSHA string `json:"git_sha"`
}

func parseClawVersionReport(raw string) (string, error) {
	payload := clawCapturedStdout(raw)
	var report clawVersionReport
	if err := json.Unmarshal([]byte(payload), &report); err != nil {
		return "", fmt.Errorf("parse Claw version JSON: %w", err)
	}
	gitSHA := strings.TrimSpace(report.GitSHA)
	if gitSHA == "" {
		return "", errors.New("Claw version JSON does not contain git_sha")
	}
	return gitSHA, nil
}

func clawExecutableVersion(ctx context.Context, executable string, cfg Config) (string, error) {
	if executable == "" {
		return "", errors.New("Claw executable not found")
	}
	output, code, err := runCapturedCommand(ctx, executable, []string{"version", "--output-format", "json"}, clawCommandEnvironment(cfg), "")
	if err != nil || code != 0 {
		return strings.TrimSpace(output), fmt.Errorf("Claw version command failed with exit code %d: %w", code, err)
	}
	return parseClawVersionReport(output)
}

func clawOfficialOrigin(raw string) bool {
	origin := strings.ToLower(strings.TrimSpace(clawCapturedStdout(raw)))
	origin = strings.TrimSuffix(origin, "/")
	origin = strings.TrimSuffix(origin, ".git")
	switch origin {
	case "https://github.com/ultraworkers/claw-code",
		"git@github.com:ultraworkers/claw-code",
		"ssh://git@github.com/ultraworkers/claw-code":
		return true
	default:
		return false
	}
}

func preparePinnedClawSource(ctx context.Context, gitPath string, cfg Config) (string, string, error) {
	root := clawManagedSourceRoot()
	var detail []string
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		origin, code, runErr := runCapturedCommand(ctx, gitPath, []string{"-C", root, "remote", "get-url", "origin"}, commandEnvironment(cfg), "")
		if runErr != nil || code != 0 {
			return root, strings.TrimSpace(origin), errors.New("managed Claw source exists but is not a usable Git repository")
		}
		if !clawOfficialOrigin(origin) {
			return root, strings.TrimSpace(origin), errors.New("managed Claw source has an unexpected origin; refusing to overwrite it")
		}
	} else if err == nil {
		return root, "", errors.New("managed Claw source path is not a directory")
	} else if !os.IsNotExist(err) {
		return root, "", err
	} else {
		if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
			return root, "", err
		}
		output, code, runErr := runCapturedCommand(ctx, gitPath, []string{"clone", "--filter=blob:none", "--no-checkout", clawUpstreamRepository, root}, commandEnvironment(cfg), filepath.Dir(root))
		detail = append(detail, strings.TrimSpace(output))
		if runErr != nil || code != 0 {
			return root, strings.Join(detail, "\n"), fmt.Errorf("Claw source clone failed with exit code %d: %w", code, runErr)
		}
	}
	output, code, runErr := runCapturedCommand(ctx, gitPath, []string{"-C", root, "fetch", "--depth", "1", "origin", clawPinnedCommit}, commandEnvironment(cfg), "")
	detail = append(detail, strings.TrimSpace(output))
	if runErr != nil || code != 0 {
		return root, strings.Join(detail, "\n"), fmt.Errorf("Claw pinned revision fetch failed with exit code %d: %w", code, runErr)
	}
	output, code, runErr = runCapturedCommand(ctx, gitPath, []string{"-C", root, "checkout", "--detach", "--force", clawPinnedCommit}, commandEnvironment(cfg), "")
	detail = append(detail, strings.TrimSpace(output))
	if runErr != nil || code != 0 {
		return root, strings.Join(detail, "\n"), fmt.Errorf("Claw pinned revision checkout failed with exit code %d: %w", code, runErr)
	}
	actual, code, runErr := runCapturedCommand(ctx, gitPath, []string{"-C", root, "rev-parse", "HEAD"}, commandEnvironment(cfg), "")
	actual = clawCapturedStdout(actual)
	if runErr != nil || code != 0 || !strings.EqualFold(actual, clawPinnedCommit) {
		return root, strings.Join(detail, "\n"), fmt.Errorf("Claw source revision verification failed: got %q", actual)
	}
	return root, strings.TrimSpace(strings.Join(detail, "\n")), nil
}

func installClawCode(ctx context.Context, project string, cfg Config) (CodingEngineStatus, Config, string, error) {
	if existing := findClawExecutable(); existing != "" {
		status := codingEngineStatus(ctx, cfg, editingEngineClaw)
		if !status.Installed {
			detail := "Existing Claw Code executable could not be verified: " + existing
			if strings.TrimSpace(status.Error) != "" {
				detail += "\n" + status.Error
			}
			return status, cfg, detail, errors.New("existing Claw Code installation could not be verified; fix or remove it before managed installation")
		}
		return status, cfg, "Existing Claw Code installation detected and verified: " + existing, nil
	}
	if !cfg.SetupDownloadsEnabled {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, "", errors.New("downloads for automatic setup are disabled")
	}
	updated, gitPath, err := ensureClawBuildDependency(ctx, project, "git", cfg)
	if err != nil {
		return codingEngineStatus(ctx, updated, editingEngineClaw), updated, gitPath, err
	}
	cfg = updated
	updated, cargoPath, err := ensureClawBuildDependency(ctx, project, "cargo", cfg)
	if err != nil {
		return codingEngineStatus(ctx, updated, editingEngineClaw), updated, "", err
	}
	cfg = updated
	_, msvcDetail, err := ensureClawMSVCToolchain(ctx, cfg)
	if err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, msvcDetail, err
	}
	sourceRoot, sourceDetail, err := preparePinnedClawSource(ctx, gitPath, cfg)
	if err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, strings.TrimSpace(msvcDetail + "\n" + sourceDetail), err
	}
	rustRoot := filepath.Join(sourceRoot, "rust")
	output, code, runErr := runClawCargoBuild(ctx, cargoPath, rustRoot, cfg)
	detail := strings.TrimSpace(msvcDetail + "\n" + sourceDetail + "\n" + output)
	if runErr != nil || code != 0 {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, detail, fmt.Errorf("Claw release build failed with exit code %d: %w", code, runErr)
	}
	name := "claw"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	built := filepath.Join(rustRoot, "target", "release", name)
	if info, statErr := os.Stat(built); statErr != nil || info.IsDir() {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, detail, errors.New("Claw build completed but the expected binary is missing")
	}
	target := clawManagedBinary()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, detail, err
	}
	data, err := os.ReadFile(built)
	if err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, detail, err
	}
	if err := writeFileAtomic(target, data, 0o755); err != nil {
		return codingEngineStatus(ctx, cfg, editingEngineClaw), cfg, detail, err
	}
	status := codingEngineStatus(ctx, cfg, editingEngineClaw)
	if !status.Installed {
		return status, cfg, detail, errors.New("Claw installation could not be verified")
	}
	return status, cfg, strings.TrimSpace(detail + "\nVerified pinned Claw revision: " + clawPinnedCommit), nil
}
