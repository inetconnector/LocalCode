// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && strings.EqualFold(os.Args[1], "--diagnose") {
		os.Exit(runDiagnostics())
	}

	if err := os.MkdirAll(filepath.Dir(logPath()), 0o755); err == nil {
		if f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			defer f.Close()
			log.SetOutput(f)
		}
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfg := loadConfig()
	defaultURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
	if runningVersion, ok := existingLocalCodeVersion(defaultURL); ok {
		if runningVersion == version {
			_ = openBrowser(defaultURL)
			return
		}
		log.Printf("Stopping older LocalCode instance %q before starting %q", runningVersion, version)
		_ = shutdownExistingLocalCode(defaultURL)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, stillRunning := existingLocalCodeVersion(defaultURL); !stillRunning {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
	}

	var setup RuntimeBootstrapResult
	var err error
	limitedMode := false
	fastStart := fastStartupRequested()
	var splash *startupSplash
	if fastStart {
		setup = fastStartupBootstrap(cfg)
		log.Printf("Fast startup enabled; runtime dependency checks are deferred to status/doctor and first use")
	} else if started, splashErr := startStartupSplash(cfg, version); splashErr != nil {
		log.Printf("Startup splash could not be started: %v", splashErr)
	} else if browserErr := openStartupBrowser(started.URL()); browserErr != nil {
		log.Printf("Startup splash browser could not be opened: %v", browserErr)
		started.Close()
	} else {
		splash = started
	}
bootstrapLoop:
	for {
		if fastStart {
			break
		}
		setupCtx, setupCancel := context.WithTimeout(context.Background(), 3*time.Hour)
		reporter := BootstrapReporter(nil)
		if splash != nil {
			reporter = splash.Update
		}
		setup, err = bootstrapRuntimeDependenciesWithProgress(setupCtx, cfg, reporter)
		setupCancel()
		if err == nil {
			break
		}
		log.Printf("Automatic runtime setup failed: %v; details: %s", err, strings.Join(setup.Details, " | "))
		if splash == nil {
			showFatal("LocalCode", localizeConfigText(cfg,
				"Die automatische Einrichtung von Ollama, Modellen oder der ausgewählten Coding-Engine ist fehlgeschlagen.",
				"Automatic setup of Ollama, models, or the selected coding engine failed.")+"\n\n"+err.Error()+"\n\nLog: "+logPath())
			return
		}
		splash.Fail(err)
		actionCtx, actionCancel := context.WithTimeout(context.Background(), 24*time.Hour)
		action := splash.WaitAction(actionCtx)
		actionCancel()
		switch action {
		case "retry":
			splash.Reset()
			cfg = loadConfig()
			continue
		case "continue":
			limitedMode = true
			if setup.Config.SchemaVersion == 0 {
				setup.Config = cfg
			}
			if setup.Ollama == nil {
				setup.Ollama = NewOllamaClient()
				if cfg.OllamaURL != "" {
					setup.Ollama.BaseURL = cfg.OllamaURL
				}
			}
			break bootstrapLoop
		default:
			splash.Close()
			return
		}
	}
	cfg = setup.Config
	ollama := setup.Ollama
	for _, detail := range setup.Details {
		log.Printf("Runtime setup: %s", detail)
	}
	if cfg.LastProject != "" {
		if full, err := ensureWithinRoot(cfg.RootProjectDir, cfg.LastProject); err != nil {
			cfg.LastProject = ""
		} else if info, err := os.Stat(full); err != nil || !info.IsDir() {
			cfg.LastProject = ""
		}
	}
	_ = saveConfig(cfg)

	state := NewAppState(cfg, ollama)
	ConfigureComputeMeshForAppState(state)
	url, err := startHTTPServer(state, cfg.Port)
	if err != nil {
		if splash != nil {
			localizedErr := fmt.Errorf(localizeConfigText(cfg, "Lokaler Server konnte nicht gestartet werden: %w", "The local server could not be started: %w"), err)
			splash.Fail(localizedErr)
			time.Sleep(250 * time.Millisecond)
			showFatal("LocalCode", localizedErr.Error()+"\n\nLog: "+logPath())
			splash.Close()
		} else {
			showFatal("LocalCode", fmt.Sprintf(localizeConfigText(cfg, "Lokaler Server konnte nicht gestartet werden:\n\n%v", "The local server could not be started:\n\n%v"), err))
		}
		return
	}
	if limitedMode {
		log.Printf("LocalCode %s started in limited mode at %s; Ollama target %s", version, url, ollama.BaseURL)
	} else {
		log.Printf("LocalCode %s started at %s using Ollama %s", version, url, ollama.BaseURL)
	}
	if remoteURLs, remoteErr := startMobileSafeProductionRemoteServer(state, cfg); remoteErr != nil {
		log.Printf("LocalCode Remote server could not be started: %v", remoteErr)
	} else if len(remoteURLs) > 0 {
		log.Printf("LocalCode Remote started: %s", strings.Join(remoteURLs, ", "))
	}
	if splash != nil {
		splash.Complete(url)
		time.AfterFunc(30*time.Second, splash.Close)
	} else if err := openBrowser(url); err != nil {
		showFatal("LocalCode", localizeConfigText(cfg, "Browser konnte nicht geöffnet werden. Öffne manuell:", "The browser could not be opened. Open this address manually:")+"\n"+url)
	}

	select {}
}

