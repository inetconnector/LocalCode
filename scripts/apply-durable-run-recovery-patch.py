from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected anchor once, found {count}")
    return text.replace(old, new, 1)


types_path = Path("src/types.go")
types = types_path.read_text(encoding="utf-8")
if "Recovery       *RunRecoveryState" not in types:
    types = replace_once(
        types,
        "\tRunStartedAt   time.Time\n\tLastProgressAt time.Time\n\n\tEvents",
        "\tRunStartedAt   time.Time\n\tLastProgressAt time.Time\n\tRecovery       *RunRecoveryState\n\n\tEvents",
        "AppState recovery field",
    )
    types = replace_once(
        types,
        "func NewAppState(cfg Config, ollama *OllamaClient) *AppState {\n\tthreads := loadThreads()\n\tstate := &AppState{",
        "func NewAppState(cfg Config, ollama *OllamaClient) *AppState {\n\tthreads := loadThreads()\n\trecovery := loadRecoverableRun()\n\tstate := &AppState{",
        "load recoverable run",
    )
    types = replace_once(
        types,
        "\t\tModel:          cfg.LastModel,\n\t\tThreads:        threads,",
        "\t\tModel:          cfg.LastModel,\n\t\tRecovery:       recovery,\n\t\tThreads:        threads,",
        "assign recovery",
    )
    types = replace_once(
        types,
        "\t}\n\treturn state\n}\n\nfunc (s *AppState) AddEvent(ev UIEvent) {",
        "\t}\n\tif recovery != nil {\n\t\tif thread := threads[recovery.ThreadID]; thread != nil && !thread.Archived && strings.EqualFold(filepath.Clean(thread.Project), filepath.Clean(recovery.Project)) {\n\t\t\tstate.CurrentThread = thread.ID\n\t\t\tstate.Project = thread.Project\n\t\t\tstate.Events = append([]UIEvent(nil), thread.Events...)\n\t\t\tif thread.Model != \"\" {\n\t\t\t\tstate.Model = thread.Model\n\t\t\t}\n\t\t}\n\t\tstate.AddEvent(recoveryStartupEvent(recovery))\n\t}\n\treturn state\n}\n\nfunc (s *AppState) AddEvent(ev UIEvent) {",
        "startup recovery event",
    )
    types = replace_once(
        types,
        "\tsubs := make([]chan UIEvent, 0, len(s.subscribers))\n\tfor ch := range s.subscribers {\n\t\tsubs = append(subs, ch)\n\t}\n\ts.mu.Unlock()\n\ts.queueThreadSave(threadSnapshot)\n\n\tfor _, ch := range subs {",
        "\tsubs := make([]chan UIEvent, 0, len(s.subscribers))\n\tfor ch := range s.subscribers {\n\t\tsubs = append(subs, ch)\n\t}\n\tjournalRunID := \"\"\n\tif s.Running {\n\t\tjournalRunID = s.RunID\n\t}\n\ts.mu.Unlock()\n\ts.queueThreadSave(threadSnapshot)\n\tif journalRunID != \"\" {\n\t\ts.journalRunEvent(journalRunID, ev)\n\t}\n\n\tfor _, ch := range subs {",
        "journal UI event",
    )
    types_path.write_text(types, encoding="utf-8", newline="\n")

