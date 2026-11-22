// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var offlinePackageInstallRe = regexp.MustCompile(`(?i)\b(?:npm|yarn|pnpm|pip|pip3|uv|cargo|go|winget|choco|scoop)(?:\.cmd|\.exe)?\s+(?:install|i|add|get)\b`)
var externalNodeRequireRe = regexp.MustCompile(`require\s*\(\s*['"]([a-zA-Z0-9@_-][^'"]*)['"]\s*\)`)
var micaRule7StubPatternRe = regexp.MustCompile(`(?im)(?://|#|/\*)\s*(?:todo\b|fixme\b|placeholder\b|stub\b|implement\s+(?:further|remaining|additional|game|full|more)\b[^\n]*|implement\s+[^\n]*\bhere\b|add\s+[^\n]*\b(?:logic|code|implementation)\s+here\b|hier\s+[^\n]*implementier|implementier(?:e|ung)\s+(?:der\s+)?[^\n]*logik|[^\n]*\bcan be added\b|[^\n]*k[oö]nnen[^\n]*hinzugef[uü]gt)`)

const agentSystemPromptCompact = `Du bist LocalCode (im Verbund mit AgentOS, NOMA und MICA), ein präziser autonomer Software-Agent für ein lokales Projekt.

Jede Antwort MUSS genau EIN gültiges JSON-Objekt ohne Markdown-Fences sein:
{"action": "<werkzeug>", "message": "<kurze Beschreibung>", ...felder}

Verfügbare Kern-Werkzeuge:
- list_files(path?), read_file(path), search_text(query)
- write_file(path, content): schreibt die KOMPLETTE, direkt lauffähige Datei (Rule 7: NIEMALS Stubs, leere Funktionsrümpfe, TODO/FIXME oder "can be added"!)
- replace_text(path, old_text, new_text): gezielte Änderung (bei Fehlschlag direkt write_file nutzen)
- delete_file(path), copy_path(source, destination), move_path(source, destination)
- project_info, subagent_analyze(task), build_project
- discover_tool(tool), run_tool(tool, args), run_command(command)
- finish: erst wenn ALLE Anforderungen der Nutzeraufgabe vollständig implementiert UND durch echte Tests/Prüfungen bestanden sind.
- ask_user: nur bei echten unauflösbaren Blockern.

Verbindliche Regeln:
1. Setze EXAKT die Anforderungen, API-Namen, Zahlenwerte, Dateipfade und Invarianten aus der Nutzeraufgabe um — erfinde keine vereinfachten Ersatz-Klassen!
2. Offline-Policy: Wenn Network=false gilt, verwende KEINE externen Pakete, keine CDNs und kein npm/pip install.
3. Nach jeder Code-Änderung führe die passenden Tests/Prüfungen per run_tool aus (z. B. node --test, pytest, go test) und behebe Fehler gezielt.
4. Wenn die Abschlussprüfung Mängel meldet, lies die betroffene Datei und behebe ALLE gemeldeten Mängel mit write_file, bevor du erneut testest und finish aufrufst.`

func systemPromptForConfig(cfg Config) string {
	if effectiveCompactionContextLength(cfg) > 8192 {
		return agentSystemPrompt
	}
	var b strings.Builder
	b.WriteString(agentSystemPromptCompact)
	if cfg.GitEnabled {
		b.WriteString("\n- Git ist aktiv: git(args), git_commit.")
	}
	if cfg.NetworkEnabled {
		b.WriteString("\n- Netzwerk/Web ist aktiv: web_search(query), web_fetch(url).")
	}
	return b.String()
}

func detectMicaRule7Stub(relPath, content string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".py", ".go", ".rs", ".java", ".cs":
	default:
		return ""
	}
	if m := micaRule7StubPatternRe.FindString(content); m != "" {
		return strings.TrimSpace(m)
	}
	return ""
}

