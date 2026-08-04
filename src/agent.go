package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AgentAction struct {
	Action      string         `json:"action"`
	Message     string         `json:"message"`
	Path        string         `json:"path,omitempty"`
	Query       string         `json:"query,omitempty"`
	Content     string         `json:"content,omitempty"`
	OldText     string         `json:"old_text,omitempty"`
	NewText     string         `json:"new_text,omitempty"`
	Command     string         `json:"command,omitempty"`
	MaxDepth    int            `json:"max_depth,omitempty"`
	URL         string         `json:"url,omitempty"`
	MaxResults  int            `json:"max_results,omitempty"`
	Server      string         `json:"server,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	URI         string         `json:"uri,omitempty"`
	PromptName  string         `json:"prompt_name,omitempty"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Args        []string       `json:"args,omitempty"`
	Source      string         `json:"source,omitempty"`
	Destination string         `json:"destination,omitempty"`
}

var actionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{
			"list_files", "read_file", "search_text", "replace_text", "write_file", "delete_file",
			"run_command", "open_terminal", "copy_path", "move_path", "git", "web_search", "web_fetch",
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
	},
	"required": []string{"action", "message"}, "additionalProperties": false,
}

const agentSystemPrompt = `Du bist LocalCodex, ein präziser autonomer Software-Agent für ein lokales Projekt.

Du arbeitest in einer kontrollierten Werkzeugschleife. Jede Antwort MUSS genau ein JSON-Objekt gemäß dem vorgegebenen Schema enthalten. Kein Markdown um das JSON und keine sichtbare Gedankenkette.

Arbeitsweise:
- Lies zuerst AGENTS.md, README.md und STATE.md, sofern vorhanden. Prüfe Projektstruktur und Git-Status.
- Rate nicht über vorhandenen Code. Lies relevante Dateien und suche gezielt.
- Verwende relative Projektpfade. Externe Pfade nur, wenn Sandbox und Nutzerfreigabe dies erlauben.
- Halte Änderungen klein und kohärent. Nutze replace_text für eindeutige kleine Änderungen und write_file für neue oder vollständig neu geschriebene Dateien.
- Führe vor dem Abschluss passende Tests, Linter und Builds tatsächlich aus.
- Verwende Git für Status, Diffs, Historie, Branches und vom Nutzer verlangte Commits. Keine History-Rewrites, Force-Pushes oder destruktiven Git-Befehle.
- Für aktuelle Fakten darfst du web_search und web_fetch verwenden. Prüfe wichtige Aussagen mit mehreren Primärquellen und nenne die URLs im Abschluss.
- MCP-Server können Tools, Ressourcen und Prompts bereitstellen. Liste zuerst Fähigkeiten, bevor du ein MCP-Tool aufrufst.
- Externe Logins sind interaktiv: öffne mit open_terminal ein sichtbares Terminal (z. B. gh auth login, npm login, docker login) und bitte den Nutzer, den Login dort abzuschließen. Erfinde keine Zugangsdaten und lies keine Geheimnisse aus.
- Kopieren und Verschieben erfolgt mit copy_path/move_path innerhalb der konfigurierten Sandbox. Für komplexe Dateioperationen darfst du run_command verwenden.
- Behaupte niemals, ein Befehl, Test, Login, Upload, Push oder Deployment sei erfolgreich gewesen, wenn das Werkzeugergebnis dies nicht bestätigt.
- STATE.md wird von der Anwendung automatisch gepflegt. Überschreibe den verwalteten Abschnitt nicht manuell.
- finish muss Ergebnis, geänderte Dateien, Tests/Prüfungen, Git-Zustand, Quellen und verbleibende Risiken zusammenfassen.
- ask_user nur, wenn eine echte Entscheidung oder interaktive Benutzeraktion blockiert.

Werkzeuge:
- list_files, read_file, search_text
- replace_text, write_file, delete_file
- run_command (nicht-interaktiv), open_terminal (interaktiv sichtbar)
- copy_path, move_path
- git mit args als Argumentliste, z. B. ["status","--short"]
- web_search mit query/max_results, web_fetch mit url
- mcp_list_tools, mcp_call_tool(server,tool,arguments)
- mcp_list_resources, mcp_read_resource(server,uri)
- mcp_list_prompts, mcp_get_prompt(server,prompt_name,arguments)
- finish, ask_user`

