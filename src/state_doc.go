// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const stateBegin = "<!-- LOCALCODE:STATE:BEGIN -->"
const stateEnd = "<!-- LOCALCODE:STATE:END -->"
const legacyStateBegin = "<!-- LOCAL" + "CODEX:STATE:BEGIN -->"
const legacyStateEnd = "<!-- LOCAL" + "CODEX:STATE:END -->"

func ensureProjectDocs(project string, cfg Config) error {
	if !cfg.CreateProjectDocs {
		return nil
	}
	readme := filepath.Join(project, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := fmt.Sprintf(`# %s

## Zweck

Dieses Repository wird mit LocalCode bearbeitet. Ergänze hier Zweck, Architektur und Nutzerhinweise.

## Entwicklung

- Build: im Projekt zu dokumentieren
- Tests: im Projekt zu dokumentieren
- Status: siehe [STATE.md](STATE.md)

## Git-Workflow

1. Vor Änderungen: Arbeitsbaum mit `+"`git status`"+` prüfen.
2. Änderungen in einem eigenen Branch durchführen.
3. Diffs und Tests vor einem Commit prüfen.
4. Keine Geheimnisse, Zugangsdaten oder generierten Binärdateien committen.
5. Destruktive Git-Befehle und Force-Push nur nach ausdrücklicher Freigabe.
`, filepath.Base(project))
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return err
		}
	}
	agents := filepath.Join(project, "AGENTS.md")
	if _, err := os.Stat(agents); os.IsNotExist(err) {
		content := `# Agent Instructions

- Lies README.md und STATE.md vor Änderungen.
- Prüfe zuerst Projektstruktur, Git-Status und relevante Tests.
- Ändere nur notwendige Dateien und erhalte bestehende Konventionen.
- Führe passende Tests, Linter und Builds aus.
- Behaupte keinen Erfolg ohne tatsächliche Prüfung.
- Halte STATE.md über die LocalCode-Verwaltung aktuell.
- Keine Geheimnisse ausgeben oder committen.
- Destruktive Befehle, externe Logins, Netzwerkzugriffe und Veröffentlichung benötigen eine ausdrückliche Genehmigung.
`
		if err := os.WriteFile(agents, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *AppState) recordAction(line string) {
	s.mu.Lock()
	line = strings.TrimSpace(line)
	if line != "" {
		s.ActionLog = append(s.ActionLog, time.Now().Format("15:04:05")+" · "+line)
		if len(s.ActionLog) > 25 {
			s.ActionLog = append([]string(nil), s.ActionLog[len(s.ActionLog)-25:]...)
		}
	}
	s.mu.Unlock()
}

func (s *AppState) UpdateProjectState(note string) {
	s.mu.RLock()
	project := s.Project
	cfg := s.Config
	running := s.Running
	model := s.Model
	task := s.LastTask
	summary := s.LastSummary
	actions := append([]string(nil), s.ActionLog...)
	s.mu.RUnlock()
	if project == "" || !cfg.AutoStateUpdate {
		return
	}
	_ = ensureProjectDocs(project, cfg)
	_ = updateStateDocument(project, cfg, running, model, task, summary, actions, note)
}

func updateStateDocument(project string, cfg Config, running bool, model, task, summary string, actions []string, note string) error {
	stateRel := strings.TrimSpace(cfg.StateFile)
	if stateRel == "" {
		stateRel = "STATE.md"
	}
	statePath, err := ensureWithinRoot(project, stateRel)
	if err != nil {
		return err
	}
	existing, _ := os.ReadFile(statePath)
	old := string(existing)

	mcpNames := []string{}
	for _, server := range cfg.MCPServers {
		if server.Enabled {
			mcpNames = append(mcpNames, server.Name+" ("+server.Transport+")")
		}
	}
	sort.Strings(mcpNames)
	if len(mcpNames) == 0 {
		mcpNames = []string{"keine aktiviert"}
	}
	if strings.TrimSpace(task) == "" {
		task = "keine laufende Aufgabe"
	}
	if strings.TrimSpace(summary) == "" {
		summary = "noch kein Abschlussbericht"
	}
	if strings.TrimSpace(note) == "" {
		note = "automatische Aktualisierung"
	}
	if len(actions) == 0 {
		actions = []string{"noch keine Agentenaktion in dieser Sitzung"}
	}

	status := "bereit"
	if running {
		status = "arbeitet"
	}
	var b strings.Builder
	b.WriteString(stateBegin + "\n")
	b.WriteString("# Aktueller Projektstatus\n\n")
	fmt.Fprintf(&b, "- **Zuletzt aktualisiert:** %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Projekt:** `%s`\n", project)
	fmt.Fprintf(&b, "- **Agentstatus:** %s\n", status)
	fmt.Fprintf(&b, "- **Modell:** `%s`\n", model)
	fmt.Fprintf(&b, "- **Letzte Aufgabe:** %s\n", task)
	fmt.Fprintf(&b, "- **Letztes Ergebnis:** %s\n", summary)
	fmt.Fprintf(&b, "- **Aktualisierungsgrund:** %s\n", note)
	fmt.Fprintf(&b, "- **Git-Branch:** `%s`\n", gitBranchName(project, cfg))
	b.WriteString("\n## Git-Status\n\n```text\n" + gitStatusSummary(project, cfg) + "\n```\n")
	b.WriteString("\n## Letzte Agentenaktionen\n\n")
	for _, action := range actions {
		b.WriteString("- " + action + "\n")
	}
	b.WriteString("\n## Laufzeit- und Sicherheitskonfiguration\n\n")
	fmt.Fprintf(&b, "- Approval-Modus: `%s`\n- Sandbox-Modus: `%s`\n- Netzwerk: `%t`\n- Websuche: `%s`\n- Git-Werkzeuge: `%t`\n- MCP-Server: %s\n", cfg.ApprovalMode, cfg.SandboxMode, cfg.NetworkEnabled, cfg.WebSearchProvider, cfg.GitEnabled, strings.Join(mcpNames, ", "))
	b.WriteString("\n## Pflegehinweis\n\nDieser verwaltete Abschnitt wird bei Projektauswahl, Agentenstart, Werkzeugaktionen und Abschluss automatisch neu geschrieben. Inhalte außerhalb der Marker bleiben erhalten.\n")
	b.WriteString(stateEnd)
	managed := b.String()

	var updated string
	start := strings.Index(old, stateBegin)
	end := strings.Index(old, stateEnd)
	endMarkerLength := len(stateEnd)
	if start < 0 || end < start {
		start = strings.Index(old, legacyStateBegin)
		end = strings.Index(old, legacyStateEnd)
		endMarkerLength = len(legacyStateEnd)
	}
	if start >= 0 && end >= start {
		end += endMarkerLength
		updated = old[:start] + managed + old[end:]
	} else if strings.TrimSpace(old) == "" {
		updated = managed + "\n\n## Manuelle Notizen\n\n"
	} else {
		updated = managed + "\n\n" + old
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(statePath, []byte(updated), 0o644)
}