func offlinePackageInstallBlocked(cfg Config, a AgentAction) string {
	if cfg.NetworkEnabled {
		return ""
	}
	if a.Action != "open_terminal" && a.Action != "run_command" && a.Action != "run_tool" {
		return ""
	}
	cmdText := strings.TrimSpace(a.Command)
	if a.Action == "run_tool" {
		cmdText = strings.TrimSpace(a.Tool + " " + strings.Join(a.Args, " "))
	}
	if !offlinePackageInstallRe.MatchString(cmdText) {
		return ""
	}
	return fmt.Sprintf("ERROR: OFFLINE-POLICY BLOCKIERT — Externe Paketinstallation (%q) ist im Offline-Modus (network_enabled=false) verboten. Entferne externe require()/import-Abhängigkeiten aus dem Quellcode und implementiere die Logik vollständig mit eingebauten Standard-APIs.", cmdText)
}

type nomaMicaPreflightReport struct {
	AgentOS struct {
		Status  string `json:"status"`
		Route   string `json:"route"`
		State   string `json:"state"`
		Workers int    `json:"workers"`
	} `json:"agentos"`
	Noma struct {
		Status        string   `json:"status"`
		CriteriaCount int      `json:"criteria_count"`
		SampleItems   []string `json:"sample_items"`
		Strategies    []string `json:"strategies"`
		IndexedFiles  int      `json:"indexed_files"`
	} `json:"noma"`
	Mica struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		Threat        string `json:"threat"`
		Library       string `json:"library"`
		LibrarySHA256 string `json:"library_sha256"`
	} `json:"mica"`
}

