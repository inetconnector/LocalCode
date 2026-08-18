// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

const (
	editReliabilityFast     = "fast"
	editReliabilityBalanced = "balanced"
	editReliabilityThorough = "thorough"
)

func editReliabilityMode(cfg Config) string {
	switch strings.ToLower(strings.TrimSpace(cfg.ResponseSpeed)) {
	case editReliabilityFast:
		return editReliabilityFast
	case editReliabilityThorough:
		return editReliabilityThorough
	default:
		return editReliabilityBalanced
	}
}

func actionCompleted(completed map[string]bool, names ...string) bool {
	for _, name := range names {
		if completed[name] {
			return true
		}
	}
	return false
}

// forcedEditReliabilityAction turns an edit request into a deterministic
// preflight -> edit -> verify pipeline. The language model still decides the
// implementation details; LocalCode decides which safety/reliability stage must
// happen next so a model cannot skip context gathering or verification simply
// because it is eager to finish.
func forcedEditReliabilityAction(intent taskIntent, completed map[string]bool, cfg Config) *AgentAction {
	mode := editReliabilityMode(cfg)
	engine := normalizeEditingEngine(cfg.EditingEngine)
	externalEngine := engine != editingEngineNative && codingEngineEnabled(cfg, engine)

	if !completed["project_info"] {
		return &AgentAction{
			Action:  "project_info",
			Message: "Projekt, Buildsystem und Verifikationsmöglichkeiten deterministisch erfassen",
		}
	}

	if mode != editReliabilityFast && !completed["subagent_analyze"] {
		return &AgentAction{
			Action:  "subagent_analyze",
			Message: "Unabhängigen Read-only-Preflight mit Repository-Intelligence durchführen",
			Task:    "Analyze the requested change before any edit. Build a repository intelligence map, identify the most relevant implementation and test files, architecture invariants, likely failure modes, and the narrowest reliable verification plan. Do not modify anything. User task: " + intent.OriginalTask,
		}
	}

	if !externalEngine {
		// Native LocalCode edits are executed as fine-grained file/tool actions by
		// the main agent. The deterministic preflight above still applies, while
		// the system prompt requires the agent to run project verification before
		// finish.
		return nil
	}

	if !actionCompleted(completed, "engine_edit", "aider_edit") {
		return &AgentAction{
			Action:  "engine_edit",
			Message: "Codeänderungen mit " + codingEngineDisplayName(engine) + " planvoll umsetzen",
			Task:    reliabilityEngineTask(intent.OriginalTask, mode),
		}
	}

	if mode == editReliabilityThorough && !actionCompleted(completed, "engine_lint", "aider_lint") {
		return &AgentAction{
			Action:  "engine_lint",
			Message: "Geänderten Code linten und gefundene Fehler reparieren",
			Task:    intent.OriginalTask,
		}
	}

	if mode != editReliabilityFast && !actionCompleted(completed, "engine_test", "aider_test") {
		return &AgentAction{
			Action:  "engine_test",
			Message: "Änderungen durch Tests und Build verifizieren und Fehler reparieren",
			Task:    intent.OriginalTask,
		}
	}

	return nil
}

func reliabilityEngineTask(task, mode string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return task
	}
	var b strings.Builder
	b.WriteString(task)
	b.WriteString("\n\nLOCALCODE RELIABILITY CONTRACT:\n")
	b.WriteString("1. Inspect the relevant existing implementation and tests before editing; do not guess APIs or project structure.\n")
	b.WriteString("2. Make the smallest coherent change that fully satisfies the task and preserve unrelated behavior.\n")
	b.WriteString("3. Do not report success if no intended project file changed.\n")
	b.WriteString("4. Inspect the resulting diff/changed files for accidental edits, placeholders, truncated files, duplicated logic, or generated artifacts that do not belong in source control.\n")
	b.WriteString("5. Leave the project in a state that can be verified by its native lint/test/build commands; do not commit or push.\n")
	if mode == editReliabilityThorough {
		b.WriteString("6. Be conservative about public API changes and trace important call sites/usages before changing a shared symbol.\n")
	}
	return b.String()
}
