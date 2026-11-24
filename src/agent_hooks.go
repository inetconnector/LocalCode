// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HookVetoExitCode is the deterministic exit code a command hook returns to
// block tool execution with a human-readable reason.
const HookVetoExitCode = 2

// HookEvent identifies a lifecycle hook event in Hooks v2.
type HookEvent string

const (
	HookEventPreToolUse   HookEvent = "PreToolUse"
	HookEventPostToolUse  HookEvent = "PostToolUse"
	HookEventOnEdit       HookEvent = "OnEdit"
	HookEventOnTestFail   HookEvent = "OnTestFail"
	HookEventStop         HookEvent = "Stop"
	HookEventSubagentStop HookEvent = "SubagentStop"
)

// HookDefinition configures a command or loopback-HTTP hook bound to a HookEvent.
type HookDefinition struct {
	Name        string            `json:"name,omitempty"`
	Event       HookEvent         `json:"event"`
	Type        string            `json:"type,omitempty"` // "command" (default) | "http"
	Command     string            `json:"command,omitempty"`
	URL         string            `json:"url,omitempty"`
	ToolMatcher string            `json:"tool_matcher,omitempty"` // optional action matcher ("*", exact, or prefix*)
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// HookPayload is the structured JSON document delivered on stdin to command
// hooks and as the POST body to loopback HTTP hooks.
type HookPayload struct {
	Event      HookEvent         `json:"event"`
	Phase      string            `json:"phase,omitempty"`
	Project    string            `json:"project"`
	Action     AgentAction       `json:"action"`
	ToolOutput string            `json:"tool_output,omitempty"`
	ToolError  string            `json:"tool_error,omitempty"`
	ExitCode   int               `json:"exit_code,omitempty"`
	SubagentID string            `json:"subagent_id,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// HookResult summarizes the execution of a single hook.
type HookResult struct {
	Event    HookEvent     `json:"event"`
	HookName string        `json:"hook_name,omitempty"`
	Blocked  bool          `json:"blocked"`
	Reason   string        `json:"reason,omitempty"`
	Output   string        `json:"output,omitempty"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// HookVetoError is returned when a hook deterministically vetoes an action
// (e.g. exit code 2 or structured block decision from a loopback HTTP hook).
type HookVetoError struct {
	Event    HookEvent
	HookName string
	Reason   string
	Output   string
	ExitCode int
}

func (e *HookVetoError) Error() string {
	if e == nil {
		return ""
	}
	name := strings.TrimSpace(e.HookName)
	if name == "" {
		name = string(e.Event)
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = fmt.Sprintf("blocked by hook (exit code %d)", e.ExitCode)
	}
	return fmt.Sprintf("hook %s vetoed action: %s", name, reason)
}

// IsHookVetoError extracts a *HookVetoError if err represents a hook veto.
func IsHookVetoError(err error) (*HookVetoError, bool) {
	var veto *HookVetoError
	if errors.As(err, &veto) {
		return veto, true
	}
	return nil, false
}

func normalizeHookEvent(raw string) (HookEvent, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "before", "pretooluse", "pre_tool_use":
		return HookEventPreToolUse, true
	case "after", "posttooluse", "post_tool_use":
		return HookEventPostToolUse, true
	case "onedit", "on_edit":
		return HookEventOnEdit, true
	case "ontestfail", "on_test_fail":
		return HookEventOnTestFail, true
	case "stop":
		return HookEventStop, true
	case "subagentstop", "subagent_stop":
		return HookEventSubagentStop, true
	default:
		return "", false
	}
}

func hookEventPhaseAlias(event HookEvent) string {
	switch event {
	case HookEventPreToolUse:
		return "before"
	case HookEventPostToolUse:
		return "after"
	default:
		return strings.ToLower(string(event))
	}
}

func hookEventRequiresToolAction(event HookEvent) bool {
	switch event {
	case HookEventPreToolUse, HookEventPostToolUse, HookEventOnEdit:
		return true
	default:
		return false
	}
}

func agentToolHookEligible(a AgentAction) bool {
	switch a.Action {
	case "", "ask_user", "finish":
		return false
	default:
		return true
	}
}

// runAgentToolHook preserves backwards-compatible hook invocation for
// HookBeforeTool / HookAfterTool while automatically enabling Hooks v2
// stdin JSON payloads, exit-code-2 deterministic vetoing, and loopback HTTP URLs.
func runAgentToolHook(ctx context.Context, project string, cfg Config, phase string, a AgentAction) (string, error) {
	var target string
	event, ok := normalizeHookEvent(phase)
	if !ok {
		return "", nil
	}
	switch event {
	case HookEventPreToolUse:
		target = strings.TrimSpace(cfg.HookBeforeTool)
	case HookEventPostToolUse:
		target = strings.TrimSpace(cfg.HookAfterTool)
	default:
		return "", nil
	}
	if target == "" || !agentToolHookEligible(a) {
		return "", nil
	}

	def := HookDefinition{
		Name:       "legacy_" + hookEventPhaseAlias(event),
		Event:      event,
		TimeoutSec: cfg.CommandTimeout,
	}
	lowerTarget := strings.ToLower(target)
	if strings.HasPrefix(lowerTarget, "http://") || strings.HasPrefix(lowerTarget, "https://") {
		def.Type = "http"
		def.URL = target
	} else {
		def.Type = "command"
		def.Command = target
	}

	payload := HookPayload{
		Event:     event,
		Phase:     phase,
		Project:   project,
		Action:    a,
		Timestamp: time.Now().UTC(),
	}

	res, err := RunHookV2(ctx, project, cfg, def, payload)
	return res.Output, err
}

// LoadProjectHooksV2 loads Hooks v2 definitions from .localcode/hooks.json
// inside the project root (if present).
func LoadProjectHooksV2(project string) ([]HookDefinition, error) {
	if strings.TrimSpace(project) == "" {
		return nil, nil
	}
	full, err := ensureWithinRoot(project, filepath.Join(".localcode", "hooks.json"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var wrapper struct {
		Hooks []HookDefinition `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Hooks) > 0 {
		return wrapper.Hooks, nil
	}
	var direct []HookDefinition
	if err := json.Unmarshal(raw, &direct); err != nil {
		return nil, fmt.Errorf("parse .localcode/hooks.json: %w", err)
	}
	return direct, nil
}

// RunHooksV2 runs all hook definitions matching payload.Event (and ToolMatcher)
// in order. Execution halts immediately on the first deterministic veto.
func RunHooksV2(ctx context.Context, project string, cfg Config, defs []HookDefinition, payload HookPayload) ([]HookResult, error) {
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now().UTC()
	}
	if payload.Project == "" {
		payload.Project = project
	}
	if payload.Phase == "" {
		payload.Phase = hookEventPhaseAlias(payload.Event)
	}

	var results []HookResult
	for _, def := range defs {
		ev, ok := normalizeHookEvent(string(def.Event))
		if !ok || ev != payload.Event {
			continue
		}
		if hookEventRequiresToolAction(ev) && !agentToolHookEligible(payload.Action) {
			continue
		}
		if !hookToolMatches(def.ToolMatcher, payload.Action.Action) {
			continue
		}
		res, err := RunHookV2(ctx, project, cfg, def, payload)
		results = append(results, res)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// RunHookV2 executes a single Hooks v2 definition (either command with JSON on
// stdin + exit-code-2 veto, or strictly SSRF-checked loopback HTTP POST).
func RunHookV2(ctx context.Context, project string, cfg Config, def HookDefinition, payload HookPayload) (HookResult, error) {
	started := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	ev, ok := normalizeHookEvent(string(def.Event))
	if !ok {
		ev = payload.Event
	}
	if payload.Event == "" {
		payload.Event = ev
	}
	if payload.Phase == "" {
		payload.Phase = hookEventPhaseAlias(payload.Event)
	}
	if payload.Project == "" {
		payload.Project = project
	}
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now().UTC()
	}

	if hookEventRequiresToolAction(payload.Event) && !agentToolHookEligible(payload.Action) {
		return HookResult{Event: payload.Event, HookName: def.Name}, nil
	}

	timeoutSec := def.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = cfg.CommandTimeout
	}
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hookType := strings.ToLower(strings.TrimSpace(def.Type))
	if hookType == "" {
		if strings.TrimSpace(def.URL) != "" && strings.TrimSpace(def.Command) == "" {
			hookType = "http"
		} else {
			hookType = "command"
		}
	}

	var res HookResult
	var err error
	switch hookType {
	case "http":
		res, err = runLoopbackHTTPHookV2(hookCtx, def, payload)
	case "command":
		res, err = runCommandHookV2(hookCtx, project, cfg, def, payload)
	default:
		return HookResult{Event: payload.Event, HookName: def.Name, Duration: time.Since(started)}, fmt.Errorf("unsupported hook type: %s", def.Type)
	}
	res.Duration = time.Since(started)
	return res, err
}

func runCommandHookV2(ctx context.Context, project string, cfg Config, def HookDefinition, payload HookPayload) (HookResult, error) {
	res := HookResult{
		Event:    payload.Event,
		HookName: def.Name,
		ExitCode: -1,
	}
	command := strings.TrimSpace(def.Command)
	if command == "" {
		res.ExitCode = 0
		return res, nil
	}
	if err := commandBlocked(cfg, command); err != nil {
		return res, err
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return res, fmt.Errorf("marshal hook payload: %w", err)
	}

	hookCfg := cfg
	hookCfg.EnvironmentVars = cloneStringMap(cfg.EnvironmentVars)
	hookCfg.EnvironmentVars["LOCALCODE_HOOK_EVENT"] = string(payload.Event)
	hookCfg.EnvironmentVars["LOCALCODE_HOOK_PHASE"] = payload.Phase
	hookCfg.EnvironmentVars["LOCALCODE_ACTION"] = payload.Action.Action
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_MESSAGE"] = payload.Action.Message
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_PATH"] = payload.Action.Path
	hookCfg.EnvironmentVars["LOCALCODE_ACTION_COMMAND"] = payload.Action.Command

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if strings.EqualFold(strings.TrimSpace(cfg.TerminalShell), "cmd") {
			cmd = exec.Command("cmd.exe", "/D", "/S", "/C", command)
		} else {
			cmd = exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
		}
	} else {
		cmd = exec.Command("/bin/sh", "-lc", command)
	}
	if strings.TrimSpace(project) != "" {
		cmd.Dir = project
	}
	cmd.Env = commandEnvironment(hookCfg)
	cmd.Stdin = bytes.NewReader(payloadJSON)
	hideCommandWindow(cmd)

	stdout := newBoundedBuffer(256 * 1024)
	stderr := newBoundedBuffer(256 * 1024)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return res, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		killProcessTree(cmd)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		waitErr = ctx.Err()
	}

	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())
	combined := combineHookOutput(outStr, errStr)
	res.Output = combined

	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	} else if waitErr == nil {
		res.ExitCode = 0
	}

	// Deterministic Veto Mechanism: exit code 2 blocks tool execution with reason.
	if res.ExitCode == HookVetoExitCode {
		reason := extractHookVetoReason(outStr, errStr)
		res.Blocked = true
		res.Reason = reason
		return res, &HookVetoError{
			Event:    payload.Event,
			HookName: def.Name,
			Reason:   reason,
			Output:   combined,
			ExitCode: res.ExitCode,
		}
	}

	// Structured JSON decision on stdout also supports deterministic blocking.
	if blocked, reason := parseHookJSONDecision(outStr); blocked {
		res.Blocked = true
		res.Reason = reason
		return res, &HookVetoError{
			Event:    payload.Event,
			HookName: def.Name,
			Reason:   reason,
			Output:   combined,
			ExitCode: res.ExitCode,
		}
	}

	if waitErr != nil {
		return res, waitErr
	}
	return res, nil
}

// ValidateLoopbackHookURL enforces that an HTTP hook strictly targets localhost
// or a loopback IP (127.0.0.0/8 or ::1) to prevent SSRF exfiltration.
func ValidateLoopbackHookURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid hook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("hook URL scheme %q is not allowed (only http/https loopback)", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("hook URL must not contain embedded credentials")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return nil, errors.New("hook URL has empty host")
	}
	if host == "localhost" {
		return u, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("SSRF blocked: HTTP hook host %q is not a loopback address", host)
	}
	return u, nil
}

func runLoopbackHTTPHookV2(ctx context.Context, def HookDefinition, payload HookPayload) (HookResult, error) {
	res := HookResult{
		Event:    payload.Event,
		HookName: def.Name,
	}
	u, err := ValidateLoopbackHookURL(def.URL)
	if err != nil {
		return res, err
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return res, fmt.Errorf("marshal hook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return res, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LocalCode-Hook-Event", string(payload.Event))
	for k, v := range def.Headers {
		k = strings.TrimSpace(k)
		if k != "" {
			req.Header.Set(k, v)
		}
	}

	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			return loopbackOnlyDialContext(dialCtx, network, addr)
		},
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("SSRF blocked: HTTP hook redirects are forbidden")
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return res, fmt.Errorf("loopback HTTP hook request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	respText := strings.TrimSpace(string(respBytes))
	res.Output = respText
	res.ExitCode = resp.StatusCode

	// Explicit JSON veto decision or HTTP 403/409/422 veto status.
	if blocked, reason := parseHookJSONDecision(respText); blocked {
		res.Blocked = true
		res.Reason = reason
		return res, &HookVetoError{
			Event:    payload.Event,
			HookName: def.Name,
			Reason:   reason,
			Output:   respText,
			ExitCode: HookVetoExitCode,
		}
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity {
		reason := extractHookVetoReason(respText, "")
		if reason == "" || reason == "blocked by hook (exit code 2)" {
			reason = fmt.Sprintf("blocked by loopback HTTP hook (HTTP %d)", resp.StatusCode)
		}
		res.Blocked = true
		res.Reason = reason
		return res, &HookVetoError{
			Event:    payload.Event,
			HookName: def.Name,
			Reason:   reason,
			Output:   respText,
			ExitCode: HookVetoExitCode,
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return res, fmt.Errorf("loopback HTTP hook returned HTTP %d: %s", resp.StatusCode, truncateText(respText, 400))
	}
	res.ExitCode = 0
	return res, nil
}

func loopbackOnlyDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return nil, fmt.Errorf("SSRF blocked: non-loopback IP %s", ip.String())
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	if !strings.EqualFold(host, "localhost") {
		return nil, fmt.Errorf("SSRF blocked: non-loopback host %q", host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if !a.IP.IsLoopback() {
			return nil, fmt.Errorf("SSRF blocked: localhost resolved to non-loopback IP %s", a.IP.String())
		}
	}
	if len(addrs) == 0 {
		return nil, errors.New("localhost resolved to no addresses")
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
}

func hookToolMatches(matcher, actionName string) bool {
	m := strings.ToLower(strings.TrimSpace(matcher))
	if m == "" || m == "*" {
		return true
	}
	a := strings.ToLower(strings.TrimSpace(actionName))
	if strings.HasSuffix(m, "*") {
		return strings.HasPrefix(a, strings.TrimSuffix(m, "*"))
	}
	return a == m
}

func parseHookJSONDecision(text string) (blocked bool, reason string) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") {
		return false, ""
	}
	var resp struct {
		Decision string `json:"decision"`
		Blocked  bool   `json:"blocked"`
		Allow    *bool  `json:"allow"`
		Reason   string `json:"reason"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
		return false, ""
	}
	decision := strings.ToLower(strings.TrimSpace(resp.Decision))
	isBlocked := resp.Blocked ||
		decision == "block" || decision == "deny" || decision == "veto" ||
		(resp.Allow != nil && !*resp.Allow)
	if !isBlocked {
		return false, ""
	}
	r := strings.TrimSpace(resp.Reason)
	if r == "" {
		r = strings.TrimSpace(resp.Message)
	}
	if r == "" {
		r = "blocked by hook policy"
	}
	return true, r
}

func extractHookVetoReason(stdout, stderr string) string {
	for _, candidate := range []string{stdout, stderr} {
		trimmed := strings.TrimSpace(candidate)
		if strings.HasPrefix(trimmed, "{") {
			var obj struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
				for _, field := range []string{obj.Reason, obj.Message, obj.Error} {
					if strings.TrimSpace(field) != "" {
						return strings.TrimSpace(field)
					}
				}
			}
		}
	}
	if s := strings.TrimSpace(stderr); s != "" {
		return s
	}
	if s := strings.TrimSpace(stdout); s != "" {
		return s
	}
	return "blocked by hook (exit code 2)"
}

func combineHookOutput(stdout, stderr string) string {
	if stdout != "" && stderr != "" {
		return stdout + "\n" + stderr
	}
	if stdout != "" {
		return stdout
	}
	return stderr
}

func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
