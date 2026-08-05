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

[Deutsch](#deutsch) · [English](#english)

## Deutsch

### Zweck

Dieses Repository wird mit LocalCode bearbeitet. Ergänze hier Zweck, Architektur und Nutzerhinweise.

### Entwicklung

- Build: im Projekt zu dokumentieren
- Tests: im Projekt zu dokumentieren
- Status: siehe [STATE.md](STATE.md)

### Git-Workflow

1. Vor Änderungen den Arbeitsbaum mit `+"`git status`"+` prüfen.
2. Änderungen in einem eigenen Branch durchführen.
3. Diffs und Tests vor einem Commit prüfen.
4. Keine Geheimnisse, Zugangsdaten oder generierten Binärdateien committen.
5. Destruktive Git-Befehle und Force-Push nur nach ausdrücklicher Freigabe.

## English

### Purpose

This repository is maintained with LocalCode. Add the project purpose, architecture and user guidance here.

### Development

- Build: document the build command for this project
- Tests: document the test commands for this project
- Status: see [STATE.md](STATE.md)

### Git workflow

1. Check the working tree with `+"`git status`"+` before making changes.
2. Work in a dedicated branch.
3. Review diffs and run tests before committing.
4. Never commit secrets, credentials or generated binaries.
5. Destructive Git commands and force-push require explicit approval.
`, filepath.Base(project))
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return err
		}
	}
	agents := filepath.Join(project, "AGENTS.md")
	if _, err := os.Stat(agents); os.IsNotExist(err) {
		content := `# Agent Instructions / Agentenanweisungen

## Deutsch

- Lies README.md und STATE.md vor Änderungen.
- Prüfe zuerst Projektstruktur, Git-Status und relevante Tests.
- Ändere nur notwendige Dateien und erhalte bestehende Konventionen.
- Führe passende Tests, Linter und Builds aus.
- Behaupte keinen Erfolg ohne tatsächliche Prüfung.
- Halte STATE.md über die LocalCode-Verwaltung aktuell.
- Keine Geheimnisse ausgeben oder committen.
- Destruktive Befehle, externe Logins, Netzwerkzugriffe und Veröffentlichung benötigen eine ausdrückliche Genehmigung.

## English

- Read README.md and STATE.md before making changes.
- Inspect the project structure, Git status and relevant tests first.
- Change only necessary files and preserve existing conventions.
- Run the appropriate tests, linters and builds.
- Never claim success without actually verifying it.
- Keep STATE.md current through LocalCode's managed section.
- Never expose or commit secrets.
- Destructive commands, external logins, network access and publishing require explicit approval.
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
	lang := resolvedLanguage(cfg)

	mcpNames := []string{}
	for _, server := range cfg.MCPServers {
		if server.Enabled {
			mcpNames = append(mcpNames, server.Name+" ("+server.Transport+")")
		}
	}
	sort.Strings(mcpNames)
	if len(mcpNames) == 0 {
		mcpNames = []string{localized(lang, "keine aktiviert", "none enabled")}
	}
	if strings.TrimSpace(task) == "" {
		task = localized(lang, "keine laufende Aufgabe", "no active task")
	}
	if strings.TrimSpace(summary) == "" {
		summary = localized(lang, "noch kein Abschlussbericht", "no final report yet")
	}
	if strings.TrimSpace(note) == "" {
		note = localized(lang, "automatische Aktualisierung", "automatic update")
	}
	if len(actions) == 0 {
		actions = []string{localized(lang, "noch keine Agentenaktion in dieser Sitzung", "no agent action in this session yet")}
	}

	status := localized(lang, "bereit", "ready")
	if running {
		status = localized(lang, "arbeitet", "working")
	}
	var b strings.Builder
	b.WriteString(stateBegin + "\n")
	b.WriteString("# " + localized(lang, "Aktueller Projektstatus", "Current project status") + "\n\n")
	fmt.Fprintf(&b, "- **%s:** %s\n", localized(lang, "Zuletzt aktualisiert", "Last updated"), time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **%s:** `%s`\n", localized(lang, "Projekt", "Project"), project)
	fmt.Fprintf(&b, "- **%s:** %s\n", localized(lang, "Agentstatus", "Agent status"), status)
	fmt.Fprintf(&b, "- **%s:** `%s`\n", localized(lang, "Modell", "Model"), model)
	fmt.Fprintf(&b, "- **%s:** %s\n", localized(lang, "Letzte Aufgabe", "Latest task"), task)
	fmt.Fprintf(&b, "- **%s:** %s\n", localized(lang, "Letztes Ergebnis", "Latest result"), summary)
	fmt.Fprintf(&b, "- **%s:** %s\n", localized(lang, "Aktualisierungsgrund", "Update reason"), note)
	fmt.Fprintf(&b, "- **%s:** `%s`\n", localized(lang, "Git-Branch", "Git branch"), gitBranchName(project, cfg))
	b.WriteString("\n## " + localized(lang, "Git-Status", "Git status") + "\n\n```text\n" + gitStatusSummary(project, cfg) + "\n```\n")
	b.WriteString("\n## " + localized(lang, "Letzte Agentenaktionen", "Recent agent actions") + "\n\n")
	for _, action := range actions {
		b.WriteString("- " + action + "\n")
	}
	b.WriteString("\n## " + localized(lang, "Laufzeit- und Sicherheitskonfiguration", "Runtime and security configuration") + "\n\n")
	fmt.Fprintf(&b, "- %s: `%s`\n- %s: `%s`\n- %s: `%t`\n- %s: `%s`\n- %s: `%t`\n- %s: %s\n", localized(lang, "Approval-Modus", "Approval mode"), cfg.ApprovalMode, localized(lang, "Sandbox-Modus", "Sandbox mode"), cfg.SandboxMode, localized(lang, "Netzwerk", "Network"), cfg.NetworkEnabled, localized(lang, "Websuche", "Web search"), cfg.WebSearchProvider, localized(lang, "Git-Werkzeuge", "Git tools"), cfg.GitEnabled, localized(lang, "MCP-Server", "MCP servers"), strings.Join(mcpNames, ", "))
	b.WriteString("\n## " + localized(lang, "Pflegehinweis", "Maintenance note") + "\n\n" + localized(lang, "Dieser verwaltete Abschnitt wird bei Projektauswahl, Agentenstart, Werkzeugaktionen und Abschluss automatisch neu geschrieben. Inhalte außerhalb der Marker bleiben erhalten.", "This managed section is rewritten automatically when a project is selected, an agent starts, tools run or a task finishes. Content outside the markers is preserved.") + "\n")
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
		updated = managed + "\n\n## " + localized(lang, "Manuelle Notizen", "Manual notes") + "\n\n"
	} else {
		updated = managed + "\n\n" + old
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(statePath, []byte(updated), 0o644)
}