func fastStartupRequested() bool {
	value := strings.TrimSpace(os.Getenv("LOCALCODE_FAST_START"))
	return value != "" && value != "0" && !strings.EqualFold(value, "false")
}

func fastStartupBootstrap(cfg Config) RuntimeBootstrapResult {
	client := NewOllamaClient()
	if cfg.OllamaURL != "" {
		client.BaseURL = cfg.OllamaURL
	}
	client.ContextLength = cfg.ContextLength
	return RuntimeBootstrapResult{
		Config:  cfg,
		Ollama:  client,
		Details: []string{"Fast startup: dependency checks deferred"},
	}
}

func runDiagnostics() int {
	cfg := loadConfig()
	tr := func(de, en string) string { return localizeConfigText(cfg, de, en) }
	fmt.Println(tr("LocalCode Diagnose", "LocalCode diagnostics"))
	fmt.Println(tr("Version:", "Version:"), version)
	fmt.Println(tr("Konfiguration:", "Configuration:"), configPath())
	fmt.Println(tr("Log:", "Log:"), logPath())
	fmt.Println(tr("Projektwurzel:", "Project root:"), cfg.RootProjectDir)
	fmt.Println("OLLAMA_HOST:", os.Getenv("OLLAMA_HOST"))
	fmt.Println(tr("Geprüfte Ollama-Adressen:", "Checked Ollama addresses:"), strings.Join(ollamaCandidates(), ", "))
	fmt.Println("Ollama.exe:", findOllamaExecutable())
	fmt.Println("GPU:", detectGPU())

	client := NewOllamaClient()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Discover(ctx); err != nil {
		fmt.Println(tr("FEHLER: Ollama nicht erreichbar:", "ERROR: Ollama is not reachable:"), err)
		return 1
	}
	fmt.Println(tr("Ollama erreichbar:", "Ollama reachable:"), client.BaseURL)
	models, err := client.Tags(ctx)
	if err != nil {
		fmt.Println(tr("FEHLER beim Lesen der Modelle:", "ERROR while reading models:"), err)
		return 1
	}
	if len(models) == 0 {
		fmt.Println(tr("FEHLER: Keine Modelle installiert.", "ERROR: No models installed."))
		return 1
	}
	fmt.Println(tr("Modelle:", "Models:"))
	for _, model := range models {
		fmt.Println(" -", model.Name)
	}
	diagCfg := cfg
	fmt.Println(tr("Standardmodell:", "Default model:"), chooseDefaultModel(models, diagCfg.LastModel))
	fmt.Println(tr("Approval-Modus:", "Approval mode:"), diagCfg.ApprovalMode)
	fmt.Println(tr("Sandbox-Modus:", "Sandbox mode:"), diagCfg.SandboxMode)
	fmt.Println(tr("Netzwerk:", "Network:"), diagCfg.NetworkEnabled, tr("Websuche:", "Web search:"), diagCfg.WebSearchProvider)
	fmt.Println(tr("Setup-Downloads:", "Setup downloads:"), diagCfg.SetupDownloadsEnabled)
	fmt.Println(tr("Git verfügbar/aktiv:", "Git available/enabled:"), gitAvailable(diagCfg.LastProject, diagCfg), diagCfg.GitEnabled)
	fmt.Println(tr("STATE.md automatisch:", "Automatic STATE.md:"), diagCfg.AutoStateUpdate, diagCfg.StateFile)
	fmt.Println(tr("Aktive MCP-Server:", "Active MCP servers:"), enabledMCPCount(diagCfg))
	fmt.Println(tr("Automatische Werkzeugerkennung:", "Automatic tool discovery:"), diagCfg.AutoDiscoverTools)
	fmt.Println(tr("Automatische offizielle Werkzeughilfe:", "Automatic official tool help:"), diagCfg.AutoResearchToolHelp)
	fmt.Println(tr("Werkzeuge:", "Tools:"))
	for _, tool := range toolInventory(diagCfg.LastProject, diagCfg, true) {
		status := tr("NICHT GEFUNDEN", "NOT FOUND")
		if tool.Available {
			status = "OK: " + tool.Path
		}
		fmt.Printf(" - %s: %s\n", tool.Name, status)
		for _, line := range tool.Diagnostics {
			fmt.Println(tr("   Diagnose:", "   Diagnostic:"), line)
		}
	}
	fmt.Println(tr("Diagnose erfolgreich.", "Diagnostics completed successfully."))
	return 0
}

func existingLocalCodeVersion(baseURL string) (string, bool) {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	resp, err := client.Get(baseURL + "/api/ping")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var v struct {
		App     string `json:"app"`
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil || (v.App != "LocalCode" && v.App != legacyProductDirName) {
		return "", false
	}
	return v.Version, true
}

func shutdownExistingLocalCode(baseURL string) error {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/shutdown", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown returned HTTP %d", resp.StatusCode)
	}
	return nil
}
