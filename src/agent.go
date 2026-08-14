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
	"regexp"
	"sort"
	"strconv"
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
	Width         int            `json:"width,omitempty"`
	Height        int            `json:"height,omitempty"`
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
	MemoryID      string         `json:"memory_id,omitempty"`
	Scope         string         `json:"scope,omitempty"`
	Skill         string         `json:"skill,omitempty"`
	Resource      string         `json:"resource,omitempty"`
	Script        string         `json:"script,omitempty"`
}

var actionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{
			"list_files", "read_file", "search_text", "replace_text", "write_file", "delete_file", "create_svg_asset", "create_image_asset", "convert_image_asset", "render_asset",
			"project_info", "subagent_analyze", "build_project", "deploy_android", "engine_edit", "engine_repo_map", "engine_lint", "engine_test", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "discover_tool", "tool_inventory", "run_tool", "run_command", "open_terminal", "copy_path", "move_path", "git", "git_commit", "web_search", "web_fetch",
			"mcp_list_tools", "mcp_call_tool", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt",
			"skill_list", "skill_read", "skill_list_resources", "skill_read_resource", "skill_copy_resource", "skill_run_script",
			"memory_remember", "memory_list", "memory_forget",
			"finish", "ask_user",
		}},
		"message": map[string]any{"type": "string"}, "path": map[string]any{"type": "string", "description": "Relative project path."},
		"query": map[string]any{"type": "string"}, "content": map[string]any{"type": "string", "minLength": 1, "description": "Complete non-empty file content for write_file or memory text for memory_remember."},
		"old_text": map[string]any{"type": "string", "minLength": 1}, "new_text": map[string]any{"type": "string"},
		"command": map[string]any{"type": "string"}, "max_depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
		"width": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096}, "height": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096},
		"url": map[string]any{"type": "string"}, "max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
		"server": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}, "uri": map[string]any{"type": "string"},
		"prompt_name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"},
		"args":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"source": map[string]any{"type": "string"}, "destination": map[string]any{"type": "string"},
		"commit_message": map[string]any{"type": "string"}, "stage_all": map[string]any{"type": "boolean"},
		"task":      map[string]any{"type": "string"},
		"memory_id": map[string]any{"type": "string"},
		"scope":     map[string]any{"type": "string", "enum": []string{"project", "global"}},
		"skill":     map[string]any{"type": "string"},
		"resource":  map[string]any{"type": "string"},
		"script":    map[string]any{"type": "string"},
	},
	"required": []string{"action", "message"}, "additionalProperties": false,
	"allOf": []map[string]any{
		conditionalRequired("read_file", "path"),
		conditionalRequired("delete_file", "path"),
		conditionalRequired("search_text", "query"),
		conditionalRequired("replace_text", "path", "old_text"),
		conditionalRequired("write_file", "path", "content"),
		conditionalRequired("create_svg_asset", "path", "content"),
		conditionalRequired("create_image_asset", "path", "content"),
		conditionalRequired("convert_image_asset", "source", "destination"),
		conditionalRequired("render_asset", "source", "destination"),
		conditionalRequired("subagent_analyze", "task"),
		conditionalRequired("run_tool", "tool"),
		conditionalRequired("discover_tool", "tool"),
		conditionalRequired("run_command", "command"),
		conditionalRequired("open_terminal", "command"),
		conditionalRequired("web_fetch", "url"),
		conditionalRequired("mcp_call_tool", "server", "tool"),
		conditionalRequired("skill_read", "skill"),
		conditionalRequired("skill_list_resources", "skill"),
		conditionalRequired("skill_read_resource", "skill", "resource"),
		conditionalRequired("skill_copy_resource", "skill", "resource", "destination"),
		conditionalRequired("skill_run_script", "skill", "script"),
		conditionalRequired("memory_remember", "content"),
		conditionalRequired("memory_forget", "memory_id"),
	},
}

func conditionalRequired(action string, fields ...string) map[string]any {
	return map[string]any{
		"if": map[string]any{
			"properties": map[string]any{"action": map[string]any{"const": action}},
			"required":   []string{"action"},
		},
		"then": map[string]any{"required": fields},
	}
}

var writeFileContentSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"content": map[string]any{"type": "string", "minLength": 1, "description": "Complete non-empty file content."},
	},
	"required":             []string{"content"},
	"additionalProperties": false,
}

