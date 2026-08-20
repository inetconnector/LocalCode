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
	deterministicSubagentTaskPrefix = "LOCALCODE_DETERMINISTIC_PREFLIGHT\n"
	modelSubagentBootstrapBytes     = 42000
	modelSubagentToolBytes          = 14000
)

type nativeChildAction struct {
	Action    string       `json:"action"`
	Message   string       `json:"message"`
	Path      string       `json:"path,omitempty"`
	Query     string       `json:"query,omitempty"`
	Name      string       `json:"name,omitempty"`
	Line      int          `json:"line,omitempty"`
	Character int          `json:"character,omitempty"`
	MaxDepth  int          `json:"max_depth,omitempty"`
	Result    *AgentResult `json:"result,omitempty"`
}

var nativeChildResultSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{"type": "string", "minLength": 1},
		"findings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string"},
					"summary":  map[string]any{"type": "string", "minLength": 1},
					"path":     map[string]any{"type": "string"},
					"symbol":   map[string]any{"type": "string"},
					"evidence": map[string]any{"type": "string"},
				},
				"required":             []string{"summary"},
				"additionalProperties": false,
			},
		},
		"tests": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string", "minLength": 1},
					"status": map[string]any{"type": "string", "minLength": 1},
					"detail": map[string]any{"type": "string"},
				},
				"required":             []string{"name", "status"},
				"additionalProperties": false,
			},
		},
		"risks": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"severity":   map[string]any{"type": "string", "minLength": 1},
					"summary":    map[string]any{"type": "string", "minLength": 1},
					"mitigation": map[string]any{"type": "string"},
				},
				"required":             []string{"severity", "summary"},
				"additionalProperties": false,
			},
		},
		"suggested_tasks": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role":         map[string]any{"type": "string", "minLength": 1},
					"objective":    map[string]any{"type": "string", "minLength": 1},
					"dependencies": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required":             []string{"role", "objective"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"summary"},
	"additionalProperties": false,
}

var nativeChildActionSchema = map[string]any{
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
		"result":    nativeChildResultSchema,
	},
	"required":             []string{"action", "message"},
	"additionalProperties": false,
	"allOf": []map[string]any{
		conditionalRequired("read_file", "path"),
		conditionalRequired("search_text", "query"),
		conditionalRequired("lsp", "path", "name"),
		conditionalRequired("finish", "result"),
	},
}

func nativeChildSystemPrompt(role AgentRole) string {
	base := `You are an isolated LocalCode child agent. You operate strictly read-only inside one project.
Return exactly one JSON action per step matching the provided schema. Allowed actions: list_files, read_file, search_text, lsp, finish.
Never write/delete/move files, run shell or Git commands, use network/web/MCP/install tools, request approvals, change configuration, access memory, or spawn another child agent.
Use repository intelligence and the provided initial evidence before reading more. Keep exploration narrow. LSP is allowed only when LocalCode can execute it without a new interactive approval.
Finish with a structured result. Do not claim code was changed, built, tested, committed, or integrated unless the supplied evidence explicitly proves that; this child cannot perform those actions.`
	switch role {
	case AgentRolePlanner:
		return base + `
ROLE: PLANNER. Convert evidence into a small dependency-aware implementation plan. Use suggested_tasks for separable follow-up work. Proposals are data only and do not execute or grant capabilities.`
	case AgentRoleReviewer:
		return base + `
ROLE: REVIEWER. Review the stated task/current evidence independently. Focus on requirement gaps, unsafe assumptions, interface regressions, missing verification, and integration risks. Do not inherit or defend the builder's reasoning.`
	default:
		return base + `
ROLE: EXPLORER. Identify task-relevant files, symbols, callers, dependencies, invariants, likely failure modes, and the narrowest useful verification.`
	}
}

func (s *AppState) runReadOnlyModelSubagent(ctx context.Context, project string, cfg Config, task, roleName string) (string, error) {
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
	agentTask, err := newReadOnlyAgentTask(project, model, task, roleName, cfg)
	if err != nil {
		return "", err
	}
	result, err := s.runNativeReadOnlyAgentTask(ctx, project, cfg, agentTask)
	if err != nil {
		return "", err
	}
	return formatAgentResult(agentTask, result), nil
}

