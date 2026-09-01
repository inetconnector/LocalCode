// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	errUnsupportedRole          = errors.New("unsupported or inert agent role")
	errEscalationBlocked        = errors.New("capability escalation blocked by agent governance")
	errWorkspaceEscapeAttempted = errors.New("workspace escape attempted by child agent")
)

type RoleSecurityProfile struct {
	Role                AgentRole         `json:"role"`
	AllowedCapabilities []AgentCapability `json:"allowed_capabilities"`
	IsMutating          bool              `json:"is_mutating"`
	MaxModelCalls       int               `json:"max_model_calls"`
	MaxToolCalls        int               `json:"max_tool_calls"`
}

var roleSecurityProfiles = map[AgentRole]RoleSecurityProfile{
	AgentRoleExplorer: {
		Role: AgentRoleExplorer,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityLSP,
		},
		IsMutating:    false,
		MaxModelCalls: 10,
		MaxToolCalls:  20,
	},
	AgentRolePlanner: {
		Role: AgentRolePlanner,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityLSP,
			AgentCapabilityPlanning,
		},
		IsMutating:    false,
		MaxModelCalls: 8,
		MaxToolCalls:  15,
	},
	AgentRoleReviewer: {
		Role: AgentRoleReviewer,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityLSP,
			AgentCapabilityReview,
		},
		IsMutating:    false,
		MaxModelCalls: 8,
		MaxToolCalls:  15,
	},
	AgentRoleBuilder: {
		Role: AgentRoleBuilder,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityLSP,
			AgentCapabilityBuilderWorktree,
		},
		IsMutating:    true,
		MaxModelCalls: 16,
		MaxToolCalls:  30,
	},
	AgentRoleIntegrator: {
		Role: AgentRoleIntegrator,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityIntegration,
		},
		IsMutating:    true,
		MaxModelCalls: 8,
		MaxToolCalls:  15,
	},
	AgentRoleTestAgent: {
		Role: AgentRoleTestAgent,
		AllowedCapabilities: []AgentCapability{
			AgentCapabilityRepositoryRead,
			AgentCapabilityTesting,
		},
		IsMutating:    false,
		MaxModelCalls: 10,
		MaxToolCalls:  20,
	},
}

type AgentFactory struct {
	mu           sync.RWMutex
	customSkills map[string]string
}

func NewAgentFactory() *AgentFactory {
	return &AgentFactory{
		customSkills: make(map[string]string),
	}
}

// MapDynamicRoleToGovernedRole inspects an un-trusted dynamic role string from a planner or external prompt.
// If the role is unmapped, unknown, or attempts escalation, it is sanitized into an inert fallback
// without granting mutating or privileged capabilities.
func (f *AgentFactory) MapDynamicRoleToGovernedRole(rawRole string) (governedRole AgentRole, quarantined bool, reason string) {
	clean := strings.ToLower(strings.TrimSpace(rawRole))

	switch clean {
	case "explorer", "explore", "analyst", "analysis", "scout":
		return AgentRoleExplorer, false, ""
	case "planner", "plan", "architect":
		return AgentRolePlanner, false, ""
	case "reviewer", "review", "auditor", "inspector":
		return AgentRoleReviewer, false, ""
	case "builder", "coder", "developer", "implementer":
		return AgentRoleBuilder, false, ""
	case "integrator", "merge_master", "merger":
		return AgentRoleIntegrator, false, ""
	case "test_agent", "tester", "qa", "verifier":
		return AgentRoleTestAgent, false, ""
	default:
		// Quarantine unrecognized or potentially dangerous role strings
		return AgentRoleExplorer, true, fmt.Sprintf("unrecognized dynamic role %q quarantined to read-only explorer", rawRole)
	}
}

// SanitizeTaskCapabilities ensures that requested capabilities strictly reside within the role's security envelope.
// Any extraneous or unauthorized capabilities are stripped.
func (f *AgentFactory) SanitizeTaskCapabilities(role AgentRole, requested []AgentCapability) []AgentCapability {
	profile, ok := roleSecurityProfiles[role]
	if !ok {
		profile = roleSecurityProfiles[AgentRoleExplorer]
	}

	allowedSet := make(map[AgentCapability]bool, len(profile.AllowedCapabilities))
	for _, cap := range profile.AllowedCapabilities {
		allowedSet[cap] = true
	}

	result := make([]AgentCapability, 0, len(profile.AllowedCapabilities))
	for _, req := range requested {
		if allowedSet[req] {
			result = append(result, req)
		}
	}

	// Always ensure minimum baseline capabilities are present
	if len(result) == 0 {
		return append([]AgentCapability{}, profile.AllowedCapabilities...)
	}

	return result
}

// ResolveDeferredTools returns only the tools strictly permitted for the given role and capabilities.
func (f *AgentFactory) ResolveDeferredTools(role AgentRole, capabilities []AgentCapability) []string {
	tools := []string{"read_file", "list_dir", "grep_search", "view_symbol"}

	hasCap := func(target AgentCapability) bool {
		for _, c := range capabilities {
			if c == target {
				return true
			}
		}
		return false
	}

	if hasCap(AgentCapabilityLSP) {
		tools = append(tools, "lsp_definition", "lsp_references", "lsp_diagnostics")
	}

	if role == AgentRoleBuilder && hasCap(AgentCapabilityBuilderWorktree) {
		tools = append(tools, "replace_text", "write_file", "git_status", "git_diff", "git_commit")
	}

	if role == AgentRoleIntegrator && hasCap(AgentCapabilityIntegration) {
		tools = append(tools, "git_merge", "git_branch", "git_clean")
	}

	if role == AgentRoleTestAgent && hasCap(AgentCapabilityTesting) {
		tools = append(tools, "run_tests", "run_build_check", "check_syntax")
	}

	return tools
}

// CreateGovernedTask produces an immutable, validated AgentTask bounded by the role governance envelope.
func (f *AgentFactory) CreateGovernedTask(missionID, rawRole, objective, workspace string, requestedCaps []AgentCapability, cfg Config) (AgentTask, error) {
	governedRole, quarantined, reason := f.MapDynamicRoleToGovernedRole(rawRole)
	sanitizedCaps := f.SanitizeTaskCapabilities(governedRole, requestedCaps)
	budget := defaultAgentBudget(governedRole, cfg)

	task := AgentTask{
		ID:                    fmt.Sprintf("task-%s-%d", governedRole, time.Now().UnixNano()),
		MissionID:             strings.TrimSpace(missionID),
		Role:                  governedRole,
		Objective:             strings.TrimSpace(objective),
		State:                 AgentTaskReady,
		Workspace:             strings.TrimSpace(workspace),
		RequestedCapabilities: requestedCaps,
		Capabilities:          sanitizedCaps,
		Budget:                budget,
	}

	if quarantined {
		task.StateReason = reason
	}

	return task, nil
}