const agentSystemPrompt = `Du bist LocalCode, ein präziser autonomer Software-Agent für ein lokales Projekt.

Du arbeitest in einer kontrollierten Werkzeugschleife. Jede Antwort MUSS genau ein JSON-Objekt gemäß dem vorgegebenen Schema enthalten. Kein Markdown um das JSON und keine sichtbare Gedankenkette.

Arbeitsweise:
- Globale/Projekt-Anweisungen, README.md, STATE.md, relevante Cursor-Regeln, lokale Skill-Hinweise, Projektstruktur und der relevante Git-Zustand werden zu Beginn bereits in den Kontext eingebettet. Lies Dateien nur erneut, wenn du einen konkreten Abschnitt brauchst oder der eingebettete Inhalt als gekürzt markiert ist.
- Rate nicht über vorhandenen Code. Lies relevante Dateien und suche gezielt.
- Verwende relative Projektpfade. Externe Pfade nur, wenn Sandbox und Nutzerfreigabe dies erlauben.
- Für echte Quellcodeänderungen ist engine_edit nur dann die bevorzugte Editing Engine, wenn in der Konfiguration eine externe Engine (Aider, Claude Code oder OpenCode) ausgewählt ist. Wenn die Konfiguration "LocalCode nativ" meldet, ist engine_edit nicht verfügbar; nutze dann list_files/read_file/search_text/replace_text/write_file direkt.
- write_file benötigt immer path und vollständigen nicht-leeren content. Melde niemals Erfolg, wenn kein Dateiinhalt geschrieben wurde.
- Für Icons, Diagramme und lokale Vektor-Bilder ist create_svg_asset bevorzugt, wenn eine SVG-Datei passt. Liefere vollständiges, gültiges SVG mit viewBox/Größe; LocalCode prüft XML-Struktur und blockiert Skripte/Event-Handler.
- Für lokale Rasterbilder und Icon-Dateien ist create_image_asset geeignet, wenn du vollständige Bildbytes als data:image/...;base64,... oder Base64 hast. Unterstützt werden PNG, JPG/JPEG, GIF, WebP, ICO und BMP; LocalCode prüft Format-Signatur, Größe und Dimensionen vor dem Schreiben.
- Wenn du lokale Rasterbilder nach PNG/JPG/JPEG/WebP/ICO konvertieren sollst, nutze convert_image_asset(source,destination,width,height). LocalCode dekodiert das Quellbild, skaliert bei expliziter Größe, encodiert das Ziel und prüft das Ergebnis erneut.
- Wenn du SVG oder lokale HTML/Canvas-Dateien in konkrete Dateien rendern sollst, nutze render_asset(source,destination,width,height). Unterstützt werden SVG/HTML/HTM als Quelle und PNG/JPG/JPEG/WebP/ICO als Ziel; LocalCode rendert mit lokalem Edge/Chrome, blockiert externe HTML-Netzwerkreferenzen und prüft die erzeugte Bilddatei.
- Halte Änderungen klein und kohärent.
- Führe vor dem Abschluss passende Tests, Linter und Builds tatsächlich aus.
- Verwende Git für Status, Diffs, Historie, Branches und vom Nutzer verlangte Commits. Keine History-Rewrites, Force-Pushes oder destruktiven Git-Befehle. Ein fehlendes Git-Repository ist bei Analyse, Build oder Deployment nur eine Information und niemals ein Grund, die Aufgabe zu unterbrechen oder nach git init zu fragen. Initialisiere Git nur, wenn der Nutzer Git ausdrücklich verlangt oder eine Git-Operation ohne Repository wirklich notwendig ist.
- Nutze subagent_analyze(task), wenn eine getrennte, unverändernde Exploration oder Testlog-Analyse sinnvoll ist. Diese Aktion liest nur Projektkontext, ausdrücklich erwähnte Dateien und Suchtreffer und liefert einen strukturierten Handoff; sie darf keine Dateien ändern, keine Befehle ausführen, keine Netzwerkzugriffe starten und keine MCP-Tools aufrufen.
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
- Wenn ein benötigter Skill nur im Skill-Index steht oder unklar ist, nutze skill_list und skill_read, bevor du ihn als Arbeitsanweisung anwendest. Wenn ein Skill auf zusätzliche Ressourcen verweist, nutze skill_list_resources und skill_read_resource für Textressourcen. Für binäre oder als Projektdatei benötigte Skill-Ressourcen nutze skill_copy_resource(skill,resource,destination); die Kopie braucht Genehmigung und erweitert keine Berechtigungen. Deklarierte Skill-Skripte oder -Commands darfst du nur mit skill_run_script(skill,script,args) starten; script muss exakt einem im Skill gelisteten scripts/commands-Eintrag entsprechen.
- Wenn der Nutzer eine stabile Präferenz, ein dauerhaft wichtiges Projektfaktum oder eine wiederverwendbare Arbeitsentscheidung nennt, nutze memory_remember. Speichere keine Passwörter, Tokens, privaten Schlüssel oder Geheimnisse. Standard-Scope ist project; global nur für ausdrücklich projektübergreifend nützliche Präferenzen. Wenn der Nutzer das Löschen/Vergessen verlangt, nutze bei Unklarheit zuerst memory_list und lösche dann per konkreter memory_id mit memory_forget.
- finish muss Ergebnis, geänderte Dateien, Tests/Prüfungen, Git-Zustand, Quellen und verbleibende Risiken zusammenfassen.
- ask_user nur, wenn eine echte Entscheidung oder interaktive Benutzeraktion blockiert.

Werkzeuge:
- list_files, read_file, search_text
- replace_text, write_file, delete_file
- create_svg_asset(path,content) für validierte lokale SVG-/Icon-Ressourcen
- create_image_asset(path,content) für validierte lokale PNG/JPG/GIF/WebP/ICO/BMP-Ressourcen aus Data-URL/Base64
- convert_image_asset(source,destination,width,height) für lokale Rasterbild-Konvertierung zu PNG/JPG/JPEG/WebP/ICO
- render_asset(source,destination,width,height) für lokales Rendering von SVG/HTML/Canvas zu PNG/JPG/JPEG/WebP oder ICO
- project_info, subagent_analyze(task), build_project, deploy_android für deterministische Projekt-, Build- und Android-Deployment-Abläufe
- engine_edit(task) für robuste mehrdateilige Codeänderungen mit der ausgewählten Engine und lokalem Backup
- engine_repo_map für eine unverändernde Repository-Analyse, engine_lint und engine_test für gezielte Qualitätsläufe
- aider_edit/aider_repo_map/aider_lint/aider_test sind nur rückwärtskompatible Aliasnamen
- discover_tool(tool), tool_inventory, run_tool(tool,args)
- run_command (komplexe Shell-Befehle, nicht-interaktiv), open_terminal (interaktiv sichtbar)
- copy_path, move_path
- git mit args als Argumentliste, z. B. ["status","--short"]
- git_commit für einen vollständigen, verifizierten Commit-Ablauf (Git initialisieren falls ausdrücklich verlangt, .gitignore ergänzen, Änderungen stagen, Commit ausführen und Ergebnis prüfen)
- web_search mit query/max_results, web_fetch mit url
- mcp_list_tools, mcp_call_tool(server,tool,arguments)
- mcp_list_resources, mcp_read_resource(server,uri)
- mcp_list_prompts, mcp_get_prompt(server,prompt_name,arguments)
- skill_list(query), skill_read(skill), skill_list_resources(skill), skill_read_resource(skill,resource), skill_copy_resource(skill,resource,destination), skill_run_script(skill,script,args)
- memory_remember(content,scope), memory_list(query,scope), memory_forget(memory_id)
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
	// Keep Running true until every final state/event write is complete. Otherwise
	// NewChat can switch the selected thread between the Running=false update and
	// the final status event, causing the old run to write into the new chat.
	s.mu.Lock()
	if s.RunID != runID {
		s.mu.Unlock()
		return
	}
	s.RunPhase = "finalizing"
	s.LastProgressAt = time.Now()
	s.mu.Unlock()
	if runAfterHook {
		s.UpdateProjectState("Agentenlauf beendet")
	} else {
		s.UpdateProjectState("Agent wartet auf Antwort oder wurde abgebrochen")
	}
	s.AddEvent(UIEvent{Type: "status", Message: "Bereit"})
	s.mu.Lock()
	if s.RunID == runID {
		s.Running = false
		s.Cancel = nil
		s.Pending = nil
		s.RunPhase = "idle"
		s.LastProgressAt = time.Now()
	}
	s.mu.Unlock()
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
	instructions := projectInstructionContext(project, userMessage)
	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	engine := normalizeEditingEngine(cfg.EditingEngine)
	engineHint := "ENGINE-HINWEIS: Für mehrdateilige Codeänderungen ist engine_edit verfügbar."
	if engine == editingEngineNative {
		engineHint = "ENGINE-HINWEIS: LocalCode nativ ist aktiv. Verwende nicht engine_edit; schreibe Änderungen mit read_file/search_text/replace_text/write_file und gib bei write_file immer vollständigen nicht-leeren content an."
	}
	capabilityContext := fmt.Sprintf("KONFIGURATION:\nApproval=%s; Sandbox=%s; Network=%t; Web=%s; Git=%t; Umgebung=%s; Tempo=%s; EditingEngine=%s\n%s\nMCP-SERVER:\n%s\nERINNERUNGEN:\n%s\nWERKZEUGERKENNUNG:\n%s", cfg.ApprovalMode, cfg.SandboxMode, cfg.NetworkEnabled, cfg.WebSearchProvider, cfg.GitEnabled, cfg.AgentEnvironment, cfg.ResponseSpeed, codingEngineDisplayName(engine), engineHint, mcpServersSummaryForAgent(ctx, project, cfg), memoryContextForAgent(cfg, project), toolInventorySummary(project, cfg))
	personalization := strings.TrimSpace(cfg.UserInstructions)
	if personalization == "" {
		personalization = "Keine zusätzlichen persönlichen Anweisungen."
	}
	language := responseLanguage(cfg)
	systemPrompt := agentSystemPrompt + "\n\nNUTZERPRÄFERENZEN:\n- Antworte in " + language + ".\n- Arbeitsmodus: " + cfg.ResponseSpeed + ".\n- Zusätzliche Anweisungen:\n" + personalization
	automationHint := taskAutomationHint(userMessage)
	qualityHint := taskQualityHint(userMessage)
	messages := []OllamaMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: fmt.Sprintf("PROJEKT: %s\n\n%s\n\nPROJEKTDOKUMENTE:\n%s\n\nPROJEKTSTRUKTUR:\n%s\n\nGIT-KONTEXT:\n%s\n\nAUFGABE:\n%s%s\n\n%s%s", filepath.Base(project), capabilityContext, instructions, tree, gitContextForTask(project, cfg, userMessage), userMessage, attachmentContext, qualityHint, automationHint)}}
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
	changedPaths := map[string]bool{}
	changedSinceVerification := false
	compactionCount := 0
	lastSignature := ""
	repeatBlocks := 0
	supervisorBlocks := 0
	invalidActionBlocks := 0
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
			invalidActionBlocks++
			if invalidActionBlocks <= 3 {
				s.AddEvent(UIEvent{Type: "warning", Message: localizeConfigText(cfg, "Ungültige Agentenaktion blockiert", "Invalid agent action blocked"), Detail: err.Error()})
				messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS: Die letzte Modellantwort enthielt keine ausführbare Agentenaktion und wurde nicht ausgeführt. Liefere jetzt genau eine gültige JSON-Aktion. Bei write_file sind path und vollständiger nicht-leerer content Pflicht. Wenn die Datei zu groß ist, nutze read_file/search_text und dann replace_text mit eindeutigem Kontext. Frage nicht nach globalen Installationen und wiederhole keine ungültige Aktion."})
				continue
			}
			s.AddEvent(UIEvent{Type: "error", Message: "Modellaufruf fehlgeschlagen", Detail: err.Error()})
			return "error"
		}
		invalidActionBlocks = 0
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
		} else if issues := completionGuardIssues(project, intent, originalTask, changedPaths, changedSinceVerification); len(issues) > 0 {
			supervisorBlocks++
			detail := "- " + strings.Join(issues, "\n- ")
			s.AddEvent(UIEvent{Type: "warning", Message: localizeConfigText(cfg, "Abschlussprüfung blockiert", "Completion guard blocked finish"), Detail: detail})
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEMHINWEIS ZUR ABSCHLUSSPRÜFUNG:\n" + detail + "\n" + completionRepairDirective(originalTask, issues)})
			continue
		}
		s.setRunPhase(runID, "tool:"+action.Action)
		result, done := s.handleAgentAction(ctx, project, action)
		if done {
			return "done"
		}
		completedActions[action.Action] = true
		toolFailed := agentToolResultFailed(result)
		if actionMutatesProject(action) && !toolFailed {
			for _, path := range mutatedActionPaths(action) {
				if strings.TrimSpace(path) != "" {
					changedPaths[path] = true
				}
			}
			changedSinceVerification = true
		}
		if actionVerifiesProject(action) && !toolFailed {
			changedSinceVerification = false
		}
		toolMessage := "TOOL RESULT for " + action.Action + ":\n" + truncateText(result, contextToolResultLimit(cfg))
		if toolFailed {
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
		incompleteAction := action
		retry := append([]OllamaMessage(nil), messages...)
		retry = append(retry, OllamaMessage{Role: "assistant", Content: content}, OllamaMessage{Role: "user", Content: "Antwort ungültig. Antworte ausschließlich mit einem JSON-Objekt des Schemas. Fehler: " + err.Error()})
		content, retryErr := s.Ollama.Chat(ctx, candidate, trimMessages(retry), actionSchema)
		if retryErr == nil {
			action, retryErr = parseAgentAction(content)
		}
		if retryErr == nil {
			return action, candidate, nil
		}
		if recovered, recoverErr := s.repairIncompleteAgentAction(ctx, candidate, messages, chooseMoreSpecificAction(incompleteAction, action), retryErr); recoverErr == nil {
			return recovered, candidate, nil
		} else {
			errs = append(errs, candidate+" repair: "+recoverErr.Error())
		}
		errs = append(errs, candidate+": "+retryErr.Error())
	}
	return zero, requestedModel, fmt.Errorf("kein Modell lieferte eine gültige Agentenaktion: %s", strings.Join(errs, " | "))
}

func chooseMoreSpecificAction(first, second AgentAction) AgentAction {
	if strings.TrimSpace(second.Action) != "" {
		if strings.TrimSpace(second.Path) != "" || strings.TrimSpace(first.Path) == "" {
			return second
		}
	}
	return first
}

func (s *AppState) repairIncompleteAgentAction(ctx context.Context, model string, messages []OllamaMessage, action AgentAction, cause error) (AgentAction, error) {
	if action.Action != "write_file" || strings.TrimSpace(action.Path) == "" || !strings.Contains(cause.Error(), "content") {
		return AgentAction{}, cause
	}
	repairMessages := append([]OllamaMessage(nil), messages...)
	repairPrompt := fmt.Sprintf(`Die vorherige Aktion war unvollständig und wurde nicht ausgeführt:
{"action":"write_file","message":%q,"path":%q}

