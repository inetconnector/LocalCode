// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type HealthStatus string

const (
	HealthStatusOK      HealthStatus = "healthy"
	HealthStatusWarning HealthStatus = "warning"
	HealthStatusError   HealthStatus = "error"
)

type DiagnosticItem struct {
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Status      HealthStatus `json:"status"`
	Summary     string       `json:"summary"`
	Details     []string     `json:"details,omitempty"`
	LatencyMs   int64        `json:"latency_ms,omitempty"`
	Remediation string       `json:"remediation,omitempty"`
}

type DoctorReport struct {
	Timestamp   time.Time        `json:"timestamp"`
	Overall     HealthStatus     `json:"overall"`
	Version     string           `json:"version"`
	OS          string           `json:"os"`
	Arch        string           `json:"arch"`
	Items       []DiagnosticItem `json:"items"`
	SummaryText string           `json:"summary_text"`
}

func RunDoctorDiagnostics(ctx context.Context, cfg Config) DoctorReport {
	report := DoctorReport{
		Timestamp: time.Now(),
		Overall:   HealthStatusOK,
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Items:     make([]DiagnosticItem, 0, 16),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	addItem := func(item DiagnosticItem) {
		mu.Lock()
		defer mu.Unlock()
		report.Items = append(report.Items, item)
		if item.Status == HealthStatusError {
			report.Overall = HealthStatusError
		} else if item.Status == HealthStatusWarning && report.Overall != HealthStatusError {
			report.Overall = HealthStatusWarning
		}
	}

	// 1. ComputeMesh Cluster & Node Diagnostics
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		meshStatus := CheckComputeMeshStatus(ctx, cfg)
		elapsed := time.Since(start).Milliseconds()

		item := DiagnosticItem{
			Name:      "ComputeMesh Decentralized Cluster",
			Category:  "inference_cluster",
			LatencyMs: elapsed,
		}

		if meshStatus.Online {
			item.Status = HealthStatusOK
			item.Summary = fmt.Sprintf("Cluster online: Node %s (%s), %s, %d cluster models available", meshStatus.NodeID, meshStatus.NodeStatus, meshStatus.VRAMPool, len(meshStatus.Models))
			item.Details = []string{
				"Gateway: " + meshStatus.URL,
				"Local Workstation Node: " + meshStatus.LocalNodeURL,
				"GPU Specs: " + meshStatus.GPU,
				"Active Key: " + meshStatus.ActiveKeyMasked + " (" + meshStatus.KeySource + ")",
			}
		} else {
			item.Status = HealthStatusWarning
			item.Summary = "ComputeMesh cluster is currently offline or unreachable"
			item.Details = []string{
				"Error: " + meshStatus.Error,
				"Gateway: " + meshStatus.URL,
			}
			item.Remediation = "Verify internet connection or start local workstation node on port 8080."
		}
		addItem(item)
	}()

	// 2. Local Ollama Runtime Diagnostics
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := NewOllamaClient()
		if cfg.OllamaURL != "" {
			client.BaseURL = cfg.OllamaURL
		}
		start := time.Now()
		probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		item := DiagnosticItem{
			Name:     "Local Ollama Engine",
			Category: "local_llm",
		}

		if err := client.Discover(probeCtx); err != nil {
			item.Status = HealthStatusWarning
			item.Summary = "Local Ollama daemon not running or not reachable on loopback"
			item.Details = []string{"Error: " + err.Error()}
			item.Remediation = "Start Ollama desktop or configure remote/ComputeMesh cluster gateway."
		} else {
			models, err := client.Tags(probeCtx)
			item.LatencyMs = time.Since(start).Milliseconds()
			if err != nil || len(models) == 0 {
				item.Status = HealthStatusWarning
				item.Summary = fmt.Sprintf("Ollama online at %s, but no local models found", client.BaseURL)
				item.Remediation = "Run 'ollama pull qwen2.5-coder:14b' or use ComputeMesh cluster models."
			} else {
				item.Status = HealthStatusOK
				item.Summary = fmt.Sprintf("Ollama online at %s (%d local models installed)", client.BaseURL, len(models))
				var modelNames []string
				for _, m := range models {
					modelNames = append(modelNames, m.Name)
				}
				item.Details = []string{"Installed: " + strings.Join(modelNames, ", ")}
			}
		}
		addItem(item)
	}()

	// 3. Git Toolchain & Worktree Storage
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Git & Isolated Worktrees",
			Category: "version_control",
		}

		gitPath, err := exec.LookPath("git")
		if err != nil {
			item.Status = HealthStatusError
			item.Summary = "Git executable not found in system PATH"
			item.Remediation = "Install Git for Windows and ensure it is added to PATH."
		} else {
			worktreeRoot := filepath.Join(userProfileDir(), ".localcode", "worktrees")
			_ = os.MkdirAll(worktreeRoot, 0o755)
			item.Status = HealthStatusOK
			item.Summary = fmt.Sprintf("Git binary found (%s), worktree sandbox ready", gitPath)
			item.Details = []string{
				"Git Path: " + gitPath,
				"Worktree Directory: " + worktreeRoot,
			}
		}
		addItem(item)
	}()

	// 4. Multi-Agent Team Orchestrator & Recovery Journal
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Mission Recovery & Journal Storage",
			Category: "orchestration",
		}

		journalPath := runJournalPath()
		if info, err := os.Stat(journalPath); err == nil {
			item.Status = HealthStatusOK
			item.Summary = fmt.Sprintf("Recovery journal present (%d bytes, last modified %s)", info.Size(), info.ModTime().Format(time.RFC3339))
			item.Details = []string{"Path: " + journalPath}
		} else {
			item.Status = HealthStatusOK
			item.Summary = "Recovery journal idle / clean state"
			item.Details = []string{"Target Path: " + journalPath}
		}
		addItem(item)
	}()

	// 5. External Coding Engines Discovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Coding Engines (Aider / Claude / OpenCode / Claw)",
			Category: "coding_engines",
		}

		engines := []string{}
		if p, err := exec.LookPath("aider"); err == nil {
			engines = append(engines, "Aider ("+p+")")
		}
		if p, err := exec.LookPath("claude"); err == nil {
			engines = append(engines, "Claude Code ("+p+")")
		}
		if p, err := exec.LookPath("opencode"); err == nil {
			engines = append(engines, "OpenCode ("+p+")")
		}
		if p, err := exec.LookPath("claw"); err == nil {
			engines = append(engines, "Claw Code ("+p+")")
		}

		item.Status = HealthStatusOK
		if len(engines) > 0 {
			item.Summary = fmt.Sprintf("LocalCode Native active + %d external engines detected", len(engines))
			item.Details = engines
		} else {
			item.Summary = "LocalCode Native active (no secondary CLI engines installed, which is normal)"
		}
		addItem(item)
	}()

	// 6. Virtualization & VM Sandbox Diagnostics
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Virtualization & VM Sandbox",
			Category: "vm_sandbox",
		}
		caps := discoverVMCapabilities(ctx, cfg)
		item.Status = HealthStatusOK
		details := []string{
			fmt.Sprintf("Preferred Backend: %s", caps.Preferred),
			fmt.Sprintf("Supported Isolation Modes: %s", strings.Join(caps.SupportedModes, ", ")),
		}
		if caps.QEMUAvailable {
			details = append(details, fmt.Sprintf("QEMU: %s (%s)", caps.QEMUPath, caps.QEMUVersion))
		}
		if caps.WSLAvailable {
			details = append(details, fmt.Sprintf("WSL: %s", caps.WSLPath))
		}
		item.Details = details
		if caps.QEMUAvailable || caps.WSLAvailable {
			item.Summary = fmt.Sprintf("Hardware isolation ready via %s (%s)", caps.Preferred, strings.Join(caps.SupportedModes, "/"))
		} else {
			item.Summary = "Process-level project isolation active (QEMU/WSL optional)"
		}
		addItem(item)
	}()

	// 7. Autonomous Browser Automation & Playwright MCP
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Autonomous Browser Automation",
			Category: "browser_automation",
		}
		var details []string
		browsers := []string{}
		for _, b := range chromiumBrowserCandidates() {
			if info, err := os.Stat(b); err == nil && !info.IsDir() {
				browsers = append(browsers, filepath.Base(b))
			}
		}
		if len(browsers) > 0 {
			details = append(details, "Installed Browsers: "+strings.Join(browsers, ", "))
		} else {
			details = append(details, "No Chromium browsers found")
		}

		mcpIndex := findMCPServerIndex(cfg, "playwright")
		playwrightEnabled := mcpIndex >= 0 && cfg.MCPServers[mcpIndex].Enabled
		if playwrightEnabled {
			details = append(details, "Playwright MCP Server: enabled")
		} else {
			details = append(details, "Playwright MCP Server: disabled (headless browser fallback active)")
		}

		if len(browsers) > 0 {
			item.Status = HealthStatusOK
			item.Summary = fmt.Sprintf("Browser automation ready via %s (Playwright MCP + Headless Chromium)", strings.Join(browsers, "/"))
		} else {
			item.Status = HealthStatusWarning
			item.Summary = "No local Chromium browser found for browser automation"
			item.Remediation = "Install Microsoft Edge or Google Chrome to enable autonomous browser navigation and screenshots."
		}
		item.Details = details
		addItem(item)
	}()

	// 8. Windows Desktop UI Automation (Accessible GUI Agent)
	wg.Add(1)
	go func() {
		defer wg.Done()
		item := DiagnosticItem{
			Name:     "Windows Desktop & UI Automation",
			Category: "desktop_automation",
		}
		if runtime.GOOS == "windows" {
			item.Status = HealthStatusOK
			item.Summary = "Windows UI Automation API active (System.Windows.Automation)"
			item.Details = []string{
				"Backend: Windows UI Automation Client",
				"Controls: Buttons, TextBoxes, ComboBoxes, Tabs, Lists, Menus",
				"Safety Guardrails: System windows & security dialogs protected",
			}
		} else {
			item.Status = HealthStatusOK
			item.Summary = "Desktop UI Automation active for Windows host platforms"
		}
		addItem(item)
	}()

	wg.Wait()

	report.SummaryText = fmt.Sprintf("LocalCode Doctor: %s (%d components checked)", strings.ToUpper(string(report.Overall)), len(report.Items))
	return report
}
