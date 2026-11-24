// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSandboxTimeout    = 2 * time.Minute
	defaultSandboxMaxOutput  = 2 * 1024 * 1024 // 2 MiB per stream
	sandboxBlackholeProxyURL = "http://127.0.0.1:0"
)

// OSSandboxPolicy defines OS-level containment, resource limits, secret
// stripping, filesystem blocks, and network egress rules for a command run.
type OSSandboxPolicy struct {
	WorkspaceRoot  string        `json:"workspace_root"`
	ReadOnlyPaths  []string      `json:"read_only_paths,omitempty"`
	BlockedPaths   []string      `json:"blocked_paths,omitempty"`
	AllowNetwork   bool          `json:"allow_network"`
	AllowedDomains []string      `json:"allowed_domains,omitempty"`
	DeniedDomains  []string      `json:"denied_domains,omitempty"`
	MemoryLimitMB  int           `json:"memory_limit_mb,omitempty"`
	MaxProcesses   int           `json:"max_processes,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty"`
	StripSecrets   bool          `json:"strip_secrets"`
}

// OSSandboxResult describes the outcome and audit trail of a sandboxed run.
type OSSandboxResult struct {
	ExitCode       int                `json:"exit_code"`
	Stdout         string             `json:"stdout"`
	Stderr         string             `json:"stderr"`
	TimedOut       bool               `json:"timed_out"`
	SandboxBackend string             `json:"sandbox_backend"`
	BlockedDomains []string           `json:"blocked_domains,omitempty"`
	AuditDigest    string             `json:"audit_digest"`
	Duration       time.Duration      `json:"duration,omitempty"`
	EgressAudit    []EgressAuditEntry `json:"egress_audit,omitempty"`
}

// DefaultSandboxBlockedPaths returns the default credential and hook paths
// that sandboxed commands are never allowed to read or mutate.
func DefaultSandboxBlockedPaths() []string {
	return []string{".ssh", ".aws", ".gnupg", ".git/hooks"}
}

// DefaultOSSandboxPolicy returns a hardened default policy rooted at workspaceRoot.
func DefaultOSSandboxPolicy(workspaceRoot string) OSSandboxPolicy {
	return OSSandboxPolicy{
		WorkspaceRoot: workspaceRoot,
		BlockedPaths:  DefaultSandboxBlockedPaths(),
		AllowNetwork:  false,
		MemoryLimitMB: 1024,
		MaxProcesses:  64,
		Timeout:       defaultSandboxTimeout,
		StripSecrets:  true,
	}
}

func normalizeOSSandboxPolicy(policy OSSandboxPolicy) (OSSandboxPolicy, error) {
	root := strings.TrimSpace(policy.WorkspaceRoot)
	if root == "" {
		return OSSandboxPolicy{}, errors.New("sandbox workspace root is required")
	}
	canonRoot, err := canonicalSandboxPath(root)
	if err != nil {
		return OSSandboxPolicy{}, fmt.Errorf("resolve sandbox workspace root: %w", err)
	}
	info, err := os.Stat(canonRoot)
	if err != nil {
		return OSSandboxPolicy{}, fmt.Errorf("sandbox workspace root not found: %w", err)
	}
	if !info.IsDir() {
		return OSSandboxPolicy{}, fmt.Errorf("sandbox workspace root is not a directory: %s", canonRoot)
	}
	policy.WorkspaceRoot = canonRoot
	if len(policy.BlockedPaths) == 0 {
		policy.BlockedPaths = DefaultSandboxBlockedPaths()
	} else {
		policy.BlockedPaths = append([]string(nil), policy.BlockedPaths...)
	}
	if policy.Timeout <= 0 {
		policy.Timeout = defaultSandboxTimeout
	}
	if policy.MemoryLimitMB < 0 {
		policy.MemoryLimitMB = 0
	}
	if policy.MaxProcesses < 0 {
		policy.MaxProcesses = 0
	}
	return policy, nil
}

