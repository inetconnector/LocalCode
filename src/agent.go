// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AgentAction struct {
	Action        string         `json:"action"`
	Message       string         `json:"message"`
	Path          string         `json:"path,omitempty"`
	Query         string         `json:"query,omitempty"`
	Content       string         `json:"content,omitempty"`
	OldText       string         `json:"old_text,omitempty"`
	NewText       string         `json:"new_text,omitempty"`
	Command       string         `json:"command,omitempty"`
	MaxDepth      int            `json:"max_depth,omitempty"`
	URL           string         `json:"url,omitempty"`
	MaxResults    int            `json:"max_results,omitempty"`
	Server        string         `json:"server,omitempty"`
	Tool          string         `json:"tool,omitempty"`
	URI           string         `json:"uri,omitempty"`
	PromptName    string         `json:"prompt_name,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	Args          []string       `json:"args,omitempty"`
	Source        string         `json:"source,omitempty"`
	Destination   string         `json:"destination,omitempty"`
	CommitMessage string         `json:"commit_message,omitempty"`
	StageAll      bool           `json:"stage_all,omitempty"`
	Task          string         `json:"task,omitempty"`
}

var actionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{
			"list_files", "read_file", "search_text", "replace_text", "write_file", "delete_file",
			"project_info", "build_project", "deploy_android", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "discover_tool", "tool_inventory", "run_tool", "run_command", "open_terminal", "copy_path", "move_path", "git", "git_commit", "web_search", "web_fetch",
			"mcp_list_tools", "mcp_call_tool", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt",
			"finish", "ask_user",
		}},
		"message": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"},
		"query": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
		"old_text": map[string]any{"type": "string"}, "new_text": map[string]any{"type": "string"},
		"command": map[string]any{"type": "string"}, "max_depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
		"url": map[string]any{"type": "string"}, "max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
		"server": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}, "uri": map[string]any{"type": "string"},
		"prompt_name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"},
		"args":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"source": map[string]any{"type": "string"}, "destination": map[string]any{"type": "string"},
		"commit_message": map[string]any{"type": "string"}, "stage_all": map[string]any{"type": "boolean"},
		"task": map[string]any{"type": "string"},
	},
	"required": []string{"action", "message"}, "additionalProperties": false,
}

const agentSystemPrompt = `Du bist LocalCode, ein präziser autonomer Software-Agent für ein lokales Projekt.

Du arbeitest in einer kontrollierten Werkzeugschleife. Jede Antwort MUSS genau ein JSON-Objekt gemäß dem vorgegebenen Schema enthalten. Kein Markdown um das JSON und keine sichtbare Gedankenkette.

Arbeitsweise:
- AGENTS.md, README.md, STATE.md, Projektstruktur und der relevante Git-Zustand werden zu Beginn bereits in den Kontext eingebettet. Lies Dateien nur erneut, wenn du einen konkreten Abschnitt brauchst oder der eingebettete Inhalt als gekürzt markiert ist.
- Rate nicht über vorhandenen Code. Lies relevante Dateien und suche gezielt.
- Verwende relative Projektpfade. Externe Pfade nur, wenn Sandbox und Nutzerfreigabe dies erlauben.
- Für echte Quellcodeänderungen ist aider_edit die bevorzugte Editing Engine. Aider liefert Repository Map, robuste Edit-Formate, Chat-Verlauf, Lint/Test-Schleifen und Git-Integration. Verwende replace_text/write_file nur für sehr kleine, eindeutig deterministische Verwaltungsänderungen.
- Halte Änderungen klein und kohärent.
- Führe vor dem Abschluss passende Tests, Linter und Builds tatsächlich aus.
- Verwende Git für Status, Diffs, Historie, Branches und vom Nutzer verlangte Commits. Keine History-Rewrites, Force-Pushes oder destruktiven Git-Befehle. Ein fehlendes Git-Repository ist bei Analyse, Build oder Deployment nur eine Information und niemals ein Grund, die Aufgabe zu unterbrechen oder nach git init zu fragen. Initialisiere Git nur, wenn der Nutzer Git ausdrücklich verlangt oder eine Git-Operation ohne Repository wirklich notwendig ist.
- Für aktuelle Fakten darfst du web_search und web_fetch verwenden. Prüfe wichtige Aussagen mit mehreren Primärquellen und nenne die URLs im Abschluss.
- LocalCode verwaltet die MCP-Server filesystem, powershell, git, fetch, github und playwright. Liste bei einem noch unbekannten Server zuerst seine Fähigkeiten. Nutze filesystem für sichere Projektdateien, powershell für PowerShell-spezifische Aufgaben, git für strukturierte Git-Aktionen, fetch für Webinhalte, github für GitHub-Objekte und playwright für zustandsbehaftete Browserautomation. Wenn eine Laufzeit oder Anmeldung fehlt, löst LocalCode Installation beziehungsweise Login kontrolliert aus; gib nicht vorschnell auf.
- Externe Programme niemals vorschnell als fehlend einstufen. Nutze zuerst discover_tool oder tool_inventory. run_tool löst bekannte Programme über PATH, Projekt-Wrapper, Android SDK, Visual-Studio-Installationen, Umgebungsvariablen und Standardpfade auf und liefert Pfad, Exitcode, STDOUT und STDERR. Fehlt ein unterstütztes Werkzeug, bietet LocalCode dem Nutzer automatisch eine kontrollierte Installation an und wiederholt danach exakt den ursprünglichen Aufruf; frage dafür nicht zusätzlich mit ask_user.
- Bevor du wegen eines Werkzeugfehlers den Nutzer fragst: Werkzeug entdecken, genaue Ausgabe auswerten, eine andere sichere Diagnose versuchen und bei unbekannter Bedienung offizielle Dokumentation mit web_search/web_fetch recherchieren. Wiederhole niemals denselben fehlgeschlagenen Befehl oder dieselbe Frage ohne neue Information.
- Nutze project_info für eine deterministische Erkennung des Buildsystems. Wenn der Nutzer kompilieren oder bauen verlangt, bevorzuge build_project statt einen geratenen Shell-Befehl. Für Android-Deployment auf ein verbundenes Gerät bevorzuge deploy_android; es baut zuerst, findet die APK, diagnostiziert ADB und installiert mit adb install -r.
- Für einzelne Android-Diagnosen bevorzugst du run_tool mit tool=adb und args=["devices","-l"]. Ein leeres Geräteverzeichnis bedeutet nicht, dass ADB fehlt; beachte device, unauthorized und offline getrennt.
- Externe Logins sind interaktiv: öffne mit open_terminal ein sichtbares Terminal (z. B. gh auth login, npm login, docker login) und bitte den Nutzer, den Login dort abzuschließen. Erfinde keine Zugangsdaten und lies keine Geheimnisse aus.
- Kopieren und Verschieben erfolgt mit copy_path/move_path innerhalb der konfigurierten Sandbox. Für komplexe Shell-Pipelines darfst du run_command verwenden; für einzelne Programme ist run_tool vorzuziehen.
- Behaupte niemals, ein Befehl, Test, Login, Upload, Push oder Deployment sei erfolgreich gewesen, wenn das Werkzeugergebnis dies nicht bestätigt.
- STATE.md wird von der Anwendung automatisch gepflegt. Überschreibe den verwalteten Abschnitt nicht manuell.
- Der Kontext kann automatisch verdichtet werden. Ein Abschnitt KOMPRIMIERTER ARBEITSKONTEXT ist verbindlicher Arbeitszustand; wiederhole keine dort bereits geklärte Frage und verliere keine dort festgehaltene Nutzerentscheidung.
- finish muss Ergebnis, geänderte Dateien, Tests/Prüfungen, Git-Zustand, Quellen und verbleibende Risiken zusammenfassen.
- ask_user nur, wenn eine echte Entscheidung oder interaktive Benutzeraktion blockiert.

