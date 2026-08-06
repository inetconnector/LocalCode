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

func ensureAiderRuntime(ctx context.Context, cfg Config) (Config, string, error) {
	if cfg.EditingEngine != "aider" || !cfg.AiderEnabled {
		return cfg, "Aider is disabled", nil
	}
	status := aiderStatus(ctx, cfg)
	if status.Installed {
		cfg.AiderExecutable = status.Executable
		return cfg, "Aider already installed: " + status.Version, nil
	}
	if !cfg.AiderAutoInstall {
		return cfg, status.Error, &AiderNotInstalledError{Status: status}
	}
	if !cfg.NetworkEnabled {
		return cfg, status.Error, errors.New("Aider is missing, but network access is disabled")
	}
	log.Printf("Aider is missing or outdated; installing pinned version %s", cfg.AiderVersion)
	installCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	status, output, err := installAider(installCtx, cfg)
	cancel()
	if err != nil {
		return cfg, output, err
	}
	cfg.AiderExecutable = status.Executable
	cfg.AiderVersion = status.ExpectedVersion
	return normalizeConfig(cfg), strings.TrimSpace(output + "\nAider verified: " + status.Version), nil
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

	updated, aiderDetail, err := ensureAiderRuntime(ctx, result.Config)
	if aiderDetail != "" {
		result.Details = append(result.Details, aiderDetail)
	}
	if err != nil {
		return result, fmt.Errorf("Aider setup failed: %w", err)
	}
	result.Config = updated
	return result, nil
}