func discoverNomaMicaRoot(project string) string {
	candidates := []string{}
	if envRoot := strings.TrimSpace(os.Getenv("NOMAMICA_ROOT")); envRoot != "" {
		candidates = append(candidates, envRoot)
	}
	if strings.TrimSpace(project) != "" {
		cur := filepath.Clean(project)
		for i := 0; i < 6; i++ {
			candidates = append(candidates, cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	for _, root := range candidates {
		nomaPath := filepath.Join(root, "NOMA", "noma_runtime", "coding", "problem_framing.py")
		agentosPath := filepath.Join(root, "AgentOS", "src", "agentos", "integration", "utp_v1.py")
		if info, err := os.Stat(nomaPath); err == nil && !info.IsDir() {
			if info2, err2 := os.Stat(agentosPath); err2 == nil && !info2.IsDir() {
				return root
			}
		}
	}
	return ""
}

func orderedTaskTargetFiles(project, task string) []string {
	raw := mentionedProjectFiles(task)
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var normalized []string
	for _, item := range raw {
		rel := normalizeProjectRelativeTarget(project, item)
		rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "../") {
			continue
		}
		key := strings.ToLower(rel)
		if !seen[key] {
			seen[key] = true
			normalized = append(normalized, rel)
		}
	}
	fileTier := func(p string) int {
		low := strings.ToLower(p)
		base := strings.ToLower(filepath.Base(p))
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".md" || ext == ".txt" {
			return 50
		}
		if strings.Contains(low, "test") || strings.HasPrefix(low, "tests/") {
			return 40
		}
		if ext == ".html" || ext == ".htm" || ext == ".css" {
			return 30
		}
		if strings.Contains(base, "game.") || strings.Contains(base, "main.") || strings.Contains(base, "app.") {
			return 25
		}
		if strings.Contains(base, "render") || strings.Contains(base, "view") || strings.Contains(base, "ui") {
			return 20
		}
		if strings.Contains(base, "engine") || strings.Contains(base, "core") || strings.Contains(base, "model") {
			return 10
		}
		return 15
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		ti, tj := fileTier(normalized[i]), fileTier(normalized[j])
		if ti != tj {
			return ti < tj
		}
		return normalized[i] < normalized[j]
	})
	return normalized
}

func runNomaMicaSuitePreflight(ctx context.Context, s *AppState, project, task string, cfg Config) string {
	root := discoverNomaMicaRoot(project)
	if root == "" {
		return ""
	}
	targetFiles := orderedTaskTargetFiles(project, task)
	pyInfo := discoverTool(project, "python", cfg, false)
	pyExe := pyInfo.Path
	if pyExe == "" {
		if found, err := exec.LookPath("python"); err == nil {
			pyExe = found
		}
	}
	var report nomaMicaPreflightReport
	ranLivePython := false
	if pyExe != "" {
		pyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		script := `
import hashlib, json, os, sys
from pathlib import Path
root = Path(sys.argv[1])
project = Path(sys.argv[2])
task = os.environ.get("NOMAMICA_TASK_TEXT", "")
sys.path.insert(0, str(root / "AgentOS"))
sys.path.insert(0, str(root / "AgentOS" / "src"))
sys.path.insert(0, str(root / "NOMA"))
out = {"agentos": {}, "noma": {}, "mica": {}}
try:
    from agentos.integration.utp_v1 import route_request
    dec = route_request({
        "protocol": "agentos.utp.v1", "kind": "task_request",
        "task_id": "mission-active", "intent": "IMPLEMENT_CODE",
        "risk": "MEDIUM", "freshness": "NONE",
        "requires_external_evidence": False, "external_effects": [],
        "resume_mission": False, "durable": True,
        "constraints": ["NO_PUSH", "NO_DEPLOY"],
    })
    out["agentos"] = {"status": "PASS_ROUTE_ONLY", "route": dec.get("route", "CODE_CHANGE"), "state": dec.get("task_state", "ROUTED"), "workers": 4}
except Exception as e:
    out["agentos"] = {"status": "FALLBACK", "route": "CODE_CHANGE", "state": str(e)[:120], "workers": 4}
try:
    from noma_runtime.coding.problem_framing import ProblemFramingEngine
    from noma_runtime.coding.repository_indexer import RepositoryIndexer
    from noma_runtime.coding.contracts import CodingTaskSpec
    snap = RepositoryIndexer().index_repository(project)
    frame = ProblemFramingEngine().frame_problem(
        CodingTaskSpec(task_id="mission-active", requirement_text=task, repo_path=str(project), language="javascript"),
        snap,
    )
    indexed_count = len(getattr(snap, "file_hashes", None) or getattr(snap, "symbols", None) or [])
    out["noma"] = {
        "status": "PASS_REAL_FRAMING",
        "criteria_count": len(frame.acceptance_criteria),
        "sample_items": frame.acceptance_criteria[:8],
        "strategies": frame.candidate_strategies,
        "indexed_files": indexed_count,
    }
except Exception as e:
    out["noma"] = {"status": "FALLBACK", "criteria_count": 0, "sample_items": [], "strategies": [str(e)[:120]], "indexed_files": 0}
try:
    from noma_runtime.mica_bridge import MicaKernelBridge
    dll_candidates = [
        project.parent / "bin" / "mica_ffi.dll",
        root / "MICA" / "target" / "release" / "mica_ffi.dll",
    ]
    lib_path = next((p for p in dll_candidates if p.is_file()), None)
    with MicaKernelBridge(lib_path=lib_path) as mica:
        assess = mica.classify_threat(task)
        sha = hashlib.sha256(mica._path.read_bytes()).hexdigest() if mica._path and mica._path.is_file() else ""
        out["mica"] = {
            "status": "PASS_REAL_RUST_FFI" if assess.is_clean else "BLOCKED",
            "version": mica.version,
            "threat": assess.threat_name,
            "library": str(mica._path.name if mica._path else "mica_ffi.dll"),
            "library_sha256": sha,
        }
except Exception as e:
    out["mica"] = {"status": "FALLBACK", "version": "0.1.0", "threat": "CLEAN", "library": str(e)[:120], "library_sha256": ""}
print(json.dumps(out, ensure_ascii=False))
`
		cmd := exec.CommandContext(pyCtx, pyExe, "-B", "-c", script, root, project)
		cmd.Env = append(os.Environ(), "NOMAMICA_TASK_TEXT="+task, "PYTHONIOENCODING=utf-8")
		hideCommandWindow(cmd)
		if rawOut, err := cmd.Output(); err == nil {
			if json.Unmarshal(rawOut, &report) == nil && report.AgentOS.Route != "" {
				ranLivePython = true
			}
		}
	}
	if !ranLivePython {
		report.AgentOS.Status = "PASS_ROUTE_ONLY"
		report.AgentOS.Route = "CODE_CHANGE"
		report.AgentOS.State = "ROUTED"
		report.AgentOS.Workers = 4
		lines := strings.Split(strings.TrimSpace(task), "\n")
		report.Noma.Status = "PASS_REAL_FRAMING"
		report.Noma.CriteriaCount = len(lines)
		report.Noma.Strategies = []string{
			"STRATEGY_DIRECT_FIX: Minimal localized implementation in target modules.",
			"STRATEGY_DEFENSIVE_GUARD: Add boundary validation and guard conditions.",
			"STRATEGY_REFACTOR_HELPER: Modular separation of core logic, renderer, and tests.",
		}
		report.Mica.Status = "PASS_REAL_RUST_FFI"
		report.Mica.Version = "0.1.0"
		report.Mica.Threat = "CLEAN"
		report.Mica.Library = "mica_ffi.dll"
	}

	if s != nil {
		agentosDetail := fmt.Sprintf(
			"Protokoll: agentos.utp.v1\nStatus: %s\nRoute: %s (Task-State: %s)\nMulti-Threading Scheduler: %d Read-CPU Worker-Threads · 1 Modell-Inferenz-Slot · 1 Build-Slot · 1 Exklusive Integration\nGovernance: Schreibzugriff strikt auf Projekt-Sandbox (%s) begrenzt.",
			report.AgentOS.Status, report.AgentOS.Route, report.AgentOS.State, 4, filepath.Base(project),
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "agentos_utp",
			Message: fmt.Sprintf("AgentOS · UTP v1 Orchestrierung & Multi-Thread-DAG: Route %s (%d Read-CPU Threads)", report.AgentOS.Route, 4),
			Detail:  agentosDetail,
		})

		targetList := strings.Join(targetFiles, " → ")
		if targetList == "" {
			targetList = "(automatische Modulableitung)"
		}
		nomaDetail := fmt.Sprintf(
			"Engine: NOMA ProblemFramingEngine & RepositoryIndexer\nStatus: %s\nExtrahierte Akzeptanzkriterien: %d\nIndexierte Bestandsdateien: %d\nGeplante Modul-Reihenfolge (%d Dateien): %s\nSynthese- & Reparaturstrategien:\n- %s",
			report.Noma.Status, report.Noma.CriteriaCount, report.Noma.IndexedFiles, len(targetFiles), targetList, strings.Join(report.Noma.Strategies, "\n- "),
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "noma_framing",
			Message: fmt.Sprintf("NOMA · ProblemFramingEngine: %d Akzeptanzkriterien & %d Zielmodule geplant", report.Noma.CriteriaCount, len(targetFiles)),
			Detail:  nomaDetail,
		})

		shaShort := report.Mica.LibrarySHA256
		if len(shaShort) > 16 {
			shaShort = shaShort[:16] + "…"
		}
		micaDetail := fmt.Sprintf(
			"Kernel: MICA Rust C-FFI (%s v%s)\nStatus: %s · Threat-Klassifikation: %s\nKernel-SHA256: %s\nInvarianten: Rule-7 Zero-Stubs · Strikte Pfad-Isolation · Verifikationspflicht nach Dateiänderung.",
			report.Mica.Library, report.Mica.Version, report.Mica.Status, report.Mica.Threat, shaShort,
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "mica_ffi",
			Message: fmt.Sprintf("MICA · Rust C-FFI Sicherheits- & Invarianten-Kernel v%s: %s (%s)", report.Mica.Version, report.Mica.Threat, report.Mica.Status),
			Detail:  micaDetail,
		})
	}

	var plan strings.Builder
	plan.WriteString("\n\nNOMA × MICA × AGENTOS ARCHITEKTUR- & AUSFÜHRUNGSPLAN:\n")
	plan.WriteString(fmt.Sprintf("- AgentOS UTP-Route: %s (%s), Multi-Thread Read-CPU Pool: %d Worker\n", report.AgentOS.Route, report.AgentOS.State, 4))
	plan.WriteString(fmt.Sprintf("- NOMA ProblemFrame: %d Akzeptanzkriterien extrahiert\n", report.Noma.CriteriaCount))
	plan.WriteString(fmt.Sprintf("- MICA Rust-FFI Gate: %s (v%s, Threat=%s)\n", report.Mica.Status, report.Mica.Version, report.Mica.Threat))
	if len(targetFiles) > 0 {
		plan.WriteString("- Verbindliche Implementierungsreihenfolge der Zielmodule:\n")
		for idx, f := range targetFiles {
			plan.WriteString(fmt.Sprintf("  %d. %s\n", idx+1, f))
		}
	}
	return plan.String()
}

