// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// RiskLevel classifies the inherent security and blast-radius tier of an agent action.
type RiskLevel string

const (
	RiskSafeRead          RiskLevel = "safe_read"
	RiskSafeLocalEdit     RiskLevel = "safe_local_edit"
	RiskBoundedTest       RiskLevel = "bounded_test"
	RiskSensitiveMutation RiskLevel = "sensitive_mutation"
	RiskCriticalBlock     RiskLevel = "critical_block"
)

// RiskDecision is the actionable authorization verdict derived from RiskLevel + RiskPolicy.
type RiskDecision string

const (
	DecisionAllow  RiskDecision = "allow"
	DecisionPrompt RiskDecision = "prompt"
	DecisionDeny   RiskDecision = "deny"
)

// RiskPolicy controls how RiskLevels map to RiskDecisions.
type RiskPolicy struct {
	ApprovalMode   string   `json:"approval_mode"` // strict | balanced | auto | dangerous
	SandboxMode    string   `json:"sandbox_mode"`  // project | workspace | unrestricted
	NetworkEnabled bool     `json:"network_enabled"`
	BlockedPaths   []string `json:"blocked_paths,omitempty"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
}

// RiskAssessment is the structured output of ClassifyActionRisk.
type RiskAssessment struct {
	Level          RiskLevel    `json:"level"`
	Decision       RiskDecision `json:"decision"`
	Reason         string       `json:"reason"`
	Signals        []string     `json:"signals,omitempty"`
	RequiresReview bool         `json:"requires_review"`
	Action         string       `json:"action"`
	Target         string       `json:"target,omitempty"`
}

// RiskPolicyFromConfig translates LocalCode's runtime Config into a RiskPolicy.
func RiskPolicyFromConfig(cfg Config) RiskPolicy {
	return RiskPolicy{
		ApprovalMode:   strings.ToLower(strings.TrimSpace(cfg.ApprovalMode)),
		SandboxMode:    strings.ToLower(strings.TrimSpace(cfg.SandboxMode)),
		NetworkEnabled: cfg.NetworkEnabled,
		BlockedPaths:   DefaultSandboxBlockedPaths(),
	}
}

var (
	reExfilPipeShell = regexp.MustCompile(`(?i)\b(curl|wget)\b[^|;\n]*\|\s*(sudo\s+)?(sh|bash|zsh|dash|ksh|ash|python|python3|perl|ruby|node|pwsh|powershell)\b`)
	reExfilPipePS    = regexp.MustCompile(`(?i)\b(iwr|irm|invoke-webrequest|invoke-restmethod|downloadstring|downloadfile)\b[^|;\n]*\|\s*(iex|invoke-expression)\b`)
	reCurlDataExfil  = regexp.MustCompile(`(?i)\bcurl\b[^\n]*(\s(-d|--data|--data-binary|--data-raw|-F|--form|-T|--upload-file)\s+['"]?@|\s--upload-file\b)`)
	reEncodedPS      = regexp.MustCompile(`(?i)\b(powershell|pwsh)(\.exe)?\b[^\n]*\s-(enc|encodedcommand)\b`)
	reUnixRmRoot     = regexp.MustCompile(`(?i)\brm\b[^\n]*(\s-[a-z]*r[a-z]*f|\s-[a-z]*f[a-z]*r|\s--no-preserve-root)[^\n]*(\s(/(\*|\s|$)|~|\.\.|\$HOME)|\s--no-preserve-root)`)
	reWinDelRecurse  = regexp.MustCompile(`(?i)\b(del|erase)\b[^\n]*(/s[^\n]*/q|/q[^\n]*/s)`)
	reWinRdRecurse   = regexp.MustCompile(`(?i)\b(rmdir|rd)\b[^\n]*(/s[^\n]*/q|/q[^\n]*/s)`)
	rePSRemoveForce  = regexp.MustCompile(`(?i)\b(remove-item|ri)\b[^\n]*-recurse[^\n]*-force|\b(remove-item|ri)\b[^\n]*-force[^\n]*-recurse`)
	reDiskWipe       = regexp.MustCompile(`(?i)\b(mkfs\.[a-z0-9]+|diskpart|format\s+[a-z]:|dd\s+if=[^\s]+\s+of=/dev/|cipher\s+/w)\b`)
	reGitForcePush   = regexp.MustCompile(`(?i)\bgit\b[^\n]*\bpush\b[^\n]*(\s(-f|--force|--force-with-lease)\b|\s\+[^\s]+)`)
	reGitHardReset   = regexp.MustCompile(`(?i)\bgit\b[^\n]*\breset\b[^\n]*--hard\b`)
	reGitCleanForce  = regexp.MustCompile(`(?i)\bgit\b[^\n]*\bclean\b[^\n]*\s-[a-z]*f`)
	reGitCheckoutAll = regexp.MustCompile(`(?i)\bgit\b[^\n]*\bcheckout\b[^\n]*--\s+(\.|\*)`)
	reGitBranchDel   = regexp.MustCompile(`(?i)\bgit\b[^\n]*\bbranch\b[^\n]*\s-D\b`)
)

// ClassifyActionRisk evaluates an AgentAction against MICA security invariants
// and Claude-Code-style auto-mode rules, returning a deterministic RiskAssessment.
func ClassifyActionRisk(projectRoot string, action AgentAction, policy RiskPolicy) RiskAssessment {
	mode := normalizeRiskApprovalMode(policy.ApprovalMode)
	blockedPaths := policy.BlockedPaths
	if len(blockedPaths) == 0 {
		blockedPaths = DefaultSandboxBlockedPaths()
	}

	target := primaryActionTarget(action)
	base := RiskAssessment{
		Action: action.Action,
		Target: target,
	}

	// 1. Check all explicit path fields for path escape and credential/protected-path access.
	pathFields := collectActionPathFields(action)
	for _, p := range pathFields {
		if isCriticalCredentialOrHookPath(projectRoot, p, blockedPaths) {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				fmt.Sprintf("blocked access to protected credential or hook path: %s", p),
				[]string{"credential_path", "protected_path"})
		}
		if escapesProjectRoot(projectRoot, p, policy.SandboxMode) {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				fmt.Sprintf("blocked path escaping project root: %s", p),
				[]string{"path_escape"})
		}
		if isDotEnvCredentialPath(p) {
			return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
				fmt.Sprintf("access to environment secret file requires review: %s", p),
				[]string{"dotenv_credential"})
		}
	}

	// 2. Inspect command/tool invocations thoroughly.
	commandLine := reconstructActionCommandLine(action)
	if commandLine != "" {
		if assessment, matched := classifyCommandRisk(projectRoot, action, commandLine, policy, mode, blockedPaths); matched {
			return assessment
		}
	}

	// 3. Classify structured action types.
	switch strings.ToLower(strings.TrimSpace(action.Action)) {
	case "", "ask_user", "finish",
		"read_file", "list_files", "project_tree", "search_text", "find_files",
		"grep", "glob", "code_map", "repo_map", "memory_list", "read_memory",
		"list_skills", "read_skill", "list_project_commands", "read_project_command",
		"list_mcp_tools", "mcp_read_resource":
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			fmt.Sprintf("read-only workspace inspection (%s)", action.Action),
			[]string{"read_only"})

	case "lsp":
		op := strings.ToLower(strings.TrimSpace(action.Name))
		if op == "rename" || op == "code_action" || op == "format" {
			return finalizeRiskAssessment(base, RiskSafeLocalEdit, mode,
				fmt.Sprintf("in-workspace LSP edit (%s) on %s", op, target),
				[]string{"local_edit", "lsp"})
		}
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			fmt.Sprintf("read-only LSP query (%s)", op),
			[]string{"read_only", "lsp"})

	case "web_search":
		if !policy.NetworkEnabled {
			return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
				"web_search requested while network access is disabled",
				[]string{"network_disabled"})
		}
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			"read-only web search",
			[]string{"read_only", "network_read"})

	case "web_fetch":
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"external web fetch requires review",
			[]string{"network_fetch"})

	case "write_file", "replace_text", "edit_file", "create_svg_asset", "create_image_asset",
		"convert_image_asset", "render_asset", "copy_path", "skill_copy_resource":
		return finalizeRiskAssessment(base, RiskSafeLocalEdit, mode,
			fmt.Sprintf("in-workspace file edit (%s) on %s", action.Action, target),
			[]string{"in_workspace_edit"})

	case "git_commit":
		return finalizeRiskAssessment(base, RiskSafeLocalEdit, mode,
			"local git commit within workspace",
			[]string{"local_git_commit"})

	case "git":
		return classifyGitAction(base, action.Args, mode)

	case "delete_file", "move_path":
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			fmt.Sprintf("destructive or relocating filesystem mutation (%s) on %s", action.Action, target),
			[]string{"filesystem_mutation"})

	case "run_command", "run_tool", "skill_run_script":
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			fmt.Sprintf("unclassified command execution requires review: %s", commandLine),
			[]string{"unclassified_command"})

	default:
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			fmt.Sprintf("mutating or external action (%s) requires review", action.Action),
			[]string{"sensitive_action"})
	}
}