Erzeuge jetzt ausschließlich den vollständigen nicht-leeren Inhalt für diese Datei. Antworte nur mit dem JSON-Objekt {"content":"..."} gemäß Schema. Schreibe keinen Markdown-Zaun. Der Inhalt muss direkt als Dateiinhalt verwendbar sein.`, action.Message, action.Path)
	repairMessages = append(repairMessages, OllamaMessage{Role: "user", Content: repairPrompt})
	content, err := s.Ollama.Chat(ctx, model, trimMessages(repairMessages), writeFileContentSchema)
	if err != nil {
		return AgentAction{}, err
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return AgentAction{}, err
	}
	if strings.TrimSpace(payload.Content) == "" {
		return AgentAction{}, errors.New("write_file content repair returned empty content")
	}
	action.Content = payload.Content
	return action, nil
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
	a = fillAgentActionFromArguments(a)
	if err := validateAgentAction(a); err != nil {
		return a, err
	}
	return a, nil
}
func mustJSON(v any) string { data, _ := json.Marshal(v); return string(data) }

func fillAgentActionFromArguments(a AgentAction) AgentAction {
	if len(a.Arguments) == 0 {
		return a
	}
	if a.Path == "" {
		a.Path = stringMapArg(a.Arguments, "path")
	}
	if a.Content == "" {
		a.Content = stringMapArg(a.Arguments, "content")
	}
	if a.Query == "" {
		a.Query = stringMapArg(a.Arguments, "query")
	}
	if a.OldText == "" {
		a.OldText = stringMapArg(a.Arguments, "old_text")
	}
	if a.NewText == "" {
		a.NewText = stringMapArg(a.Arguments, "new_text")
	}
	if a.Command == "" {
		a.Command = stringMapArg(a.Arguments, "command")
	}
	if a.Task == "" {
		a.Task = stringMapArg(a.Arguments, "task")
	}
	if a.Tool == "" {
		a.Tool = stringMapArg(a.Arguments, "tool")
	}
	if a.Source == "" {
		a.Source = stringMapArg(a.Arguments, "source")
	}
	if a.Destination == "" {
		a.Destination = stringMapArg(a.Arguments, "destination")
	}
	if a.Width == 0 {
		a.Width = intMapArg(a.Arguments, "width")
	}
	if a.Height == 0 {
		a.Height = intMapArg(a.Arguments, "height")
	}
	if a.MemoryID == "" {
		a.MemoryID = stringMapArg(a.Arguments, "memory_id")
	}
	if a.Scope == "" {
		a.Scope = stringMapArg(a.Arguments, "scope")
	}
	if a.Skill == "" {
		a.Skill = stringMapArg(a.Arguments, "skill")
	}
	if a.Resource == "" {
		a.Resource = stringMapArg(a.Arguments, "resource")
	}
	if a.Script == "" {
		a.Script = stringMapArg(a.Arguments, "script")
	}
	return a
}

func stringMapArg(values map[string]any, name string) string {
	if values == nil {
		return ""
	}
	value, ok := values[name]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func intMapArg(values map[string]any, name string) int {
	if values == nil {
		return 0
	}
	value, ok := values[name]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func validateAgentAction(a AgentAction) error {
	require := func(name, value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s action requires non-empty %s", a.Action, name)
		}
		return nil
	}
	switch a.Action {
	case "read_file", "delete_file":
		return require("path", a.Path)
	case "search_text":
		return require("query", a.Query)
	case "replace_text":
		if err := require("path", a.Path); err != nil {
			return err
		}
		return require("old_text", a.OldText)
	case "write_file":
		if err := require("path", a.Path); err != nil {
			return err
		}
		return require("content", a.Content)
	case "create_svg_asset", "create_image_asset":
		if err := require("path", a.Path); err != nil {
			return err
		}
		return require("content", a.Content)
	case "convert_image_asset", "render_asset":
		if err := require("source", a.Source); err != nil {
			return err
		}
		return require("destination", a.Destination)
	case "subagent_analyze":
		return require("task", a.Task)
	case "run_tool", "discover_tool":
		return require("tool", a.Tool)
	case "run_command", "open_terminal":
		return require("command", a.Command)
	case "web_fetch":
		return require("url", a.URL)
	case "mcp_call_tool":
		if err := require("server", a.Server); err != nil {
			return err
		}
		return require("tool", a.Tool)
	case "skill_read":
		return require("skill", a.Skill)
	case "skill_list_resources":
		return require("skill", a.Skill)
	case "skill_read_resource":
		if err := require("skill", a.Skill); err != nil {
			return err
		}
		return require("resource", a.Resource)
	case "skill_copy_resource":
		if err := require("skill", a.Skill); err != nil {
			return err
		}
		if err := require("resource", a.Resource); err != nil {
			return err
		}
		return require("destination", a.Destination)
	case "skill_run_script":
		if err := require("skill", a.Skill); err != nil {
			return err
		}
		return require("script", a.Script)
	case "memory_remember":
		return require("content", a.Content)
	case "memory_forget":
		return require("memory_id", a.MemoryID)
	}
	return nil
}

func completionGuardIssues(project string, intent taskIntent, originalTask string, changedPaths map[string]bool, changedSinceVerification bool) []string {
	if intent.Kind != "edit" {
		return nil
	}
	var issues []string
	if len(changedPaths) == 0 {
		issues = append(issues, "Es wurde keine erfolgreiche Projektänderung erkannt.")
	}
	mentioned := mentionedProjectFiles(originalTask)
	for _, path := range mentioned {
		if _, ok := changedPaths[path]; !ok {
			changedPaths[path] = false
		}
	}
	paths := make([]string, 0, len(changedPaths))
	for path := range changedPaths {
		if concreteProjectPath(path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var combined strings.Builder
	for _, path := range paths {
		content, err := readProjectFile(project, path)
		if err != nil {
			if isMentionedPath(path, mentioned) {
				issues = append(issues, fmt.Sprintf("Die ausdrücklich erwähnte Datei `%s` konnte nicht gelesen werden: %v.", path, err))
			}
			continue
		}
		if strings.TrimSpace(content) == "" {
			issues = append(issues, fmt.Sprintf("`%s` ist leer.", path))
			continue
		}
		combined.WriteString("\n--- ")
		combined.WriteString(path)
		combined.WriteString(" ---\n")
		combined.WriteString(content)
		if marker := firstPlaceholderMarker(content); marker != "" {
			issues = append(issues, fmt.Sprintf("`%s` enthält noch Platzhalter-/Nicht-implementiert-Text (%s).", path, marker))
		}
	}
	if taskRequestsVerification(originalTask) && changedSinceVerification {
		issues = append(issues, "Nach der letzten Dateiänderung wurde keine passende Prüfung ausgeführt, obwohl die Aufgabe eine Funktions-, Syntax-, Test-, Lint- oder Build-Prüfung verlangt.")
	}
	for _, reviewIssue := range completionReviewIssues(originalTask, paths, changedSinceVerification) {
		issues = append(issues, reviewIssue)
	}
	for _, missing := range missingRequestedCapabilityMarkers(originalTask, combined.String()) {
		issues = append(issues, missing)
	}
	for _, missing := range missingInteractiveImplementationMarkers(originalTask, combined.String()) {
		issues = append(issues, missing)
	}
	for _, missing := range missingArtifactImplementationMarkers(originalTask, combined.String(), paths) {
		issues = append(issues, missing)
	}
	for _, missing := range missingGameImplementationMarkers(originalTask, combined.String()) {
		issues = append(issues, missing)
	}
	return issues
}

func completionRepairDirective(task string, issues []string) string {
	var b strings.Builder
	b.WriteString("Behebe die Ursachen mit konkreten Werkzeugaktionen. Behandle keine Symptome und melde erst finish, wenn diese Punkte wirklich erfüllt sind.\n")
	b.WriteString("Priorität: fehlende Laufzeitlogik gehört in Quellcode-/Konfigurations-/Asset-Dateien, nicht in README oder reine Beschreibungstexte. Ändere Dokumentation erst, nachdem die Funktion selbst existiert.\n")
	b.WriteString("Nutze den Abschluss-Review wie einen zweiten Prüfer: reine Dokumentationsänderungen erfüllen keine Implementierungsaufgabe, und Code-/App-/Tool-Änderungen brauchen nach der letzten Änderung eine passende Prüfung.\n")
	b.WriteString("Wenn eine UI-Kontrolle, Taste, ein Button, Menü, Formular oder Status fehlt, implementiere sowohl das sichtbare Element als auch den Event-Handler und die Zustandsänderung.\n")
	b.WriteString("Wenn Tests/Prüfungen fehlen oder fehlschlagen, installiere nicht reflexhaft global. Nutze vorhandene, projektlokale, verwaltete oder nicht-invasive Prüfungen.\n")
	if containsNormalizedAny(normalizedQuestion(task), []string{"spiel", "game", "app", "tool", "browser", "ui"}) {
		b.WriteString("Bei Apps, Spielen und Tools: behebe zuerst die Kernmechanik und den Datenfluss in der Laufzeitlogik; kosmetische Änderungen zählen nicht als Reparatur fehlender Funktion.\n")
	}
	if len(issues) > 1 {
		b.WriteString("Bearbeite den wichtigsten fehlenden Punkt in der nächsten Aktion vollständig, verifiziere danach, und lass die Abschlussprüfung die verbleibenden Punkte erneut bestimmen.\n")
	}
	return b.String()
}

func actionMutatesProject(a AgentAction) bool {
	switch a.Action {
	case "replace_text", "write_file", "delete_file", "create_svg_asset", "create_image_asset", "convert_image_asset", "render_asset", "skill_copy_resource", "skill_run_script", "copy_path", "move_path", "git_commit", "engine_edit", "aider_edit", "engine_lint", "aider_lint", "engine_test", "aider_test":
		return true
	default:
		return false
	}
}

func mutatedActionPaths(a AgentAction) []string {
	switch a.Action {
	case "replace_text", "write_file", "delete_file", "create_svg_asset", "create_image_asset":
		return []string{a.Path}
	case "convert_image_asset", "render_asset":
		return []string{a.Destination}
	case "skill_copy_resource":
		return []string{a.Destination}
	case "skill_run_script":
		return []string{"."}
	case "copy_path":
		return []string{a.Destination}
	case "move_path":
		return []string{a.Source, a.Destination}
	case "engine_edit", "aider_edit", "engine_lint", "aider_lint", "engine_test", "aider_test", "git_commit":
		return []string{"."}
	default:
		return nil
	}
}

func actionVerifiesProject(a AgentAction) bool {
	switch a.Action {
	case "build_project", "deploy_android", "engine_lint", "aider_lint", "engine_test", "aider_test":
		return true
	case "run_tool":
		return commandLooksLikeVerification(strings.Join(append([]string{a.Tool}, a.Args...), " "))
	case "run_command", "open_terminal":
		return commandLooksLikeVerification(a.Command)
	default:
		return false
	}
}

func agentToolResultFailed(result string) bool {
	lowerResult := strings.ToLower(result)
	return strings.Contains(lowerResult, "error:") ||
		strings.Contains(lowerResult, "status: fehler") ||
		strings.Contains(lowerResult, "status: timeout") ||
		strings.Contains(lowerResult, "exitcode: 1") ||
		strings.Contains(lowerResult, "exitcode: -1")
}

func mentionedProjectFiles(task string) []string {
	seen := map[string]bool{}
	var files []string
	fields := strings.FieldsFunc(task, func(r rune) bool {
		switch r {
		case ' ', '\t', '\r', '\n', '"', '\'', '`', '(', ')', '[', ']', '{', '}', '<', '>', ',', ';', ':', '!', '?':
			return true
		default:
			return false
		}
	})
	for _, field := range fields {
		for _, candidate := range expandMentionedFileCandidate(field) {
			candidate = strings.Trim(candidate, "./\\")
			if candidate == "" || strings.Contains(candidate, "://") {
				continue
			}
			if taskTokenNamesTechnology(candidate) {
				continue
			}
			if !knownProjectFileExt(candidate) {
				continue
			}
			candidate = filepath.ToSlash(filepath.Clean(candidate))
			if strings.HasPrefix(candidate, "../") || candidate == ".." || filepath.IsAbs(candidate) {
				continue
			}
			if !seen[candidate] {
				seen[candidate] = true
				files = append(files, candidate)
			}
		}
	}
	sort.Strings(files)
	return files
}

