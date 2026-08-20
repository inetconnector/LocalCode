// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"time"
)

type AgentRole string

const (
	AgentRoleExplorer AgentRole = "explorer"
	AgentRolePlanner  AgentRole = "planner"
	AgentRoleReviewer AgentRole = "reviewer"
)

type AgentTaskState string

const (
	AgentTaskPending   AgentTaskState = "pending"
	AgentTaskRunning   AgentTaskState = "running"
	AgentTaskCompleted AgentTaskState = "completed"
	AgentTaskBlocked   AgentTaskState = "blocked"
	AgentTaskFailed    AgentTaskState = "failed"
)

type AgentResultStatus string

const (
	AgentResultCompleted       AgentResultStatus = "completed"
	AgentResultBlocked         AgentResultStatus = "blocked"
	AgentResultFallback        AgentResultStatus = "fallback"
	AgentResultBudgetExhausted AgentResultStatus = "budget_exhausted"
)

type AgentCapability string

const (
	AgentCapabilityRepositoryRead AgentCapability = "repository-read"
	AgentCapabilityLSP            AgentCapability = "lsp"
	AgentCapabilityPlanning       AgentCapability = "planning"
	AgentCapabilityReview         AgentCapability = "review"
)

type AgentBudget struct {
	ModelCalls           int   `json:"model_calls"`
	ToolCalls            int   `json:"tool_calls"`
	EstimatedTokenBudget int64 `json:"estimated_token_budget"`
	TimeSeconds          int   `json:"time_seconds"`
}

type AgentUsage struct {
	ModelCalls      int   `json:"model_calls"`
	ToolCalls       int   `json:"tool_calls"`
	EstimatedTokens int64 `json:"estimated_tokens"`
	ElapsedMillis   int64 `json:"elapsed_millis"`
}

type Finding struct {
	Category string `json:"category,omitempty"`
	Summary  string `json:"summary"`
	Path     string `json:"path,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type TestResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Risk struct {
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	Mitigation string `json:"mitigation,omitempty"`
}

type AgentTaskProposal struct {
	Role         string            `json:"role"`
	Objective    string            `json:"objective"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Capabilities []AgentCapability `json:"capabilities,omitempty"`
}

type AgentResult struct {
	Status         AgentResultStatus  `json:"status"`
	Summary        string             `json:"summary"`
	Findings       []Finding          `json:"findings,omitempty"`
	ChangedFiles   []string           `json:"changed_files,omitempty"`
	Commits        []string           `json:"commits,omitempty"`
	Tests          []TestResult       `json:"tests,omitempty"`
	Risks          []Risk             `json:"risks,omitempty"`
	SuggestedTasks []AgentTaskProposal `json:"suggested_tasks,omitempty"`
	Usage          AgentUsage         `json:"usage"`
}

type AgentTask struct {
	ID           string            `json:"id"`
	ParentID     string            `json:"parent_id,omitempty"`
	MissionID    string            `json:"mission_id,omitempty"`
	Role         AgentRole         `json:"role"`
	Objective    string            `json:"objective"`
	Dependencies []string          `json:"dependencies,omitempty"`
	State        AgentTaskState    `json:"state"`
	Workspace    string            `json:"workspace,omitempty"`
	Worktree     string            `json:"worktree,omitempty"`
	Capabilities []AgentCapability `json:"capabilities"`
	Model        string            `json:"model,omitempty"`
	Budget       AgentBudget       `json:"budget"`
	Result       AgentResult       `json:"result,omitempty"`
}

func normalizeAgentRole(raw string) (AgentRole, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "explorer", "explore", "analysis", "analyze":
		return AgentRoleExplorer, nil
	case "planner", "plan":
		return AgentRolePlanner, nil
	case "reviewer", "review":
		return AgentRoleReviewer, nil
	default:
		return "", fmt.Errorf("unsupported child-agent role %q; allowed roles: explorer, planner, reviewer", raw)
	}
}

func capabilitiesForAgentRole(role AgentRole) []AgentCapability {
	base := []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP}
	switch role {
	case AgentRolePlanner:
		return append(base, AgentCapabilityPlanning)
	case AgentRoleReviewer:
		return append(base, AgentCapabilityReview)
	default:
		return base
	}
}

func defaultAgentBudget(role AgentRole, cfg Config) AgentBudget {
	budget := AgentBudget{ModelCalls: 8, ToolCalls: 12, EstimatedTokenBudget: 100000, TimeSeconds: 180}
	switch role {
	case AgentRolePlanner:
		budget = AgentBudget{ModelCalls: 6, ToolCalls: 8, EstimatedTokenBudget: 80000, TimeSeconds: 150}
	case AgentRoleReviewer:
		budget = AgentBudget{ModelCalls: 6, ToolCalls: 10, EstimatedTokenBudget: 85000, TimeSeconds: 150}
	}
	if cfg.ModelTimeout > 0 {
		maxSeconds := cfg.ModelTimeout * budget.ModelCalls
		if maxSeconds > 0 && maxSeconds < budget.TimeSeconds {
			budget.TimeSeconds = maxSeconds
		}
	}
	if budget.TimeSeconds < 30 {
		budget.TimeSeconds = 30
	}
	if budget.TimeSeconds > int((5 * time.Minute).Seconds()) {
		budget.TimeSeconds = int((5 * time.Minute).Seconds())
	}
	return budget
}

func normalizeAgentBudget(budget AgentBudget, role AgentRole, cfg Config) AgentBudget {
	defaults := defaultAgentBudget(role, cfg)
	if budget.ModelCalls <= 0 || budget.ModelCalls > defaults.ModelCalls {
		budget.ModelCalls = defaults.ModelCalls
	}
	if budget.ToolCalls <= 0 || budget.ToolCalls > defaults.ToolCalls {
		budget.ToolCalls = defaults.ToolCalls
	}
	if budget.EstimatedTokenBudget <= 0 || budget.EstimatedTokenBudget > defaults.EstimatedTokenBudget {
		budget.EstimatedTokenBudget = defaults.EstimatedTokenBudget
	}
	if budget.TimeSeconds <= 0 || budget.TimeSeconds > defaults.TimeSeconds {
		budget.TimeSeconds = defaults.TimeSeconds
	}
	return budget
}

func newReadOnlyAgentTask(project, model, objective, roleName string, cfg Config) (AgentTask, error) {
	role, err := normalizeAgentRole(roleName)
	if err != nil {
		return AgentTask{}, err
	}
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return AgentTask{}, fmt.Errorf("subagent task is empty")
	}
	return AgentTask{
		ID:           fmt.Sprintf("subagent-%s-%d", role, time.Now().UnixNano()),
		Role:         role,
		Objective:    objective,
		State:        AgentTaskPending,
		Workspace:    project,
		Capabilities: capabilitiesForAgentRole(role),
		Model:        strings.TrimSpace(model),
		Budget:       defaultAgentBudget(role, cfg),
	}, nil
}
