// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const mcpProtocolVersion = "2026-07-28"

var mcpProtocolVersions = []string{"2026-07-28", "2025-11-25", "2025-06-18"}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpPendingResult struct {
	response mcpRPCResponse
	err      error
}

type mcpStdioSession struct {
	key       string
	server    MCPServerConfig
	project   string
	protocol  string
	command   *exec.Cmd
	stdin     io.WriteCloser
	writeMu   sync.Mutex
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int64]chan mcpPendingResult
	stderrMu  sync.Mutex
	stderr    bytes.Buffer
	done      chan struct{}
	closeOnce sync.Once
}

type mcpHTTPSession struct {
	server   MCPServerConfig
	project  string
	endpoint string
	session  string
	protocol string
	client   *http.Client
	mu       sync.Mutex
}

type mcpManager struct {
	mu    sync.Mutex
	stdio map[string]*mcpStdioSession
	http  map[string]*mcpHTTPSession
}

var defaultMCPManager = newMCPManager()

func newMCPManager() *mcpManager {
	return &mcpManager{stdio: map[string]*mcpStdioSession{}, http: map[string]*mcpHTTPSession{}}
}

func (m *mcpManager) Close() {
	m.mu.Lock()
	sessions := make([]*mcpStdioSession, 0, len(m.stdio))
	for _, session := range m.stdio {
		sessions = append(sessions, session)
	}
	m.stdio = map[string]*mcpStdioSession{}
	m.http = map[string]*mcpHTTPSession{}
	m.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func (m *mcpManager) ResetServer(name string) {
	m.mu.Lock()
	var sessions []*mcpStdioSession
	for key, session := range m.stdio {
		if strings.EqualFold(session.server.Name, name) {
			sessions = append(sessions, session)
			delete(m.stdio, key)
		}
	}
	for key, session := range m.http {
		if strings.EqualFold(session.server.Name, name) {
			delete(m.http, key)
		}
	}
	m.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func findMCPServer(cfg Config, name string) (MCPServerConfig, error) {
	requested := strings.TrimSpace(name)
	for _, server := range cfg.MCPServers {
		if server.Enabled && strings.EqualFold(server.Name, requested) {
			return server, nil
		}
	}
	return MCPServerConfig{}, fmt.Errorf("enabled MCP server not found: %s", requested)
}

func mcpServersSummary(cfg Config) string {
	var lines []string
	for _, s := range cfg.MCPServers {
		if s.Enabled {
			label := s.DisplayName
			if strings.TrimSpace(label) == "" {
				label = s.Name
			}
			lines = append(lines, fmt.Sprintf("- %s [%s]", label, s.Transport))
		}
	}
	if len(lines) == 0 {
		return localizeConfigText(cfg, "Keine MCP-Server aktiviert.", "No MCP servers enabled.")
	}
	return strings.Join(lines, "\n")
}

func expandMCPValue(value, project string) string {
	replacements := map[string]string{
		"PROJECT_ROOT": project,
		"APP_DATA":     appDataDir(),
		"USER_HOME":    userProfileDir(),
	}
	value = os.Expand(value, func(key string) string {
		if replacement, ok := replacements[key]; ok {
			return replacement
		}
		return os.Getenv(key)
	})
	return os.ExpandEnv(value)
}

func expandMCPArgs(args []string, project string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = expandMCPValue(arg, project)
	}
	return out
}

func mcpServerKey(server MCPServerConfig, project string) string {
	payload, _ := json.Marshal(server)
	sum := sha256.Sum256(append(payload, []byte("\n"+project)...))
	return strings.ToLower(server.Name) + ":" + hex.EncodeToString(sum[:8])
}

func mcpCall(ctx context.Context, cfg Config, project, serverName, method string, params any) (string, error) {
	server, err := findMCPServer(cfg, serverName)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(server.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(server.Transport)) {
	case "builtin":
		return mcpCallBuiltin(callCtx, cfg, project, server, method, params)
	case "stdio":
		return defaultMCPManager.callStdio(callCtx, cfg, project, server, method, params)
	case "http", "streamable-http":
		return defaultMCPManager.callHTTP(callCtx, cfg, project, server, method, params)
	default:
		return "", fmt.Errorf("unsupported MCP transport %q", server.Transport)
	}
}

func mcpInitializeRequest(id int64, protocol string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "initialize", "params": map[string]any{
		"protocolVersion": protocol,
		"capabilities": map[string]any{
			"roots": map[string]any{"listChanged": false},
		},
		"clientInfo": mcpClientInfo{Name: "LocalCode", Version: version},
	}}
}