func expandMentionedFileCandidate(field string) []string {
	candidate := strings.Trim(field, "./\\")
	parts := strings.FieldsFunc(candidate, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) <= 1 {
		return []string{candidate}
	}
	for _, part := range parts {
		if part == "" || !knownProjectFileExt(part) {
			return []string{candidate}
		}
	}
	return parts
}

func knownProjectFileExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".html", ".css", ".json", ".md", ".txt", ".yaml", ".yml", ".toml", ".xml", ".py", ".java", ".kt", ".cs", ".cpp", ".c", ".h", ".rs", ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".ico":
		return true
	default:
		return false
	}
}

func taskTokenNamesTechnology(token string) bool {
	switch strings.ToLower(strings.Trim(token, ".:/\\ ")) {
	case "node.js", "deno.js", "vue.js", "react.js", "next.js", "nuxt.js", "three.js", "d3.js", "pixi.js", "socket.io":
		return true
	default:
		return false
	}
}

func concreteProjectPath(path string) bool {
	path = strings.TrimSpace(path)
	return path != "" && path != "." && path != string(filepath.Separator)
}

func isMentionedPath(path string, mentioned []string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	for _, candidate := range mentioned {
		if clean == filepath.ToSlash(filepath.Clean(candidate)) {
			return true
		}
	}
	return false
}

func firstPlaceholderMarker(content string) string {
	lower := strings.ToLower(content)
	markers := []string{
		"placeholder",
		"platzhalter",
		"not implemented",
		"nicht implementiert",
		"can be added",
		"kann ergänzt",
		"kann spaeter",
		"kann später",
		"noch zu implementieren",
		"implementierung folgt",
		"coming soon",
		"goes here",
		"todo: implement",
		"todo implement",
		"tbd",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func taskRequestsVerification(task string) bool {
	lower := normalizedQuestion(task)
	for _, marker := range []string{"test", "teste", "pruef", "prüf", "syntax", "lint", "build", "kompil", "verifizier", "funktioniert", "works"} {
		if strings.Contains(lower, normalizedQuestion(marker)) {
			return true
		}
	}
	return false
}

func completionReviewIssues(task string, changedPaths []string, changedSinceVerification bool) []string {
	var issues []string
	if taskRequiresRuntimeImplementation(task) && !hasRuntimeImplementationPath(changedPaths) {
		issues = append(issues, "Abschluss-Review: Die Aufgabe verlangt eine Implementierung, aber geändert wurden keine erkennbaren Laufzeit-, Quellcode-, Konfigurations- oder Artefaktdateien. Reine Dokumentation zählt hier nicht.")
	}
	if taskRequiresPostEditVerification(task) && changedSinceVerification {
		issues = append(issues, "Abschluss-Review: Nach einer Code-/App-/Tool-Änderung fehlt eine passende Prüfung nach der letzten Änderung.")
	}
	return issues
}

func taskRequiresRuntimeImplementation(task string) bool {
	normalizedTask := normalizedQuestion(task)
	if taskLooksDocumentationOnly(normalizedTask) || taskLooksPureFileOperation(normalizedTask) {
		return false
	}
	return containsNormalizedAny(normalizedTask, []string{
		"implementiere", "baue", "bau ", "entwickle", "fixe", "behebe", "repariere", "ändere", "aendere", "ergänze", "ergaenze",
		"refaktoriere", "portiere", "implement", "develop", "fix", "repair", "change", "refactor", "port",
		"app", "browser", "web", "ui", "spiel", "game", "tool", "feature", "funktion", "function", "bug", "code", "api",
	})
}

func taskRequiresPostEditVerification(task string) bool {
	normalizedTask := normalizedQuestion(task)
	if taskLooksDocumentationOnly(normalizedTask) || taskLooksPureFileOperation(normalizedTask) {
		return false
	}
	if containsNormalizedAny(normalizedTask, []string{"bild", "image", "grafik", "graphic", "zeichne", "male", "draw", "paint", "diagramm", "diagram", "asset"}) &&
		!containsNormalizedAny(normalizedTask, []string{"app", "browser", "web", "ui", "spiel", "game", "tool", "code"}) {
		return false
	}
	return containsNormalizedAny(normalizedTask, []string{
		"implementiere", "baue", "bau ", "entwickle", "fixe", "behebe", "repariere", "refaktoriere", "portiere",
		"implement", "develop", "fix", "repair", "refactor", "port",
		"app", "browser", "web", "ui", "spiel", "game", "tool", "feature", "funktion", "function", "bug", "code", "api",
	})
}

func taskLooksDocumentationOnly(normalizedTask string) bool {
	if !containsNormalizedAny(normalizedTask, []string{"readme", "dokumentation", "documentation", "docs", "markdown", "state.md", "agents.md", "changelog", "notiz", "note"}) {
		return false
	}
	return !containsNormalizedAny(normalizedTask, []string{"app", "browser", "web", "ui", "spiel", "game", "tool", "code", "funktion", "function", "bug", "api", "runtime"})
}

func taskLooksPureFileOperation(normalizedTask string) bool {
	if !containsNormalizedAny(normalizedTask, []string{"kopiere", "copy", "verschiebe", "move", "benenne um", "rename", "konvertiere", "convert"}) {
		return false
	}
	return !containsNormalizedAny(normalizedTask, []string{"implementiere", "implement", "baue", "develop", "app", "spiel", "game", "tool", "code", "funktion", "function", "bug", "api"})
}

func hasRuntimeImplementationPath(paths []string) bool {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "." {
			return true
		}
		if runtimeImplementationExt(filepath.Ext(path)) {
			return true
		}
	}
	return false
}

func runtimeImplementationExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".html", ".htm", ".css", ".json", ".yaml", ".yml", ".toml", ".xml", ".py", ".java", ".kt", ".cs", ".cpp", ".c", ".h", ".rs", ".sh", ".ps1", ".bat", ".cmd", ".svg":
		return true
	default:
		return false
	}
}