func classifyCommandRisk(projectRoot string, action AgentAction, cmdLine string, policy RiskPolicy, mode string, blockedPaths []string) (RiskAssessment, bool) {
	base := RiskAssessment{
		Action: action.Action,
		Target: cmdLine,
	}

	// Critical 1: Exfiltration / remote code execution pipes.
	if reExfilPipeShell.MatchString(cmdLine) || reExfilPipePS.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
			"blocked remote code execution / exfiltration pipe",
			[]string{"exfiltration_pipe", "remote_code_exec"}), true
	}
	if reCurlDataExfil.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
			"blocked data exfiltration upload command",
			[]string{"data_exfiltration"}), true
	}
	if reEncodedPS.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
			"blocked obfuscated encoded PowerShell command",
			[]string{"obfuscated_exec"}), true
	}

	// Critical 2: Destructive recursive wipes & disk formatting.
	if reUnixRmRoot.MatchString(cmdLine) || reWinDelRecurse.MatchString(cmdLine) || reWinRdRecurse.MatchString(cmdLine) || reDiskWipe.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
			"blocked destructive recursive deletion or disk wipe command",
			[]string{"destructive_delete"}), true
	}
	if rePSRemoveForce.MatchString(cmdLine) {
		lower := strings.ToLower(cmdLine)
		if strings.Contains(lower, `c:\`) || strings.Contains(lower, " /") || strings.Contains(lower, "..") || strings.Contains(lower, "~") || strings.Contains(lower, "$home") {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				"blocked destructive forced recursive Remove-Item command",
				[]string{"destructive_delete"}), true
		}
	}

	// Critical 3: Command tokens referencing credentials, protected paths, or escaping workspace.
	hasDotEnvRef := false
	for _, tok := range extractPathCandidateTokens(cmdLine) {
		if isCriticalCredentialOrHookPath(projectRoot, tok, blockedPaths) {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				fmt.Sprintf("blocked command accessing protected credential or hook path: %s", tok),
				[]string{"credential_path", "protected_path"}), true
		}
		if escapesProjectRoot(projectRoot, tok, policy.SandboxMode) {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				fmt.Sprintf("blocked command escaping project root: %s", tok),
				[]string{"path_escape"}), true
		}
		if isDotEnvCredentialPath(tok) {
			hasDotEnvRef = true
		}
	}
	if hasDotEnvRef {
		// Reading/uploading .env alongside network tools is a critical exfil block.
		lower := strings.ToLower(cmdLine)
		if strings.Contains(lower, "curl ") || strings.Contains(lower, "wget ") || strings.Contains(lower, "iwr ") || strings.Contains(lower, "invoke-webrequest") || strings.Contains(lower, "nc ") || strings.Contains(lower, "scp ") {
			return finalizeRiskAssessment(base, RiskCriticalBlock, mode,
				"blocked potential .env credential exfiltration over network",
				[]string{"dotenv_credential", "data_exfiltration"}), true
		}
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"command references .env credential file and requires review",
			[]string{"dotenv_credential"}), true
	}

	// Sensitive 4: Destructive or history-rewriting Git operations.
	if reGitForcePush.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"git force-push can overwrite remote history and requires review",
			[]string{"git_force_push"}), true
	}
	if reGitHardReset.MatchString(cmdLine) || reGitCleanForce.MatchString(cmdLine) || reGitCheckoutAll.MatchString(cmdLine) || reGitBranchDel.MatchString(cmdLine) {
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"destructive git reset/clean/branch-delete requires review",
			[]string{"git_destructive"}), true
	}

	// Git structured or shell read-only commands.
	fields := strings.Fields(cmdLine)
	if len(fields) >= 2 && strings.EqualFold(filepath.Base(fields[0]), "git") && !hasUnsafeShellMeta(cmdLine) {
		return classifyGitAction(base, fields[1:], mode), true
	}

	// Bounded local test / lint / build commands (no unsafe shell chaining).
	if isBoundedLocalTestCommand(cmdLine) {
		return finalizeRiskAssessment(base, RiskBoundedTest, mode,
			fmt.Sprintf("bounded local test/build/lint verification: %s", cmdLine),
			[]string{"bounded_test"}), true
	}

	// Read-only shell inspection commands (no unsafe shell chaining).
	if isSafeReadCommand(cmdLine) {
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			fmt.Sprintf("read-only local inspection command: %s", cmdLine),
			[]string{"read_only_command"}), true
	}

	return RiskAssessment{}, false
}

func classifyGitAction(base RiskAssessment, args []string, mode string) RiskAssessment {
	if len(args) == 0 {
		return finalizeRiskAssessment(base, RiskSafeRead, mode, "read-only git invocation", []string{"git_read"})
	}
	joined := strings.ToLower("git " + strings.Join(args, " "))
	if reGitForcePush.MatchString(joined) {
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"git force-push can overwrite remote history and requires review",
			[]string{"git_force_push"})
	}
	if reGitHardReset.MatchString(joined) || reGitCleanForce.MatchString(joined) || reGitCheckoutAll.MatchString(joined) || reGitBranchDel.MatchString(joined) {
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			"destructive git reset/clean/branch-delete requires review",
			[]string{"git_destructive"})
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "status", "diff", "log", "show", "rev-parse", "ls-files", "blame", "describe", "tag":
		if sub == "tag" && len(args) > 1 {
			for _, a := range args[1:] {
				if a == "-d" || a == "--delete" || a == "-f" || a == "--force" {
					return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
						"git tag mutation requires review", []string{"git_mutation"})
				}
			}
		}
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			fmt.Sprintf("read-only git %s", sub), []string{"git_read"})
	case "branch":
		for _, a := range args[1:] {
			if a == "-d" || a == "-D" || a == "-m" || a == "-M" || a == "--delete" {
				return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
					"git branch mutation requires review", []string{"git_mutation"})
			}
		}
		return finalizeRiskAssessment(base, RiskSafeRead, mode,
			"read-only git branch listing", []string{"git_read"})
	case "add", "commit", "stash":
		return finalizeRiskAssessment(base, RiskSafeLocalEdit, mode,
			fmt.Sprintf("local git %s within workspace", sub), []string{"local_git_edit"})
	default:
		return finalizeRiskAssessment(base, RiskSensitiveMutation, mode,
			fmt.Sprintf("git %s modifies repository or remote state and requires review", sub),
			[]string{"git_mutation"})
	}
}

func isBoundedLocalTestCommand(cmdLine string) bool {
	if hasUnsafeShellMeta(cmdLine) {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(cmdLine))
	if len(fields) == 0 {
		return false
	}
	exe := strings.ToLower(strings.TrimSuffix(filepath.Base(fields[0]), ".exe"))
	sub := ""
	if len(fields) > 1 {
		sub = strings.ToLower(fields[1])
	}
	sub2 := ""
	if len(fields) > 2 {
		sub2 = strings.ToLower(fields[2])
	}

	switch exe {
	case "go":
		return sub == "test" || sub == "vet" || sub == "build" || sub == "fmt" || sub == "version" || sub == "list"
	case "gofmt", "pytest", "vitest", "jest", "eslint", "tsc", "mypy", "flake8", "black", "prettier", "golangci-lint":
		return true
	case "ruff":
		return sub == "check" || sub == "format" || sub == ""
	case "cargo":
		return sub == "test" || sub == "check" || sub == "clippy" || sub == "build" || sub == "fmt"
	case "dotnet":
		return sub == "test" || sub == "build"
	case "mvn", "mvnw", "gradle", "gradlew":
		return sub == "test" || sub == "check" || sub == "compile" || sub == "verify"
	case "npm", "pnpm", "yarn", "bun":
		if sub == "test" {
			return true
		}
		if sub == "run" && (sub2 == "test" || sub2 == "lint" || sub2 == "build" || sub2 == "check" || sub2 == "typecheck") {
			return true
		}
		return false
	case "npx":
		return sub == "tsc" || sub == "vitest" || sub == "jest" || sub == "eslint" || sub == "prettier"
	case "python", "python3", "py":
		return sub == "-m" && (sub2 == "pytest" || sub2 == "unittest" || sub2 == "py_compile" || sub2 == "mypy" || sub2 == "ruff")
	case "ctest":
		return true
	default:
		return false
	}
}

func isSafeReadCommand(cmdLine string) bool {
	if hasUnsafeShellMeta(cmdLine) {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(cmdLine))
	if len(fields) == 0 {
		return false
	}
	exe := strings.ToLower(strings.TrimSuffix(filepath.Base(fields[0]), ".exe"))
	switch exe {
	case "ls", "dir", "pwd", "whoami", "cat", "head", "tail", "wc", "rg", "grep", "findstr", "tree", "stat", "file", "echo":
		return true
	default:
		return false
	}
}

func hasUnsafeShellMeta(cmdLine string) bool {
	for _, meta := range []string{"|", ";", "&&", "||", "`", "$(", ">", "<", "\n", "\r"} {
		if strings.Contains(cmdLine, meta) {
			return true
		}
	}
	return false
}

func isCriticalCredentialOrHookPath(projectRoot, rawPath string, blockedPaths []string) bool {
	if _, matched := matchBlockedSandboxPath(projectRoot, rawPath, blockedPaths); matched {
		return true
	}
	cleaned := strings.Trim(strings.TrimSpace(rawPath), `"'`+"`")
	if cleaned == "" {
		return false
	}
	slashed := strings.ToLower(strings.ReplaceAll(expandHomeTilde(cleaned), `\`, `/`))
	base := filepath.Base(slashed)

	criticalExactBases := map[string]bool{
		"id_rsa":          true,
		"id_rsa.pub":      false,
		"id_ed25519":      true,
		"id_ecdsa":        true,
		"id_dsa":          true,
		"authorized_keys": true,
		".netrc":          true,
		".git-credentials": true,
		".npmrc":          true,
		".pypirc":         true,
		"credentials":     true,
		"credentials.json": true,
		"secrets.json":    true,
	}
	if criticalExactBases[base] {
		return true
	}
	criticalPathSegments := []string{
		".ssh",
		".aws",
		".gnupg",
		".git/hooks",
		".kube/config",
	}
	for _, seg := range criticalPathSegments {
		if pathContainsSegments(slashed, seg) {
			return true
		}
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	}
	return false
}

func isDotEnvCredentialPath(rawPath string) bool {
	cleaned := strings.Trim(strings.TrimSpace(rawPath), `"'`+"`")
	if cleaned == "" {
		return false
	}
	slashed := strings.ToLower(strings.ReplaceAll(cleaned, `\`, `/`))
	base := filepath.Base(slashed)
	if base == ".env" {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		// Template/example files contain placeholders, not live credentials.
		switch base {
		case ".env.example", ".env.sample", ".env.template", ".env.dist":
			return false
		default:
			return true
		}
	}
	return false
}

func escapesProjectRoot(projectRoot, rawPath, sandboxMode string) bool {
	if strings.EqualFold(strings.TrimSpace(sandboxMode), "unrestricted") {
		return false
	}
	cleaned := strings.Trim(strings.TrimSpace(rawPath), `"'`+"`")
	if cleaned == "" {
		return false
	}
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	// Ignore short Windows command switches like /C, /D, /S, /Q.
	if strings.HasPrefix(cleaned, "/") && !strings.Contains(cleaned[1:], "/") && !strings.Contains(cleaned, `\`) && len(cleaned) <= 3 {
		return false
	}

	expanded := expandHomeTilde(cleaned)
	slashed := filepath.ToSlash(expanded)
	hasDotDot := slashed == ".." ||
		strings.HasPrefix(slashed, "../") ||
		strings.HasSuffix(slashed, "/..") ||
		strings.Contains(slashed, "/../")
	isHomeRef := cleaned == "~" || strings.HasPrefix(cleaned, "~/") || strings.HasPrefix(cleaned, `~\`)
	isAbs := filepath.IsAbs(expanded) || isWindowsDrivePath(expanded) || strings.HasPrefix(expanded, `\\`)

	if !hasDotDot && !isHomeRef && !isAbs {
		return false
	}
	if strings.TrimSpace(projectRoot) == "" {
		return hasDotDot || isHomeRef || isAbs
	}
	if isBenignNullDevice(expanded) {
		return false
	}
	_, err := ensureWithinRoot(projectRoot, expanded)
	return err != nil
}

func collectActionPathFields(a AgentAction) []string {
	var out []string
	for _, p := range []string{a.Path, a.Source, a.Destination, a.Script} {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func reconstructActionCommandLine(a AgentAction) string {
	switch strings.ToLower(strings.TrimSpace(a.Action)) {
	case "run_command":
		return strings.TrimSpace(a.Command)
	case "run_tool":
		parts := []string{}
		if t := strings.TrimSpace(a.Tool); t != "" {
			parts = append(parts, t)
		}
		parts = append(parts, a.Args...)
		return strings.TrimSpace(strings.Join(parts, " "))
	case "git":
		if len(a.Args) == 0 {
			return ""
		}
		return "git " + strings.Join(a.Args, " ")
	case "skill_run_script":
		parts := []string{}
		if s := strings.TrimSpace(a.Script); s != "" {
			parts = append(parts, s)
		}
		parts = append(parts, a.Args...)
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return strings.TrimSpace(a.Command)
	}
}

func primaryActionTarget(a AgentAction) string {
	for _, candidate := range []string{a.Path, a.Destination, a.Source, a.Command, a.URL, a.Tool} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	if len(a.Args) > 0 {
		return strings.Join(a.Args, " ")
	}
	return ""
}

func normalizeRiskApprovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict", "balanced", "auto", "dangerous":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "balanced"
	}
}

func finalizeRiskAssessment(base RiskAssessment, level RiskLevel, mode, reason string, signals []string) RiskAssessment {
	base.Level = level
	base.Reason = reason
	base.Signals = signals

	switch level {
	case RiskCriticalBlock:
		// MICA invariant: critical blocks are always denied, even in auto/dangerous mode.
		base.Decision = DecisionDeny
		base.RequiresReview = true
	case RiskSensitiveMutation:
		if mode == "strict" {
			base.Decision = DecisionDeny
		} else {
			base.Decision = DecisionPrompt
		}
		base.RequiresReview = true
	case RiskBoundedTest:
		if mode == "strict" {
			base.Decision = DecisionPrompt
			base.RequiresReview = true
		} else {
			base.Decision = DecisionAllow
			base.RequiresReview = false
		}
	case RiskSafeLocalEdit:
		if mode == "auto" || mode == "dangerous" {
			base.Decision = DecisionAllow
			base.RequiresReview = false
		} else {
			base.Decision = DecisionPrompt
			base.RequiresReview = true
		}
	case RiskSafeRead:
		base.Decision = DecisionAllow
		base.RequiresReview = false
	default:
		base.Decision = DecisionPrompt
		base.RequiresReview = true
	}
	return base
}