func (s *AppState) runNativeReadOnlyAgentTask(ctx context.Context, project string, cfg Config, task AgentTask) (AgentResult, error) {
	task.Budget = normalizeAgentBudget(task.Budget, task.Role, cfg)
	started := time.Now()
	usage := AgentUsage{}
	finishUsage := func() AgentUsage {
		usage.ElapsedMillis = time.Since(started).Milliseconds()
		return usage
	}

	if strings.TrimSpace(task.Model) == "" || s.Ollama == nil {
		return deterministicAgentFallback(project, cfg, task, usage, AgentResultFallback, "local model is unavailable")
	}

	totalCtx, cancelTotal := context.WithTimeout(ctx, time.Duration(task.Budget.TimeSeconds)*time.Second)
	defer cancelTotal()
	bootstrap := modelSubagentBootstrap(project, cfg, task.Objective)
	messages := []OllamaMessage{
		{Role: "system", Content: nativeChildSystemPrompt(task.Role)},
		{Role: "user", Content: nativeChildTaskPrompt(task, truncateText(bootstrap, modelSubagentBootstrapBytes))},
	}
	lastSignature := ""

	for {
		if err := totalCtx.Err(); err != nil {
			usage = finishUsage()
			return deterministicAgentFallback(project, cfg, task, usage, AgentResultBudgetExhausted, "child-agent time budget exhausted")
		}
		if usage.ModelCalls >= task.Budget.ModelCalls {
			usage = finishUsage()
			return deterministicAgentFallback(project, cfg, task, usage, AgentResultBudgetExhausted, "child-agent model-call budget exhausted")
		}
		estimatedRequestTokens := estimateChildMessagesTokens(messages)
		if usage.EstimatedTokens+estimatedRequestTokens > task.Budget.EstimatedTokenBudget {
			usage = finishUsage()
			return deterministicAgentFallback(project, cfg, task, usage, AgentResultBudgetExhausted, "child-agent estimated token budget exhausted")
		}

		stepTimeout := time.Duration(cfg.ModelTimeout) * time.Second
		if stepTimeout <= 0 || stepTimeout > 90*time.Second {
			stepTimeout = 90 * time.Second
		}
		if deadline, ok := totalCtx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < stepTimeout {
				stepTimeout = remaining
			}
		}
		stepCtx, cancelStep := context.WithTimeout(totalCtx, stepTimeout)
		raw, chatErr := s.Ollama.Chat(stepCtx, task.Model, messages, nativeChildActionSchema)
		cancelStep()
		usage.ModelCalls++
		usage.EstimatedTokens += estimatedRequestTokens + estimateChildTokens(raw)
		if chatErr != nil {
			usage = finishUsage()
			return deterministicAgentFallback(project, cfg, task, usage, AgentResultFallback, "model child failed: "+chatErr.Error())
		}

		var action nativeChildAction
		if err := json.Unmarshal([]byte(raw), &action); err != nil {
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEM: Invalid child-agent JSON. Return exactly one valid read-only action."})
			continue
		}
		if err := validateNativeChildAction(action); err != nil {
			messages = append(messages,
				OllamaMessage{Role: "assistant", Content: raw},
				OllamaMessage{Role: "user", Content: "SYSTEM: Child action blocked: " + err.Error() + ". Use only list_files/read_file/search_text/lsp/finish."},
			)
			continue
		}
		signature := nativeChildActionSignature(action)
		if signature == lastSignature && action.Action != "finish" {
			messages = append(messages, OllamaMessage{Role: "user", Content: "SYSTEM: Identical child action blocked. Gather different evidence or finish."})
			continue
		}
		lastSignature = signature
		messages = append(messages, OllamaMessage{Role: "assistant", Content: raw})

		if action.Action == "finish" {
			result := normalizeNativeChildResult(action.Result, task.Role)
			result.Status = AgentResultCompleted
			result.Usage = finishUsage()
			return result, nil
		}
		if usage.ToolCalls >= task.Budget.ToolCalls {
			usage = finishUsage()
			return deterministicAgentFallback(project, cfg, task, usage, AgentResultBudgetExhausted, "child-agent tool-call budget exhausted")
		}

		s.AddEvent(UIEvent{
			Type:    "agent_step",
			Message: childRoleDisplayName(cfg, task.Role) + ": " + strings.TrimSpace(action.Message),
			Detail:  childBudgetDetail(cfg, task.Budget, usage),
			Action:  "subagent:" + string(task.Role) + ":" + action.Action,
			Path:    action.Path,
		})
		result, toolErr := s.executeNativeChildAction(totalCtx, project, cfg, action)
		usage.ToolCalls++
		if toolErr != nil {
			result = "ERROR: " + toolErr.Error()
		}
		messages = append(messages, OllamaMessage{Role: "user", Content: "READ-ONLY TOOL RESULT for " + action.Action + ":\n" + truncateText(result, modelSubagentToolBytes)})
	}
}

func validateNativeChildAction(action nativeChildAction) error {
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
		candidate := AgentAction{Action: "lsp", Path: action.Path, Name: action.Name, Query: action.Query, Line: action.Line, Character: action.Character, Message: action.Message}
		return validateAgentAction(candidate)
	case "finish":
		if action.Result == nil || strings.TrimSpace(action.Result.Summary) == "" {
			return errors.New("finish requires structured result.summary")
		}
		return nil
	default:
		return fmt.Errorf("child action %q is outside the read-only capability set", action.Action)
	}
}

func (s *AppState) executeNativeChildAction(ctx context.Context, project string, cfg Config, action nativeChildAction) (string, error) {
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
		candidate := AgentAction{Action: "lsp", Path: action.Path, Name: action.Name, Query: action.Query, Line: action.Line, Character: action.Character, Message: action.Message}
		if actionNeedsApproval(cfg, project, candidate) {
			return "", errors.New("LSP skipped inside child agent because current approval policy requires interactive confirmation")
		}
		return runLSPAction(ctx, project, cfg, candidate)
	default:
		return "", fmt.Errorf("unsupported read-only child action %q", action.Action)
	}
}