func taskQualityHint(task string) string {
	normalizedTask := normalizedQuestion(task)
	intent := classifyTaskIntent(task)
	if intent.Kind != "edit" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nINTERNE QUALITÄTSANFORDERUNGEN:\n")
	b.WriteString("- Leite aus der Nutzeraufgabe vor dem Umsetzen eine interne Abnahmeliste ab. Implementiere jede verlangte Fähigkeit mit echtem Codepfad, Zustand und Ereignisfluss; reine Begriffe, README-Behauptungen, Alerts oder Beispielwerte reichen nicht.\n")
	b.WriteString("- Behandle Ursachen statt Symptome: Wenn eine Fähigkeit fehlt, ändere die betroffene Logik, nicht nur sichtbare Texte oder Dokumentation.\n")
	b.WriteString("- Diese Regeln gelten sprach- und projektunabhängig: Go, Python, JavaScript/TypeScript, HTML/CSS, Java/Kotlin, C/C++, C#, Rust, Shell, Konfigurationsdateien, Dokumente, Assets und andere Dateitypen müssen mit der jeweils passenden Projektstruktur und Verifikation umgesetzt werden.\n")
	b.WriteString("- Erkenne die Technik aus Dateien, Projektstruktur und project_info. Verwende danach passende Prüfungen: build_project oder projektspezifische Tests, Compiler/Syntaxchecks, Linter, Parser, Formatprüfung oder Datei-/Hash-/Existenzprüfung. Nutze nicht automatisch JavaScript-Prüfungen für andere Sprachen.\n")
	b.WriteString("- Installiere keine Werkzeuge global als Reflex auf eine fehlende Prüfung. Bevorzuge vorhandene LocalCode-Werkzeuge, projektlokale Abhängigkeiten, verwaltete Installationspfade oder eine nicht-invasive Alternative; frage nur nach Installation, wenn die Nutzeraufgabe das wirklich verlangt oder keine sichere Prüfung möglich ist.\n")
	b.WriteString("- Für Apps, Browser-UIs, Spiele und Tools müssen erwartbare Kernabläufe bedienbar sein: Eingaben werden verdrahtet, Zustände werden gehalten, Fehler-/Endzustände bleiben sichtbar und Reset/Undo/Restart-Aktionen setzen alle relevanten Daten konsistent zurück.\n")
	b.WriteString("- Wenn die Aufgabe Buttons, Tastatur, Formulare, Menüs oder andere UI-Kontrollen nennt, müssen Event-Handler und sichtbare Zustandsänderungen implementiert sein.\n")
	b.WriteString("- Für Dateioperationen wie Kopieren, Verschieben, Umbenennen oder Konvertieren: führe die Operation wirklich aus, prüfe Quelle/Ziel, Dateigröße oder Inhalt/Hash soweit sinnvoll, und melde keine Fertigstellung, wenn Zielpfade fehlen.\n")
	b.WriteString("- Für Bild-, Zeichen-, Diagramm- oder Asset-Aufgaben: erstelle ein konkretes Artefakt im passenden Format (z. B. SVG/HTML-Canvas/CSS/Projektasset oder ein vorhandenes verfügbares Bildwerkzeug), prüfe dass die Datei existiert, nicht leer ist und syntaktisch/strukturell zum Format passt. Wenn echte Raster-KI-Bildgenerierung lokal nicht verfügbar ist, wähle ein lokales darstellbares Format oder benenne diese Grenze offen.\n")
	b.WriteString("- Nach jeder Codeänderungsrunde verifiziere passend zur Technik: Syntax-/Unit-/Build-Prüfung und bei sichtbaren Browser-UIs nach Möglichkeit zusätzlich Browser-/DOM-Smoke-Test. Melde finish erst nach erfolgreicher Prüfung nach der letzten Änderung.\n")
	if containsNormalizedAny(normalizedTask, []string{"pac man", "pacman", "pac-man"}) {
		b.WriteString("- Für einen Pac-Man-Klon sind intern mindestens erforderlich: rechteckiges Maze, Wände, Pellets mit Entfernung und Score-Erhöhung, spielbarer Pac-Man, automatisch bewegte Geister, Wandkollisionen, Geisterkollisionen mit Lebensverlust, Game-over-Zustand, Gewinnzustand, Pause, Restart per Taste und Button sowie Tastatursteuerung per Pfeiltasten und WASD.\n")
		b.WriteString("- Parse Startpositionen aus dem Maze oder halte feste Startdaten; setze Gegner beim Restart/Lebensverlust nie zufällig auf mögliche Wände.\n")
		b.WriteString("- Nach Änderungen an game.js muss mindestens eine JavaScript-Syntaxprüfung wie node -c game.js laufen. Wenn Playwright oder ein Browserwerkzeug verfügbar ist, prüfe zusätzlich, dass index.html lädt und Canvas/HUD vorhanden sind.\n")
	}
	return b.String()
}

func commandLooksLikeVerification(command string) bool {
	if regexp.MustCompile(`(?i)\bnode(?:\.exe)?\s+-(?:c|check)\b`).MatchString(command) {
		return true
	}
	languageSpecific := []string{
		`(?i)\bpython(?:\d+(?:\.\d+)?)?(?:\.exe)?\s+-m\s+py_compile\b`,
		`(?i)\bgo(?:\.exe)?\s+(test|vet|build|fmt)\b`,
		`(?i)\bcargo(?:\.exe)?\s+(test|check|build|clippy|fmt)\b`,
		`(?i)\brustc(?:\.exe)?\b`,
		`(?i)\bjavac(?:\.exe)?\b`,
		`(?i)\bjava(?:\.exe)?\s+.*\btest\b`,
		`(?i)\bmvn(?:\.cmd|\.exe)?\s+(test|verify|package|compile)\b`,
		`(?i)\bgradle(?:\.bat|\.cmd|\.exe)?\s+(test|build|check)\b`,
		`(?i)\bdotnet(?:\.exe)?\s+(test|build|format)\b`,
		`(?i)\btsc(?:\.cmd|\.exe)?\b`,
		`(?i)\beslint(?:\.cmd|\.exe)?\b`,
		`(?i)\bphp(?:\.exe)?\s+-l\b`,
		`(?i)\bruby(?:\.exe)?\s+-c\b`,
		`(?i)\b(?:bash|sh)(?:\.exe)?\s+-n\b`,
		`(?i)\bshellcheck(?:\.exe)?\b`,
		`(?i)\b(?:gcc|g\+\+|clang|clang\+\+)(?:\.exe)?\b.*\s+-fsyntax-only\b`,
		`(?i)\bcmake(?:\.exe)?\s+--build\b`,
		`(?i)\bmake(?:\.exe)?\s+(test|check|all)?\b`,
	}
	for _, pattern := range languageSpecific {
		if regexp.MustCompile(pattern).MatchString(command) {
			return true
		}
	}
	lower := normalizedQuestion(command)
	for _, marker := range []string{"test", "go test", "npm test", "pnpm test", "yarn test", "pytest", "vitest", "lint", "vet", "build", "tsc", "check", "playwright"} {
		if strings.Contains(lower, normalizedQuestion(marker)) {
			return true
		}
	}
	return false
}

