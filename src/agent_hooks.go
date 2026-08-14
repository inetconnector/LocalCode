// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"time"
)

func agentToolHookEligible(a AgentAction) bool {
	switch a.Action {
	case "", "ask_user", "finish":
		return false
	default:
		return true
	}
}

func runAgentToolHook(ctx context.Context, project string, cfg Config, phase string, a AgentAction) (string, error) {
	var command string
	switch phase {
	case "before":
		command = strings.TrimSpace(cfg.HookBeforeTool)
	case "after":
		command = strings.TrimSpace(cfg.HookAfterTool)
	default:
		return "", nil
	}
	if command == "" || !agentToolHookEligible(a) {
		return "", nil
	}
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	hookCfg := cfg
	hookCfg.EnvironmentVars = cloneStringMap(cfg.EnvironmentVars)
	hookCfg.EnvironmentVars["LOCALCODE_HOOK_PHASE"] = phase
	hookCfg.EnvironmentVars["LOCALCODE_ACTION"] = a.Action
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_MESSAGE"] = a.Message
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_PATH"] = a.Path
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_COMMAND"] = a.Command
	return runProjectCommand(hookCtx, project, command, hookCfg)
}

func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