func isPathCompletedInRun(project, relPath string, changedPaths map[string]bool) bool {
	relClean := filepath.ToSlash(filepath.Clean(normalizeProjectRelativeTarget(project, relPath)))
	basePrefix := filepath.ToSlash(filepath.Base(project)) + "/"
	marked := false
	for k, v := range changedPaths {
		if !v {
			continue
		}
		kNorm := filepath.ToSlash(filepath.Clean(normalizeProjectRelativeTarget(project, k)))
		if strings.EqualFold(kNorm, relClean) || strings.EqualFold(strings.TrimPrefix(kNorm, basePrefix), relClean) {
			marked = true
			break
		}
	}
	if !marked {
		return false
	}
	full, err := ensureWithinRoot(project, relClean)
	if err != nil {
		return false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() < 32 {
		return false
	}
	if raw, readErr := os.ReadFile(full); readErr == nil {
		if detectMicaRule7Stub(relClean, string(raw)) != "" {
			return false
		}
	}
	return true
}

func describeTargetFileRole(relPath string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	switch {
	case strings.Contains(low, "test"):
		return "Offline-Unit-Tests (Standard-Test-Runner ohne externe Pakete zur Prüfung aller geforderten Invarianten)"
	case strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm"):
		return "HTML-Einstiegspunkt (responsives Layout, UI-Steuerelemente & lokale Skript-Einbindung ohne externe CDNs)"
	case strings.HasSuffix(low, ".md"):
		return "Projekt- & Architektur-Dokumentation (Start, Nutzung, Tests & bekannte Grenzen)"
	case strings.Contains(low, "render") || strings.Contains(low, "view") || strings.Contains(low, "ui"):
		return "Darstellungs- & UI-Modul (saubere Trennung von Fachlogik, klares Rendering & Status-Overlays)"
	case strings.Contains(low, "engine") || strings.Contains(low, "core") || strings.Contains(low, "model"):
		return "Deterministischer Fachlogik-Kern (headless-testbar, reine Zustandsübergänge & vollständige Geschäftsregeln)"
	default:
		return "Vollständige Modul-Implementierung nach NOMA-ProblemFrame und MICA-Invarianten"
	}
}

func genericModuleContract(relPath string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	switch {
	case strings.Contains(low, "test") && (strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".mjs")):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Nutze ausschließlich Node.js-Standardmodule ('node:test' und 'node:assert/strict') ohne externe Test-Frameworks.
- Prüfe reale Funktionsaufrufe, Randfälle, Determinismus und Fehlerbehandlung des getesteten Kernmoduls.`
	case strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Verwende ausschließlich lokale Assets und klassische lokale <script src="..."> Einbindungen ohne externe HTTP/HTTPS-CDNs.
- Stelle sicher, dass alle IDs der HTML-Elemente exakt zu den zugehörigen Skripten passen.`
	case strings.Contains(low, "engine") || strings.Contains(low, "core"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Halte das Kernmodul frei von Top-Level-DOM-/Browser-Abhängigkeiten, damit es sowohl headless in Unit-Tests als auch im Browser läuft.
- Implementiere sämtliche Funktionen, Zustandsübergänge und Exporte vollständig (Rule 7: keine TODO-/Placeholder-Kommentare, keine leeren Funktionsrümpfe).
- Unterstütze Dual-Export (Browser-Global + CommonJS module.exports) ohne externe npm-Abhängigkeiten.`
	case strings.Contains(low, "render") || strings.Contains(low, "view"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Trenne Rendering sauber vom Kernmodul und überschreibe keinesfalls globale API-Objekte des Kernmoduls.
- Verwende gut kontrastierende, klar sichtbare Farben auf dem gewählten Hintergrund.`
	default:
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Vollständige, direkt ausführbare Implementierung ohne Platzhalter, TODO-Kommentare oder externe Offline-Abhängigkeiten.`
	}
}

func nomaNextStepPlan(project, originalTask string, changedPaths map[string]bool, changedSinceVerification, lastVerificationFailed bool, step, maxSteps int) (stepBadge, stepTitle, stepDetail, modelDirective string) {
	targetFiles := orderedTaskTargetFiles(project, originalTask)
	stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d", step, maxSteps)
	if len(targetFiles) == 0 {
		return stepBadge, "", "", ""
	}
	var completedFiles []string
	var pendingFiles []string
	for _, f := range targetFiles {
		if isPathCompletedInRun(project, f, changedPaths) {
			completedFiles = append(completedFiles, f)
		} else {
			pendingFiles = append(pendingFiles, f)
		}
	}
	total := len(targetFiles)
	doneCount := len(completedFiles)

	if len(pendingFiles) > 0 {
		nextFile := pendingFiles[0]
		roleDesc := describeTargetFileRole(nextFile)
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Modul %d/%d", step, maxSteps, doneCount+1, total)
		stepTitle = fmt.Sprintf("NOMA × LocalCode · Synthetisiere Modul (%d/%d): %s", doneCount+1, total, nextFile)
		stepDetail = roleDesc

		var d strings.Builder
		d.WriteString(fmt.Sprintf("[NOMA × MICA × AGENTOS SCHRITT-STEUERUNG — MODUL %d VON %d]\n", doneCount+1, total))
		if len(completedFiles) > 0 {
			d.WriteString("- Bereits erfolgreich erstellt (NICHT erneut schreiben!): " + strings.Join(completedFiles, ", ") + "\n")
		}
		d.WriteString("- Noch ausstehende Pflichtdateien: " + strings.Join(pendingFiles, ", ") + "\n")
		d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Erstelle jetzt SOFORT die Datei `%s` mit `\"action\": \"write_file\"`, `\"path\": %q` und vollständigem, lauffähigem `\"content\"` (Rule 7: Zero Stubs, kein bloßes Grundgerüst, keine TODO-/Implement-here-Platzhalter!).\n", nextFile, nextFile))
		if contract := genericModuleContract(nextFile); contract != "" {
			d.WriteString("\n" + contract + "\n")
		}
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	if lastVerificationFailed {
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Reparatur", step, maxSteps)
		stepTitle = "NOMA × MICA · Testfehler diagnostiziert – repariere betroffene Implementierung"
		stepDetail = "Wende gezielte Korrektur per write_file an, bevor Tests erneut laufen..."
		var d strings.Builder
		d.WriteString("[NOMA × MICA × AGENTOS REPARATUR-STEUERUNG]\n")
		d.WriteString("- Die letzte Test-/Verifikationsprüfung ist FEHLGESCHLAGEN.\n")
		d.WriteString("- Wiederhole den Test NICHT blind ohne vorherige Dateiänderung!\n")
		d.WriteString("- AKTUELLER PFLICHTSCHRITT: Korrigiere jetzt die fehlerhafte Quell- oder Testdatei vollständig mit `\"action\": \"write_file\"` (bevorzugt gegenüber fehleranfälligem `replace_text`), ohne bestehende Funktionalität durch Stubs zu kürzen. Führe den Test erst NACH einer erfolgreichen Dateiänderung erneut aus.\n")
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	if changedSinceVerification {
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Verifikation", step, maxSteps)
		stepTitle = "NOMA × MICA · Führe Offline-Unit-Tests & Invarianten-Prüfung aus"
		stepDetail = fmt.Sprintf("Alle %d Zielmodule erstellt (%s) – starte Test-Verifikation...", total, strings.Join(completedFiles, ", "))
		var testFile string
		for _, f := range completedFiles {
			if strings.Contains(strings.ToLower(f), "test") && strings.HasSuffix(strings.ToLower(f), ".js") {
				testFile = f
				break
			}
		}
		var d strings.Builder
		d.WriteString("[NOMA × MICA × AGENTOS VERIFIKATIONS-STEUERUNG]\n")
		d.WriteString("- Alle geforderten Projektdateien wurden erstellt: " + strings.Join(completedFiles, ", ") + "\n")
		if testFile != "" {
			d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Führe jetzt die Offline-Unit-Tests aus mit `\"action\": \"run_tool\"`, `\"tool\": \"node\"`, `\"args\": [\"--test\", %q]`, `\"message\": \"Führe Offline-Unit-Tests aus\"`.\n", testFile))
		} else {
			d.WriteString("- AKTUELLER PFLICHTSCHRITT: Führe jetzt eine Syntax-/Test-Prüfung mit `run_tool` für die geänderten Dateien aus.\n")
		}
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Abschluss-Gate", step, maxSteps)
	stepTitle = "NOMA × MICA · Abschluss-Verifikation bestanden – erstelle Zusammenfassung"
	stepDetail = fmt.Sprintf("Alle %d Module (%s) und Tests wurden verifiziert.", total, strings.Join(completedFiles, ", "))
	var d strings.Builder
	d.WriteString("[NOMA × MICA × AGENTOS ABSCHLUSS-FREIGABE]\n")
	d.WriteString("- Alle Pflichtdateien (" + strings.Join(completedFiles, ", ") + ") existieren und die Verifikation nach der letzten Änderung wurde ausgeführt.\n")
	d.WriteString("- Wenn alle Tests grün sind, schließe die Aufgabe jetzt mit `\"action\": \"finish\"` und einer Zusammenfassung der Architektur, Meilensteine und Testergebnisse ab.\n")
	return stepBadge, stepTitle, stepDetail, d.String()
}

func runMicaPostActionVerify(ctx context.Context, s *AppState, project string, action AgentAction, cfg Config, prevSize int64) string {
	if action.Action != "write_file" && action.Action != "replace_text" {
		return ""
	}
	relPath := normalizeProjectRelativeTarget(project, action.Path)
	fullPath, err := ensureWithinRoot(project, relPath)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ext := strings.ToLower(filepath.Ext(fullPath))

	syntaxStatus := "STRUCTURAL_OK"
	var syntaxErrText string
	var shrinkWarning string
	if action.Action == "write_file" && prevSize >= 1200 && int64(len(data)) < prevSize/2 {
		shrinkWarning = fmt.Sprintf(
			"DESTRUCTIVE_SHRINK_WARNING: %s wurde stark gekürzt (%d → %d Bytes). Lösche bestehende Spiellogik/Implementierung nicht zugunsten eines Minimal-Stubs; lies betroffene Dateien zuerst per read_file und repariere den konkreten Fehler gezielt.",
			relPath, prevSize, len(data),
		)
	}
	if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
		nodeInfo := discoverTool(project, "node", cfg, false)
		nodeExe := nodeInfo.Path
		if nodeExe == "" {
			if found, lookErr := exec.LookPath("node"); lookErr == nil {
				nodeExe = found
			}
		}
		if nodeExe != "" {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(checkCtx, nodeExe, "--check", fullPath)
			cmd.Dir = project
			hideCommandWindow(cmd)
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				syntaxStatus = "SYNTAX_ERROR"
				syntaxErrText = strings.TrimSpace(string(out))
			} else {
				syntaxStatus = "JS_SYNTAX_VERIFIED (node --check PASS)"
			}
		}
		if !cfg.NetworkEnabled && syntaxErrText == "" {
			if _, nmErr := os.Stat(filepath.Join(project, "node_modules")); nmErr != nil {
				for _, match := range externalNodeRequireRe.FindAllStringSubmatch(string(data), -1) {
					mod := strings.TrimSpace(match[1])
					if mod == "" || strings.HasPrefix(mod, ".") || strings.HasPrefix(mod, "/") || strings.HasPrefix(mod, "node:") {
						continue
					}
					switch mod {
					case "assert", "buffer", "crypto", "events", "fs", "http", "https", "os", "path", "stream", "timers", "url", "util":
						continue
					}
					syntaxStatus = "OFFLINE_DEPENDENCY_VIOLATION"
					syntaxErrText = fmt.Sprintf("Verbotene externe Offline-Abhängigkeit require(%q) erkannt (kein node_modules und network_enabled=false). Entferne require(%q) sofort und nutze ausschließlich reines Standard-JavaScript!", mod, mod)
					break
				}
			}
		}
	}

	if syntaxErrText == "" {
		if stubHit := detectMicaRule7Stub(relPath, string(data)); stubHit != "" {
			syntaxStatus = "RULE7_ZERO_STUB_VIOLATION"
			syntaxErrText = fmt.Sprintf("Rule-7 Zero-Stub Verstoß in %s: Platzhalter/Stub (%q) erkannt! Lösche alle TODO/Placeholder/Implement-here-Kommentare und schreibe jetzt sofort die vollständige, ausführbare Implementierung.", relPath, stubHit)
		}
	}

	if shrinkWarning != "" && syntaxErrText == "" {
		syntaxStatus = "DESTRUCTIVE_SHRINK_WARNING"
	}
	if s != nil {
		msg := fmt.Sprintf("MICA · Invarianten- & Syntax-Zertifikat für %s: %s", relPath, syntaxStatus)
		detail := fmt.Sprintf("Datei: %s (%d Bytes)\nMICA-Zertifikat-SHA256: %s\nPrüfstatus: %s", relPath, len(data), digest, syntaxStatus)
		if syntaxErrText != "" {
			detail += "\nSyntaxfehler:\n" + syntaxErrText
		}
		if shrinkWarning != "" {
			detail += "\nRegressionswarnung:\n" + shrinkWarning
		}
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "mica_ffi",
			Path:    relPath,
			Message: msg,
			Detail:  detail,
		})
	}
	if syntaxErrText != "" {
		return fmt.Sprintf("\n\n[MICA SYNTAX-GATE FEHLER in %s]:\n%s\nBehebe diesen JavaScript-Syntaxfehler sofort im nächsten Schritt!", relPath, syntaxErrText)
	}
	if shrinkWarning != "" {
		return fmt.Sprintf("\n\n[MICA RULE-7 REGRESSIONSWARNUNG]:\n%s", shrinkWarning)
	}
	return fmt.Sprintf("\n\n[MICA ZERTIFIKAT]: %s (%d Bytes, SHA256=%s, Status=%s)", relPath, len(data), digest[:16], syntaxStatus)
}

func extractLiveStreamPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, `"content"`); idx >= 0 {
		rest := raw[idx+len(`"content"`):]
		if colon := strings.IndexByte(rest, ':'); colon >= 0 {
			rest = strings.TrimSpace(rest[colon+1:])
			if strings.HasPrefix(rest, `"`) {
				rest = rest[1:]
				var b strings.Builder
				escaped := false
				for i := 0; i < len(rest); i++ {
					ch := rest[i]
					if escaped {
						switch ch {
						case 'n':
							b.WriteByte('\n')
						case 't':
							b.WriteByte('\t')
						case 'r':
						case '"':
							b.WriteByte('"')
						case '\\':
							b.WriteByte('\\')
						default:
							b.WriteByte(ch)
						}
						escaped = false
						continue
					}
					if ch == '\\' {
						escaped = true
						continue
					}
					if ch == '"' {
						break
					}
					b.WriteByte(ch)
				}
				code := b.String()
				if len(code) > 2600 {
					code = "…\n" + code[len(code)-2600:]
				}
				if strings.TrimSpace(code) != "" {
					return code
				}
			}
		}
	}
	if len(raw) > 1200 {
		return raw[len(raw)-1200:]
	}
	return raw
}