func commandLooksGlobalInstall(command string) bool {
	patterns := []string{
		`(?i)\bnpm(?:\.cmd|\.exe)?\s+(?:install|i|add)\b[^\r\n]*\s-(?:g|-global)\b`,
		`(?i)\bnpm(?:\.cmd|\.exe)?\s+(?:install|i|add)\b[^\r\n]*\s--global\b`,
		`(?i)\byarn(?:\.cmd|\.exe)?\s+global\s+add\b`,
		`(?i)\bpnpm(?:\.cmd|\.exe)?\s+(?:add|install)\b[^\r\n]*\s-(?:g|-global)\b`,
		`(?i)\bpython(?:\d+(?:\.\d+)?)?(?:\.exe)?\s+-m\s+pip\s+install\b[^\r\n]*\s--user\b`,
		`(?i)\bpip(?:\d+(?:\.\d+)?)?(?:\.exe)?\s+install\b[^\r\n]*\s--user\b`,
		`(?i)\bcargo(?:\.exe)?\s+install\b`,
		`(?i)\bgo(?:\.exe)?\s+install\b`,
		`(?i)\bwinget(?:\.exe)?\s+install\b`,
		`(?i)\bchoco(?:\.exe)?\s+install\b`,
		`(?i)\bscoop(?:\.cmd|\.ps1|\.exe)?\s+install\b`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(command) {
			return true
		}
	}
	return false
}

func taskExplicitlyRequestsGlobalInstall(task string) bool {
	normalizedTask := normalizedQuestion(task)
	return containsNormalizedAny(normalizedTask, []string{"global installieren", "global install", "installiere global", "systemweit installieren", "install system-wide", "installiere das werkzeug", "installiere playwright", "install playwright"})
}