// RunSandboxedCommand executes an executable inside the OS-level sandbox
// enforcing workspace containment, blocked paths, env secret stripping,
// process-tree & resource limits, and optional domain-allowlist egress proxy.
func RunSandboxedCommand(ctx context.Context, policy OSSandboxPolicy, workDir string, name string, args []string, extraEnv map[string]string) (OSSandboxResult, error) {
	started := time.Now()
	if strings.TrimSpace(name) == "" {
		return OSSandboxResult{}, errors.New("sandbox command executable is empty")
	}
	normPolicy, err := normalizeOSSandboxPolicy(policy)
	if err != nil {
		return OSSandboxResult{}, err
	}

	resolvedWorkDir, err := ValidateSandboxPaths(normPolicy, workDir, args)
	if err != nil {
		res := OSSandboxResult{
			ExitCode:       -1,
			SandboxBackend: defaultSandboxBackendName(),
		}
		res.AuditDigest = computeSandboxAuditDigest(normPolicy, resolvedWorkDir, name, args, res)
		return res, err
	}

	var proxy *EgressProxy
	proxyURL := ""
	if normPolicy.AllowNetwork && (len(normPolicy.AllowedDomains) > 0 || len(normPolicy.DeniedDomains) > 0) {
		proxy, err = StartEgressProxy(EgressProxyConfig{
			AllowedDomains: normPolicy.AllowedDomains,
			DeniedDomains:  normPolicy.DeniedDomains,
		})
		if err != nil {
			return OSSandboxResult{}, fmt.Errorf("start sandbox egress proxy: %w", err)
		}
		defer proxy.Close()
		proxyURL = proxy.URL()
	}

	env := BuildSandboxEnv(os.Environ(), extraEnv, normPolicy, proxyURL)

	runCtx := ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	var cancel context.CancelFunc
	if normPolicy.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, normPolicy.Timeout)
		defer cancel()
	}

	res, runErr := runOSSandboxPlatform(runCtx, normPolicy, resolvedWorkDir, name, args, env, proxyURL)
	res.Duration = time.Since(started)
	if proxy != nil {
		res.BlockedDomains = proxy.BlockedDomains()
		res.EgressAudit = proxy.AuditLog()
	}
	res.AuditDigest = computeSandboxAuditDigest(normPolicy, resolvedWorkDir, name, args, res)
	return res, runErr
}

// RunSandboxedShell runs a shell command string through RunSandboxedCommand
// using the host's native non-interactive shell.
func RunSandboxedShell(ctx context.Context, policy OSSandboxPolicy, workDir string, command string, extraEnv map[string]string) (OSSandboxResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return OSSandboxResult{}, errors.New("sandbox shell command is empty")
	}
	if runtime.GOOS == "windows" {
		return RunSandboxedCommand(ctx, policy, workDir, "powershell.exe", []string{
			"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command,
		}, extraEnv)
	}
	return RunSandboxedCommand(ctx, policy, workDir, "/bin/sh", []string{"-lc", command}, extraEnv)
}

// IsSecretEnvKey reports whether an environment variable name holds sensitive
// credentials or API keys that must be stripped from sandboxed subprocesses.
func IsSecretEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "" {
		return false
	}
	prefixes := []string{
		"AWS_",
		"ANTHROPIC_",
		"OPENAI_",
		"GEMINI_",
		"COMPUTEMESH_",
		"LOCALCODE_ANDROID_KEYSTORE",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	exact := map[string]bool{
		"GITHUB_TOKEN": true,
		"GH_TOKEN":     true,
		"NPM_TOKEN":    true,
		"PYPI_TOKEN":   true,
	}
	if exact[upper] {
		return true
	}
	suffixes := []string{
		"_SECRET",
		"_PASSWORD",
		"_TOKEN",
		"_API_KEY",
		"_SECRET_KEY",
		"_PRIVATE_KEY",
		"_ACCESS_KEY",
		"_AUTH_TOKEN",
	}
	for _, s := range suffixes {
		if strings.HasSuffix(upper, s) {
			return true
		}
	}
	return false
}