func normalizeNativeChildResult(result *AgentResult, role AgentRole) AgentResult {
	if result == nil {
		return AgentResult{Status: AgentResultBlocked, Summary: "Child agent returned no structured result."}
	}
	out := *result
	out.Status = AgentResultCompleted
	out.Summary = strings.TrimSpace(out.Summary)
	out.ChangedFiles = nil
	out.Commits = nil
	for i := range out.Findings {
		out.Findings[i].Summary = strings.TrimSpace(out.Findings[i].Summary)
	}
	for i := range out.SuggestedTasks {
		out.SuggestedTasks[i].Role = strings.TrimSpace(out.SuggestedTasks[i].Role)
		out.SuggestedTasks[i].Objective = strings.TrimSpace(out.SuggestedTasks[i].Objective)
	}
	if role != AgentRolePlanner {
		out.SuggestedTasks = nil
	}
	return out
}

func deterministicAgentFallback(project string, cfg Config, task AgentTask, usage AgentUsage, status AgentResultStatus, reason string) (AgentResult, error) {
	fallback, err := runReadOnlySubagent(project, cfg, task.Objective)
	if err != nil {
		return AgentResult{}, err
	}
	return AgentResult{
		Status:  status,
		Summary: "Model child could not complete within its isolated runtime; deterministic read-only evidence is attached.",
		Findings: []Finding{{
			Category: "deterministic-fallback",
			Summary:  strings.TrimSpace(reason),
			Evidence: truncateText(fallback, 50000),
		}},
		Risks: []Risk{{Severity: "info", Summary: "No child mutation occurred; the parent should treat fallback evidence as analysis only."}},
		Usage: usage,
	}, nil
}

func modelSubagentBootstrap(project string, cfg Config, task string) string {
	var b strings.Builder
	b.WriteString("PROJECT INFO\n")
	b.WriteString(truncateText(projectInfo(project, cfg), 6000))
	b.WriteString("\n\nTASK-RANKED REPOSITORY INTELLIGENCE\n")
	if intel, err := repositoryIntelligence(project, task); err == nil {
		b.WriteString(truncateText(intel, 15000))
	} else {
		b.WriteString("ERROR: " + err.Error())
	}
	b.WriteString("\n\nREFERENCE GRAPH\n")
	if graph, err := repositoryReferenceGraph(project, task); err == nil {
		b.WriteString(truncateText(graph, 18000))
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

func nativeChildTaskPrompt(task AgentTask, bootstrap string) string {
	payload := map[string]any{
		"task_id":      task.ID,
		"role":         task.Role,
		"objective":    task.Objective,
		"capabilities": task.Capabilities,
		"budget":       task.Budget,
	}
	data, _ := json.Marshal(payload)
	return "CHILD TASK:\n" + string(data) + "\n\nINITIAL READ-ONLY EVIDENCE:\n" + bootstrap
}

func formatAgentResult(task AgentTask, result AgentResult) string {
	payload := map[string]any{
		"task_id": task.ID,
		"role":    task.Role,
		"result":  result,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "STRUCTURED CHILD AGENT RESULT\n{\"error\":\"result serialization failed\"}"
	}
	return "STRUCTURED CHILD AGENT RESULT\n" + string(data)
}

func nativeChildActionSignature(action nativeChildAction) string {
	copyAction := action
	copyAction.Message = ""
	copyAction.Result = nil
	data, _ := json.Marshal(copyAction)
	return string(data)
}

func estimateChildMessagesTokens(messages []OllamaMessage) int64 {
	var total int64
	for _, message := range messages {
		total += estimateChildTokens(message.Role)
		total += estimateChildTokens(message.Content)
	}
	return total
}

func estimateChildTokens(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	// Deliberately labelled estimated: LocalCode's current Ollama Chat helper
	// returns structured content but not usage metadata. This bound protects local
	// context volume until the backend interface can expose exact provider usage.
	return int64((len([]rune(text)) + 3) / 4)
}

func childRoleDisplayName(cfg Config, role AgentRole) string {
	switch role {
	case AgentRolePlanner:
		return localizeConfigText(cfg, "Planer-Subagent", "Planner subagent")
	case AgentRoleReviewer:
		return localizeConfigText(cfg, "Review-Subagent", "Reviewer subagent")
	default:
		return localizeConfigText(cfg, "Explorer-Subagent", "Explorer subagent")
	}
}

func childBudgetDetail(cfg Config, budget AgentBudget, usage AgentUsage) string {
	de := fmt.Sprintf("Budget: Modell %d/%d, Tools %d/%d, geschätzte Tokens %d/%d", usage.ModelCalls, budget.ModelCalls, usage.ToolCalls, budget.ToolCalls, usage.EstimatedTokens, budget.EstimatedTokenBudget)
	en := fmt.Sprintf("Budget: model %d/%d, tools %d/%d, estimated tokens %d/%d", usage.ModelCalls, budget.ModelCalls, usage.ToolCalls, budget.ToolCalls, usage.EstimatedTokens, budget.EstimatedTokenBudget)
	return localizeConfigText(cfg, de, en)
}
