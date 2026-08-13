// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ApprovalRule is a persistent, inspectable prefix rule modeled after the
// command-prefix rules used by mature coding agents. The most restrictive
// matching decision wins: forbidden > prompt > allow.
type ApprovalRule struct {
	ID            string    `json:"id"`
	Scope         string    `json:"scope"` // project | global
	Project       string    `json:"project,omitempty"`
	Decision      string    `json:"decision"` // allow | prompt | forbidden
	Pattern       []string  `json:"pattern"`
	Justification string    `json:"justification,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type ApprovalDecision struct {
	Approved bool
	Persist  bool
	Scope    string
}

func normalizeApprovalRule(rule ApprovalRule) (ApprovalRule, error) {
	rule.Scope = strings.ToLower(strings.TrimSpace(rule.Scope))
	if rule.Scope == "" {
		rule.Scope = "project"
	}
	if rule.Scope != "project" && rule.Scope != "global" {
		return ApprovalRule{}, errors.New("approval rule scope must be project or global")
	}
	rule.Decision = strings.ToLower(strings.TrimSpace(rule.Decision))
	if rule.Decision == "" {
		rule.Decision = "allow"
	}
	if rule.Decision != "allow" && rule.Decision != "prompt" && rule.Decision != "forbidden" {
		return ApprovalRule{}, errors.New("approval rule decision must be allow, prompt, or forbidden")
	}
	cleaned := make([]string, 0, len(rule.Pattern))
	for _, part := range rule.Pattern {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return ApprovalRule{}, errors.New("approval rule pattern is empty")
	}
	rule.Pattern = cleaned
	if rule.ID == "" {
		rule.ID = newID()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	if rule.Scope == "global" {
		rule.Project = ""
	} else if strings.TrimSpace(rule.Project) != "" {
		if abs, err := filepath.Abs(rule.Project); err == nil {
			rule.Project = filepath.Clean(abs)
		}
	}
	return rule, nil
}

func normalizeApprovalRules(rules []ApprovalRule) []ApprovalRule {
	result := make([]ApprovalRule, 0, len(rules))
	seen := map[string]bool{}
	for _, rule := range rules {
		normalized, err := normalizeApprovalRule(rule)
		if err != nil {
			continue
		}
		key := normalized.Scope + "\x00" + strings.ToLower(normalized.Project) + "\x00" + normalized.Decision + "\x00" + strings.Join(normalized.Pattern, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, normalized)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func approvalActionTokens(a AgentAction) []string {
	switch a.Action {
	case "git":
		return append([]string{"git"}, a.Args...)
	case "git_commit":
		return []string{"git", "commit"}
	case "run_tool":
		tool := canonicalToolName(a.Tool)
		if tool == "" {
			tool = strings.ToLower(strings.TrimSpace(a.Tool))
		}
		return append([]string{tool}, a.Args...)
	case "run_command":
		command := strings.Join(strings.Fields(strings.TrimSpace(a.Command)), " ")
		if command == "" {
			return nil
		}
		return []string{"shell", command}
	case "write_file", "replace_text", "delete_file", "create_svg_asset", "create_image_asset":
		return []string{a.Action, filepath.ToSlash(filepath.Clean(a.Path))}
	case "convert_image_asset", "render_asset":
		return []string{a.Action, filepath.ToSlash(filepath.Clean(a.Destination))}
	case "skill_copy_resource":
		return []string{a.Action, a.Skill, filepath.ToSlash(filepath.Clean(a.Resource)), filepath.ToSlash(filepath.Clean(a.Destination))}
	case "skill_run_script":
		return append([]string{a.Action, a.Skill, strings.TrimSpace(a.Script)}, a.Args...)
	case "copy_path", "move_path":
		return []string{a.Action, filepath.ToSlash(filepath.Clean(a.Source)), filepath.ToSlash(filepath.Clean(a.Destination))}
	case "web_search", "web_fetch", "build_project", "deploy_android", "open_terminal", "engine_edit", "engine_repo_map", "engine_lint", "engine_test", "engine_undo", "install_engine", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "install_aider":
		return []string{a.Action}
	case "mcp_call_tool":
		return []string{"mcp", a.Server, a.Tool}
	default:
		return []string{a.Action}
	}
}

func rulePatternMatches(pattern, tokens []string) bool {
	if len(pattern) == 0 || len(tokens) < len(pattern) {
		return false
	}
	for i := range pattern {
		if !strings.EqualFold(strings.TrimSpace(pattern[i]), strings.TrimSpace(tokens[i])) {
			return false
		}
	}
	return true
}

func approvalRuleDecision(cfg Config, project string, action AgentAction) (decision, justification string, matched bool) {
	tokens := approvalActionTokens(action)
	if len(tokens) == 0 {
		return "", "", false
	}
	projectAbs, _ := filepath.Abs(project)
	rank := map[string]int{"allow": 1, "prompt": 2, "forbidden": 3}
	bestRank := 0
	for _, rule := range cfg.ApprovalRules {
		normalized, err := normalizeApprovalRule(rule)
		if err != nil {
			continue
		}
		if normalized.Scope == "project" {
			ruleProject, _ := filepath.Abs(normalized.Project)
			if normalized.Project == "" || !strings.EqualFold(filepath.Clean(ruleProject), filepath.Clean(projectAbs)) {
				continue
			}
		}
		if !rulePatternMatches(normalized.Pattern, tokens) {
			continue
		}
		currentRank := rank[normalized.Decision]
		if currentRank > bestRank {
			decision, justification, matched = normalized.Decision, normalized.Justification, true
			bestRank = currentRank
		}
	}
	return decision, justification, matched
}

func persistentApprovalPattern(action AgentAction) ([]string, bool) {
	tokens := approvalActionTokens(action)
	if len(tokens) == 0 {
		return nil, false
	}
	switch action.Action {
	case "delete_file", "move_path":
		return nil, false
	case "git":
		if err := validateGitArgs(action.Args); err != nil {
			return nil, false
		}
		if len(tokens) >= 2 {
			// Match the Git subcommand, like Codex prefix rules, while still
			// keeping destructive commands blocked by validateGitArgs.
			return tokens[:2], true
		}
	case "git_commit":
		return []string{"git", "commit"}, true
	case "run_tool":
		// Preserve the complete argument vector. This makes "always allow"
		// precise instead of granting blanket access to the executable.
		return tokens, true
	case "run_command":
		// Complex shell text is stored as one exact normalized token.
		return tokens, true
	case "write_file", "replace_text", "create_svg_asset", "create_image_asset", "convert_image_asset", "render_asset", "skill_copy_resource":
		return tokens, true
	case "build_project", "deploy_android", "web_search", "web_fetch", "open_terminal", "mcp_call_tool", "copy_path", "skill_run_script", "engine_edit", "engine_repo_map", "engine_lint", "engine_test", "engine_undo", "install_engine", "aider_edit", "aider_repo_map", "aider_lint", "aider_test", "install_aider":
		return tokens, true
	}
	return tokens, true
}

func (s *AppState) addApprovalRule(project string, action AgentAction, scope string) (ApprovalRule, error) {
	pattern, ok := persistentApprovalPattern(action)
	if !ok {
		return ApprovalRule{}, errors.New("this action cannot be persistently allowed")
	}
	rule, err := normalizeApprovalRule(ApprovalRule{
		Scope:         scope,
		Project:       project,
		Decision:      "allow",
		Pattern:       pattern,
		Justification: "Approved by the user from the LocalCode approval prompt.",
	})
	if err != nil {
		return ApprovalRule{}, err
	}
	s.mu.Lock()
	cfg := s.Config
	cfg.ApprovalRules = normalizeApprovalRules(append(cfg.ApprovalRules, rule))
	s.mu.Unlock()
	if err := saveConfig(cfg); err != nil {
		return ApprovalRule{}, err
	}
	s.mu.Lock()
	s.Config = cfg
	s.mu.Unlock()
	return rule, nil
}
