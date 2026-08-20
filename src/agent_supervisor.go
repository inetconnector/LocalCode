// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type taskIntent struct {
	Kind         string
	OriginalTask string
	GitRequested bool
	WebQuery     string
}

func classifyTaskIntent(task string) taskIntent {
	normalized := normalizedQuestion(task)
	intent := taskIntent{Kind: "general", OriginalTask: strings.TrimSpace(task)}
	containsAny := func(markers ...string) bool {
		for _, marker := range markers {
			if strings.Contains(normalized, normalizedQuestion(marker)) {
				return true
			}
		}
		return false
	}
	intent.GitRequested = containsAny("git", "repository", "repo", "commit", "branch", "push", "pull request", "versionierung")
	switch {
	case containsAny("verteile", "auf das handy", "installiere die app", "deploy", "adb install"):
		intent.Kind = "deploy_android"
	case containsAny("kompiliere", "kompilieren", "baue das projekt", "build das projekt", "build project", "erstelle den build"):
		intent.Kind = "build"
	case containsAny("neueste nachrichten", "aktuelle nachrichten", "schau im internet", "suche im internet", "recherchiere im internet", "websuche"):
		intent.Kind = "web_research"
		intent.WebQuery = deriveWebQuery(task, "")
	case containsAny("initialisiere git", "git init", "repository anlegen", "repo anlegen"):
		intent.Kind = "git_init"
		intent.GitRequested = true
	case containsAny("implementiere", "baue", "bau ", "entwickle", "erstelle", "fixe", "behebe", "repariere", "ändere", "aendere", "ergänze", "ergaenze", "füge hinzu", "fuege hinzu", "kopiere", "verschiebe", "benenne um", "male", "zeichne", "generiere ein bild", "portiere", "refaktoriere", "übersetze", "uebersetze", "implement", "develop", "create", "fix", "repair", "change", "add feature", "copy", "move", "rename", "draw", "paint", "generate image", "refactor", "translate"):
		intent.Kind = "edit"
	case containsAny("analysiere das projekt", "analysiere projekt", "pruefe das projekt", "untersuche das projekt", "projekt analysieren"):
		intent.Kind = "analyze"
	}
	return intent
}

func deriveWebQuery(task, actionMessage string) string {
	candidate := strings.TrimSpace(actionMessage)
	generic := normalizedQuestion(candidate)
	for _, value := range []string{"", "web search", "web_search", "internetsuche", "suche im internet", "neueste nachrichten", "aktuelle nachrichten"} {
		if generic == normalizedQuestion(value) {
			candidate = ""
			break
		}
	}
	if candidate == "" {
		candidate = strings.TrimSpace(task)
	}
	candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "SYSTEMHINWEIS:"))
	if normalizedQuestion(candidate) == "schau im internet die neuesten nachrichten" || normalizedQuestion(candidate) == "neueste nachrichten" {
		candidate = "wichtigste aktuelle Nachrichten Deutschland und international " + time.Now().Format("2006-01-02")
	}
	if candidate == "" {
		candidate = "aktuelle Nachrichten " + time.Now().Format("2006-01-02")
	}
	return candidate
}