Werkzeuge:
- list_files, read_file, search_text
- replace_text, write_file, delete_file
- project_info, build_project, deploy_android für deterministische Projekt-, Build- und Android-Deployment-Abläufe
- aider_edit(task) für robuste mehrdateilige Codeänderungen mit Repository Map, Backup, Edit-Format, Lint und Tests
- aider_repo_map für die Aider-Repository-Map, aider_lint und aider_test für gezielte Aider-Qualitätsläufe
- discover_tool(tool), tool_inventory, run_tool(tool,args)
- run_command (komplexe Shell-Befehle, nicht-interaktiv), open_terminal (interaktiv sichtbar)
- copy_path, move_path
- git mit args als Argumentliste, z. B. ["status","--short"]
- git_commit für einen vollständigen, verifizierten Commit-Ablauf (Git initialisieren falls ausdrücklich verlangt, .gitignore ergänzen, Änderungen stagen, Commit ausführen und Ergebnis prüfen)
- web_search mit query/max_results, web_fetch mit url
- mcp_list_tools, mcp_call_tool(server,tool,arguments)
- mcp_list_resources, mcp_read_resource(server,uri)
- mcp_list_prompts, mcp_get_prompt(server,prompt_name,arguments)
- finish, ask_user`

func (s *AppState) StartAgent(userMessage, model string, attachments []Attachment) error {
	return s.StartAgentForThread(userMessage, model, attachments, "", "")
}

// StartAgentForThread starts a run in the explicitly requested task. This keeps
// multiple LocalCode windows from accidentally sending a prompt to whichever
// task another window selected most recently. An empty project/thread pair
// preserves the legacy single-window behavior.
func (s *AppState) StartAgentForThread(userMessage, model string, attachments []Attachment, projectOverride, threadID string) error {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" && len(attachments) == 0 {
		return errors.New("prompt is empty")
	}
	if userMessage == "" {
		userMessage = "Analysiere die angehängten Dateien im Kontext dieses Projekts und leite die erforderlichen Schritte ab."
	}

	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return errors.New("agent is already running")
	}
	if threadID != "" {
		t := s.Threads[threadID]
		if t == nil || t.Archived {
			s.mu.Unlock()
			return errors.New("Chat nicht gefunden")
		}
		if projectOverride != "" && !strings.EqualFold(filepath.Clean(projectOverride), filepath.Clean(t.Project)) {
			s.mu.Unlock()
			return errors.New("task does not belong to the requested project")
		}
		s.CurrentThread = threadID
		s.Project = t.Project
		s.Events = append([]UIEvent(nil), t.Events...)
		if model == "" && t.Model != "" {
			model = t.Model
		}
	}
	if projectOverride != "" {
		s.Project = projectOverride
	}
	project := s.Project
	if model == "" {
		model = s.Model
	}
	if project == "" {
		s.mu.Unlock()
		return errors.New("no project selected")
	}
	if model == "" {
		s.mu.Unlock()
		return errors.New("no model selected")
	}

	continuation := s.Continuation
	isContinuation := continuation != nil && continuation.Project == project && continuation.ThreadID == s.CurrentThread && len(continuation.Messages) > 0 && likelyContinuationAnswer(continuation.Question, userMessage)
	if continuation != nil {
		// Consume a real answer, but discard a stale question when the user has
		// clearly started a different task.
		s.Continuation = nil
	}
	if isContinuation && continuation.Model != "" {
		model = continuation.Model
	}

	ctx, cancel := context.WithCancel(context.Background())
	runID := newID()
	now := time.Now()
	s.Running = true
	s.Cancel = cancel
	s.RunID = runID
	s.RunPhase = "starting"
	s.RunStartedAt = now
	s.LastProgressAt = now
	s.Model = model
	s.Config.LastModel = model
	s.Config.LastProject = project
	if s.CurrentThread == "" || s.Threads[s.CurrentThread] == nil || s.Threads[s.CurrentThread].Project != project {
		t := newThread(project, model)
		s.Threads[t.ID] = t
		s.CurrentThread = t.ID
		s.Events = nil
		isContinuation = false
		continuation = nil
	}
	if t := s.Threads[s.CurrentThread]; t != nil {
		if !isContinuation && (t.Title == "" || t.Title == "Neuer Chat") {
			t.Title = threadTitle(userMessage)
		}
		t.Model = model
		t.UpdatedAt = time.Now()
	}
	if !isContinuation {
		s.LastTask = userMessage
		s.LastSummary = ""
		s.ActionLog = nil
	}
	cfg := s.Config
	s.mu.Unlock()

	_ = saveConfig(cfg)
	_ = ensureProjectDocs(project, cfg)
	if isContinuation {
		s.UpdateProjectState("Nutzer hat Rückfrage beantwortet")
	} else {
		s.UpdateProjectState("Agentenaufgabe gestartet")
	}
	s.AddEvent(UIEvent{Type: "user", Message: userMessage, Attachments: attachmentSummaries(attachments)})
	if isContinuation {
		go s.runAgentContinuation(ctx, runID, project, model, userMessage, attachments, continuation)
	} else {
		go s.runAgent(ctx, runID, project, model, userMessage, attachments)
	}
	return nil
}

func (s *AppState) StopAgent() bool {
	s.mu.Lock()
	cancel := s.Cancel
	running := s.Running
	if running {
		s.RunPhase = "cancelling"
		s.LastProgressAt = time.Now()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if running {
		s.AddEvent(UIEvent{Type: "warning", Message: "Abbruch angefordert", Detail: "Der laufende Modell- oder Werkzeugaufruf wird kontrolliert beendet."})
	}
	return running
}

func (s *AppState) ForceStopAgent() bool {
	s.mu.Lock()
	cancel := s.Cancel
	pending := s.Pending
	wasRunning := s.Running
	if wasRunning {
		// Invalidate the current run so a late goroutine cannot switch the UI back
		// into a running state or execute an after-task hook.
		s.RunID = newID()
		s.Running = false
		s.Cancel = nil
		s.Pending = nil
		s.Continuation = nil
		s.RunPhase = "idle"
		s.LastProgressAt = time.Now()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if pending != nil {
		select {
		case pending.Result <- ApprovalDecision{Approved: false}:
		default:
		}
	}
	if wasRunning {
		s.AddEvent(UIEvent{Type: "warning", Message: "Vorgang zwangsweise zurückgesetzt", Detail: "Die Benutzeroberfläche ist wieder freigegeben. Der vorherige Kontext wurde verworfen."})
		s.AddEvent(UIEvent{Type: "status", Message: "Bereit"})
	}
	return wasRunning
}

func (s *AppState) setRunPhase(runID, phase string) {
	s.mu.Lock()
	if s.Running && s.RunID == runID {
		s.RunPhase = phase
		s.LastProgressAt = time.Now()
	}
	s.mu.Unlock()
}

func (s *AppState) finishAgentRun(runID, project string, runAfterHook bool) {
	s.mu.RLock()
	cfg := s.Config
	active := s.RunID == runID
	s.mu.RUnlock()
	if !active {
		return
	}
	if runAfterHook {
		if hook := strings.TrimSpace(cfg.HookAfterTask); hook != "" {
			hookCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.CommandTimeout)*time.Second)
			out, hookErr := runProjectCommand(hookCtx, project, hook, cfg)
			cancel()
			if hookErr != nil {
				s.AddEvent(UIEvent{Type: "warning", Message: "Hook nach Aufgabe fehlgeschlagen", Detail: truncateText(out+"\n"+hookErr.Error(), 12000)})
			} else {
				s.AddEvent(UIEvent{Type: "tool_result", Message: "Hook nach Aufgabe ausgeführt", Detail: truncateText(out, 12000), Action: "hook_after_task"})
			}
		}
	}
	s.mu.Lock()
	if s.RunID != runID {
		s.mu.Unlock()
		return
	}
	s.Running = false
	s.Cancel = nil
	s.Pending = nil
	s.RunPhase = "idle"
	s.LastProgressAt = time.Now()
	s.mu.Unlock()
	if runAfterHook {
		s.UpdateProjectState("Agentenlauf beendet")
	} else {
		s.UpdateProjectState("Agent wartet auf Antwort oder wurde abgebrochen")
	}
	s.AddEvent(UIEvent{Type: "status", Message: "Bereit"})
}

func (s *AppState) prepareAttachmentContext(ctx context.Context, userMessage string, attachments []Attachment) (string, func(), error) {
	prepared, err := prepareAttachments(ctx, attachments)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		if prepared.Dir != "" {
			_ = os.RemoveAll(prepared.Dir)
		}
	}
	attachmentContext := prepared.Context
	if len(prepared.Images) > 0 {
		s.AddEvent(UIEvent{Type: "status", Message: "Analysiere Bilder", Detail: strings.Join(attachmentNames(prepared.Images), ", ")})
		visionModel, pulled, err := s.Ollama.EnsureVisionModel(ctx)
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
		if pulled {
			s.AddEvent(UIEvent{Type: "warning", Message: "Vision-Modell automatisch installiert", Detail: visionModel})
		}
		analysis, err := s.Ollama.DescribeImages(ctx, visionModel, userMessage, prepared.Images)
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
		attachmentContext += "\n\nBILDANALYSE (" + visionModel + "):\n" + analysis
		s.AddEvent(UIEvent{Type: "agent_step", Message: "Bilder analysiert", Detail: visionModel + " · " + strings.Join(attachmentNames(prepared.Images), ", ")})
	}
	return attachmentContext, cleanup, nil
}

func (s *AppState) runAgent(ctx context.Context, runID, project, model, userMessage string, attachments []Attachment) {
	runAfterHook := false
	defer func() { s.finishAgentRun(runID, project, runAfterHook) }()

	attachmentContext, cleanup, err := s.prepareAttachmentContext(ctx, userMessage, attachments)
	if err != nil {
		s.AddEvent(UIEvent{Type: "error", Message: "Dateianhänge konnten nicht vorbereitet werden", Detail: err.Error()})
		return
	}
	defer cleanup()

	tree, err := projectTree(project, "", 4, 800)
	if err != nil {
		s.AddEvent(UIEvent{Type: "error", Message: "Projekt konnte nicht gelesen werden", Detail: err.Error()})
		return
	}
	instructions := projectInstructionContext(project)
	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	capabilityContext := fmt.Sprintf("KONFIGURATION:\nApproval=%s; Sandbox=%s; Network=%t; Web=%s; Git=%t; Umgebung=%s; Tempo=%s\nMCP-SERVER:\n%s\nWERKZEUGERKENNUNG:\n%s", cfg.ApprovalMode, cfg.SandboxMode, cfg.NetworkEnabled, cfg.WebSearchProvider, cfg.GitEnabled, cfg.AgentEnvironment, cfg.ResponseSpeed, mcpServersSummary(cfg), toolInventorySummary(project, cfg))
	personalization := strings.TrimSpace(cfg.UserInstructions)
	if personalization == "" {
		personalization = "Keine zusätzlichen persönlichen Anweisungen."
	}
	language := responseLanguage(cfg)
	systemPrompt := agentSystemPrompt + "\n\nNUTZERPRÄFERENZEN:\n- Antworte in " + language + ".\n- Arbeitsmodus: " + cfg.ResponseSpeed + ".\n- Zusätzliche Anweisungen:\n" + personalization
	automationHint := taskAutomationHint(userMessage)
	messages := []OllamaMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: fmt.Sprintf("PROJEKT: %s\n\n%s\n\nPROJEKTDOKUMENTE:\n%s\n\nPROJEKTSTRUKTUR:\n%s\n\nGIT-KONTEXT:\n%s\n\nAUFGABE:\n%s%s\n\n%s", filepath.Base(project), capabilityContext, instructions, tree, gitContextForTask(project, cfg, userMessage), userMessage, attachmentContext, automationHint)}}
	s.AddEvent(UIEvent{Type: "status", Message: "Agent arbeitet", Detail: model})

	if hook := strings.TrimSpace(cfg.HookBeforeTask); hook != "" {
		hookCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.CommandTimeout)*time.Second)
		out, hookErr := runProjectCommand(hookCtx, project, hook, cfg)
		cancel()
		if hookErr != nil {
			s.AddEvent(UIEvent{Type: "error", Message: "Hook vor Aufgabe fehlgeschlagen", Detail: truncateText(out+"\n"+hookErr.Error(), 12000)})
			return
		}
		s.AddEvent(UIEvent{Type: "agent_step", Message: "Hook vor Aufgabe ausgeführt", Detail: truncateText(out, 12000)})
	}

	outcome := s.executeAgentLoop(ctx, runID, project, model, messages, cfg, "", userMessage)
	runAfterHook = outcome == "done"
}

func (s *AppState) runAgentContinuation(ctx context.Context, runID, project, model, userMessage string, attachments []Attachment, continuation *AgentContinuation) {
	runAfterHook := false
	defer func() { s.finishAgentRun(runID, project, runAfterHook) }()

	attachmentContext, cleanup, err := s.prepareAttachmentContext(ctx, userMessage, attachments)
	if err != nil {
		s.AddEvent(UIEvent{Type: "error", Message: "Dateianhänge konnten nicht vorbereitet werden", Detail: err.Error()})
		return
	}
	defer cleanup()

	messages := append([]OllamaMessage(nil), continuation.Messages...)
	answer := "ANTWORT DES NUTZERS AUF DIE RÜCKFRAGE:\nFrage: " + continuation.Question + "\nAntwort: " + userMessage + attachmentContext + "\n\nSetze die bestehende Aufgabe jetzt fort. Stelle dieselbe Frage nicht erneut, außer die Antwort ist wirklich unverständlich."
	messages = append(messages, OllamaMessage{Role: "user", Content: answer})
	s.AddEvent(UIEvent{Type: "status", Message: "Agent setzt Aufgabe fort", Detail: model})

	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	if continuation.SuggestedAction != nil {
		if isAffirmativeAnswer(userMessage) {
			action := *continuation.SuggestedAction
			s.AddEvent(UIEvent{Type: "agent_step", Message: "Bestätigte Aktion wird direkt ausgeführt", Action: action.Action, Detail: action.Message})
			result, actionErr := s.executeConfirmedContinuationAction(ctx, project, cfg, action)
			messages = append(messages, OllamaMessage{Role: "assistant", Content: mustJSON(action)})
			toolText := "TOOL RESULT for confirmed " + action.Action + ":\n" + truncateText(result, 120000)
			if actionErr != nil {
				toolText += "\n\n" + toolFailureRecoveryDirective(action, result, actionErr, continuation.OriginalTask)
			}
			messages = append(messages, OllamaMessage{Role: "user", Content: toolText + "\n\nDie Nutzerbestätigung wurde verbraucht. Frage nicht erneut nach derselben Aktion; verifiziere das Ergebnis und fahre fort."})
		} else if isNegativeAnswer(userMessage) {
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS: Die vorgeschlagene Aktion wurde abgelehnt. Führe sie nicht aus und fahre, soweit möglich, ohne sie fort."})
		}
	}
	originalTask := continuation.OriginalTask
	if strings.TrimSpace(originalTask) == "" {
		originalTask = s.LastTask
	}
	outcome := s.executeAgentLoop(ctx, runID, project, model, messages, cfg, continuation.Question, originalTask)
	runAfterHook = outcome == "done"
}

func (s *AppState) executeAgentLoop(ctx context.Context, runID, project, model string, messages []OllamaMessage, cfg Config, previousQuestion, originalTask string) string {
	maxSteps := cfg.MaxAgentSteps
	if cfg.ResponseSpeed == "fast" && maxSteps > 35 {
		maxSteps = 35
	}
	if cfg.ResponseSpeed == "thorough" && maxSteps < 90 {
		maxSteps = 90
	}
	if maxSteps <= 0 {
		maxSteps = 60
	}
	intent := classifyTaskIntent(originalTask)
	completedActions := map[string]bool{}
	failedActions := map[string]int{}
	compactionCount := 0
	lastSignature := ""
	repeatBlocks := 0
	supervisorBlocks := 0
	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			s.AddEvent(UIEvent{Type: "warning", Message: "Vorgang abgebrochen"})
			return "cancelled"
		}
		if compacted, didCompact := s.compactAgentMessages(ctx, model, messages, cfg, originalTask); didCompact {
			messages = compacted
			compactionCount++
		}
		s.setRunPhase(runID, "model")
		s.AddEvent(UIEvent{Type: "progress", Message: fmt.Sprintf("Modellschritt %d von %d", step, maxSteps), Detail: model})
		var action AgentAction
		usedModel := model
		var err error
		if forced := forcedActionForIntent(intent, completedActions, cfg); forced != nil {
			action = *forced
			s.AddEvent(UIEvent{Type: "agent_step", Message: "Deterministische Aufgabensteuerung: " + action.Message, Action: action.Action})
		} else {
			modelTimeout := time.Duration(cfg.ModelTimeout) * time.Second
			if modelTimeout <= 0 {
				modelTimeout = 4 * time.Minute
			}
			stepCtx, stepCancel := context.WithTimeout(ctx, modelTimeout)
			action, usedModel, err = s.nextAgentAction(stepCtx, model, messages)
			stepCancel()
		}
		fallbackTask := originalTask
		if strings.TrimSpace(fallbackTask) == "" {
			s.mu.RLock()
			fallbackTask = s.LastTask
			s.mu.RUnlock()
		}
		action = normalizeAgentAction(action, fallbackTask)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				s.AddEvent(UIEvent{Type: "warning", Message: "Vorgang abgebrochen"})
				return "cancelled"
			}
			if errors.Is(err, context.DeadlineExceeded) {
				s.AddEvent(UIEvent{Type: "error", Message: "Modellaufruf wegen Zeitüberschreitung beendet", Detail: fmt.Sprintf("Zeitlimit: %d Sekunden. Das Zeitlimit kann in den Einstellungen geändert werden.", cfg.ModelTimeout)})
				return "timeout"
			}
			s.AddEvent(UIEvent{Type: "error", Message: "Modellaufruf fehlgeschlagen", Detail: err.Error()})
			return "error"
		}
		if usedModel != model {
			s.AddEvent(UIEvent{Type: "warning", Message: "Modell automatisch gewechselt", Detail: model + " konnte keine nutzbare Aktion liefern. Verwende " + usedModel + "."})
			model = usedModel
			s.mu.Lock()
			s.Model = usedModel
			s.Config.LastModel = usedModel
			cfg = s.Config
			s.mu.Unlock()
			_ = saveConfig(cfg)
		}
		messages = append(messages, OllamaMessage{Role: "assistant", Content: mustJSON(action)})
		if allowed, hint := actionAllowedForIntent(intent, action); !allowed {
			supervisorBlocks++
			s.AddEvent(UIEvent{Type: "warning", Message: "Aktion passt nicht zur Nutzeraufgabe und wurde blockiert", Detail: action.Action + ": " + action.Message})
			if supervisorBlocks >= 2 && (intent.Kind == "analyze" || intent.Kind == "web_research") {
				report := supervisedFallbackReport(intent, project, cfg, messages)
				s.AddEvent(UIEvent{Type: "final", Message: report})
				s.recordAction("Supervisor hat die Aufgabe kontrolliert abgeschlossen")
				s.UpdateProjectState("Supervisor-Abschluss nach blockierter Agentendrift")
				return "done"
			}
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS: " + hint + " Fahre jetzt mit einer passenden Aktion fort."})
			continue
		}
		if action.Action == "ask_user" && previousQuestion != "" && sameQuestion(action.Message, previousQuestion) {
			s.AddEvent(UIEvent{Type: "warning", Message: "Wiederholte Rückfrage blockiert", Detail: "Die Frage wurde bereits beantwortet. Der Agent muss die vorhandene Antwort auswerten und mit einer anderen Diagnose fortfahren."})
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS: Diese Rückfrage wurde bereits beantwortet und darf nicht erneut gestellt werden. Nutze die Nutzerantwort, prüfe Werkzeugpfad, Exitcode, STDOUT und STDERR, verwende bei Bedarf discover_tool/run_tool und recherchiere offizielle Dokumentation. Fahre jetzt fort."})
			continue
		}
		if action.Action == "ask_user" {
			if blocked, hint := blockedAvoidanceQuestion(fallbackTask, action.Message); blocked {
				supervisorBlocks++
				s.AddEvent(UIEvent{Type: "warning", Message: "Unnötige Rückfrage blockiert", Detail: action.Message})
				if supervisorBlocks >= 2 && (intent.Kind == "analyze" || intent.Kind == "web_research") {
					report := supervisedFallbackReport(intent, project, cfg, messages)
					s.AddEvent(UIEvent{Type: "final", Message: report})
					s.recordAction("Supervisor hat die Aufgabe kontrolliert abgeschlossen")
					s.UpdateProjectState("Supervisor-Abschluss nach wiederholter unnötiger Rückfrage")
					return "done"
				}
				messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS: " + hint + " Fahre jetzt mit einer konkreten Werkzeugaktion fort."})
				continue
			}
		}
		signature := actionSignature(action)
		if signature == lastSignature && action.Action != "finish" {
			repeatBlocks++
			s.AddEvent(UIEvent{Type: "warning", Message: "Identische Werkzeugaktion blockiert", Detail: action.Action + " wurde unmittelbar zuvor bereits ohne neue Information angefordert."})
			hint := "SYSTEMHINWEIS: Die identische Aktion wurde blockiert. Wähle eine andere Diagnose. Entdecke das Werkzeug, verwende einen absoluten Pfad, werte die vollständige Ausgabe aus oder recherchiere offizielle Dokumentation."
			if repeatBlocks >= 2 {
				hint += " Stelle keine weitere gleichartige Rückfrage; schließe mit einer präzisen Fehlerdiagnose ab, falls keine sichere Alternative existiert."
			}
			messages = append(messages, OllamaMessage{Role: "user", Content: hint})
			continue
		}
		lastSignature = signature
		if action.Action == "ask_user" {
			s.mu.Lock()
			s.Continuation = &AgentContinuation{
				Project:         project,
				ThreadID:        s.CurrentThread,
				Model:           model,
				Question:        action.Message,
				Messages:        append([]OllamaMessage(nil), messages...),
				SuggestedAction: suggestedActionForQuestion(action.Message),
				OriginalTask:    originalTask,
				CompactionCount: compactionCount,
			}
			s.mu.Unlock()
			s.AddEvent(UIEvent{Type: "question", Message: action.Message})
			s.recordAction("Rückfrage: " + action.Message)
			s.UpdateProjectState("Agent wartet auf Nutzer")
			return "question"
		}
		if action.Action != "finish" {
			s.AddEvent(UIEvent{Type: "agent_step", Message: action.Message, Action: action.Action, Path: action.Path, Command: action.Command})
		}
		s.setRunPhase(runID, "tool:"+action.Action)
		result, done := s.handleAgentAction(ctx, project, action)
		if done {
			return "done"
		}
		completedActions[action.Action] = true
		toolMessage := "TOOL RESULT for " + action.Action + ":\n" + truncateText(result, 120000)
		lowerResult := strings.ToLower(result)
		if strings.Contains(lowerResult, "error:") || strings.Contains(lowerResult, "status: fehler") || strings.Contains(lowerResult, "status: timeout") || strings.Contains(lowerResult, "exitcode: 1") || strings.Contains(lowerResult, "exitcode: -1") {
			failedActions[actionSignature(action)]++
			toolMessage += "\n\n" + toolFailureRecoveryDirective(action, result, errors.New("Werkzeugaktion fehlgeschlagen"), originalTask)
			if failedActions[actionSignature(action)] >= 2 {
				toolMessage += "\nDie gleiche Aktion ist bereits mehrfach fehlgeschlagen. Verwende jetzt eine andere Diagnose oder beende mit einer präzisen Fehlerursache; keine identische Wiederholung."
			}
		}
		messages = append(messages, OllamaMessage{Role: "user", Content: toolMessage})
	}
	s.AddEvent(UIEvent{Type: "error", Message: "Agent hat das Schrittlimit erreicht", Detail: fmt.Sprintf("Nach %d Schritten beendet.", maxSteps)})
	return "limit"
}

func projectInstructionContext(project string) string {
	var parts []string
	for _, name := range []string{"AGENTS.md", "README.md", "STATE.md"} {
		if content, err := readProjectFile(project, name); err == nil {
			parts = append(parts, "--- "+name+" ---\n"+truncateText(content, 18000))
		}
	}
	if len(parts) == 0 {
		return "Keine Projektdokumente vorhanden."
	}
	return strings.Join(parts, "\n\n")
}

func (s *AppState) modelCandidates(ctx context.Context, requested string) []string {
	models, err := s.Ollama.Tags(ctx)
	if err != nil {
		return []string{requested}
	}
	installed := map[string]bool{}
	for _, m := range models {
		installed[m.Name] = true
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] && installed[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	add(requested)
	add("qwen2.5-coder:14b")
	add("qwen2.5-coder:7b")
	add("gpt-oss:20b")
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.Name), "coder") {
			add(m.Name)
		}
	}
	for _, m := range models {
		add(m.Name)
	}
	return out
}

func (s *AppState) nextAgentAction(ctx context.Context, requestedModel string, messages []OllamaMessage) (AgentAction, string, error) {
	var zero AgentAction
	candidates := s.modelCandidates(ctx, requestedModel)
	if len(candidates) == 0 {
		return zero, requestedModel, errors.New("kein installiertes Ollama-Modell gefunden")
	}
	errs := []string{}
	for _, candidate := range candidates {
		content, err := s.Ollama.Chat(ctx, candidate, messages, actionSchema)
		if err != nil {
			if ctx.Err() != nil {
				return zero, candidate, ctx.Err()
			}
			errs = append(errs, candidate+": "+err.Error())
			continue
		}
		action, err := parseAgentAction(content)
		if err == nil {
			return action, candidate, nil
		}
		retry := append([]OllamaMessage(nil), messages...)
		retry = append(retry, OllamaMessage{Role: "assistant", Content: content}, OllamaMessage{Role: "user", Content: "Antwort ungültig. Antworte ausschließlich mit einem JSON-Objekt des Schemas. Fehler: " + err.Error()})
		content, retryErr := s.Ollama.Chat(ctx, candidate, trimMessages(retry), actionSchema)
		if retryErr == nil {
			action, retryErr = parseAgentAction(content)
		}
		if retryErr == nil {
			return action, candidate, nil
		}
		errs = append(errs, candidate+": "+retryErr.Error())
	}
	return zero, requestedModel, fmt.Errorf("kein Modell lieferte eine gültige Agentenaktion: %s", strings.Join(errs, " | "))
}

func parseAgentAction(content string) (AgentAction, error) {
	var a AgentAction
	content = strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(content), &a); err != nil {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start < 0 || end <= start || json.Unmarshal([]byte(content[start:end+1]), &a) != nil {
			return a, err
		}
	}
	if a.Action == "" {
		return a, errors.New("missing action")
	}
	if strings.TrimSpace(a.Message) == "" {
		a.Message = a.Action
	}
	return a, nil
}
func mustJSON(v any) string { data, _ := json.Marshal(v); return string(data) }
func trimMessages(messages []OllamaMessage) []OllamaMessage {
	if len(messages) <= 30 {
		return messages
	}
	out := make([]OllamaMessage, 0, 30)
	out = append(out, messages[0], messages[1])
	out = append(out, messages[len(messages)-28:]...)
	return out
}

func (s *AppState) handleAgentAction(ctx context.Context, project string, a AgentAction) (string, bool) {
	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	var result string
	var err error
	switch a.Action {
	case "discover_tool":
		info := discoverTool(project, a.Tool, cfg, true)
		data, _ := json.MarshalIndent(info, "", "  ")
		result = string(data)
		if !info.Available {
			missing := &ToolNotFoundError{Info: info, Detail: result}
			if info.InstallSupported {
				newCfg, installDetail, installed, installErr := s.offerInstallMissingTool(ctx, project, cfg, missing)
				if installErr != nil {
					err = installErr
					result += "\n\n" + installDetail
				} else if installed {
					cfg = newCfg
					info = discoverTool(project, a.Tool, cfg, true)
					data, _ = json.MarshalIndent(info, "", "  ")
					result = installDetail + "\n\nWERKZEUG NACH INSTALLATION:\n" + string(data)
				} else {
					err = missing
				}
			} else {
				if cfg.AutoResearchToolHelp && cfg.NetworkEnabled && cfg.WebSearchProvider != "disabled" {
					query := info.DisplayName + " official installation command line documentation Windows"
					if info.DocsURL != "" {
						if u, parseErr := url.Parse(info.DocsURL); parseErr == nil && u.Hostname() != "" {
							query = "site:" + u.Hostname() + " " + query
						}
					}
					if results, searchErr := webSearch(ctx, cfg, query, 3); searchErr == nil && len(results) > 0 {
						result += "\n\nAutomatisch recherchierte offizielle Hilfe:\n" + formatWebResults(results)
					}
				}
				err = missing
			}
		}
	case "project_info":
		result = projectInfo(project, cfg)
	case "tool_inventory":
		infos := toolInventory(project, cfg, false)
		data, _ := json.MarshalIndent(infos, "", "  ")
		result = string(data)
	case "list_files":
		result, err = projectTree(project, a.Path, a.MaxDepth, 1000)
	case "read_file":
		result, err = readProjectFile(project, a.Path)
	case "search_text":
		result, err = searchProject(project, a.Query, a.Path, 180)
	case "git":
		if !cfg.GitEnabled {
			err = errors.New("Git tools disabled in settings")
			break
		}
		if err = validateGitArgs(a.Args); err != nil {
			break
		}
		if gitActionIsReadOnly(a.Args) {
			result, err = s.executeActionWithToolRepair(ctx, project, cfg, a)
		} else {
			return s.performApproved(ctx, project, a)
		}
	case "web_search":
		if actionNeedsApproval(cfg, project, a) {
			return s.performApproved(ctx, project, a)
		}
		var r []WebResult
		r, err = webSearch(ctx, cfg, a.Query, a.MaxResults)
		result = formatWebResults(r)
	case "web_fetch":
		if actionNeedsApproval(cfg, project, a) {
			return s.performApproved(ctx, project, a)
		}
		result, err = webFetch(ctx, cfg, a.URL)
	case "mcp_list_tools", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt":
		var preparation string
		var wait bool
		cfg, preparation, wait, err = s.prepareMCPServer(ctx, project, cfg, a)
		if err != nil {
			break
		}
		if wait {
			s.AddEvent(UIEvent{Type: "question", Message: preparation})
			return preparation, true
		}
		if preparation != "" {
			return preparation, false
		}
		method := map[string]string{"mcp_list_tools": "tools/list", "mcp_list_resources": "resources/list", "mcp_read_resource": "resources/read", "mcp_list_prompts": "prompts/list", "mcp_get_prompt": "prompts/get"}[a.Action]
		params := map[string]any{}
		if a.Action == "mcp_read_resource" {
			params["uri"] = a.URI
		}
		if a.Action == "mcp_get_prompt" {
			params["name"] = a.PromptName
			params["arguments"] = a.Arguments
		}
		result, err = mcpCall(ctx, cfg, project, a.Server, method, params)
	case "mcp_call_tool", "replace_text", "write_file", "delete_file", "build_project", "deploy_android", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "run_tool", "run_command", "open_terminal", "copy_path", "move_path", "git_commit":
		return s.performApproved(ctx, project, a)
	case "ask_user":
		s.AddEvent(UIEvent{Type: "question", Message: a.Message})
		s.recordAction("Rückfrage: " + a.Message)
		s.UpdateProjectState("Agent wartet auf Nutzer")
		return a.Message, true
	case "finish":
		s.mu.Lock()
		s.LastSummary = a.Message
		s.mu.Unlock()
		s.AddEvent(UIEvent{Type: "final", Message: a.Message})
		s.recordAction("Aufgabe abgeschlossen")
		s.UpdateProjectState("Agent hat Aufgabe abgeschlossen")
		return a.Message, true
	default:
		err = fmt.Errorf("unsupported action %s", a.Action)
	}
	if err != nil {
		detail := strings.TrimSpace(result)
		if detail != "" {
			detail += "\n\n"
		}
		detail += "ERROR: " + err.Error()
		s.AddEvent(UIEvent{Type: "tool_error", Message: a.Message, Detail: detail, Action: a.Action, Path: a.Path, Command: a.Command})
		return detail, false
	}
	s.AddEvent(UIEvent{Type: "tool_result", Message: a.Message, Detail: truncateText(result, 30000), Action: a.Action, Path: a.Path, Command: a.Command})
	s.recordAction(a.Action + ": " + a.Message)
	s.UpdateProjectState("Werkzeugaktion " + a.Action)
	return result, false
}

func (s *AppState) performApproved(ctx context.Context, project string, a AgentAction) (string, bool) {
	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	if a.Action == "mcp_call_tool" {
		var preparation string
		var wait bool
		var prepareErr error
		cfg, preparation, wait, prepareErr = s.prepareMCPServer(ctx, project, cfg, a)
		if prepareErr != nil {
			return strings.TrimSpace(preparation + "\n\nERROR: " + prepareErr.Error()), false
		}
		if wait {
			s.AddEvent(UIEvent{Type: "question", Message: preparation})
			return preparation, true
		}
		if preparation != "" {
			return preparation, false
		}
	}
	if missing := missingToolForAction(project, cfg, a); missing != nil && missing.Info.InstallSupported {
		newCfg, installDetail, installed, installErr := s.offerInstallMissingTool(ctx, project, cfg, missing)
		if installErr != nil {
			return strings.TrimSpace(installDetail + "\n\nERROR: " + installErr.Error()), false
		}
		if !installed {
			return strings.TrimSpace(installDetail + "\n\nDie Aktion wurde nicht ausgeführt, weil das benötigte Werkzeug fehlt."), false
		}
		cfg = newCfg
	}
	preview, err := previewAction(project, cfg, a)
	if err != nil {
		return "ERROR: " + err.Error(), false
	}
	approved := true
	if actionNeedsApproval(cfg, project, a) {
		approved, err = s.requestApprovalWithPreview(ctx, project, a, preview)
		if err != nil {
			return "ERROR: " + err.Error(), false
		}
	}
	if !approved {
		return "REJECTED BY USER", false
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})
	result, err := s.executeActionWithToolRepair(ctx, project, cfg, a)
	if err != nil {
		detail := strings.TrimSpace(result)
		if detail != "" {
			detail += "\n\n"
		}
		detail += "ERROR: " + err.Error()
		s.AddEvent(UIEvent{Type: "tool_error", Message: a.Message, Detail: detail, Preview: preview, Action: a.Action, Path: a.Path, Command: a.Command})
		return detail, false
	}
	s.AddEvent(UIEvent{Type: "action_done", Message: a.Message, Detail: truncateText(result, 30000), Preview: preview, Action: a.Action, Path: a.Path, Command: a.Command})
	s.recordAction(a.Action + ": " + a.Message)
	s.UpdateProjectState("Aktion " + a.Action + " ausgeführt")
	if a.Action == "open_terminal" {
		return result, true
	}
	return result, false
}

func actionNeedsApproval(cfg Config, project string, a AgentAction) bool {
	if decision, _, matched := approvalRuleDecision(cfg, project, a); matched {
		return decision != "allow"
	}
	if cfg.ApprovalMode == "dangerous" {
		return false
	}
	switch a.Action {
	case "discover_tool", "tool_inventory", "project_info", "list_files", "read_file", "search_text", "mcp_list_tools", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt":
		return false
	case "web_search", "web_fetch":
		return cfg.ApprovalMode == "strict"
	case "replace_text", "write_file", "aider_edit":
		return cfg.ApprovalMode != "auto"
	case "aider_repo_map", "aider_lint", "aider_test":
		return cfg.ApprovalMode == "strict"
	case "git":
		if gitActionIsReadOnly(a.Args) {
			return false
		}
		return true
	case "git_commit":
		return true
	case "run_tool":
		if cfg.ApprovalMode == "auto" && toolActionLooksReadOnly(a.Tool, a.Args) {
			return false
		}
		return true
	case "run_command":
		if cfg.ApprovalMode == "auto" && commandLooksReadOnly(a.Command) {
			return false
		}
		return true
	default:
		return true
	}
}

func commandLooksReadOnly(command string) bool {
	c := strings.ToLower(strings.TrimSpace(command))
	for _, p := range []string{"git status", "git diff", "git log", "go test", "go vet", "go list", "npm test", "npm run lint", "pytest", "cargo test", "dotnet test", "dir", "type ", "findstr ", "where ", "echo "} {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

func toolActionLooksReadOnly(tool string, args []string) bool {
	t := canonicalToolName(tool)
	joined := strings.ToLower(strings.Join(args, " "))
	switch t {
	case "adb":
		return strings.HasPrefix(joined, "devices") || strings.HasPrefix(joined, "version") || strings.HasPrefix(joined, "get-state")
	case "git":
		return strings.HasPrefix(joined, "status") || strings.HasPrefix(joined, "diff") || strings.HasPrefix(joined, "log") || strings.HasPrefix(joined, "show")
	case "go":
		return strings.HasPrefix(joined, "test") || strings.HasPrefix(joined, "vet") || strings.HasPrefix(joined, "list") || strings.HasPrefix(joined, "version")
	case "java", "node", "npm", "npx", "python", "dotnet", "cargo", "cmake", "ninja", "gradle":
		return strings.Contains(joined, "version") || strings.HasPrefix(joined, "--version") || strings.HasPrefix(joined, "-version")
	}
	return false
}

func (s *AppState) requestApprovalWithPreview(ctx context.Context, project string, a AgentAction, preview string) (bool, error) {
	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	if decision, justification, matched := approvalRuleDecision(cfg, project, a); matched {
		switch decision {
		case "allow":
			s.AddEvent(UIEvent{Type: "approval", Message: localizeConfigText(cfg, "Durch dauerhafte Regel genehmigt", "Approved by persistent rule"), Detail: justification, Action: a.Action, Path: a.Path})
			return true, nil
		case "forbidden":
			s.AddEvent(UIEvent{Type: "approval", Message: localizeConfigText(cfg, "Durch dauerhafte Regel blockiert", "Blocked by persistent rule"), Detail: justification, Action: a.Action, Path: a.Path})
			return false, errors.New("action forbidden by approval rule")
		}
	}
	pending := &PendingAction{ID: newID(), Action: a, Preview: preview, Result: make(chan ApprovalDecision, 1)}
	s.mu.Lock()
	s.Pending = pending
	cfg = s.Config
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.Pending != nil && s.Pending.ID == pending.ID {
			s.Pending = nil
		}
		s.mu.Unlock()
	}()
	s.AddEvent(UIEvent{ID: pending.ID, Type: "approval_required", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})
	select {
	case response := <-pending.Result:
		if response.Approved && response.Persist {
			rule, ruleErr := s.addApprovalRule(project, a, response.Scope)
			if ruleErr != nil {
				s.AddEvent(UIEvent{Type: "warning", Message: localizeConfigText(cfg, "Dauerhafte Freigabe konnte nicht gespeichert werden", "Persistent approval could not be saved"), Detail: ruleErr.Error(), Action: a.Action, Path: a.Path})
				return false, ruleErr
			}
			s.AddEvent(UIEvent{Type: "approval_rule", Message: localizeConfigText(cfg, "Dauerhafte Freigabe gespeichert", "Persistent approval saved"), Detail: strings.Join(rule.Pattern, " "), Action: a.Action, Path: a.Path})
		}
		msg := localizeConfigText(cfg, "Abgelehnt", "Rejected")
		if response.Approved {
			msg = localizeConfigText(cfg, "Genehmigt", "Approved")
		}
		s.AddEvent(UIEvent{Type: "approval", Message: msg, Action: a.Action, Path: a.Path})
		return response.Approved, nil
	case <-ctx.Done():
		s.AddEvent(UIEvent{Type: "approval", Message: localizeConfigText(cfg, "Genehmigung abgebrochen", "Approval cancelled"), Action: a.Action, Path: a.Path})
		return false, ctx.Err()
	case <-time.After(30 * time.Minute):
		s.AddEvent(UIEvent{Type: "approval", Message: localizeConfigText(cfg, "Genehmigung abgelaufen", "Approval expired"), Action: a.Action, Path: a.Path})
		return false, errors.New("approval timed out")
	}
}

func previewAction(project string, cfg Config, a AgentAction) (string, error) {
	switch a.Action {
	case "replace_text":
		full, err := ensureWithinRoot(project, a.Path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return "", err
		}
		original := string(data)
		count := strings.Count(original, a.OldText)
		if count != 1 {
			return "", fmt.Errorf("old_text muss genau einmal vorkommen, gefunden: %d", count)
		}
		return simpleDiff(original, strings.Replace(original, a.OldText, a.NewText, 1)), nil
	case "write_file":
		full, err := ensureWithinRoot(project, a.Path)
		if err != nil {
			return "", err
		}
		old := ""
		if data, e := os.ReadFile(full); e == nil {
			if !isProbablyText(data) {
				return "", fmt.Errorf("binary file: %s", a.Path)
			}
			old = string(data)
		}
		return simpleDiff(old, a.Content), nil
	case "delete_file":
		old, err := readProjectFile(project, a.Path)
		if err != nil {
			return "", err
		}
		return simpleDiff(old, ""), nil
	case "run_tool":
		if strings.TrimSpace(a.Tool) == "" {
			return "", errors.New("tool is empty")
		}
		info := discoverTool(project, a.Tool, cfg, false)
		path := info.Path
		if path == "" {
			path = "<wird gesucht>"
		}
		return fmt.Sprintf("Tool: %s\nPfad: %s\nArgumente: %s", a.Tool, path, quoteArgs(a.Args)), nil
	case "run_command":
		if strings.TrimSpace(a.Command) == "" {
			return "", errors.New("command is empty")
		}
		if err := commandBlocked(cfg, a.Command); err != nil {
			return "", err
		}
		return "$ " + a.Command, nil
	case "open_terminal":
		return "Interaktives Terminal im Projekt öffnen:\n$ " + a.Command, nil
	case "copy_path":
		return "Copy: " + a.Source + " -> " + a.Destination, nil
	case "move_path":
		return "Move: " + a.Source + " -> " + a.Destination, nil
	case "git":
		if err := validateGitArgs(a.Args); err != nil {
			return "", err
		}
		return previewGit(a.Args), nil
	case "git_commit":
		message := strings.TrimSpace(a.CommitMessage)
		if message == "" {
			message = deriveCommitMessage(a.Message)
		}
		return "Git-Commit-Ablauf:\n1. Repository und .gitignore prüfen\n2. Änderungen sicher stagen (Visual-Studio-.vs ausgeschlossen)\n3. Commit mit Nachricht: " + message + "\n4. Commit und Arbeitsbaum verifizieren", nil
	case "web_search":
		return "Web search: " + a.Query, nil
	case "web_fetch":
		return "Web fetch: " + a.URL, nil
	case "aider_edit":
		task := strings.TrimSpace(a.Task)
		if task == "" {
			task = strings.TrimSpace(a.Message)
		}
		if task == "" {
			return "", errors.New("Aider task is empty")
		}
		files := relevantFilesForAider(project, task, 12)
		return "Aider Editing Engine ausführen\nAufgabe: " + task + "\nModell: " + cfg.AiderMainModel + "\nVorausgewählte Dateien: " + strings.Join(files, ", ") + "\nVor Änderungen wird ein lokales Backup erzeugt; anschließend laufen konfigurierte Linter und Tests.", nil
	case "aider_repo_map":
		return "Aider Repository Map erzeugen und anzeigen. Es werden keine Dateien verändert.", nil
	case "aider_lint":
		return "Aider-Lintlauf ausführen und gefundene Probleme gemäß Konfiguration reparieren.", nil
	case "aider_test":
		return "Aider-Testlauf ausführen und gefundene Probleme gemäß Konfiguration reparieren.", nil
	case "build_project":
		return "Projekt mit dem automatisch erkannten Buildsystem bauen. Fehlende bekannte Werkzeuge werden nach separater Genehmigung installiert.", nil
	case "deploy_android":
		return "Android-Projekt bauen, ein autorisiertes verbundenes Gerät erkennen und die neueste Debug-APK mit adb install -r übertragen.", nil
	case "mcp_call_tool":
		return fmt.Sprintf("MCP %s tool %s\nArguments: %s", a.Server, a.Tool, mustJSON(a.Arguments)), nil
	default:
		return "", errors.New("action does not require approval")
	}
}

func executeAction(ctx context.Context, project string, cfg Config, a AgentAction) (string, error) {
	switch a.Action {
	case "replace_text":
		return replaceText(project, a.Path, a.OldText, a.NewText)
	case "write_file":
		return writeProjectFile(project, a.Path, a.Content)
	case "delete_file":
		return deleteProjectFile(project, a.Path)
	case "build_project":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout < 10*time.Minute {
			timeout = 10 * time.Minute
		}
		bctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return buildProject(bctx, project, cfg)
	case "deploy_android":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout < 15*time.Minute {
			timeout = 15 * time.Minute
		}
		dctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return deployAndroid(dctx, project, cfg)
	case "run_tool":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		tctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return runResolvedTool(tctx, project, a.Tool, a.Args, cfg)
	case "run_command":
		return executeCommand(ctx, project, a.Command, cfg)
	case "open_terminal":
		if err := openInteractiveTerminal(project, a.Command, cfg); err != nil {
			return "", err
		}
		return "Interaktives Terminal geöffnet. Schließe den Login oder Vorgang dort ab und teile das Ergebnis anschließend mit.", nil
	case "copy_path":
		return copyPath(cfg, project, a.Source, a.Destination)
	case "move_path":
		return movePath(cfg, project, a.Source, a.Destination)
	case "git_commit":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		gctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return commitGitChanges(gctx, project, cfg, a.CommitMessage, a.StageAll)
	case "git":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		gctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if len(a.Args) > 0 && strings.EqualFold(a.Args[0], "init") {
			return initializeGitRepository(gctx, project, cfg)
		}
		return runGit(gctx, project, a.Args, cfg)
	case "web_search":
		r, err := webSearch(ctx, cfg, a.Query, a.MaxResults)
		return formatWebResults(r), err
	case "web_fetch":
		return webFetch(ctx, cfg, a.URL)
	case "mcp_call_tool":
		return mcpCall(ctx, cfg, project, a.Server, "tools/call", map[string]any{"name": a.Tool, "arguments": a.Arguments})
	default:
		return "", errors.New("unsupported approved action")
	}
}