agent_path = Path("src/agent.go")
agent = agent_path.read_text(encoding="utf-8")
if "beginRunJournal(runID" not in agent:
    agent = replace_once(
        agent,
        "\tcfg := s.Config\n\ts.mu.Unlock()\n\n\tagentMessage := userMessage",
        "\tcfg := s.Config\n\tthreadID := s.CurrentThread\n\ts.mu.Unlock()\n\n\tagentMessage := userMessage",
        "capture thread id",
    )
    agent = replace_once(
        agent,
        "\t}\n\n\t_ = saveConfig(cfg)\n\t_ = ensureProjectDocs(project, cfg)",
        "\t}\n\n\tjournalTask := agentMessage\n\tif isContinuation && continuation != nil && strings.TrimSpace(continuation.OriginalTask) != \"\" {\n\t\tjournalTask = continuation.OriginalTask\n\t}\n\ts.beginRunJournal(runID, project, model, journalTask, threadID, now)\n\n\t_ = saveConfig(cfg)\n\t_ = ensureProjectDocs(project, cfg)",
        "begin journal",
    )
    agent = replace_once(
        agent,
        "\t\t\ts.AddEvent(UIEvent{Type: \"status\", Message: localizeConfigText(cfg, \"Bereit\", \"Ready\")})\n\t\t\treturn nil\n\t\t}\n\t\ts.mu.Lock()",
        "\t\t\ts.AddEvent(UIEvent{Type: \"status\", Message: localizeConfigText(cfg, \"Bereit\", \"Ready\")})\n\t\t\ts.finishRunJournal(runID, \"memory_failed\")\n\t\t\treturn nil\n\t\t}\n\t\ts.mu.Lock()",
        "memory failure terminal",
    )
    agent = replace_once(
        agent,
        "\t\ts.UpdateProjectState(localizeConfigText(cfg, \"Erinnerungsaktion abgeschlossen\", \"Memory action completed\"))\n\t\ts.AddEvent(UIEvent{Type: \"status\", Message: localizeConfigText(cfg, \"Bereit\", \"Ready\")})\n\t\treturn nil",
        "\t\ts.UpdateProjectState(localizeConfigText(cfg, \"Erinnerungsaktion abgeschlossen\", \"Memory action completed\"))\n\t\ts.AddEvent(UIEvent{Type: \"status\", Message: localizeConfigText(cfg, \"Bereit\", \"Ready\")})\n\t\ts.finishRunJournal(runID, \"memory_completed\")\n\t\treturn nil",
        "memory success terminal",
    )
    agent = replace_once(
        agent,
        "func (s *AppState) StopAgent() bool {\n\ts.mu.Lock()\n\tcancel := s.Cancel\n\trunning := s.Running",
        "func (s *AppState) StopAgent() bool {\n\ts.mu.Lock()\n\tcancel := s.Cancel\n\trunning := s.Running\n\trunID := s.RunID",
        "stop capture run id",
    )
    agent = replace_once(
        agent,
        "\ts.mu.Unlock()\n\tif cancel != nil {\n\t\tcancel()\n\t}\n\tif running {\n\t\ts.AddEvent(UIEvent{Type: \"warning\", Message: \"Abbruch angefordert\"",
        "\ts.mu.Unlock()\n\tif running {\n\t\ts.journalRunPhase(runID, \"cancelling\")\n\t}\n\tif cancel != nil {\n\t\tcancel()\n\t}\n\tif running {\n\t\ts.AddEvent(UIEvent{Type: \"warning\", Message: \"Abbruch angefordert\"",
        "stop journal phase",
    )
    agent = replace_once(
        agent,
        "func (s *AppState) ForceStopAgent() bool {\n\ts.mu.Lock()\n\tcancel := s.Cancel\n\tpending := s.Pending\n\twasRunning := s.Running",
        "func (s *AppState) ForceStopAgent() bool {\n\ts.mu.Lock()\n\tcancel := s.Cancel\n\tpending := s.Pending\n\twasRunning := s.Running\n\trunID := s.RunID",
        "force stop capture run id",
    )
    agent = replace_once(
        agent,
        "\ts.mu.Unlock()\n\tif cancel != nil {\n\t\tcancel()\n\t}\n\tif pending != nil {",
        "\ts.mu.Unlock()\n\tif wasRunning {\n\t\ts.finishRunJournal(runID, \"force_stopped\")\n\t}\n\tif cancel != nil {\n\t\tcancel()\n\t}\n\tif pending != nil {",
        "force stop journal terminal",
    )
    agent = replace_once(
        agent,
        "func (s *AppState) setRunPhase(runID, phase string) {\n\ts.mu.Lock()\n\tif s.Running && s.RunID == runID {\n\t\ts.RunPhase = phase\n\t\ts.LastProgressAt = time.Now()\n\t}\n\ts.mu.Unlock()\n}",
        "func (s *AppState) setRunPhase(runID, phase string) {\n\ts.mu.Lock()\n\tupdated := s.Running && s.RunID == runID\n\tif updated {\n\t\ts.RunPhase = phase\n\t\ts.LastProgressAt = time.Now()\n\t}\n\ts.mu.Unlock()\n\tif updated {\n\t\ts.journalRunPhase(runID, phase)\n\t}\n}",
        "set phase journal",
    )
    agent = replace_once(
        agent,
        "\ts.mu.Unlock()\n}\n\nfunc (s *AppState) prepareAttachmentContext",
        "\ts.mu.Unlock()\n\toutcome := \"completed\"\n\tif !runAfterHook {\n\t\toutcome = \"waiting_for_user_or_cancelled\"\n\t}\n\ts.finishRunJournal(runID, outcome)\n}\n\nfunc (s *AppState) prepareAttachmentContext",
        "finish journal terminal",
    )
    agent = replace_once(
        agent,
        "\ts.mu.RLock()\n\tcfg := s.Config\n\ts.mu.RUnlock()\n\tengine := normalizeEditingEngine(cfg.EditingEngine)",
        "\ts.mu.RLock()\n\tcfg := s.Config\n\ts.mu.RUnlock()\n\trecoveryContext, recoveredTask := s.consumeRecoveryContextForTask(project, userMessage)\n\teffectiveTask := userMessage\n\tif recoveredTask != \"\" {\n\t\teffectiveTask = recoveredTask\n\t\ts.journalRunTask(runID, effectiveTask)\n\t\tinstructions += \"\\n\\n\" + recoveryContext\n\t\ts.AddEvent(UIEvent{Type: \"recovery\", Message: \"Unterbrochene Aufgabe wird aus dem letzten bestätigten Zustand fortgesetzt\", Detail: \"Aktuelle Dateien und Git-Zustand werden vor mutierenden Aktionen neu geprüft.\"})\n\t}\n\tengine := normalizeEditingEngine(cfg.EditingEngine)",
        "consume recovery",
    )
    agent = replace_once(agent, "automationHint := taskAutomationHint(userMessage)", "automationHint := taskAutomationHint(effectiveTask)", "automation task")
    agent = replace_once(agent, "qualityHint := taskQualityHint(userMessage)", "qualityHint := taskQualityHint(effectiveTask)", "quality task")
    agent = replace_once(agent, "gitContextForTask(project, cfg, userMessage), userMessage, attachmentContext", "gitContextForTask(project, cfg, effectiveTask), effectiveTask, attachmentContext", "recovered prompt task")
    agent = replace_once(agent, "executeAgentLoop(ctx, runID, project, model, messages, cfg, \"\", userMessage)", "executeAgentLoop(ctx, runID, project, model, messages, cfg, \"\", effectiveTask)", "recovered loop task")
    agent_path.write_text(agent, encoding="utf-8", newline="\n")
