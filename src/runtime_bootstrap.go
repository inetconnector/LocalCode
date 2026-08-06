// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strings"
	"time"
)

const defaultCodingModel = "qwen2.5-coder:14b"

type RuntimeBootstrapResult struct {
	Config  Config
	Ollama  *OllamaClient
	Models  []ModelInfo
	Details []string
}

func normalizeConfiguredOllamaModel(model string) string {
	model = strings.TrimSpace(model)
	lower := strings.ToLower(model)
	for _, prefix := range []string{"ollama_chat/", "ollama/"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(model[len(prefix):])
		}
	}
	return model
}

func comparableModelName(model string) string {
	model = strings.ToLower(strings.TrimSpace(normalizeConfiguredOllamaModel(model)))
	if model == "" {
		return ""
	}
	lastSlash := strings.LastIndex(model, "/")
	lastColon := strings.LastIndex(model, ":")
	if lastColon <= lastSlash {
		model += ":latest"
	}
	return model
}

func modelInstalled(models []ModelInfo, wanted string) bool {
	wanted = comparableModelName(wanted)
	if wanted == "" {
		return false
	}
	for _, model := range models {
		if comparableModelName(model.Name) == wanted {
			return true
		}
	}
	return false
}

func requiredOllamaModels(cfg Config, models []ModelInfo) []string {
	candidates := []string{
		cfg.AiderMainModel,
		cfg.AiderArchitectModel,
		cfg.AiderEditorModel,
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.OpenCodeModel)), "ollama/") {
		candidates = append(candidates, strings.TrimSpace(cfg.OpenCodeModel[len("ollama/"):]))
	}
	if len(models) == 0 {
		// A stale LastModel from a previous installation must not trigger a
		// second multi-gigabyte download on a fresh machine. Bootstrap with the
		// configured coding default, then let the user select additional models.
		candidates = append(candidates, cfg.OllamaDefaultModel)
	} else {
		candidates = append(candidates, cfg.LastModel)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, candidate := range candidates {
		candidate = normalizeConfiguredOllamaModel(candidate)
		if candidate == "" {
			continue
		}
		key := comparableModelName(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if !modelInstalled(models, candidate) {
			out = append(out, candidate)
		}
	}
	if len(models) == 0 && len(out) == 0 {
		out = append(out, defaultCodingModel)
	}
	sort.Strings(out)
	return out
}

func ensureOllamaInstalledAndRunning(ctx context.Context, cfg Config, client *OllamaClient) (string, error) {
	probeCtx, probeCancel := context.WithTimeout(ctx, 35*time.Second)
	probeErr := client.EnsureRunning(probeCtx)
	probeCancel()
	if probeErr == nil {
		return "Ollama is already installed and reachable at " + client.BaseURL, nil
	}
	if runtime.GOOS != "windows" || !cfg.OllamaAutoInstall {
		return "", errors.New("Ollama is not installed or reachable and automatic installation is disabled")
	}
	if !cfg.NetworkEnabled {
		return "", errors.New("Ollama is missing, but network access is disabled")
	}
	log.Printf("Ollama is missing; starting official automatic installation")
	installDetail, err := installOllama(ctx)
	if err != nil {
		return installDetail, fmt.Errorf("automatic Ollama installation failed: %w", err)
	}
	startCtx, startCancel := context.WithTimeout(ctx, 90*time.Second)
	startErr := client.EnsureRunning(startCtx)
	startCancel()
	if startErr != nil {
		return installDetail, fmt.Errorf("Ollama was installed but could not be started: %w", startErr)
	}
	return strings.TrimSpace(installDetail + "\nOllama reachable at " + client.BaseURL), nil
}

func ensureConfiguredModels(ctx context.Context, cfg Config, client *OllamaClient, models []ModelInfo) ([]ModelInfo, []string, error) {
	missing := requiredOllamaModels(cfg, models)
	if len(missing) == 0 {
		return models, nil, nil
	}
	if !cfg.OllamaAutoPull {
		return models, nil, fmt.Errorf("required Ollama models are missing and automatic model download is disabled: %s", strings.Join(missing, ", "))
	}
	if !cfg.NetworkEnabled {
		return models, nil, fmt.Errorf("required Ollama models are missing but network access is disabled: %s", strings.Join(missing, ", "))
	}
	details := []string{}
	for _, model := range missing {
		log.Printf("Pulling required Ollama model %s", model)
		pullCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		err := client.Pull(pullCtx, model)
		cancel()
		if err != nil {
			return models, details, fmt.Errorf("automatic model download failed for %s: %w", model, err)
		}
		details = append(details, "Installed Ollama model: "+model)
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	refreshed, err := client.Tags(refreshCtx)
	cancel()
	if err != nil {
		return models, details, fmt.Errorf("models were downloaded but could not be enumerated: %w", err)
	}
	return refreshed, details, nil
}

func ensureSelectedEditingEngineRuntime(ctx context.Context, cfg Config) (Config, string, error) {
	engine := normalizeEditingEngine(cfg.EditingEngine)
	if engine == editingEngineNative {
		return cfg, "LocalCode native editing engine is ready", nil
	}
	if !codingEngineEnabled(cfg, engine) {
		return cfg, codingEngineDisplayName(engine) + " is disabled", nil
	}
	status := codingEngineStatus(ctx, cfg, engine)
	if status.Installed {
		switch engine {
		case editingEngineAider:
			cfg.AiderExecutable = status.Executable
		case editingEngineClaude:
			cfg.ClaudeCodeExecutable = status.Executable
		case editingEngineOpenCode:
			cfg.OpenCodeExecutable = status.Executable
		}
		detail := status.DisplayName + " already installed: " + status.Version
		if !status.Authenticated {
			detail += " (login required before first use)"
		}
		return normalizeConfig(cfg), detail, nil
	}
	if !codingEngineAutoInstall(cfg, engine) {
		return cfg, status.Error, &CodingEngineNotInstalledError{Status: status}
	}
	if !cfg.NetworkEnabled {
		return cfg, status.Error, fmt.Errorf("%s is missing, but network access is disabled", status.DisplayName)
	}
	log.Printf("%s is missing or outdated; starting automatic installation", status.DisplayName)
	installCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	status, updated, output, err := installCodingEngine(installCtx, cfg.RootProjectDir, engine, cfg)
	cancel()
	if err != nil {
		return cfg, output, err
	}
	detail := strings.TrimSpace(output + "\n" + status.DisplayName + " verified: " + status.Version)
	if !status.Authenticated {
		detail += "\nLogin is still required and can be opened from Settings > Configuration."
	}
	return normalizeConfig(updated), detail, nil
}

// ensureAiderRuntime is retained for backward-compatible callers and tests.
func ensureAiderRuntime(ctx context.Context, cfg Config) (Config, string, error) {
	if !cfg.AiderEnabled || normalizeEditingEngine(cfg.EditingEngine) == editingEngineNative {
		return cfg, "Aider is disabled", nil
	}
	cfg.EditingEngine = editingEngineAider
	return ensureSelectedEditingEngineRuntime(ctx, cfg)
}

func bootstrapRuntimeDependencies(ctx context.Context, cfg Config) (RuntimeBootstrapResult, error) {
	result := RuntimeBootstrapResult{Config: cfg, Ollama: NewOllamaClient()}
	if cfg.OllamaURL != "" {
		result.Ollama.BaseURL = cfg.OllamaURL
	}
	result.Ollama.ContextLength = cfg.ContextLength

	detail, err := ensureOllamaInstalledAndRunning(ctx, cfg, result.Ollama)
	if detail != "" {
		result.Details = append(result.Details, detail)
	}
	if err != nil {
		return result, err
	}

	tagsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	models, err := result.Ollama.Tags(tagsCtx)
	cancel()
	if err != nil {
		return result, fmt.Errorf("Ollama models could not be read: %w", err)
	}
	models, modelDetails, err := ensureConfiguredModels(ctx, cfg, result.Ollama, models)
	result.Details = append(result.Details, modelDetails...)
	if err != nil {
		return result, err
	}
	if len(models) == 0 {
		return result, errors.New("Ollama has no installed model after automatic setup")
	}
	result.Models = models
	result.Config.LastModel = chooseDefaultModel(models, result.Config.LastModel)

	updated, engineDetail, err := ensureSelectedEditingEngineRuntime(ctx, result.Config)
	if engineDetail != "" {
		result.Details = append(result.Details, engineDetail)
	}
	if err != nil {
		return result, fmt.Errorf("editing engine setup failed: %w", err)
	}
	result.Config = updated
	return result, nil
}
