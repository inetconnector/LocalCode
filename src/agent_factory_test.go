// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
)

func TestAgentFactoryDynamicRoleMappingAndQuarantine(t *testing.T) {
	factory := NewAgentFactory()

	// 1. Valid mapped roles
	validCases := []struct {
		input string
		want  AgentRole
	}{
		{"explorer", AgentRoleExplorer},
		{"analyst", AgentRoleExplorer},
		{"planner", AgentRolePlanner},
		{"architect", AgentRolePlanner},
		{"reviewer", AgentRoleReviewer},
		{"auditor", AgentRoleReviewer},
		{"builder", AgentRoleBuilder},
		{"developer", AgentRoleBuilder},
		{"integrator", AgentRoleIntegrator},
		{"test_agent", AgentRoleTestAgent},
		{"qa", AgentRoleTestAgent},
	}

	for _, c := range validCases {
		role, quarantined, reason := factory.MapDynamicRoleToGovernedRole(c.input)
		if quarantined {
			t.Fatalf("expected role %q to not be quarantined, got reason: %s", c.input, reason)
		}
		if role != c.want {
			t.Fatalf("MapDynamicRoleToGovernedRole(%q) = %q, want %q", c.input, role, c.want)
		}
	}

	// 2. Unrecognized / Malicious / Dynamic roles must be quarantined to read-only Explorer
	quarantineCases := []string{
		"root",
		"admin",
		"shell_executor",
		"installer",
		"network_hacker",
		"unknown_custom_role",
	}

	for _, q := range quarantineCases {
		role, quarantined, reason := factory.MapDynamicRoleToGovernedRole(q)
		if !quarantined {
			t.Fatalf("expected role %q to be quarantined", q)
		}
		if role != AgentRoleExplorer {
			t.Fatalf("quarantined role %q must fall back to AgentRoleExplorer, got %q", q, role)
		}
		if reason == "" {
			t.Fatalf("quarantined role %q must have quarantine reason", q)
		}
	}
}

func TestAgentFactoryCapabilitySanitization(t *testing.T) {
	factory := NewAgentFactory()

	// Explorer trying to request Builder worktree and Integration capabilities
	requested := []AgentCapability{
		AgentCapabilityRepositoryRead,
		AgentCapabilityBuilderWorktree, // Unauthorized
		AgentCapabilityIntegration,     // Unauthorized
		AgentCapabilityLSP,
	}

	sanitized := factory.SanitizeTaskCapabilities(AgentRoleExplorer, requested)
	if len(sanitized) != 2 {
		t.Fatalf("expected 2 sanitized capabilities, got %d: %+v", len(sanitized), sanitized)
	}

	for _, cap := range sanitized {
		if cap == AgentCapabilityBuilderWorktree || cap == AgentCapabilityIntegration {
			t.Fatalf("unauthorized capability %q was not stripped by governance", cap)
		}
	}
}

func TestAgentFactoryDeferredToolResolution(t *testing.T) {
	factory := NewAgentFactory()

	// Explorer tools
	explorerTools := factory.ResolveDeferredTools(AgentRoleExplorer, []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP})
	for _, tool := range explorerTools {
		if tool == "write_file" || tool == "replace_text" || tool == "git_merge" {
			t.Fatalf("read-only explorer resolved mutating tool: %s", tool)
		}
	}

	// Builder tools
	builderTools := factory.ResolveDeferredTools(AgentRoleBuilder, []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityBuilderWorktree})
	hasWrite := false
	for _, tool := range builderTools {
		if tool == "write_file" {
			hasWrite = true
			break
		}
	}
	if !hasWrite {
		t.Fatal("builder missing write_file tool")
	}
}

func TestAgentFactoryCreateGovernedTask(t *testing.T) {
	factory := NewAgentFactory()
	cfg := Config{}

	// Create task with dynamic role
	task, err := factory.CreateGovernedTask("mission-123", "developer", "Implement cache", "/tmp/project", []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityBuilderWorktree}, cfg)
	if err != nil {
		t.Fatalf("CreateGovernedTask failed: %v", err)
	}

	if task.Role != AgentRoleBuilder {
		t.Fatalf("expected role builder, got %q", task.Role)
	}
	if task.State != AgentTaskReady {
		t.Fatalf("expected state ready, got %q", task.State)
	}
	if task.Budget.ModelCalls <= 0 || task.Budget.TimeSeconds <= 0 {
		t.Fatalf("invalid budget allocated: %+v", task.Budget)
	}
}
