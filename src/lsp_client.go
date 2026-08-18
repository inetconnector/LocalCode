// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type lspServerCandidate struct {
	Tool       string
	Args       []string
	LanguageID string
}

type lspServerSpec struct {
	Tool       string
	Executable string
	Args       []string
	LanguageID string
}

type lspRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type lspEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *lspRPCError    `json:"error,omitempty"`
}

type lspReadResult struct {
	Envelope lspEnvelope
	Err      error
}

type lspClient struct {
	stdin         io.WriteCloser
	cmd           *exec.Cmd
	workspaceURI  string
	workspaceName string
	mu            sync.Mutex
	nextID        int
	diagnostics   []json.RawMessage
	incoming      chan lspReadResult
}

func lspCandidatesForPath(path string) []lspServerCandidate {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return []lspServerCandidate{{Tool: "gopls", Args: []string{"serve"}, LanguageID: "go"}}
	case ".ts":
		return []lspServerCandidate{{Tool: "typescript-language-server", Args: []string{"--stdio"}, LanguageID: "typescript"}}
	case ".tsx":
		return []lspServerCandidate{{Tool: "typescript-language-server", Args: []string{"--stdio"}, LanguageID: "typescriptreact"}}
	case ".js", ".mjs", ".cjs":
		return []lspServerCandidate{{Tool: "typescript-language-server", Args: []string{"--stdio"}, LanguageID: "javascript"}}
	case ".jsx":
		return []lspServerCandidate{{Tool: "typescript-language-server", Args: []string{"--stdio"}, LanguageID: "javascriptreact"}}
	case ".py":
		return []lspServerCandidate{
			{Tool: "basedpyright-langserver", Args: []string{"--stdio"}, LanguageID: "python"},
			{Tool: "pyright-langserver", Args: []string{"--stdio"}, LanguageID: "python"},
			{Tool: "pylsp", LanguageID: "python"},
		}
	case ".rs":
		return []lspServerCandidate{{Tool: "rust-analyzer", LanguageID: "rust"}}
	case ".c":
		return []lspServerCandidate{{Tool: "clangd", LanguageID: "c"}}
	case ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp":
		return []lspServerCandidate{{Tool: "clangd", LanguageID: "cpp"}}
	case ".cs":
		return []lspServerCandidate{{Tool: "csharp-ls", LanguageID: "csharp"}}
	case ".java":
		return []lspServerCandidate{{Tool: "jdtls", LanguageID: "java"}}
	case ".kt", ".kts":
		return []lspServerCandidate{{Tool: "kotlin-language-server", LanguageID: "kotlin"}}
	default:
		return nil
	}
}

func resolveLSPServer(project string, cfg Config, path string) (lspServerSpec, error) {
	candidates := lspCandidatesForPath(path)
	if len(candidates) == 0 {
		return lspServerSpec{}, fmt.Errorf("no LocalCode LSP profile for %s", filepath.Ext(path))
	}
	searched := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		searched = append(searched, candidate.Tool)
		info := discoverTool(project, candidate.Tool, cfg, false)
		if !info.Available || strings.TrimSpace(info.Path) == "" {
			continue
		}
		return lspServerSpec{
			Tool:       candidate.Tool,
			Executable: info.Path,
			Args:       append([]string(nil), candidate.Args...),
			LanguageID: candidate.LanguageID,
		}, nil
	}
	return lspServerSpec{}, fmt.Errorf("no language server available; searched: %s", strings.Join(searched, ", "))
}