func mcpRequest(id int64, method string, params any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

func mcpNotification(method string, params any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
}

func (m *mcpManager) callStdio(ctx context.Context, cfg Config, project string, server MCPServerConfig, method string, params any) (string, error) {
	key := mcpServerKey(server, project)
	m.mu.Lock()
	session := m.stdio[key]
	m.mu.Unlock()
	if session == nil || session.isClosed() {
		var err error
		session, err = startMCPStdioSession(ctx, cfg, project, server, key)
		if err != nil {
			return "", err
		}
		m.mu.Lock()
		if existing := m.stdio[key]; existing != nil && !existing.isClosed() {
			m.mu.Unlock()
			session.close()
			session = existing
		} else {
			m.stdio[key] = session
			m.mu.Unlock()
		}
	}
	response, err := session.request(ctx, method, params)
	if err != nil {
		if session.isClosed() {
			m.mu.Lock()
			delete(m.stdio, key)
			m.mu.Unlock()
		}
		return "", fmt.Errorf("MCP %s/%s: %w; stderr=%s", server.Name, method, err, session.stderrText())
	}
	return prettyJSON(response.Result), nil
}

func quoteCmdToken(value string) string {
	if value == "" {
		return `""`
	}
	if !strings.ContainsAny(value, " \t&|<>^()\"") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func newMCPProcess(command string, args []string) *exec.Cmd {
	if strings.EqualFold(filepath.Ext(command), ".cmd") || strings.EqualFold(filepath.Ext(command), ".bat") {
		parts := []string{quoteCmdToken(command)}
		for _, arg := range args {
			parts = append(parts, quoteCmdToken(arg))
		}
		return exec.Command("cmd.exe", "/D", "/S", "/C", strings.Join(parts, " "))
	}
	return exec.Command(command, args...)
}

func startMCPStdioSession(ctx context.Context, cfg Config, project string, server MCPServerConfig, key string) (*mcpStdioSession, error) {
	command := strings.TrimSpace(expandMCPValue(server.Command, project))
	if command == "" {
		return nil, errors.New("MCP stdio command is empty")
	}
	args := expandMCPArgs(server.Args, project)
	if resolved, available := commandAvailable(command, project); available {
		command = resolved
	}
	cmd := newMCPProcess(command, args)
	hideCommandWindow(cmd)
	if server.ProjectScoped && strings.TrimSpace(project) != "" {
		cmd.Dir = project
	}
	env := os.Environ()
	for k, v := range cfg.EnvironmentVars {
		env = append(env, k+"="+expandMCPValue(v, project))
	}
	for k, v := range server.Env {
		env = append(env, k+"="+expandMCPValue(v, project))
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	session := &mcpStdioSession{
		key: key, server: server, project: project, command: cmd, stdin: stdin,
		pending: map[int64]chan mcpPendingResult{}, done: make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MCP server %s: %w", command, err)
	}
	go session.readStdout(stdout)
	go session.readStderr(stderr)
	go session.waitProcess()

	initialized := false
	var errorsByVersion []string
	for _, protocol := range mcpProtocolVersions {
		response, initErr := session.requestWithPayload(ctx, mcpInitializeRequest(session.nextID.Add(1), protocol))
		if initErr == nil && response.Error == nil {
			session.protocol = protocol
			initialized = true
			break
		}
		if initErr != nil {
			errorsByVersion = append(errorsByVersion, protocol+": "+initErr.Error())
		} else if response.Error != nil {
			errorsByVersion = append(errorsByVersion, protocol+": "+response.Error.Message)
		}
	}
	if !initialized {
		session.close()
		return nil, fmt.Errorf("MCP initialize failed for %s: %s", server.Name, strings.Join(errorsByVersion, "; "))
	}
	if err := session.notify("notifications/initialized", map[string]any{}); err != nil {
		session.close()
		return nil, err
	}
	return session, nil
}

func (s *mcpStdioSession) isClosed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *mcpStdioSession) request(ctx context.Context, method string, params any) (mcpRPCResponse, error) {
	id := s.nextID.Add(1)
	return s.requestWithPayload(ctx, mcpRequest(id, method, params))
}

func requestID(payload map[string]any) (int64, error) {
	value, ok := payload["id"]
	if !ok {
		return 0, errors.New("JSON-RPC request has no id")
	}
	switch id := value.(type) {
	case int64:
		return id, nil
	case int:
		return int64(id), nil
	case float64:
		return int64(id), nil
	default:
		return 0, fmt.Errorf("unsupported JSON-RPC id %T", value)
	}
}

func (s *mcpStdioSession) requestWithPayload(ctx context.Context, payload map[string]any) (mcpRPCResponse, error) {
	id, err := requestID(payload)
	if err != nil {
		return mcpRPCResponse{}, err
	}
	responseCh := make(chan mcpPendingResult, 1)
	s.pendingMu.Lock()
	s.pending[id] = responseCh
	s.pendingMu.Unlock()
	if err := s.write(payload); err != nil {
		s.removePending(id)
		return mcpRPCResponse{}, err
	}
	select {
	case result := <-responseCh:
		return result.response, result.err
	case <-ctx.Done():
		s.removePending(id)
		return mcpRPCResponse{}, ctx.Err()
	case <-s.done:
		s.removePending(id)
		return mcpRPCResponse{}, errors.New("MCP server process stopped")
	}
}

func (s *mcpStdioSession) removePending(id int64) {
	s.pendingMu.Lock()
	delete(s.pending, id)
	s.pendingMu.Unlock()
}

func (s *mcpStdioSession) notify(method string, params any) error {
	return s.write(mcpNotification(method, params))
}

func (s *mcpStdioSession) write(value any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.isClosed() {
		return errors.New("MCP server session is closed")
	}
	encoder := json.NewEncoder(s.stdin)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func (s *mcpStdioSession) readStdout(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var message map[string]json.RawMessage
		if err := json.Unmarshal(line, &message); err != nil {
			continue
		}
		if methodRaw, ok := message["method"]; ok {
			var method string
			_ = json.Unmarshal(methodRaw, &method)
			if idRaw, hasID := message["id"]; hasID {
				s.handleServerRequest(idRaw, method, message["params"])
			}
			continue
		}
		var response mcpRPCResponse
		if err := json.Unmarshal(line, &response); err != nil {
			continue
		}
		id, ok := numericRPCID(response.ID)
		if !ok {
			continue
		}
		s.pendingMu.Lock()
		ch := s.pending[id]
		delete(s.pending, id)
		s.pendingMu.Unlock()
		if ch != nil {
			if response.Error != nil {
				ch <- mcpPendingResult{response: response, err: fmt.Errorf("JSON-RPC %d: %s", response.Error.Code, response.Error.Message)}
			} else {
				ch <- mcpPendingResult{response: response}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		s.failAllPending(err)
	}
	s.close()
}

func (s *mcpStdioSession) handleServerRequest(idRaw json.RawMessage, method string, params json.RawMessage) {
	var id any
	if err := json.Unmarshal(idRaw, &id); err != nil {
		return
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	switch method {
	case "roots/list":
		roots := []map[string]string{}
		if strings.TrimSpace(s.project) != "" {
			if uri, err := fileURI(s.project); err == nil {
				roots = append(roots, map[string]string{"uri": uri, "name": filepath.Base(s.project)})
			}
		}
		response["result"] = map[string]any{"roots": roots}
	case "ping":
		response["result"] = map[string]any{}
	default:
		response["error"] = mcpRPCError{Code: -32601, Message: "LocalCode does not implement client method " + method}
	}
	_ = s.write(response)
}

func fileURI(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	normalized := filepath.ToSlash(absolute)
	if len(normalized) >= 2 && normalized[1] == ':' {
		normalized = "/" + normalized
	}
	return (&url.URL{Scheme: "file", Path: normalized}).String(), nil
}

func numericRPCID(value any) (int64, bool) {
	switch id := value.(type) {
	case float64:
		return int64(id), true
	case json.Number:
		n, err := id.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(id, 10, 64)
		return n, err == nil
	case int64:
		return id, true
	case int:
		return int64(id), true
	default:
		return 0, false
	}
}

func (s *mcpStdioSession) readStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 32*1024), 4<<20)
	for scanner.Scan() {
		s.stderrMu.Lock()
		if s.stderr.Len() > 128<<10 {
			current := s.stderr.String()
			s.stderr.Reset()
			s.stderr.WriteString(current[len(current)/2:])
		}
		s.stderr.WriteString(scanner.Text())
		s.stderr.WriteByte('\n')
		s.stderrMu.Unlock()
	}
}

func (s *mcpStdioSession) stderrText() string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	return truncateText(strings.TrimSpace(s.stderr.String()), 4000)
}

func (s *mcpStdioSession) waitProcess() {
	err := s.command.Wait()
	if err != nil {
		s.failAllPending(err)
	}
	s.close()
}

func (s *mcpStdioSession) failAllPending(err error) {
	s.pendingMu.Lock()
	pending := s.pending
	s.pending = map[int64]chan mcpPendingResult{}
	s.pendingMu.Unlock()
	for _, ch := range pending {
		ch <- mcpPendingResult{err: err}
	}
}

func (s *mcpStdioSession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.stdin.Close()
		if s.command != nil && s.command.Process != nil {
			killProcessTree(s.command)
		}
		s.failAllPending(errors.New("MCP session closed"))
	})
}

