// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	modelSubagentMaxSteps       = 8
	modelSubagentBootstrapBytes = 42000
	modelSubagentToolBytes      = 14000
)

var modelSubagentActionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action":    map[string]any{"type": "string", "enum": []string{"list_files", "read_file", "search_text", "lsp", "finish"}},
		"message":   map[string]any{"type": "string"},
		"path":      map[string]any{"type": "string"},
		"query":     map[string]any{"type": "string"},
		"name":      map[string]any{"type": "string"},
		"line":      map[string]any{"type": "integer", "minimum": 1},
		"character": map[string]any{"type": "integer", "minimum": 1},
		"max_depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 6},
	},
	"required":             []string{"action", "message"},
	"additionalProperties": false,
	"allOf": []map[string]any{
		conditionalRequired("read_file", "path"),
		conditionalRequired("search_text", "query"),
		conditionalRequired("lsp", "path", "name"),
	},
}

const modelSubagentSystemPrompt = `Du bist ein strikt read-only LocalCode-Subagent. Deine Aufgabe ist unabhängige Repository-Exploration für den Parent-Agenten.

Du darfst ausschließlich genau eine JSON-Aktion pro Schritt liefern: list_files, read_file, search_text, lsp oder finish.
Du darfst niemals Dateien schreiben/löschen/verschieben, Shell/Git/MCP/Netzwerk ausführen, Installationen starten, Genehmigungen anfordern oder einen weiteren Subagenten starten.
Nutze die eingebettete Repository-Intelligence zuerst. Lies nur konkrete relevante Dateien/Ausschnitte nach. Nutze LSP für Definitionen/Referenzen/Symbole/Call-Hierarchy, wenn es ohne neue Nutzerfreigabe zulässig und verfügbar ist.
Beende mit finish. Der finish-Text muss knapp enthalten: relevante Dateien/Symbole, gefundene Abhängigkeiten/Call-Sites, Risiken/Invarianten, empfohlene Änderung und passende Verifikation. Erteile keine Aussage, dass etwas implementiert oder getestet wurde, wenn du es nur analysiert hast.`

func (s *AppState) runReadOnlyModelSubagent(ctx context.Context, project string, cfg Config, task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", errors.New("subagent task is empty")
	}
	if strings.HasPrefix(task, deterministicSubagentTaskPrefix) {
		deterministicTask := strings.TrimSpace(strings.TrimPrefix(task, deterministicSubagentTaskPrefix))
		return runReadOnlySubagent(project, cfg, deterministicTask)
	}
	s.mu.RLock()
	model := strings.TrimSpace(s.Model)
	s.mu.RUnlock()
	if model == "" || s.Ollama == nil {
		return runReadOnlySubagent(project, cfg, task)
	}

	bootstrap := modelSubagentBootstrap(project, cfg, task)
	messages := []OllamaMessage{
		{Role: "system", Content: modelSubagentSystemPrompt},
		{Role: "user", Content: "SUBAGENT TASK:\n" + task + "\n\nINITIAL READ-ONLY EVIDENCE:\n" + truncateText(bootstrap, modelSubagentBootstrapBytes)},
	}
	var trace strings.Builder
	lastSignature := ""
	for step := 1; step <= modelSubagentMaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		timeout := time.Duration(cfg.ModelTimeout) * time.Second
		if timeout <= 0 || timeout > 2*time.Minute {
			timeout = 90 * time.Second
		}
		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		raw, err := s.Ollama.Chat(stepCtx, model, messages, modelSubagentActionSchema)
		cancel()
		if err != nil {
			fallback, fallbackErr := runReadOnlySubagent(project, cfg, task)
			if fallbackErr != nil {
				return "", fmt.Errorf("model subagent failed: %w; deterministic fallback failed: %v", err, fallbackErr)
			}
			return "MODEL SUBAGENT FALLBACK\nReason: " + err.Error() + "\n\n" + fallback, nil
		}
		var action AgentAction
		if err := json.Unmarshal([]byte(raw), &action); err != nil {
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEM: Ungültiges JSON. Liefere genau eine gültige read-only Subagent-Aktion."})
			continue
		}
		action = normalizeAgentAction(action, task)
		if err := validateModelSubagentAction(action); err != nil {
			messages = append(messages,
				OllamaMessage{Role: "assistant", Content: mustJSON(actionForModelContext(action, cfg))},
				OllamaMessage{Role: "user", Content: "SYSTEM: Aktion blockiert: " + err.Error() + ". Verwende nur list_files/read_file/search_text/lsp/finish."},
			)
			continue
		}
		signature := actionSignature(action)
		if signature == lastSignature && action.Action != "finish" {
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEM: Identische Subagent-Aktion wurde blockiert. Nutze andere Evidenz oder finish."})
			continue
		}
		lastSignature = signature
		messages = append(messages, OllamaMessage{Role: "assistant", Content: mustJSON(actionForModelContext(action, cfg))})
		if action.Action == "finish" {
			return "MODEL READ-ONLY SUBAGENT HANDOFF\nModel: " + model + fmt.Sprintf("\nSteps: %d\n\n", step) + strings.TrimSpace(action.Message) + modelSubagentTrace(trace.String()), nil
		}

		s.AddEvent(UIEvent{Type: "agent_step", Message: localizeConfigText(cfg, "Subagent: ", "Subagent: ") + action.Message, Action: "subagent:" + action.Action, Path: action.Path})
		result, toolErr := s.executeModelSubagentAction(ctx, project, cfg, action)
		if toolErr != nil {
			result = "ERROR: " + toolErr.Error()
		}
		fmt.Fprintf(&trace, "\n- %s: %s", action.Action, truncateText(strings.TrimSpace(action.Message), 300))
		messages = append(messages, OllamaMessage{Role: "user", Content: "READ-ONLY TOOL RESULT for " + action.Action + ":\n" + truncateText(result, modelSubagentToolBytes)})
	}

	fallback, err := runReadOnlySubagent(project, cfg, task)
	if err != nil {
		return "", err
	}
	return "MODEL SUBAGENT STEP BUDGET EXHAUSTED\nThe child was stopped after the hard read-only step budget.\n" + modelSubagentTrace(trace.String()) + "\n\nDETERMINISTIC FALLBACK:\n" + fallback, nil
}

