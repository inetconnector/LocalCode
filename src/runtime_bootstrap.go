// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
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

type BootstrapProgress struct {
	Percent int    `json:"percent"`
	Stage   string `json:"stage"`
	Detail  string `json:"detail,omitempty"`
}

type BootstrapReporter func(BootstrapProgress)

func reportBootstrap(cfg Config, reporter BootstrapReporter, percent int, de, en, detail string) {
	if reporter == nil {
		return
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	reporter(BootstrapProgress{
		Percent: percent,
		Stage:   localizeConfigText(cfg, de, en),
		Detail:  strings.TrimSpace(detail),
	})
}

func localizedBootstrapError(cfg Config, de, en string, args ...any) error {
	return fmt.Errorf(localizeConfigText(cfg, de, en), args...)
}

func formatDownloadAmount(bytes int64) string {
	if bytes < 0 {
		return "unknown"
	}
	const (
		kiB = int64(1 << 10)
		miB = int64(1 << 20)
		giB = int64(1 << 30)
	)
	switch {
	case bytes >= giB:
		return fmt.Sprintf("%.2f GiB", float64(bytes)/float64(giB))
	case bytes >= miB:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(miB))
	case bytes >= kiB:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(kiB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func reportOllamaInstallProgress(cfg Config, reporter BootstrapReporter, phase string, written, total int64) {
	switch phase {
	case "download":
		percent := 14
		displayWritten := written
		if total > 0 && displayWritten > total {
			displayWritten = total
		}
		detail := formatDownloadAmount(displayWritten)
		if total > 0 {
			percent += int((displayWritten * 3) / total)
			detail += " / " + formatDownloadAmount(total)
		}
		reportBootstrap(cfg, reporter, percent,
			"Ollama-Installer wird heruntergeladen …",
			"Downloading the Ollama installer …",
			detail)
	case "verify":
		reportBootstrap(cfg, reporter, 18,
			"Digitale Signatur des Ollama-Installers wird geprüft …",
			"Verifying the Ollama installer signature …",
			"Authenticode")
	case "install":
		reportBootstrap(cfg, reporter, 19,
			"Ollama wird installiert …",
			"Installing Ollama …",
			localizeConfigText(cfg, "Der offizielle Installer läuft im Hintergrund.", "The official installer is running in the background."))
	case "locate":
		reportBootstrap(cfg, reporter, 20,
			"Ollama-Installation wird überprüft …",
			"Verifying the Ollama installation …",
			"ollama.exe")
	}
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
	candidates := []string{}
	switch normalizeEditingEngine(cfg.EditingEngine) {
	case editingEngineAider:
		if cfg.AiderEnabled {
			candidates = append(candidates,
				cfg.AiderMainModel,
				cfg.AiderArchitectModel,
				cfg.AiderEditorModel,
			)
		}
	case editingEngineOpenCode:
		model := strings.TrimSpace(cfg.OpenCodeModel)
		if cfg.OpenCodeEnabled && strings.HasPrefix(strings.ToLower(model), "ollama/") {
			candidates = append(candidates, strings.TrimSpace(model[len("ollama/"):]))
		}
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
	return ensureOllamaInstalledAndRunningWithProgress(ctx, cfg, client, nil)
}

func ensureOllamaInstalledAndRunningWithProgress(ctx context.Context, cfg Config, client *OllamaClient, reporter BootstrapReporter) (string, error) {
	reportBootstrap(cfg, reporter, 8, "Ollama wird gesucht und geprüft …", "Checking for Ollama …", client.BaseURL)
	probeCtx, probeCancel := context.WithTimeout(ctx, 35*time.Second)
	probeErr := client.EnsureRunning(probeCtx)
	probeCancel()
	if probeErr == nil {
		reportBootstrap(cfg, reporter, 22, "Ollama ist erreichbar.", "Ollama is reachable.", client.BaseURL)
		return "Ollama is already installed and reachable at " + client.BaseURL, nil
	}
	if runtime.GOOS != "windows" || !cfg.OllamaAutoInstall {
		return "", localizedBootstrapError(cfg,
			"Ollama ist nicht installiert oder nicht erreichbar und die automatische Installation ist deaktiviert.",
			"Ollama is not installed or reachable and automatic installation is disabled.")
	}
	if !cfg.SetupDownloadsEnabled {
		return "", localizedBootstrapError(cfg,
			"Ollama fehlt, aber Downloads für die automatische Einrichtung sind deaktiviert.",
			"Ollama is missing, but downloads for automatic setup are disabled.")
	}
	reportBootstrap(cfg, reporter, 14, "Ollama fehlt und wird automatisch installiert …", "Ollama is missing and will be installed automatically …", "https://ollama.com/download/OllamaSetup.exe")
	log.Printf("Ollama is missing; starting official automatic installation")
	installDetail, err := installOllama(ctx, func(phase string, written, total int64) {
		reportOllamaInstallProgress(cfg, reporter, phase, written, total)
	})
	if err != nil {
		return installDetail, localizedBootstrapError(cfg,
			"Die automatische Ollama-Installation ist fehlgeschlagen: %w",
			"Automatic Ollama installation failed: %w", err)
	}
	reportBootstrap(cfg, reporter, 19, "Ollama wurde installiert und wird gestartet …", "Ollama was installed and is starting …", findOllamaExecutable())
	startCtx, startCancel := context.WithTimeout(ctx, 90*time.Second)
	startErr := client.EnsureRunning(startCtx)
	startCancel()
	if startErr != nil {
		return installDetail, localizedBootstrapError(cfg,
			"Ollama wurde installiert, konnte aber nicht gestartet werden: %w",
			"Ollama was installed but could not be started: %w", startErr)
	}
	reportBootstrap(cfg, reporter, 22, "Ollama ist installiert und erreichbar.", "Ollama is installed and reachable.", client.BaseURL)
	return strings.TrimSpace(installDetail + "\nOllama reachable at " + client.BaseURL), nil
}

func ensureConfiguredModels(ctx context.Context, cfg Config, client *OllamaClient, models []ModelInfo) ([]ModelInfo, []string, error) {
	return ensureConfiguredModelsWithProgress(ctx, cfg, client, models, nil)
}

func ensureConfiguredModelsWithProgress(ctx context.Context, cfg Config, client *OllamaClient, models []ModelInfo, reporter BootstrapReporter) ([]ModelInfo, []string, error) {
	reportBootstrap(cfg, reporter, 28, "Installierte Ollama-Modelle werden geprüft …", "Checking installed Ollama models …", fmt.Sprintf("%d model(s) found", len(models)))
	missing := requiredOllamaModels(cfg, models)
	if len(missing) == 0 {
		reportBootstrap(cfg, reporter, 68, "Alle benötigten Modelle sind vorhanden.", "All required models are installed.", "")
		return models, nil, nil
	}
	if !cfg.OllamaAutoPull {
		return models, nil, localizedBootstrapError(cfg,
			"Benötigte Ollama-Modelle fehlen und der automatische Modelldownload ist deaktiviert: %s",
			"Required Ollama models are missing and automatic model download is disabled: %s", strings.Join(missing, ", "))
	}
	if !cfg.SetupDownloadsEnabled {
		return models, nil, localizedBootstrapError(cfg,
			"Benötigte Ollama-Modelle fehlen, aber Downloads für die automatische Einrichtung sind deaktiviert: %s",
			"Required Ollama models are missing, but downloads for automatic setup are disabled: %s", strings.Join(missing, ", "))
	}
	details := []string{}
	for index, model := range missing {
		log.Printf("Pulling required Ollama model %s", model)
		basePercent := 30 + (index * 36 / len(missing))
		span := 36 / len(missing)
		reportBootstrap(cfg, reporter, basePercent, "Ollama-Modell wird geladen …", "Downloading Ollama model …", model)
		pullCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		var pullProgress func(status string, completed, total int64)
		if reporter != nil {
			pullProgress = func(status string, completed, total int64) {
				percent := basePercent
				if total > 0 && completed >= 0 {
					percent += int(float64(span) * (float64(completed) / float64(total)))
				}
				detail := model
				if strings.TrimSpace(status) != "" {
					detail += " · " + strings.TrimSpace(status)
				}
				if total > 0 {
					detail += fmt.Sprintf(" · %.1f / %.1f GB", float64(completed)/(1<<30), float64(total)/(1<<30))
				}
				reportBootstrap(cfg, reporter, percent, "Ollama-Modell wird geladen …", "Downloading Ollama model …", detail)
			}
		}
		err := client.PullWithProgress(pullCtx, model, pullProgress)
		cancel()
		if err != nil {
			return models, details, localizedBootstrapError(cfg,
				"Der automatische Download des Modells %s ist fehlgeschlagen: %w",
				"Automatic model download failed for %s: %w", model, err)
		}
		details = append(details, "Installed Ollama model: "+model)
	}
	reportBootstrap(cfg, reporter, 68, "Modelldownload abgeschlossen; Bestand wird geprüft …", "Model download completed; verifying models …", "")
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	refreshed, err := client.Tags(refreshCtx)
	cancel()
	if err != nil {
		return models, details, localizedBootstrapError(cfg,
			"Die Modelle wurden geladen, konnten danach aber nicht aufgelistet werden: %w",
			"Models were downloaded but could not be enumerated: %w", err)
	}
	return refreshed, details, nil
}

func ensureSelectedEditingEngineRuntime(ctx context.Context, cfg Config) (Config, string, error) {
	return ensureSelectedEditingEngineRuntimeWithProgress(ctx, cfg, nil)
}

func ensureSelectedEditingEngineRuntimeWithProgress(ctx context.Context, cfg Config, reporter BootstrapReporter) (Config, string, error) {
	engine := normalizeEditingEngine(cfg.EditingEngine)
	displayName := codingEngineDisplayName(engine)
	reportBootstrap(cfg, reporter, 74, "Coding-Agent-Engine wird geprüft …", "Checking coding-agent engine …", displayName)
	if engine == editingEngineNative {
		reportBootstrap(cfg, reporter, 94, "Die native LocalCode-Engine ist bereit.", "The native LocalCode engine is ready.", "")
		return cfg, "LocalCode native editing engine is ready", nil
	}
	if !codingEngineEnabled(cfg, engine) {
		cfg.EditingEngine = editingEngineNative
		detail := localizeConfigText(cfg,
			displayName+" ist deaktiviert; LocalCode verwendet stattdessen die native Engine.",
			displayName+" is disabled; LocalCode is using the native engine instead.")
		reportBootstrap(cfg, reporter, 94,
			"Die ausgewählte Engine ist deaktiviert; native Engine wird verwendet.",
			"The selected engine is disabled; using the native engine.", displayName)
		return normalizeConfig(cfg), detail, nil
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
		reportBootstrap(cfg, reporter, 94, "Coding-Agent-Engine ist bereit.", "Coding-agent engine is ready.", status.DisplayName+" "+status.Version)
		return normalizeConfig(cfg), detail, nil
	}
	if !codingEngineAutoInstall(cfg, engine) {
		return cfg, status.Error, &CodingEngineNotInstalledError{Status: status}
	}
	if !cfg.SetupDownloadsEnabled {
		return cfg, status.Error, localizedBootstrapError(cfg,
			"%s fehlt, aber Downloads für die automatische Einrichtung sind deaktiviert.",
			"%s is missing, but downloads for automatic setup are disabled.", status.DisplayName)
	}
	reportBootstrap(cfg, reporter, 78, "Coding-Agent-Engine wird installiert …", "Installing coding-agent engine …", status.DisplayName)
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
	reportBootstrap(cfg, reporter, 94, "Coding-Agent-Engine wurde installiert.", "Coding-agent engine was installed.", status.DisplayName+" "+status.Version)
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
	return bootstrapRuntimeDependenciesWithProgress(ctx, cfg, nil)
}

func bootstrapRuntimeDependenciesWithProgress(ctx context.Context, cfg Config, reporter BootstrapReporter) (RuntimeBootstrapResult, error) {
	result := RuntimeBootstrapResult{Config: cfg, Ollama: NewOllamaClient()}
	reportBootstrap(cfg, reporter, 2, "LocalCode wird vorbereitet …", "Preparing LocalCode …", "")
	if cfg.OllamaURL != "" {
		result.Ollama.BaseURL = cfg.OllamaURL
	}
	result.Ollama.ContextLength = cfg.ContextLength

	detail, err := ensureOllamaInstalledAndRunningWithProgress(ctx, cfg, result.Ollama, reporter)
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
		return result, localizedBootstrapError(cfg,
			"Die installierten Ollama-Modelle konnten nicht gelesen werden: %w",
			"Ollama models could not be read: %w", err)
	}
	models, modelDetails, err := ensureConfiguredModelsWithProgress(ctx, cfg, result.Ollama, models, reporter)
	result.Details = append(result.Details, modelDetails...)
	if err != nil {
		return result, err
	}
	if len(models) == 0 {
		return result, localizedBootstrapError(cfg,
			"Nach der automatischen Einrichtung ist kein Ollama-Modell installiert.",
			"Ollama has no installed model after automatic setup.")
	}
	result.Models = models
	result.Config.LastModel = chooseDefaultModel(models, result.Config.LastModel)

	updated, engineDetail, err := ensureSelectedEditingEngineRuntimeWithProgress(ctx, result.Config, reporter)
	if engineDetail != "" {
		result.Details = append(result.Details, engineDetail)
	}
	if err != nil {
		return result, localizedBootstrapError(cfg,
			"Die Einrichtung der Coding-Agent-Engine ist fehlgeschlagen: %w",
			"Coding-agent engine setup failed: %w", err)
	}
	result.Config = updated
	reportBootstrap(result.Config, reporter, 100, "LocalCode ist startbereit.", "LocalCode is ready to start.", "")
	return result, nil
}