func (m *mcpManager) callHTTP(ctx context.Context, cfg Config, project string, server MCPServerConfig, method string, params any) (string, error) {
	key := mcpServerKey(server, project)
	m.mu.Lock()
	session := m.http[key]
	if session == nil {
		timeout := time.Duration(server.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		session = &mcpHTTPSession{server: server, project: project, endpoint: strings.TrimSpace(expandMCPValue(server.URL, project)), client: &http.Client{Timeout: timeout}}
		m.http[key] = session
	}
	m.mu.Unlock()
	return session.call(ctx, cfg, method, params)
}

func (s *mcpHTTPSession) call(ctx context.Context, cfg Config, method string, params any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endpoint == "" {
		return "", errors.New("MCP HTTP URL is empty")
	}
	if s.protocol == "" {
		var initErrors []string
		for _, protocol := range mcpProtocolVersions {
			response, sessionID, err := mcpHTTPPost(ctx, s.client, cfg, s.server, s.project, s.endpoint, "", protocol, mcpInitializeRequest(1, protocol))
			if err == nil && response.Error == nil {
				s.protocol = protocol
				s.session = sessionID
				_, _, _ = mcpHTTPPost(ctx, s.client, cfg, s.server, s.project, s.endpoint, s.session, protocol, mcpNotification("notifications/initialized", map[string]any{}))
				break
			}
			if err != nil {
				initErrors = append(initErrors, protocol+": "+err.Error())
			} else if response.Error != nil {
				initErrors = append(initErrors, protocol+": "+response.Error.Message)
			}
		}
		if s.protocol == "" {
			return "", fmt.Errorf("MCP initialize failed for %s: %s", s.server.Name, strings.Join(initErrors, "; "))
		}
	}
	response, sessionID, err := mcpHTTPPost(ctx, s.client, cfg, s.server, s.project, s.endpoint, s.session, s.protocol, mcpRequest(2, method, params))
	if sessionID != "" {
		s.session = sessionID
	}
	if err != nil {
		return "", err
	}
	if response.Error != nil {
		return "", fmt.Errorf("MCP %s: %s", method, response.Error.Message)
	}
	return prettyJSON(response.Result), nil
}

func resolveMCPHeaders(ctx context.Context, cfg Config, project string, server MCPServerConfig) map[string]string {
	headers := map[string]string{}
	for key, value := range server.Headers {
		headers[key] = expandMCPValue(value, project)
	}
	if strings.EqualFold(server.Preset, "github") {
		auth := strings.TrimSpace(headers["Authorization"])
		if auth == "" || auth == "Bearer" || strings.Contains(auth, "${") {
			token := strings.TrimSpace(os.Getenv(server.AuthEnv))
			if token == "" {
				token = githubCLIToken(ctx, project, cfg)
			}
			if token != "" {
				headers["Authorization"] = "Bearer " + token
			}
		}
	}
	return headers
}

func githubCLIToken(ctx context.Context, project string, cfg Config) string {
	info := discoverTool(project, "gh", cfg, false)
	if !info.Available {
		return ""
	}
	tokenCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := runDirectTool(tokenCtx, project, info.Path, []string{"auth", "token"}, cfg)
	if result.Err != nil {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func mcpHTTPPost(ctx context.Context, client *http.Client, cfg Config, server MCPServerConfig, project, endpoint, session, protocol string, payload any) (mcpRPCResponse, string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocol)
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	for key, value := range resolveMCPHeaders(ctx, cfg, project, server) {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	response, err := client.Do(req)
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	defer response.Body.Close()
	if id := response.Header.Get("Mcp-Session-Id"); id != "" {
		session = id
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return mcpRPCResponse{}, session, err
	}
	if response.StatusCode == http.StatusAccepted && len(bytes.TrimSpace(body)) == 0 {
		return mcpRPCResponse{}, session, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return mcpRPCResponse{}, session, fmt.Errorf("MCP HTTP %d: %s", response.StatusCode, truncateText(string(body), 4000))
	}
	var rpc mcpRPCResponse
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		rpc, err = parseMCPSSE(body)
	} else if len(bytes.TrimSpace(body)) > 0 {
		err = json.Unmarshal(body, &rpc)
	}
	return rpc, session, err
}

func parseMCPSSE(body []byte) (mcpRPCResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	var dataLines []string
	flush := func() (mcpRPCResponse, bool) {
		if len(dataLines) == 0 {
			return mcpRPCResponse{}, false
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		var response mcpRPCResponse
		if json.Unmarshal([]byte(data), &response) == nil && response.ID != nil {
			return response, true
		}
		return mcpRPCResponse{}, false
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if response, ok := flush(); ok {
				return response, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if response, ok := flush(); ok {
		return response, nil
	}
	if err := scanner.Err(); err != nil {
		return mcpRPCResponse{}, err
	}
	return mcpRPCResponse{}, errors.New("no JSON-RPC response found in SSE")
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var output bytes.Buffer
	if json.Indent(&output, raw, "", "  ") == nil {
		return output.String()
	}
	return string(raw)
}