func (s *AppState) StartAgent(userMessage, model string, attachments []Attachment) error {
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
	isContinuation := continuation != nil && continuation.Project == project && continuation.ThreadID == s.CurrentThread && len(continuation.Messages) > 0
	if isContinuation {
		s.Continuation = nil
		if continuation.Model != "" {
			model = continuation.Model
		}
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
		case pending.Result <- false:
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
	capabilityContext := fmt.Sprintf("KONFIGURATION:\nApproval=%s; Sandbox=%s; Network=%t; Web=%s; Git=%t; Umgebung=%s; Tempo=%s\nMCP-SERVER:\n%s", cfg.ApprovalMode, cfg.SandboxMode, cfg.NetworkEnabled, cfg.WebSearchProvider, cfg.GitEnabled, cfg.AgentEnvironment, cfg.ResponseSpeed, mcpServersSummary(cfg))
	personalization := strings.TrimSpace(cfg.UserInstructions)
	if personalization == "" {
		personalization = "Keine zusätzlichen persönlichen Anweisungen."
	}
	language := strings.TrimSpace(cfg.PreferredLanguage)
	if language == "" {
		language = "Deutsch"
	}
	systemPrompt := agentSystemPrompt + "\n\nNUTZERPRÄFERENZEN:\n- Antworte in " + language + ".\n- Arbeitsmodus: " + cfg.ResponseSpeed + ".\n- Zusätzliche Anweisungen:\n" + personalization
	messages := []OllamaMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: fmt.Sprintf("PROJEKT: %s\n\n%s\n\nPROJEKTDOKUMENTE:\n%s\n\nPROJEKTSTRUKTUR:\n%s\n\nGIT-STATUS:\n%s\n\nAUFGABE:\n%s%s", filepath.Base(project), capabilityContext, instructions, tree, gitStatusSummary(project), userMessage, attachmentContext)}}
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

	outcome := s.executeAgentLoop(ctx, runID, project, model, messages, cfg)
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
	outcome := s.executeAgentLoop(ctx, runID, project, model, messages, cfg)
	runAfterHook = outcome == "done"
}

func (s *AppState) executeAgentLoop(ctx context.Context, runID, project, model string, messages []OllamaMessage, cfg Config) string {
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
	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			s.AddEvent(UIEvent{Type: "warning", Message: "Vorgang abgebrochen"})
			return "cancelled"
		}
		s.setRunPhase(runID, "model")
		s.AddEvent(UIEvent{Type: "progress", Message: fmt.Sprintf("Modellschritt %d von %d", step, maxSteps), Detail: model})
		modelTimeout := time.Duration(cfg.ModelTimeout) * time.Second
		if modelTimeout <= 0 {
			modelTimeout = 4 * time.Minute
		}
		stepCtx, stepCancel := context.WithTimeout(ctx, modelTimeout)
		action, usedModel, err := s.nextAgentAction(stepCtx, model, trimMessages(messages))
		stepCancel()
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
		if action.Action == "ask_user" {
			s.mu.Lock()
			s.Continuation = &AgentContinuation{
				Project:  project,
				ThreadID: s.CurrentThread,
				Model:    model,
				Question: action.Message,
				Messages: append([]OllamaMessage(nil), messages...),
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
		messages = append(messages, OllamaMessage{Role: "user", Content: "TOOL RESULT for " + action.Action + ":\n" + truncateText(result, 120000)})
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
			result, err = gitRead(project, a.Args...)
		} else {
			return s.performApproved(ctx, project, a)
		}
	case "web_search":
		if actionNeedsApproval(cfg, a) {
			return s.performApproved(ctx, project, a)
		}
		var r []WebResult
		r, err = webSearch(ctx, cfg, a.Query, a.MaxResults)
		result = formatWebResults(r)
	case "web_fetch":
		if actionNeedsApproval(cfg, a) {
			return s.performApproved(ctx, project, a)
		}
		result, err = webFetch(ctx, cfg, a.URL)
	case "mcp_list_tools":
		result, err = mcpCall(ctx, cfg, a.Server, "tools/list", map[string]any{})
	case "mcp_list_resources":
		result, err = mcpCall(ctx, cfg, a.Server, "resources/list", map[string]any{})
	case "mcp_read_resource":
		result, err = mcpCall(ctx, cfg, a.Server, "resources/read", map[string]any{"uri": a.URI})
	case "mcp_list_prompts":
		result, err = mcpCall(ctx, cfg, a.Server, "prompts/list", map[string]any{})
	case "mcp_get_prompt":
		result, err = mcpCall(ctx, cfg, a.Server, "prompts/get", map[string]any{"name": a.PromptName, "arguments": a.Arguments})
	case "mcp_call_tool", "replace_text", "write_file", "delete_file", "run_command", "open_terminal", "copy_path", "move_path":
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
		s.AddEvent(UIEvent{Type: "tool_error", Message: a.Message, Detail: err.Error(), Action: a.Action, Path: a.Path, Command: a.Command})
		return "ERROR: " + err.Error(), false
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
	preview, err := previewAction(project, cfg, a)
	if err != nil {
		return "ERROR: " + err.Error(), false
	}
	approved := true
	if actionNeedsApproval(cfg, a) {
		approved, err = s.requestApprovalWithPreview(ctx, a, preview)
		if err != nil {
			return "ERROR: " + err.Error(), false
		}
	}
	if !approved {
		return "REJECTED BY USER", false
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})
	result, err := executeAction(ctx, project, cfg, a)
	if err != nil {
		s.AddEvent(UIEvent{Type: "tool_error", Message: a.Message, Detail: err.Error(), Preview: preview, Action: a.Action, Path: a.Path, Command: a.Command})
		return "ERROR: " + err.Error(), false
	}
	s.AddEvent(UIEvent{Type: "action_done", Message: a.Message, Detail: truncateText(result, 30000), Preview: preview, Action: a.Action, Path: a.Path, Command: a.Command})
	s.recordAction(a.Action + ": " + a.Message)
	s.UpdateProjectState("Aktion " + a.Action + " ausgeführt")
	if a.Action == "open_terminal" {
		return result, true
	}
	return result, false
}