func validateModelSubagentAction(action AgentAction) error {
	switch action.Action {
	case "list_files":
		return nil
	case "read_file":
		if strings.TrimSpace(action.Path) == "" {
			return errors.New("read_file requires path")
		}
		return nil
	case "search_text":
		if strings.TrimSpace(action.Query) == "" {
			return errors.New("search_text requires query")
		}
		return nil
	case "lsp":
		return validateAgentAction(action)
	case "finish":
		if strings.TrimSpace(action.Message) == "" {
			return errors.New("finish requires a handoff message")
		}
		return nil
	default:
		return fmt.Errorf("subagent action %q is outside the read-only capability set", action.Action)
	}
}

func (s *AppState) executeModelSubagentAction(ctx context.Context, project string, cfg Config, action AgentAction) (string, error) {
	switch action.Action {
	case "list_files":
		depth := action.MaxDepth
		if depth <= 0 {
			depth = 4
		}
		return projectTree(project, action.Path, depth, 700)
	case "read_file":
		return readProjectFile(project, action.Path)
	case "search_text":
		return searchProject(project, action.Query, action.Path, 100)
	case "lsp":
		if actionNeedsApproval(cfg, project, action) {
			return "", errors.New("LSP skipped inside subagent because current approval policy requires interactive confirmation; use AST/repo-map/search fallback or a persistent LSP allow rule")
		}
		return runLSPAction(ctx, project, cfg, action)
	default:
		return "", fmt.Errorf("unsupported read-only subagent action %q", action.Action)
	}
}

func modelSubagentBootstrap(project string, cfg Config, task string) string {
	var b strings.Builder
	b.WriteString("PROJECT INFO\n")
	b.WriteString(truncateText(projectInfo(project, cfg), 6000))
	b.WriteString("\n\nTASK-RANKED REPOSITORY GRAPH\n")
	if graph, err := repositoryReferenceGraph(project, task); err == nil {
		b.WriteString(truncateText(graph, 30000))
	} else {
		b.WriteString("ERROR: " + err.Error())
	}
	b.WriteString("\n\nPROJECT TREE\n")
	if tree, err := projectTree(project, "", 3, 350); err == nil {
		b.WriteString(truncateText(tree, 7000))
	} else {
		b.WriteString("ERROR: " + err.Error())
	}
	return b.String()
}

func modelSubagentTrace(trace string) string {
	trace = strings.TrimSpace(trace)
	if trace == "" {
		return ""
	}
	return "\n\nREAD-ONLY TRACE:" + trace
}
