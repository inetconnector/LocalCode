from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected anchor once, found {count}")
    return text.replace(old, new, 1)


run_path = Path("src/run_journal.go")
run = run_path.read_text(encoding="utf-8")
if "func writeRunJournalUnlocked(" not in run:
    run = replace_once(
        run,
        "func writeRunJournal(state RunRecoveryState) error {\n\trunJournalFileMu.Lock()\n\tdefer runJournalFileMu.Unlock()\n\tstate.SchemaVersion = runJournalSchemaVersion",
        "func writeRunJournal(state RunRecoveryState) error {\n\trunJournalFileMu.Lock()\n\tdefer runJournalFileMu.Unlock()\n\treturn writeRunJournalUnlocked(state)\n}\n\nfunc writeRunJournalUnlocked(state RunRecoveryState) error {\n\tstate.SchemaVersion = runJournalSchemaVersion",
        "split journal writer",
    )
    run = replace_once(
        run,
        "func (s *AppState) updateRunJournal(runID string, mutate func(*RunRecoveryState)) {\n\tif strings.TrimSpace(runID) == \"\" {\n\t\treturn\n\t}\n\tstate, err := loadRunJournal()\n\tif err != nil || state == nil || state.RunID != runID || state.Terminal {\n\t\treturn\n\t}\n\tmutate(state)\n\t_ = writeRunJournal(*state)\n}",
        "func (s *AppState) updateRunJournal(runID string, mutate func(*RunRecoveryState)) {\n\tif strings.TrimSpace(runID) == \"\" {\n\t\treturn\n\t}\n\trunJournalFileMu.Lock()\n\tdefer runJournalFileMu.Unlock()\n\tstate, err := loadRunJournal()\n\tif err != nil || state == nil || state.RunID != runID || state.Terminal {\n\t\treturn\n\t}\n\tmutate(state)\n\t_ = writeRunJournalUnlocked(*state)\n}",
        "serialize journal updates",
    )
    run = replace_once(
        run,
        "func (s *AppState) finishRunJournal(runID, outcome string) {\n\tif strings.TrimSpace(runID) == \"\" {\n\t\treturn\n\t}\n\tstate, err := loadRunJournal()\n\tif err != nil || state == nil || state.RunID != runID {\n\t\treturn\n\t}\n\tstate.Terminal = true\n\tstate.Phase = \"idle\"\n\tstate.Outcome = sanitizeRunJournalText(outcome, 200)\n\tstate.Events = append(state.Events, RunJournalEvent{At: time.Now(), Type: \"run_end\", Message: state.Outcome})\n\t_ = writeRunJournal(*state)\n}",
        "func (s *AppState) finishRunJournal(runID, outcome string) {\n\tif strings.TrimSpace(runID) == \"\" {\n\t\treturn\n\t}\n\trunJournalFileMu.Lock()\n\tdefer runJournalFileMu.Unlock()\n\tstate, err := loadRunJournal()\n\tif err != nil || state == nil || state.RunID != runID {\n\t\treturn\n\t}\n\tstate.Terminal = true\n\tstate.Phase = \"idle\"\n\tstate.Outcome = sanitizeRunJournalText(outcome, 200)\n\tstate.Events = append(state.Events, RunJournalEvent{At: time.Now(), Type: \"run_end\", Message: state.Outcome})\n\t_ = writeRunJournalUnlocked(*state)\n}",
        "serialize terminal update",
    )
    run = replace_once(
        run,
        "func recoveryStartupEvent(recovery *RunRecoveryState) UIEvent {",
        "func recoveryStartupEvent(cfg Config, recovery *RunRecoveryState) UIEvent {",
        "localized recovery signature",
    )
    run = replace_once(
        run,
        '\t\tMessage:   "Unterbrochenen Agentenlauf erkannt",\n\t\tDetail:    fmt.Sprintf("Phase: %s · Aufgabe: %s\\nMit „Weiter“ kann LocalCode den letzten bestätigten Zustand sicher erneut prüfen. Mutierende Aktionen werden nicht blind wiederholt.", recovery.Phase, sanitizeRunJournalText(recovery.Task, 800)),',
        '\t\tMessage:   localizeConfigText(cfg, "Unterbrochenen Agentenlauf erkannt", "Interrupted agent run detected"),\n\t\tDetail: fmt.Sprintf(localizeConfigText(cfg,\n\t\t\t"Phase: %s · Aufgabe: %s\\nMit „Weiter“ kann LocalCode den letzten bestätigten Zustand sicher erneut prüfen. Mutierende Aktionen werden nicht blind wiederholt.",\n\t\t\t"Phase: %s · Task: %s\\nUse ‘Continue’ to safely re-check the last confirmed state. Mutating actions are never replayed blindly."), recovery.Phase, sanitizeRunJournalText(recovery.Task, 800)),',
        "localized recovery UI",
    )
    run_path.write_text(run, encoding="utf-8", newline="\n")

types_path = Path("src/types.go")
types = types_path.read_text(encoding="utf-8")
types = types.replace("state.AddEvent(recoveryStartupEvent(recovery))", "state.AddEvent(recoveryStartupEvent(cfg, recovery))")
types_path.write_text(types, encoding="utf-8", newline="\n")

agent_path = Path("src/agent.go")
agent = agent_path.read_text(encoding="utf-8")
old = 's.AddEvent(UIEvent{Type: "recovery", Message: "Unterbrochene Aufgabe wird aus dem letzten bestätigten Zustand fortgesetzt", Detail: "Aktuelle Dateien und Git-Zustand werden vor mutierenden Aktionen neu geprüft."})'
new = 's.AddEvent(UIEvent{Type: "recovery", Message: localizeConfigText(cfg, "Unterbrochene Aufgabe wird aus dem letzten bestätigten Zustand fortgesetzt", "Interrupted task is resuming from the last confirmed state"), Detail: localizeConfigText(cfg, "Aktuelle Dateien und Git-Zustand werden vor mutierenden Aktionen neu geprüft.", "Current files and Git state are re-checked before any mutating action.")})'
if old in agent:
    agent = agent.replace(old, new, 1)
agent_path.write_text(agent, encoding="utf-8", newline="\n")