func missingRequestedCapabilityMarkers(task, content string) []string {
	normalizedTask := normalizedQuestion(task)
	normalizedContent := normalizedQuestion(content)
	type requirement struct {
		taskMarkers    []string
		contentMarkers []string
		issue          string
	}
	requirements := []requirement{
		{[]string{"pause", "pausieren"}, []string{"pause", "paused", "pausiert"}, "Die Aufgabe verlangt Pause/Pausieren, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
		{[]string{"restart", "neustart", "neu starten"}, []string{"restart", "reset", "new game", "neustart", "neu starten"}, "Die Aufgabe verlangt Neustart/Restart, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
		{[]string{"game over"}, []string{"game over", "gameover"}, "Die Aufgabe verlangt einen Game-over-Zustand, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
		{[]string{"win-state", "win state", "gewinn", "gewonnen", "sieg"}, []string{"win", "won", "victory", "gewinn", "gewonnen", "sieg"}, "Die Aufgabe verlangt einen Gewinnzustand, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
		{[]string{"score", "punkte", "punktestand"}, []string{"score", "punkte", "punktestand"}, "Die Aufgabe verlangt Punkte/Score, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
		{[]string{"lives", "leben"}, []string{"lives", "leben"}, "Die Aufgabe verlangt Leben/Lives, aber in den geänderten Dateien ist kein entsprechender Implementierungsmarker erkennbar."},
	}
	var issues []string
	for _, req := range requirements {
		if containsNormalizedAny(normalizedTask, req.taskMarkers) && !containsNormalizedAny(normalizedContent, req.contentMarkers) {
			issues = append(issues, req.issue)
		}
	}
	return issues
}

func missingInteractiveImplementationMarkers(task, content string) []string {
	normalizedTask := normalizedQuestion(task)
	if !containsNormalizedAny(normalizedTask, []string{"app", "browser", "web", "ui", "spiel", "game", "tool", "button", "taste", "keyboard", "canvas", "dom", "formular", "form"}) {
		return nil
	}
	type check struct {
		taskMarkers []string
		re          string
		issue       string
	}
	checks := []check{
		{[]string{"button", "knopf", "schaltflaeche", "schaltfläche"}, `(?i)addEventListener\s*\(\s*['"]click['"]|onclick\s*=`, "Die Aufgabe verlangt einen Button/eine Schaltfläche, aber es ist keine verdrahtete Button-Interaktion erkennbar."},
		{[]string{"taste", "tasten", "keyboard", "tastatur", "pfeiltasten", "wasd"}, `(?i)addEventListener\s*\(\s*['"]keydown['"]|onkeydown|KeyboardEvent|case\s+['"]Arrow|key\.toLowerCase\(\)|case\s+['"]w['"]`, "Die Aufgabe verlangt Tastatursteuerung, aber es ist keine konkrete Keydown-/Tastenlogik erkennbar."},
		{[]string{"canvas"}, `(?i)<canvas\b|getContext\s*\(\s*['"]2d['"]|requestAnimationFrame`, "Die Aufgabe verlangt Canvas, aber es ist keine konkrete Canvas-Renderlogik erkennbar."},
		{[]string{"score", "punkte", "zaehler", "zähler", "counter"}, `(?i)\bscore\b|\bcounter\b|\bpoints\b|\bpunkte\b|\bcount\s*(\+\+|\+=|=\s*count\s*\+)`, "Die Aufgabe verlangt Score/Zähler, aber es ist keine konkrete Zählerlogik erkennbar."},
		{[]string{"leben", "lives", "health", "hp"}, `(?i)\blives\b|\bleben\b|\bhealth\b|\bhp\b`, "Die Aufgabe verlangt Leben/Health, aber es ist kein entsprechender Spiel-/App-Zustand erkennbar."},
		{[]string{"zustand", "state", "status", "game over", "gewinn", "win", "pause"}, `(?i)\bstate\b|\bstatus\b|\bpaused\b|\bgameState\b|\bsetState\b|\buseState\b|\bstatusText\b|game\s*over|\bwin\b|\bwon\b|\bvictory\b`, "Die Aufgabe verlangt Zustände/Status, aber es ist keine konkrete Zustandslogik erkennbar."},
		{[]string{"restart", "neustart", "reset", "zuruecksetzen", "zurücksetzen"}, `(?i)\brestart\w*\b|\breset\w*\b|\bnewGame\b`, "Die Aufgabe verlangt Restart/Reset, aber keine konkrete Resetlogik ist erkennbar."},
		{[]string{"browser-app", "browser app", "web-app", "web app", "lokale browser"}, `(?i)<script\b|type\s*=\s*["']module["']|src\s*=|DOMContentLoaded|defer`, "Die Aufgabe verlangt eine Browser-App, aber es ist keine konkrete Script-/App-Verdrahtung erkennbar."},
	}
	var issues []string
	for _, check := range checks {
		if containsNormalizedAny(normalizedTask, check.taskMarkers) && !regexp.MustCompile(check.re).MatchString(content) {
			issues = append(issues, check.issue)
		}
	}
	if containsNormalizedAny(normalizedTask, []string{"funktioniert", "works", "spielbar", "benutzbar", "usable"}) {
		if regexp.MustCompile(`(?i)alert\s*\(\s*['"][^'"]*(?:win|game over|fertig|done)[^'"]*['"]\s*\)\s*;?\s*(?:reset|restart)\w*\s*\(`).MatchString(content) {
			issues = append(issues, "Endzustände dürfen nicht nur kurz per alert erscheinen und sofort zurückgesetzt werden; der Zustand muss im Programm sichtbar gehalten werden.")
		}
		if regexp.MustCompile(`(?i)(not implemented|nicht implementiert|can be added|placeholder|todo)`).MatchString(content) {
			issues = append(issues, "Die Umsetzung enthält noch Nicht-implementiert- oder Platzhaltertext.")
		}
	}
	return issues
}

func missingArtifactImplementationMarkers(task, content string, changedPaths []string) []string {
	normalizedTask := normalizedQuestion(task)
	if !containsNormalizedAny(normalizedTask, []string{"bild", "image", "grafik", "graphic", "zeichne", "male", "draw", "paint", "diagramm", "diagram", "svg", "canvas", "asset"}) {
		return nil
	}
	if !hasConcreteVisualArtifactPath(changedPaths) {
		return []string{"Die Aufgabe verlangt ein Bild/Diagramm/visuelles Asset, aber es wurde keine konkrete Artefaktdatei wie SVG, HTML/Canvas, CSS oder Bilddatei geändert."}
	}
	if regexp.MustCompile(`(?i)<svg\b|<canvas\b|<img\b|drawImage\s*\(|getContext\s*\(|\.(png|jpg|jpeg|webp|gif|bmp|ico)\b|viewBox\s*=`).MatchString(content) {
		return nil
	}
	return []string{"Die Aufgabe verlangt ein Bild/Diagramm/visuelles Asset, aber in den geänderten Dateien ist kein konkretes darstellbares Artefakt erkennbar."}
}

func hasConcreteVisualArtifactPath(paths []string) bool {
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".svg", ".html", ".htm", ".css", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".ico":
			return true
		}
	}
	return false
}

func missingGameImplementationMarkers(task, content string) []string {
	normalizedTask := normalizedQuestion(task)
	if !containsNormalizedAny(normalizedTask, []string{"pac man", "pacman", "pac-man", "spiel", "game", "clone", "klon"}) {
		return nil
	}
	normalizedContent := normalizedQuestion(content)
	var issues []string
	if containsNormalizedAny(normalizedTask, []string{"labyrinth", "maze"}) && !regexp.MustCompile(`(?i)\bmaze\b|labyrinth|wall|wand|#`).MatchString(content) {
		issues = append(issues, "Die Aufgabe verlangt ein Labyrinth/Maze, aber die geänderten Dateien enthalten keine erkennbare Maze-/Wand-Implementierung.")
	}
	if containsNormalizedAny(normalizedTask, []string{"pellet", "pellets", "punkte"}) {
		if !regexp.MustCompile(`(?i)\bpellet(s)?\b|powerPellet|dot(s)?`).MatchString(content) {
			issues = append(issues, "Die Aufgabe verlangt Pellets/Punkte, aber es ist keine konkrete Pellet-Implementierung erkennbar.")
		}
		if !regexp.MustCompile(`(?i)\bscore\s*(\+\+|\+=|=\s*score\s*\+)`).MatchString(content) {
			issues = append(issues, "Die Aufgabe verlangt Score durch Spielaktionen, aber keine konkrete Score-Erhöhung ist erkennbar.")
		}
	}
	if containsNormalizedAny(normalizedTask, []string{"geist", "geister", "ghost", "ghosts", "gegner"}) && !regexp.MustCompile(`(?i)\bghost(s)?\b|enemy|enemies|gegner|geist`).MatchString(content) {
		issues = append(issues, "Die Aufgabe verlangt Geister/Gegner, aber in den geänderten Dateien ist keine Gegnerimplementierung erkennbar.")
	}
	if containsNormalizedAny(normalizedTask, []string{"kollision", "collision", "collisions"}) && !regexp.MustCompile(`(?i)collision|collisions|kollision|isvalidmove|wallat|canmove`).MatchString(normalizedContent) {
		issues = append(issues, "Die Aufgabe verlangt Kollisionen, aber keine erkennbare Kollisionslogik ist vorhanden.")
	}
	if containsNormalizedAny(normalizedTask, []string{"pac man", "pacman", "pac-man"}) {
		issues = append(issues, pacManImplementationIssues(content)...)
	}
	return issues
}

func pacManImplementationIssues(content string) []string {
	var issues []string
	rows := extractConstStringArray(content, "MAZE")
	if len(rows) == 0 {
		issues = append(issues, "Ein Pac-Man-Klon benötigt ein konkretes MAZE-Array.")
	} else {
		width := len(rows[0])
		if len(rows) < 15 || width < 15 {
			issues = append(issues, "Das Pac-Man-Maze ist zu klein; erwartet werden mindestens 15 Zeilen und 15 Spalten.")
		}
		for _, row := range rows {
			if len(row) != width {
				issues = append(issues, "Das Pac-Man-Maze ist nicht rechteckig; alle Zeilen müssen gleich lang sein.")
				break
			}
		}
		joined := strings.Join(rows, "")
		if !strings.Contains(joined, "#") || !strings.Contains(joined, ".") {
			issues = append(issues, "Das Pac-Man-Maze muss Wände und Pellets enthalten.")
		}
		if !strings.Contains(joined, "P") && !regexp.MustCompile(`(?i)pacmanStart`).MatchString(content) {
			issues = append(issues, "Pac-Mans Startposition muss aus dem Maze oder aus festen Startdaten abgeleitet werden.")
		}
		ghostMarkers := 0
		for _, marker := range []string{"A", "B", "C", "G"} {
			ghostMarkers += strings.Count(joined, marker)
		}
		if ghostMarkers < 3 && !regexp.MustCompile(`(?i)ghostStarts\s*=\s*\[`).MatchString(content) {
			issues = append(issues, "Mindestens drei feste Geist-Startpositionen müssen im Maze oder in ghostStarts definiert sein.")
		}
	}
	checks := []struct {
		re    string
		issue string
	}{
		{`(?i)maze\s*=\s*MAZE\.map`, "Pellets müssen in einer veränderbaren Maze-Kopie gespeichert werden."},
		{`(?i)\bpelletsLeft\s*--`, "Beim Einsammeln von Pellets muss pelletsLeft reduziert werden."},
		{`(?i)\bscore\s*(\+=|=\s*score\s*\+)`, "Beim Einsammeln von Pellets muss der Score konkret erhöht werden."},
		{`(?i)\bgameState\b|\bstate\s*=\s*['"](?:playing|paused|won|gameover)`, "Game Over, Pause und Gewinn müssen als Spielzustand gehalten werden, nicht nur per alert."},
		{`(?i)restartButton.+addEventListener|addEventListener.+restartButton`, "Der Restart-Button muss an die Neustartlogik gebunden sein."},
		{`(?i)key\.toLowerCase\(\)|case\s+['"]w['"]`, "WASD-Steuerung muss zusätzlich zu den Pfeiltasten implementiert sein."},
		{`(?i)isWallAtTile|out\s+of\s+bounds|row\s*<\s*0|col\s*<\s*0`, "Wandkollisionen müssen Außenränder/undefinierte Felder sicher als Wand behandeln."},
	}
	for _, check := range checks {
		if !regexp.MustCompile(check.re).MatchString(content) {
			issues = append(issues, check.issue)
		}
	}
	if regexp.MustCompile(`(?i)ghost\.\s*[xy]\s*=\s*Math\.floor\s*\(\s*Math\.random\s*\(`).MatchString(content) {
		issues = append(issues, "Geister dürfen beim Restart oder Lebensverlust nicht zufällig auf mögliche Wände gesetzt werden; nutze feste ghostStarts.")
	}
	if regexp.MustCompile(`(?i)(won|you win|game over)[\s\S]{0,120}resetGame\s*\(`).MatchString(content) {
		issues = append(issues, "Gewinn/Game-over darf nicht sofort resetGame aufrufen; der Endzustand muss sichtbar stehen bleiben.")
	}
	return issues
}

func extractConstStringArray(content, name string) []string {
	re := regexp.MustCompile(`(?s)\bconst\s+` + regexp.QuoteMeta(name) + `\s*=\s*\[(.*?)\]`)
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return nil
	}
	rowRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	rowMatches := rowRe.FindAllStringSubmatch(match[1], -1)
	rows := make([]string, 0, len(rowMatches))
	for _, row := range rowMatches {
		if len(row) > 1 {
			rows = append(rows, row[1])
		}
	}
	return rows
}

func containsNormalizedAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, normalizedQuestion(needle)) {
			return true
		}
	}
	return false
}

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
	case "subagent_analyze":
		result, err = runReadOnlySubagent(project, cfg, a.Task)
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
	case "skill_list":
		result = formatSkillList(project, a.Query)
	case "skill_read":
		result, err = readSkillByName(project, a.Skill)
	case "skill_list_resources":
		result, err = listSkillResources(project, a.Skill)
	case "skill_read_resource":
		result, err = readSkillResource(project, a.Skill, a.Resource)
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
	case "memory_remember", "memory_list", "memory_forget":
		result, err = s.executeMemoryAction(project, a)
	case "mcp_call_tool", "replace_text", "write_file", "delete_file", "create_svg_asset", "create_image_asset", "convert_image_asset", "render_asset", "skill_copy_resource", "skill_run_script", "build_project", "deploy_android", "engine_edit", "engine_repo_map", "engine_lint", "engine_test", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "run_tool", "run_command", "open_terminal", "copy_path", "move_path", "git_commit":
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
	case "discover_tool", "tool_inventory", "project_info", "subagent_analyze", "list_files", "read_file", "search_text", "skill_list", "skill_read", "skill_list_resources", "skill_read_resource", "mcp_list_tools", "mcp_list_resources", "mcp_read_resource", "mcp_list_prompts", "mcp_get_prompt":
		return false
	case "web_search", "web_fetch":
		return cfg.ApprovalMode == "strict"
	case "replace_text", "write_file", "create_svg_asset", "create_image_asset", "convert_image_asset", "render_asset", "skill_copy_resource", "engine_edit", "aider_edit":
		return cfg.ApprovalMode != "auto"
	case "skill_run_script":
		return true
	case "engine_repo_map", "engine_lint", "engine_test", "aider_repo_map", "aider_lint", "aider_test":
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
		updated := strings.Replace(original, a.OldText, a.NewText, 1)
		if err := validateManagedStateWrite(project, cfg, a.Path, updated); err != nil {
			return "", err
		}
		return simpleDiff(original, updated), nil
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
		if err := validateManagedStateWrite(project, cfg, a.Path, a.Content); err != nil {
			return "", err
		}
		return simpleDiff(old, a.Content), nil
	case "create_svg_asset":
		if err := validateSVGAsset(a.Path, a.Content); err != nil {
			return "", err
		}
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
		return simpleDiff(old, strings.TrimSpace(a.Content)+"\n"), nil
	case "create_image_asset":
		data, info, err := validateImageAsset(a.Path, a.Content)
		if err != nil {
			return "", err
		}
		full, err := ensureWithinRoot(project, a.Path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Image asset write preview\nPath: %s\nFormat: %s\nDimensions: %dx%d\nBytes: %d\nExisting: %s", a.Path, info.Format, info.Width, info.Height, len(data), describePathState("target", full)), nil
	case "render_asset":
		plan, err := validateRenderAsset(project, a.Source, a.Destination, a.Width, a.Height)
		if err != nil {
			return "", err
		}
		renderer := "<wird gesucht>"
		for _, candidate := range chromiumBrowserCandidates() {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				renderer = candidate
				break
			}
		}
		return fmt.Sprintf("Render asset preview\nSource: %s\nDestination: %s\nRenderer: %s\nSource format: %s\nTarget format: %s\nDimensions: %dx%d\nNetwork: external HTML references blocked; browser uses a dead local proxy", a.Source, a.Destination, renderer, plan.SourceExt, plan.DestinationExt, plan.Width, plan.Height), nil
	case "convert_image_asset":
		sourceFull, err := ensureWithinRoot(project, a.Source)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(sourceFull)
		if err != nil {
			return "", err
		}
		img, sourceInfo, err := decodeConvertibleImage(data, a.Source)
		if err != nil {
			return "", err
		}
		var destInfo imageAssetInfo
		if strings.EqualFold(filepath.Ext(strings.TrimSpace(a.Destination)), ".webp") {
			width, height := normalizeConvertDimensions(img, a.Width, a.Height)
			destInfo = imageAssetInfo{Format: "webp", Width: width, Height: height}
		} else {
			_, info, err := encodeImageForDestination(img, a.Destination, a.Width, a.Height)
			if err != nil {
				return "", err
			}
			destInfo = info
		}
		full, err := ensureWithinRoot(project, a.Destination)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Image conversion preview\nSource: %s (%s %dx%d)\nDestination: %s (%s %dx%d)\nExisting: %s", a.Source, sourceInfo.Format, sourceInfo.Width, sourceInfo.Height, a.Destination, destInfo.Format, destInfo.Width, destInfo.Height, describePathState("target", full)), nil
	case "skill_copy_resource":
		skill, full, rel, err := resolveSkillResourcePath(project, a.Skill, a.Resource)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(full)
		if err != nil {
			return "", err
		}
		if info.Size() > maxSkillCopyResourceBytes {
			return "", fmt.Errorf("skill resource exceeds %d bytes", maxSkillCopyResourceBytes)
		}
		target, err := ensureWithinRoot(project, a.Destination)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill resource copy preview\nSkill: %s\nResource: %s\nDestination: %s\nBytes: %d\nExisting: %s", skill.Name, filepath.ToSlash(rel), a.Destination, info.Size(), describePathState("target", target)), nil
	case "skill_run_script":
		skill, command, source, err := resolveSkillScriptCommand(project, cfg, a.Skill, a.Script, a.Args)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Skill script preview\nSkill: %s\nDeclared: %s\nSource: %s\nCommand: %s", skill.Name, a.Script, source, command), nil
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
	case "engine_edit", "aider_edit":
		task := strings.TrimSpace(a.Task)
		if task == "" {
			task = strings.TrimSpace(a.Message)
		}
		if task == "" {
			return "", errors.New("editing-engine task is empty")
		}
		engine := normalizeEditingEngine(cfg.EditingEngine)
		name := codingEngineDisplayName(engine)
		model := selectedEngineModel(cfg, "")
		detail := name + " Editing Engine ausführen\nAufgabe: " + task + "\nModell: " + model + "\nVor Änderungen wird ein lokales Backup erzeugt."
		if engine == editingEngineAider {
			files := relevantFilesForAider(project, task, 12)
			detail += "\nVorausgewählte Dateien: " + strings.Join(files, ", ") + "\nAnschließend laufen konfigurierte Linter und Tests."
		}
		return detail, nil
	case "engine_repo_map", "aider_repo_map":
		return codingEngineDisplayName(cfg.EditingEngine) + " Repository-Analyse erzeugen und anzeigen. Es werden keine Dateien verändert.", nil
	case "engine_lint", "aider_lint":
		return codingEngineDisplayName(cfg.EditingEngine) + "-Lintlauf ausführen und gefundene Probleme gemäß Konfiguration reparieren.", nil
	case "engine_test", "aider_test":
		return codingEngineDisplayName(cfg.EditingEngine) + "-Testlauf ausführen und gefundene Probleme gemäß Konfiguration reparieren.", nil
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