func isGitRepository(project string, cfg Config) bool {
	info, err := os.Stat(filepath.Join(project, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

func gitContextForTask(project string, cfg Config, task string) string {
	intent := classifyTaskIntent(task)
	info := discoverTool(project, "git", cfg, false)
	if !info.Available {
		if intent.GitRequested {
			return "Git ist nicht gefunden. Die Werkzeugreparatur darf nach Genehmigung Git installieren."
		}
		return "Git ist nicht gefunden. Für diese Aufgabe ist Git nicht erforderlich; nicht danach fragen und ohne Git fortfahren."
	}
	if !isGitRepository(project, cfg) {
		if intent.GitRequested {
			return "Git ist verfügbar, aber der Projektordner ist noch kein Repository. Die Nutzeraufgabe verlangt Git; git init ist zulässig."
		}
		return "Git ist verfügbar, aber der Projektordner ist kein Repository. Git ist für diese Aufgabe nicht erforderlich. Nicht initialisieren, nicht danach fragen und ohne Git fortfahren."
	}
	return gitStatusSummary(project, cfg)
}

func isAffirmativeAnswer(answer string) bool {
	a := normalizedQuestion(answer)
	for _, value := range []string{"ja", "j", "ok", "okay", "mach", "mach das", "tu das", "bitte", "einverstanden", "genehmigt", "weiter"} {
		if a == normalizedQuestion(value) || strings.HasPrefix(a, normalizedQuestion(value)+" ") {
			return true
		}
	}
	return false
}

func isNegativeAnswer(answer string) bool {
	a := normalizedQuestion(answer)
	for _, value := range []string{"nein", "n", "nicht", "abbrechen", "lass es", "stop"} {
		if a == normalizedQuestion(value) || strings.HasPrefix(a, normalizedQuestion(value)+" ") {
			return true
		}
	}
	return false
}

func suggestedActionForQuestion(question string) *AgentAction {
	q := normalizedQuestion(question)
	if strings.Contains(q, "git") && (strings.Contains(q, "initialis") || strings.Contains(q, "erstellen") || strings.Contains(q, "anlegen")) {
		return &AgentAction{Action: "git", Message: "Git-Repository initialisieren", Args: []string{"init"}}
	}
	if strings.Contains(q, "install") {
		for _, name := range []string{"adb", "git", "java", "jdk", "gradle", "dotnet", "node", "npm", "python", "cmake", "msbuild"} {
			if strings.Contains(q, name) {
				return &AgentAction{Action: "discover_tool", Message: name + " erkennen und bei Bedarf die Installation anbieten", Tool: name}
			}
		}
	}
	return nil
}

func forcedActionForIntent(intent taskIntent, completed map[string]bool, cfg Config) *AgentAction {
	switch intent.Kind {
	case "analyze":
		if !completed["project_info"] {
			return &AgentAction{Action: "project_info", Message: "Projektstruktur, Buildsystem und verfügbare Werkzeuge deterministisch erfassen"}
		}
	case "build":
		if !completed["project_info"] {
			return &AgentAction{Action: "project_info", Message: "Buildsystem und benötigte Werkzeuge erkennen"}
		}
		if !completed["build_project"] {
			return &AgentAction{Action: "build_project", Message: "Projekt mit dem erkannten Buildsystem kompilieren"}
		}
	case "deploy_android":
		if !completed["project_info"] {
			return &AgentAction{Action: "project_info", Message: "Android-Projekt, Buildsystem und Werkzeuge erkennen"}
		}
		if !completed["deploy_android"] {
			return &AgentAction{Action: "deploy_android", Message: "Android-App bauen und auf das autorisierte verbundene Gerät installieren"}
		}
	case "web_research":
		if !completed["web_search"] {
			return &AgentAction{Action: "web_search", Message: "Aktuelle Informationen recherchieren", Query: intent.WebQuery, MaxResults: 8}
		}
	case "git_init":
		if !completed["git"] {
			return &AgentAction{Action: "git", Message: "Git-Repository initialisieren", Args: []string{"init"}}
		}
	case "edit":
		if action := forcedEditReliabilityAction(intent, completed, cfg); action != nil {
			return action
		}
	}
	return nil
}

func toolFailureRecoveryDirective(action AgentAction, result string, err error, task string) string {
	text := strings.ToLower(result + "\n" + fmt.Sprint(err))
	var directives []string
	actionTool := normalizedQuestion(action.Tool)
	actionCommand := normalizedQuestion(action.Command)
	posixRemoveMissing := (actionTool == "rm" || strings.HasPrefix(actionCommand, "rm ") || strings.Contains(text, "tool 'rm'") || strings.Contains(text, "tool rm") || strings.Contains(text, "werkzeug 'rm'") || strings.Contains(text, "werkzeug rm")) &&
		(strings.Contains(text, "not found") || strings.Contains(text, "not recognized") || strings.Contains(text, "command not found") || strings.Contains(text, "wurde nicht gefunden"))
	if strings.Contains(text, "not a git repository") || strings.Contains(text, "kein git-repository") {
		if classifyTaskIntent(task).GitRequested {
			directives = append(directives, "Git ist für die Nutzeraufgabe erforderlich. Verwende git init deterministisch, verifiziere danach mit git rev-parse --is-inside-work-tree und fahre erst dann fort.")
		} else {
			directives = append(directives, "Git ist für diese Aufgabe nicht erforderlich. Ignoriere den fehlenden Repository-Status und fahre mit Projektdateien, Buildsystem oder Recherche fort. Frage nicht nach git init.")
		}
	}
	if strings.Contains(text, "old_text muss genau einmal") || strings.Contains(text, "old_text must occur exactly once") {
		directives = append(directives, "Die Textstelle war nicht eindeutig. Nutze search_text, lies die konkrete Datei vollständig und verwende danach einen eindeutigen größeren Kontext oder write_file. Frage den Nutzer nicht nach einer Zeilennummer, wenn sie aus dem Projekt ermittelbar ist.")
	}
	if strings.Contains(text, "native engine is handled by localcode tools") || strings.Contains(text, "localcode nativ") && strings.Contains(text, "engine_edit") {
		directives = append(directives, "Die native Engine ist kein externer Bearbeitungsprozess. Verwende jetzt direkte Dateiwerkzeuge: list_files/read_file/search_text und danach replace_text oder write_file mit vollständigem nicht-leerem content. Frage den Nutzer nicht nach einer anderen Methode.")
	}
	if strings.Contains(text, "search query is empty") || strings.Contains(text, "query is empty") {
		directives = append(directives, "Erzeuge eine konkrete Suchanfrage aus der aktuellen Nutzeraufgabe und rufe web_search erneut genau einmal auf.")
	}
	if posixRemoveMissing {
		directives = append(directives, "rm ist ein POSIX-Werkzeug und auf Windows kein passender Standardweg. Verwende fuer Dateioperationen delete_file, copy_path oder move_path; falls Shell zwingend noetig ist, nutze PowerShell wie Remove-Item mit expliziten Projektpfaden. Frage nicht nach manueller rm-Ausfuehrung.")
	} else if strings.Contains(text, "not recognized") || strings.Contains(text, "command not found") || strings.Contains(text, "wurde nicht gefunden") {
		directives = append(directives, "Nutze discover_tool mit dem Programmnamen. Prüfe PATH, Projekt-Wrapper, Visual-Studio-, Android-SDK- und Standardpfade. Wenn eine unterstützte Installation fehlt, löst LocalCode die Installationsgenehmigung aus.")
	}
	if strings.Contains(text, "no observable project changes") || strings.Contains(text, "keine projektänderungen") || strings.Contains(text, "geänderte dateien: keine erkannt") {
		directives = append(directives, "Die Editing Engine hat keinen beobachtbaren Zielzustand erzeugt. Lies die relevanten Dateien erneut, prüfe Schreibrechte und Task-Scope und führe einen geänderten, konkreteren Editierplan aus. Ein erfolgreicher Exitcode ohne Änderung zählt nicht als Erfolg.")
	}
	if len(directives) == 0 {
		directives = append(directives, "Werte Exitcode, STDOUT und STDERR aus, ändere die Diagnose und wiederhole nicht unverändert denselben fehlgeschlagenen Aufruf.")
	}
	return "SYSTEMHINWEIS ZUR FEHLERREPARATUR:\n- " + strings.Join(directives, "\n- ")
}

func (s *AppState) executeConfirmedContinuationAction(ctx context.Context, project string, cfg Config, action AgentAction) (string, error) {
	preview, err := previewAction(project, cfg, action)
	if err != nil {
		return "", err
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: action.Message, Action: action.Action, Preview: preview})
	result, err := s.executeActionWithToolRepair(ctx, project, cfg, action)
	if err != nil {
		s.AddEvent(UIEvent{Type: "tool_error", Message: action.Message, Detail: result + "\n\nERROR: " + err.Error(), Action: action.Action, Preview: preview})
		return result, err
	}
	s.AddEvent(UIEvent{Type: "action_done", Message: action.Message, Detail: truncateText(result, 30000), Action: action.Action, Preview: preview})
	s.recordAction(action.Action + ": " + action.Message)
	s.UpdateProjectState("Bestätigte Aktion " + action.Action + " ausgeführt")
	return result, nil
}

func actionAllowedForIntent(intent taskIntent, action AgentAction) (bool, string) {
	if (action.Action == "run_command" || action.Action == "open_terminal") && commandLooksGlobalInstall(action.Command) && !taskExplicitlyRequestsGlobalInstall(intent.OriginalTask) {
		return false, "Globale Installationen sind für diese Aufgabe nicht angefordert. Installiere keine Werkzeuge global und frage nicht nach globaler Installation. Nutze vorhandene/verwaltete Werkzeuge, projektlokale Abhängigkeiten oder eine nicht-invasive Prüfung wie Syntax-, Build-, Datei- oder DOM-Smoke-Checks."
	}
	if action.Action == "run_tool" && commandLooksGlobalInstall(strings.Join(append([]string{action.Tool}, action.Args...), " ")) && !taskExplicitlyRequestsGlobalInstall(intent.OriginalTask) {
		return false, "Globale Installationen sind für diese Aufgabe nicht angefordert. Installiere keine Werkzeuge global und frage nicht nach globaler Installation. Nutze vorhandene/verwaltete Werkzeuge, projektlokale Abhängigkeiten oder eine nicht-invasive Prüfung."
	}
	if intent.Kind == "analyze" {
		switch action.Action {
		case "project_info", "subagent_analyze", "tool_inventory", "discover_tool", "list_files", "read_file", "search_text", "lsp", "command_list", "command_read", "engine_repo_map", "aider_repo_map", "web_search", "web_fetch", "mcp_list_tools", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt", "skill_list", "skill_read", "skill_list_resources", "skill_read_resource", "memory_list", "finish", "ask_user":
			return true, ""
		case "git":
			if gitActionIsReadOnly(action.Args) {
				return true, ""
			}
		}
		return false, "Die Nutzeraufgabe verlangt nur eine Analyse. Verändere keine Dateien, initialisiere kein Git und führe keinen mutierenden Befehl aus. Nutze ausschließlich Lese-, Such- und Diagnosewerkzeuge und liefere anschließend den Analysebericht."
	}
	if intent.Kind == "web_research" {
		switch action.Action {
		case "web_search", "web_fetch", "finish", "ask_user":
			return true, ""
		default:
			return false, "Die aktuelle Aufgabe ist eine Internetrecherche. Verwende web_search und web_fetch, nicht Git-, Datei- oder Buildaktionen."
		}
	}
	return true, ""
}

func supervisedFallbackReport(intent taskIntent, project string, cfg Config, messages []OllamaMessage) string {
	switch intent.Kind {
	case "analyze":
		info := projectInfo(project, cfg)
		tree, err := projectTree(project, "", 3, 220)
		if err != nil {
			tree = "Projektstruktur konnte nicht vollständig gelesen werden: " + err.Error()
		}
		gitNote := "Git ist für die Analyse nicht erforderlich."
		if isGitRepository(project, cfg) {
			gitNote = "Git-Repository erkannt.\n" + gitStatusSummary(project, cfg)
		}
		return "Projektanalyse kontrolliert abgeschlossen.\n\n" + info + "\n" + gitNote + "\n\nProjektstruktur (gekürzt):\n" + truncateText(tree, 14000) + "\n\nEs wurden keine Dateien verändert. Der Supervisor hat wiederholte, nicht zur Analyse passende Aktionen blockiert."
	case "web_research":
		for i := len(messages) - 1; i >= 0; i-- {
			content := strings.TrimSpace(messages[i].Content)
			if strings.Contains(content, "TOOL RESULT for web_search:") {
				return "Internetrecherche abgeschlossen.\n\n" + truncateText(strings.TrimPrefix(content, "TOOL RESULT for web_search:\n"), 30000)
			}
		}
		return "Die Internetrecherche konnte trotz automatischer Suchanfrage nicht in eine verlässliche Antwort überführt werden. Prüfe die vollständige Werkzeugausgabe im Ausgabenbereich."
	}
	return "Der Agent wurde kontrolliert beendet, weil wiederholt unpassende Aktionen angefordert wurden. Die vollständigen Werkzeugausgaben bleiben im Ausgabenbereich erhalten."
}