// BuildSandboxEnv constructs a clean subprocess environment with optional
// secret stripping and network/egress-proxy configuration.
func BuildSandboxEnv(baseEnv []string, extraEnv map[string]string, policy OSSandboxPolicy, proxyURL string) []string {
	type envPair struct {
		key string
		val string
	}
	order := make([]string, 0, len(baseEnv)+len(extraEnv)+8)
	values := make(map[string]envPair, len(baseEnv)+len(extraEnv)+8)

	normKey := func(k string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(k)
		}
		return k
	}
	setEntry := func(k, v string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		nk := normKey(k)
		if _, exists := values[nk]; !exists {
			order = append(order, nk)
		}
		values[nk] = envPair{key: k, val: v}
	}

	for _, kv := range baseEnv {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		k := kv[:idx]
		v := kv[idx+1:]
		if policy.StripSecrets && IsSecretEnvKey(k) {
			continue
		}
		setEntry(k, v)
	}

	if len(extraEnv) > 0 {
		keys := make([]string, 0, len(extraEnv))
		for k := range extraEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if policy.StripSecrets && IsSecretEnvKey(k) {
				continue
			}
			setEntry(k, extraEnv[k])
		}
	}

	if !policy.AllowNetwork {
		setEntry("HTTP_PROXY", sandboxBlackholeProxyURL)
		setEntry("HTTPS_PROXY", sandboxBlackholeProxyURL)
		setEntry("ALL_PROXY", sandboxBlackholeProxyURL)
		setEntry("http_proxy", sandboxBlackholeProxyURL)
		setEntry("https_proxy", sandboxBlackholeProxyURL)
		setEntry("all_proxy", sandboxBlackholeProxyURL)
		setEntry("NO_PROXY", "")
		setEntry("no_proxy", "")
	} else if strings.TrimSpace(proxyURL) != "" {
		setEntry("HTTP_PROXY", proxyURL)
		setEntry("HTTPS_PROXY", proxyURL)
		setEntry("ALL_PROXY", proxyURL)
		setEntry("http_proxy", proxyURL)
		setEntry("https_proxy", proxyURL)
		setEntry("all_proxy", proxyURL)
		setEntry("NO_PROXY", "")
		setEntry("no_proxy", "")
	}

	out := make([]string, 0, len(order))
	for _, nk := range order {
		pair := values[nk]
		out = append(out, pair.key+"="+pair.val)
	}
	return out
}

// ValidateSandboxPaths verifies that workDir is canonically within WorkspaceRoot,
// not inside any blocked path, and that no command argument escapes the workspace
// or references a protected/blocked path (.ssh, .aws, .gnupg, .git/hooks, etc.).
func ValidateSandboxPaths(policy OSSandboxPolicy, workDir string, args []string) (string, error) {
	if strings.TrimSpace(policy.WorkspaceRoot) == "" {
		return "", errors.New("sandbox workspace root is empty")
	}
	blocked := policy.BlockedPaths
	if len(blocked) == 0 {
		blocked = DefaultSandboxBlockedPaths()
	}

	targetDir := strings.TrimSpace(workDir)
	if targetDir == "" {
		targetDir = policy.WorkspaceRoot
	}
	resolvedDir, err := ensureExistingProjectDir(policy.WorkspaceRoot, targetDir)
	if err != nil {
		return "", fmt.Errorf("sandbox working directory denied: %w", err)
	}
	if matched, ok := matchBlockedSandboxPath(policy.WorkspaceRoot, resolvedDir, blocked); ok {
		return "", fmt.Errorf("sandbox working directory targets protected path (%s): %s", matched, resolvedDir)
	}

	readOnlyCanon := make([]string, 0, len(policy.ReadOnlyPaths))
	for _, ro := range policy.ReadOnlyPaths {
		if c, cErr := canonicalSandboxPath(ro); cErr == nil {
			readOnlyCanon = append(readOnlyCanon, c)
		}
	}

	for _, arg := range args {
		for _, token := range extractPathCandidateTokens(arg) {
			if matched, ok := matchBlockedSandboxPath(policy.WorkspaceRoot, token, blocked); ok {
				return "", fmt.Errorf("sandbox argument targets protected path (%s): %s", matched, token)
			}
			if err := validateSandboxArgContainment(policy.WorkspaceRoot, resolvedDir, readOnlyCanon, token); err != nil {
				return "", err
			}
		}
	}
	return resolvedDir, nil
}