func lspFileURI(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func startLSPClient(ctx context.Context, project string, cfg Config, spec lspServerSpec) (*lspClient, error) {
	cmd := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	cmd.Dir = project
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	client := &lspClient{
		stdin:         stdin,
		cmd:           cmd,
		workspaceURI:  lspFileURI(project),
		workspaceName: filepath.Base(project),
		nextID:        1,
		incoming:      make(chan lspReadResult, 16),
	}
	go client.readLoop(bufio.NewReader(stdout))
	if err := client.initialize(ctx, spec); err != nil {
		client.close()
		return nil, err
	}
	return client, nil
}

func (c *lspClient) close() {
	if c == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	_, _ = c.request(shutdownCtx, "shutdown", map[string]any{})
	_ = c.notify("exit", map[string]any{})
	_ = c.stdin.Close()
	if c.cmd != nil {
		killProcessTree(c.cmd)
		_ = c.cmd.Wait()
	}
}

func (c *lspClient) initialize(ctx context.Context, spec lspServerSpec) error {
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   c.workspaceURI,
		"workspaceFolders": []map[string]any{{
			"uri":  c.workspaceURI,
			"name": c.workspaceName,
		}},
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"workspaceFolders": true,
				"symbol":           map[string]any{"dynamicRegistration": false},
			},
			"textDocument": map[string]any{
				"definition":     map[string]any{"linkSupport": true},
				"references":     map[string]any{},
				"hover":          map[string]any{},
				"documentSymbol": map[string]any{},
				"implementation": map[string]any{},
				"callHierarchy":  map[string]any{},
			},
		},
		"clientInfo": map[string]any{"name": "LocalCode", "version": version},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return fmt.Errorf("LSP initialize via %s: %w", spec.Tool, err)
	}
	return c.notify("initialized", map[string]any{})
}

func (c *lspClient) openDocument(path, languageID, text string) error {
	return c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        lspFileURI(path),
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	})
}

func (c *lspClient) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *lspClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		var envelope lspEnvelope
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case incoming := <-c.incoming:
			if incoming.Err != nil {
				return nil, incoming.Err
			}
			envelope = incoming.Envelope
		}
		if envelope.Method != "" {
			if len(envelope.ID) > 0 && string(envelope.ID) != "null" {
				if err := c.replyToServerRequest(envelope); err != nil {
					return nil, err
				}
				continue
			}
			if envelope.Method == "textDocument/publishDiagnostics" {
				c.diagnostics = append(c.diagnostics, append(json.RawMessage(nil), envelope.Params...))
			}
			continue
		}
		responseID, _ := strconv.Atoi(strings.Trim(string(envelope.ID), `"`))
		if responseID != id {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("LSP %s error %d: %s", method, envelope.Error.Code, envelope.Error.Message)
		}
		return envelope.Result, nil
	}
}

func (c *lspClient) readLoop(reader *bufio.Reader) {
	for {
		envelope, err := readLSPEnvelope(reader)
		c.incoming <- lspReadResult{Envelope: envelope, Err: err}
		if err != nil {
			return
		}
	}
}

func (c *lspClient) replyToServerRequest(envelope lspEnvelope) error {
	var result any
	switch envelope.Method {
	case "workspace/configuration":
		var params struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(envelope.Params, &params)
		result = make([]any, len(params.Items))
	case "workspace/workspaceFolders":
		result = []map[string]any{{"uri": c.workspaceURI, "name": c.workspaceName}}
	case "workspace/applyEdit":
		result = map[string]any{"applied": false, "failureReason": "LocalCode LSP navigation is read-only"}
	default:
		result = nil
	}
	var id any
	if err := json.Unmarshal(envelope.ID, &id); err != nil {
		return err
	}
	return c.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (c *lspClient) write(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = c.stdin.Write(data)
	return err
}

func readLSPEnvelope(reader *bufio.Reader) (lspEnvelope, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return lspEnvelope{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			contentLength, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return lspEnvelope{}, fmt.Errorf("invalid LSP Content-Length: %w", err)
			}
		}
	}
	if contentLength < 0 || contentLength > 16*1024*1024 {
		return lspEnvelope{}, fmt.Errorf("invalid LSP content length %d", contentLength)
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return lspEnvelope{}, err
	}
	var envelope lspEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return lspEnvelope{}, fmt.Errorf("invalid LSP JSON: %w", err)
	}
	return envelope, nil
}

func lspPositionParams(path string, line, character int) map[string]any {
	if line < 1 {
		line = 1
	}
	if character < 1 {
		character = 1
	}
	return map[string]any{
		"textDocument": map[string]any{"uri": lspFileURI(path)},
		"position":     map[string]any{"line": line - 1, "character": character - 1},
	}
}