func actionNeedsApproval(cfg Config, a AgentAction) bool {
	if cfg.ApprovalMode == "dangerous" {
		return false
	}
	switch a.Action {
	case "list_files", "read_file", "search_text", "mcp_list_tools", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt":
		return false
	case "web_search", "web_fetch":
		return cfg.ApprovalMode == "strict"
	case "replace_text", "write_file":
		return cfg.ApprovalMode != "auto"
	case "git":
		if gitActionIsReadOnly(a.Args) {
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

func (s *AppState) requestApprovalWithPreview(ctx context.Context, a AgentAction, preview string) (bool, error) {
	pending := &PendingAction{ID: newID(), Action: a, Preview: preview, Result: make(chan bool, 1)}
	s.mu.Lock()
	s.Pending = pending
	s.mu.Unlock()
	s.AddEvent(UIEvent{ID: pending.ID, Type: "approval_required", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})
	select {
	case approved := <-pending.Result:
		s.mu.Lock()
		if s.Pending != nil && s.Pending.ID == pending.ID {
			s.Pending = nil
		}
		s.mu.Unlock()
		msg := "Abgelehnt"
		if approved {
			msg = "Genehmigt"
		}
		s.AddEvent(UIEvent{Type: "approval", Message: msg, Action: a.Action, Path: a.Path})
		return approved, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(30 * time.Minute):
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
	case "web_search":
		return "Web search: " + a.Query, nil
	case "web_fetch":
		return "Web fetch: " + a.URL, nil
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
	case "run_command":
		return executeCommand(project, a.Command, cfg)
	case "open_terminal":
		if err := openInteractiveTerminal(project, a.Command, cfg); err != nil {
			return "", err
		}
		return "Interaktives Terminal geöffnet. Schließe den Login oder Vorgang dort ab und teile das Ergebnis anschließend mit.", nil
	case "copy_path":
		return copyPath(cfg, project, a.Source, a.Destination)
	case "move_path":
		return movePath(cfg, project, a.Source, a.Destination)
	case "git":
		timeout := time.Duration(cfg.CommandTimeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		gctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return runGit(gctx, project, a.Args)
	case "web_search":
		r, err := webSearch(ctx, cfg, a.Query, a.MaxResults)
		return formatWebResults(r), err
	case "web_fetch":
		return webFetch(ctx, cfg, a.URL)
	case "mcp_call_tool":
		return mcpCall(ctx, cfg, a.Server, "tools/call", map[string]any{"name": a.Tool, "arguments": a.Arguments})
	default:
		return "", errors.New("unsupported approved action")
	}
}