func validateSandboxArgContainment(workspaceRoot, workDir string, readOnlyCanon []string, token string) error {
	cleaned := strings.Trim(strings.TrimSpace(token), `"'`+"`")
	if cleaned == "" {
		return nil
	}
	// Ignore URLs (handled by network / egress policy, not filesystem containment).
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ftp://") {
		return nil
	}
	// Ignore harmless Windows command switches like /C, /D, /S, /Q, /PID.
	if runtime.GOOS == "windows" && strings.HasPrefix(cleaned, "/") && !strings.Contains(cleaned[1:], "/") && !strings.Contains(cleaned, `\`) {
		return nil
	}

	expanded := expandHomeTilde(cleaned)
	slashed := filepath.ToSlash(expanded)

	// Explicit parent traversal ("..") must stay within WorkspaceRoot.
	hasDotDot := slashed == ".." ||
		strings.HasPrefix(slashed, "../") ||
		strings.HasSuffix(slashed, "/..") ||
		strings.Contains(slashed, "/../")
	if hasDotDot {
		candidate := expanded
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workDir, candidate)
		}
		if _, err := ensureWithinRoot(workspaceRoot, candidate); err != nil {
			if isAllowedReadOnlyPath(candidate, readOnlyCanon) {
				return nil
			}
			return fmt.Errorf("sandbox argument escapes workspace root: %s (%w)", token, err)
		}
		return nil
	}

	// Home-relative or explicit absolute paths must stay within WorkspaceRoot or ReadOnlyPaths.
	isHomeRef := cleaned == "~" || strings.HasPrefix(cleaned, "~/") || strings.HasPrefix(cleaned, `~\`)
	isAbsPath := filepath.IsAbs(expanded) || isWindowsDrivePath(expanded) || strings.HasPrefix(expanded, `\\`)
	if isHomeRef || isAbsPath {
		if _, err := ensureWithinRoot(workspaceRoot, expanded); err == nil {
			return nil
		}
		if isAllowedReadOnlyPath(expanded, readOnlyCanon) {
			return nil
		}
		if isBenignNullDevice(expanded) {
			return nil
		}
		return fmt.Errorf("sandbox argument path outside workspace root: %s", token)
	}

	return nil
}

func isAllowedReadOnlyPath(candidate string, readOnlyCanon []string) bool {
	if len(readOnlyCanon) == 0 {
		return false
	}
	canon, err := canonicalSandboxPath(candidate)
	if err != nil {
		return false
	}
	for _, ro := range readOnlyCanon {
		if pathWithin(ro, canon) {
			return true
		}
	}
	return false
}

func isBenignNullDevice(path string) bool {
	p := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	return p == "/dev/null" || p == "nul" || p == "nul:"
}

func isWindowsDrivePath(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	isAlpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	return isAlpha && s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

func expandHomeTilde(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func extractPathCandidateTokens(arg string) []string {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Trim(strings.TrimSpace(s), `"'`+"`,;()[]{}")
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
		// Also inspect flag value in --flag=value or -f=value
		if eq := strings.IndexByte(s, '='); eq > 0 && eq < len(s)-1 {
			rhs := strings.Trim(strings.TrimSpace(s[eq+1:]), `"'`+"`")
			if rhs != "" && !seen[rhs] {
				seen[rhs] = true
				out = append(out, rhs)
			}
		}
	}

	add(trimmed)
	// Also split shell-like command strings so embedded path arguments are checked.
	for _, part := range splitShellLikeTokens(trimmed) {
		add(part)
	}
	return out
}

func splitShellLikeTokens(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if quote != 0 {
			if r == quote {
				quote = 0
				flush()
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		switch r {
		case '"', '\'', '`':
			flush()
			quote = r
		case ' ', '\t', '\r', '\n', ';', '|', '&', '<', '>':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func matchBlockedSandboxPath(workspaceRoot, rawCandidate string, blocked []string) (string, bool) {
	cleaned := strings.Trim(strings.TrimSpace(rawCandidate), `"'`+"`")
	if cleaned == "" {
		return "", false
	}
	expanded := expandHomeTilde(cleaned)
	slashed := strings.ToLower(strings.ReplaceAll(expanded, `\`, `/`))
	for strings.HasPrefix(slashed, "./") {
		slashed = strings.TrimPrefix(slashed, "./")
	}

	homeDir, _ := os.UserHomeDir()
	var canonCandidate string
	var canonCandidateErr error
	candidateResolved := false
	resolveCandidate := func() (string, error) {
		if !candidateResolved {
			candidateResolved = true
			c := expanded
			if !filepath.IsAbs(c) && !isWindowsDrivePath(c) && workspaceRoot != "" {
				c = filepath.Join(workspaceRoot, c)
			}
			canonCandidate, canonCandidateErr = canonicalSandboxPath(c)
		}
		return canonCandidate, canonCandidateErr
	}

	for _, b := range blocked {
		bTrim := strings.TrimSpace(b)
		if bTrim == "" {
			continue
		}
		bSlash := strings.Trim(strings.ToLower(strings.ReplaceAll(bTrim, `\`, `/`)), "/")
		if bSlash == "" {
			continue
		}
		// 1. Segment match anywhere in candidate path (e.g. ~/.ssh/id_rsa, .git/hooks/pre-commit, C:/Users/x/.aws/credentials).
		if pathContainsSegments(slashed, bSlash) {
			return bTrim, true
		}
		// 2. Canonical containment match (catches symlinks/junctions resolving into blocked dirs).
		if cand, err := resolveCandidate(); err == nil {
			var blockedTargets []string
			if filepath.IsAbs(bTrim) || isWindowsDrivePath(bTrim) {
				blockedTargets = append(blockedTargets, bTrim)
			} else {
				if workspaceRoot != "" {
					blockedTargets = append(blockedTargets, filepath.Join(workspaceRoot, filepath.FromSlash(bTrim)))
				}
				if homeDir != "" {
					blockedTargets = append(blockedTargets, filepath.Join(homeDir, filepath.FromSlash(bTrim)))
				}
			}
			for _, bt := range blockedTargets {
				if canonBlocked, bErr := canonicalSandboxPath(bt); bErr == nil {
					if pathWithin(canonBlocked, cand) {
						return bTrim, true
					}
				}
			}
		}
	}
	return "", false
}

func pathContainsSegments(normalizedPath, normalizedPattern string) bool {
	if normalizedPath == normalizedPattern {
		return true
	}
	if strings.HasPrefix(normalizedPath, normalizedPattern+"/") {
		return true
	}
	if strings.HasSuffix(normalizedPath, "/"+normalizedPattern) {
		return true
	}
	return strings.Contains(normalizedPath, "/"+normalizedPattern+"/")
}

type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = defaultSandboxMaxOutput
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := len(p)
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			b.buf.Write(p[:remaining])
			b.truncated = true
		} else {
			b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return total, nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return b.buf.String() + "\n...[truncated]"
	}
	return b.buf.String()
}

func computeSandboxAuditDigest(policy OSSandboxPolicy, workDir, name string, args []string, res OSSandboxResult) string {
	payload := struct {
		Backend        string   `json:"backend"`
		WorkspaceRoot  string   `json:"workspace_root"`
		WorkDir        string   `json:"work_dir"`
		Command        string   `json:"command"`
		Args           []string `json:"args"`
		ExitCode       int      `json:"exit_code"`
		TimedOut       bool     `json:"timed_out"`
		AllowNetwork   bool     `json:"allow_network"`
		AllowedDomains []string `json:"allowed_domains,omitempty"`
		DeniedDomains  []string `json:"denied_domains,omitempty"`
		BlockedDomains []string `json:"blocked_domains,omitempty"`
		StripSecrets   bool     `json:"strip_secrets"`
		StdoutBytes    int      `json:"stdout_bytes"`
		StderrBytes    int      `json:"stderr_bytes"`
	}{
		Backend:        res.SandboxBackend,
		WorkspaceRoot:  policy.WorkspaceRoot,
		WorkDir:        workDir,
		Command:        name,
		Args:           args,
		ExitCode:       res.ExitCode,
		TimedOut:       res.TimedOut,
		AllowNetwork:   policy.AllowNetwork,
		AllowedDomains: policy.AllowedDomains,
		DeniedDomains:  policy.DeniedDomains,
		BlockedDomains: res.BlockedDomains,
		StripSecrets:   policy.StripSecrets,
		StdoutBytes:    len(res.Stdout),
		StderrBytes:    len(res.Stderr),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
