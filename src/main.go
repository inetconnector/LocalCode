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

	ollama := NewOllamaClient()
	if cfg.OllamaURL != "" {
		ollama.BaseURL = cfg.OllamaURL
	}
	ollama.ContextLength = cfg.ContextLength
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	err := ollama.EnsureRunning(ctx)
	cancel()
	if err != nil {
		log.Printf("Ollama unavailable: %v", err)
		showFatal("LocalCode", "Ollama konnte nicht gestartet oder erreicht werden.\n\n"+err.Error()+"\n\nLog: "+logPath())
		return
	}

	models, err := ollama.Tags(context.Background())
	if err != nil {
		showFatal("LocalCode", "Ollama-Modelle konnten nicht gelesen werden:\n\n"+err.Error())
		return
	}
	if len(models) == 0 {
		showFatal("LocalCode", "In Ollama ist kein Modell installiert. Installiere mindestens gpt-oss:20b oder qwen2.5-coder:14b.")
		return
	}

	cfg.LastModel = chooseDefaultModel(models, cfg.LastModel)
	if cfg.LastProject != "" {
		if full, err := ensureWithinRoot(cfg.RootProjectDir, cfg.LastProject); err != nil {
			cfg.LastProject = ""
		} else if info, err := os.Stat(full); err != nil || !info.IsDir() {
			cfg.LastProject = ""
		}
	}
	_ = saveConfig(cfg)

	state := NewAppState(cfg, ollama)
	url, err := startHTTPServer(state, cfg.Port)
	if err != nil {
		showFatal("LocalCode", fmt.Sprintf("Lokaler Server konnte nicht gestartet werden:\n\n%v", err))
		return
	}
	log.Printf("LocalCode %s started at %s using Ollama %s", version, url, ollama.BaseURL)
	if err := openBrowser(url); err != nil {
		showFatal("LocalCode", "Browser konnte nicht geöffnet werden. Öffne manuell:\n"+url)
	}

	select {}
}

func runDiagnostics() int {
	fmt.Println("LocalCode Diagnose")
	fmt.Println("Version:", version)
	fmt.Println("Konfiguration:", configPath())
	fmt.Println("Log:", logPath())
	fmt.Println("Projektwurzel:", loadConfig().RootProjectDir)
	fmt.Println("OLLAMA_HOST:", os.Getenv("OLLAMA_HOST"))
	fmt.Println("Geprüfte Ollama-Adressen:", strings.Join(ollamaCandidates(), ", "))
	fmt.Println("Ollama.exe:", findOllamaExecutable())
	fmt.Println("GPU:", detectGPU())

	client := NewOllamaClient()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Discover(ctx); err != nil {
		fmt.Println("FEHLER: Ollama nicht erreichbar:", err)
		return 1
	}
	fmt.Println("Ollama erreichbar:", client.BaseURL)
	models, err := client.Tags(ctx)
	if err != nil {
		fmt.Println("FEHLER beim Lesen der Modelle:", err)
		return 1
	}
	if len(models) == 0 {
		fmt.Println("FEHLER: Keine Modelle installiert.")
		return 1
	}
	fmt.Println("Modelle:")
	for _, model := range models {
		fmt.Println(" -", model.Name)
	}
	diagCfg := loadConfig()
	fmt.Println("Standardmodell:", chooseDefaultModel(models, diagCfg.LastModel))
	fmt.Println("Approval-Modus:", diagCfg.ApprovalMode)
	fmt.Println("Sandbox-Modus:", diagCfg.SandboxMode)
	fmt.Println("Netzwerk:", diagCfg.NetworkEnabled, "Websuche:", diagCfg.WebSearchProvider)
	fmt.Println("Git verfügbar/aktiv:", gitAvailable(), diagCfg.GitEnabled)
	fmt.Println("STATE.md automatisch:", diagCfg.AutoStateUpdate, diagCfg.StateFile)
	fmt.Println("Aktive MCP-Server:", enabledMCPCount(diagCfg))
	fmt.Println("Automatische Werkzeugerkennung:", diagCfg.AutoDiscoverTools)
	fmt.Println("Automatische offizielle Werkzeughilfe:", diagCfg.AutoResearchToolHelp)
	fmt.Println("Werkzeuge:")
	for _, tool := range toolInventory(diagCfg.LastProject, diagCfg, true) {
		status := "NICHT GEFUNDEN"
		if tool.Available {
			status = "OK: " + tool.Path
		}
		fmt.Printf(" - %s: %s\n", tool.Name, status)
		for _, line := range tool.Diagnostics {
			fmt.Println("   Diagnose:", line)
		}
	}
	fmt.Println("Diagnose erfolgreich.")
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
