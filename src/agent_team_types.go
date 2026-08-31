// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"time"
)

type AgentRole string

const (
	AgentRoleExplorer   AgentRole = "explorer"
	AgentRolePlanner    AgentRole = "planner"
	AgentRoleReviewer   AgentRole = "reviewer"
	AgentRoleBuilder    AgentRole = "builder"
	AgentRoleIntegrator AgentRole = "integrator"
	AgentRoleTestAgent  AgentRole = "test_agent"
)

type AgentTaskState string

const (
	// Legacy standalone child-agent states remain valid for compatibility with
	// the first Native Agent Teams runtime merged in PR #40.
	AgentTaskPending   AgentTaskState = "pending"
	AgentTaskRunning   AgentTaskState = "running"
	AgentTaskCompleted AgentTaskState = "completed"
	AgentTaskBlocked   AgentTaskState = "blocked"
	AgentTaskFailed    AgentTaskState = "failed"

	// Graph-specific states add explicit planning/readiness semantics without
	// changing the existing standalone child-agent execution contract.
	AgentTaskProposed  AgentTaskState = "proposed"
	AgentTaskReady     AgentTaskState = "ready"
	AgentTaskSucceeded AgentTaskState = "succeeded"
	AgentTaskCancelled AgentTaskState = "cancelled"
	AgentTaskRetryable AgentTaskState = "retryable"
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
	AgentCapabilityRepositoryRead  AgentCapability = "repository-read"
	AgentCapabilityLSP             AgentCapability = "lsp"
	AgentCapabilityPlanning        AgentCapability = "planning"
	AgentCapabilityReview          AgentCapability = "review"
	AgentCapabilityBuilderWorktree AgentCapability = "builder-worktree"
	AgentCapabilityIntegration     AgentCapability = "integration"
	AgentCapabilityTesting         AgentCapability = "testing"
)

type EvaluationDecision string

const (
	DecisionPass   EvaluationDecision = "PASS"
	DecisionFail   EvaluationDecision = "FAIL"
	DecisionRepair EvaluationDecision = "REPAIR"
)

type RepairProposal struct {
	Summary         string   `json:"summary"`
	FailingTests    []string `json:"failing_tests,omitempty"`
	AffectedPaths   []string `json:"affected_paths,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
}

type IntegrationStatus string

const (
	IntegrationSuccess  IntegrationStatus = "success"
	IntegrationConflict IntegrationStatus = "conflict"
	IntegrationFailed   IntegrationStatus = "failed"
)

type IntegrationResult struct {
	Status       IntegrationStatus `json:"status"`
	TargetBranch string            `json:"target_branch"`
	MergedCommit string            `json:"merged_commit,omitempty"`
	ChangedFiles []string          `json:"changed_files,omitempty"`
	Detail       string            `json:"detail,omitempty"`
}

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
	ID           string            `json:"id"`
	Role         string            `json:"role"`
	Objective    string            `json:"objective"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Capabilities []AgentCapability `json:"capabilities,omitempty"`
}

type AgentResult struct {
	Status         AgentResultStatus   `json:"status"`
	Summary        string              `json:"summary"`
	Findings       []Finding           `json:"findings,omitempty"`
	ChangedFiles   []string            `json:"changed_files,omitempty"`
	Commits        []string            `json:"commits,omitempty"`
	Tests          []TestResult        `json:"tests,omitempty"`
	Risks          []Risk              `json:"risks,omitempty"`
	SuggestedTasks []AgentTaskProposal `json:"suggested_tasks,omitempty"`
	Usage          AgentUsage          `json:"usage"`
}

type AgentTask struct {
	ID                    string            `json:"id"`
	ParentID              string            `json:"parent_id,omitempty"`
	MissionID             string            `json:"mission_id,omitempty"`
	Role                  AgentRole         `json:"role"`
	Objective             string            `json:"objective"`
	Dependencies          []string          `json:"dependencies,omitempty"`
	State                 AgentTaskState    `json:"state"`
	StateReason           string            `json:"state_reason,omitempty"`
	Workspace             string            `json:"workspace,omitempty"`
	Worktree              string            `json:"worktree,omitempty"`
	RequestedCapabilities []AgentCapability `json:"requested_capabilities,omitempty"`
	Capabilities          []AgentCapability `json:"capabilities"`
	Model                 string            `json:"model,omitempty"`
	Budget                AgentBudget       `json:"budget"`
	Result                AgentResult       `json:"result,omitempty"`
}

func normalizeAgentRole(raw string) (AgentRole, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "explorer", "explore", "analysis", "analyze":
		return AgentRoleExplorer, nil
	case "planner", "plan":
		return AgentRolePlanner, nil
	case "reviewer", "review":
		return AgentRoleReviewer, nil
	case "builder", "build", "coder":
		return AgentRoleBuilder, nil
	case "integrator", "integration", "merge":
		return AgentRoleIntegrator, nil
	case "test_agent", "test", "tester", "testing":
		return AgentRoleTestAgent, nil
	default:
		return "", fmt.Errorf("unsupported child-agent role %q; allowed roles: explorer, planner, reviewer, builder, integrator, test_agent", raw)
	}
}

func capabilitiesForAgentRole(role AgentRole) []AgentCapability {
	base := []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP}
	switch role {
	case AgentRolePlanner:
		return append(base, AgentCapabilityPlanning)
	case AgentRoleReviewer:
		return append(base, AgentCapabilityReview)
	case AgentRoleBuilder:
		return append(base, AgentCapabilityBuilderWorktree)
	case AgentRoleIntegrator:
		return append(base, AgentCapabilityIntegration)
	case AgentRoleTestAgent:
		return append(base, AgentCapabilityTesting)
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
	case AgentRoleBuilder:
		budget = AgentBudget{ModelCalls: 10, ToolCalls: 16, EstimatedTokenBudget: 120000, TimeSeconds: 240}
	case AgentRoleIntegrator:
		budget = AgentBudget{ModelCalls: 6, ToolCalls: 10, EstimatedTokenBudget: 80000, TimeSeconds: 180}
	case AgentRoleTestAgent:
		budget = AgentBudget{ModelCalls: 8, ToolCalls: 12, EstimatedTokenBudget: 90000, TimeSeconds: 180}
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