func normalizeLSPOperation(operation string) string {
	operation = strings.ToLower(strings.TrimSpace(operation))
	operation = strings.NewReplacer("_", "", "-", "", " ", "").Replace(operation)
	return operation
}

func runLSPAction(ctx context.Context, project string, cfg Config, action AgentAction) (string, error) {
	if strings.TrimSpace(action.Path) == "" {
		return "", errors.New("lsp action requires path")
	}
	full, err := ensureWithinRoot(project, action.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("LSP path is a directory: %s", action.Path)
	}
	text, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if !isProbablyText(text) {
		return "", fmt.Errorf("LSP path is binary or non-UTF-8: %s", action.Path)
	}
	spec, err := resolveLSPServer(project, cfg, full)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 15 * time.Second
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return defaultLSPPool.withDocumentClient(queryCtx, project, cfg, spec, full, string(text), func(client *lspClient) (string, error) {
		operation := normalizeLSPOperation(action.Name)
		var method string
		var params any
		switch operation {
		case "definition", "gotodefinition":
			method = "textDocument/definition"
			params = lspPositionParams(full, action.Line, action.Character)
		case "references", "findreferences":
			method = "textDocument/references"
			params = lspPositionParams(full, action.Line, action.Character)
			params.(map[string]any)["context"] = map[string]any{"includeDeclaration": true}
		case "hover":
			method = "textDocument/hover"
			params = lspPositionParams(full, action.Line, action.Character)
		case "documentsymbol", "symbols":
			method = "textDocument/documentSymbol"
			params = map[string]any{"textDocument": map[string]any{"uri": lspFileURI(full)}}
		case "workspacesymbol":
			method = "workspace/symbol"
			params = map[string]any{"query": strings.TrimSpace(action.Query)}
		case "implementation", "gotoimplementation":
			method = "textDocument/implementation"
			params = lspPositionParams(full, action.Line, action.Character)
		case "preparecallhierarchy":
			method = "textDocument/prepareCallHierarchy"
			params = lspPositionParams(full, action.Line, action.Character)
		case "incomingcalls", "outgoingcalls":
			prepared, prepareErr := client.request(queryCtx, "textDocument/prepareCallHierarchy", lspPositionParams(full, action.Line, action.Character))
			if prepareErr != nil {
				return "", prepareErr
			}
			var items []json.RawMessage
			if err := json.Unmarshal(prepared, &items); err != nil || len(items) == 0 {
				return formatLSPResult(spec, "textDocument/prepareCallHierarchy", prepared, client.diagnostics), err
			}
			if operation == "incomingcalls" {
				method = "callHierarchy/incomingCalls"
			} else {
				method = "callHierarchy/outgoingCalls"
			}
			var item any
			if err := json.Unmarshal(items[0], &item); err != nil {
				return "", err
			}
			params = map[string]any{"item": item}
		default:
			return "", fmt.Errorf("unsupported LSP operation %q", action.Name)
		}
		result, err := client.request(queryCtx, method, params)
		if err != nil {
			return "", err
		}
		return formatLSPResult(spec, method, result, client.diagnostics), nil
	})
}

func formatLSPResult(spec lspServerSpec, method string, result json.RawMessage, diagnostics []json.RawMessage) string {
	var compact bytes.Buffer
	if len(result) == 0 {
		result = json.RawMessage("null")
	}
	if err := json.Indent(&compact, result, "", "  "); err != nil {
		compact.Write(result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "LSP RESULT\nServer: %s\nMethod: %s\nResult:\n%s\n", spec.Tool, method, compact.String())
	if len(diagnostics) > 0 {
		b.WriteString("Diagnostics observed while querying:\n")
		for _, diagnostic := range diagnostics {
			var pretty bytes.Buffer
			if json.Indent(&pretty, diagnostic, "", "  ") == nil {
				b.Write(pretty.Bytes())
			} else {
				b.Write(diagnostic)
			}
			b.WriteByte('\n')
		}
	}
	return truncateText(b.String(), 60000)
}