func validateManagedStateWrite(project string, cfg Config, path, content string) error {
	stateRel := strings.TrimSpace(cfg.StateFile)
	if stateRel == "" {
		stateRel = "STATE.md"
	}
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	cleanState := filepath.ToSlash(filepath.Clean(stateRel))
	if !strings.EqualFold(cleanPath, cleanState) {
		return nil
	}
	existing, err := readProjectFile(project, stateRel)
	if err != nil {
		return nil
	}
	hasCurrentMarkers := strings.Contains(existing, stateBegin) || strings.Contains(existing, legacyStateBegin)
	if !hasCurrentMarkers {
		return nil
	}
	hasModernPair := strings.Contains(content, stateBegin) && strings.Contains(content, stateEnd)
	hasLegacyPair := strings.Contains(content, legacyStateBegin) && strings.Contains(content, legacyStateEnd)
	if !hasModernPair && !hasLegacyPair {
		return errors.New("STATE.md contains a LocalCode-managed section; write_file/replace_text must preserve the managed state markers instead of overwriting the handoff state")
	}
	return nil
}

func resolveSkillScriptCommand(project string, cfg Config, skillName, script string, args []string) (localSkillSummary, string, string, error) {
	skill, err := findSkillByName(project, skillName)
	if err != nil {
		return localSkillSummary{}, "", "", err
	}
	script = strings.TrimSpace(script)
	if script == "" {
		return localSkillSummary{}, "", "", errors.New("script is empty")
	}
	matched := ""
	for _, declared := range skill.Scripts {
		if strings.TrimSpace(declared) == script {
			matched = strings.TrimSpace(declared)
			break
		}
	}
	if matched == "" {
		return localSkillSummary{}, "", "", fmt.Errorf("script %q is not declared by skill %s", script, skill.Name)
	}
	if len(args) > 64 {
		return localSkillSummary{}, "", "", errors.New("too many script arguments")
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return localSkillSummary{}, "", "", errors.New("script arguments must not contain control line breaks")
		}
		if len(arg) > 4096 {
			return localSkillSummary{}, "", "", errors.New("script argument is too long")
		}
	}
	command := matched
	source := "declared command"
	if skillScriptLooksLikeRelativePath(matched) {
		if cfg.AgentEnvironment == "wsl" || cfg.TerminalShell == "wsl" {
			return localSkillSummary{}, "", "", errors.New("relative skill script files are not supported in WSL mode; declare an explicit WSL-compatible command in the skill")
		}
		_, full, rel, err := resolveSkillResourcePath(project, skill.Name, matched)
		if err != nil {
			return localSkillSummary{}, "", "", err
		}
		source = filepath.ToSlash(rel)
		command = invocationForSkillScriptPath(cfg, full)
	}
	for _, arg := range args {
		command += " " + shellQuoteForConfiguredShell(cfg, arg)
	}
	if err := commandBlocked(cfg, command); err != nil {
		return localSkillSummary{}, "", "", err
	}
	return skill, command, source, nil
}

func skillScriptLooksLikeRelativePath(script string) bool {
	if script == "" || filepath.IsAbs(script) || len(strings.Fields(script)) != 1 {
		return false
	}
	if strings.ContainsAny(script, "\r\n;&|<>`$(){}[]!*?") {
		return false
	}
	normalized := filepath.ToSlash(script)
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
		return false
	}
	return strings.Contains(normalized, "/") || filepath.Ext(normalized) != ""
}

func invocationForSkillScriptPath(cfg Config, path string) string {
	quoted := shellQuoteForConfiguredShell(cfg, path)
	if cfg.TerminalShell != "cmd" && cfg.AgentEnvironment != "wsl" {
		return "& " + quoted
	}
	return quoted
}

func shellQuoteForConfiguredShell(cfg Config, value string) string {
	if cfg.AgentEnvironment == "wsl" {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	if cfg.TerminalShell == "cmd" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func executeAction(ctx context.Context, project string, cfg Config, a AgentAction) (string, error) {
	switch a.Action {
	case "replace_text":
		if current, err := readProjectFile(project, a.Path); err == nil {
			updated := strings.Replace(current, a.OldText, a.NewText, 1)
			if err := validateManagedStateWrite(project, cfg, a.Path, updated); err != nil {
				return "", err
			}
		}
		return replaceText(project, a.Path, a.OldText, a.NewText)
	case "write_file":
		if err := validateManagedStateWrite(project, cfg, a.Path, a.Content); err != nil {
			return "", err
		}
		return writeProjectFile(project, a.Path, a.Content)
	case "create_svg_asset":
		return createSVGAsset(project, a.Path, a.Content)
	case "create_image_asset":
		return createImageAsset(project, a.Path, a.Content)
	case "convert_image_asset":
		return convertImageAsset(ctx, project, cfg, a.Source, a.Destination, a.Width, a.Height)
	case "render_asset":
		return renderAsset(ctx, project, cfg, a.Source, a.Destination, a.Width, a.Height)
	case "skill_copy_resource":
		return copySkillResource(project, a.Skill, a.Resource, a.Destination)
	case "skill_run_script":
		skill, command, source, err := resolveSkillScriptCommand(project, cfg, a.Skill, a.Script, a.Args)
		if err != nil {
			return "", err
		}
		result, err := executeCommand(ctx, project, command, cfg)
		header := fmt.Sprintf("SKILL SCRIPT EXECUTED\nSkill: %s\nDeclared: %s\nSource: %s\nCommand: %s\n\n", skill.Name, a.Script, source, command)
		return header + result, err
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
