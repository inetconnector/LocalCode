// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const lspPoolMaxEntries = 8

type lspDocumentState struct {
	Version int
	Hash    [sha256.Size]byte
}

type lspPoolEntry struct {
	mu        sync.Mutex
	client    *lspClient
	project   string
	spec      lspServerSpec
	documents map[string]lspDocumentState
	lastUsed  time.Time
}

type lspClientPool struct {
	mu         sync.Mutex
	entries    map[string]*lspPoolEntry
	maxEntries int
	start      func(context.Context, string, Config, lspServerSpec) (*lspClient, error)
}

var defaultLSPPool = newLSPClientPool(lspPoolMaxEntries)

func newLSPClientPool(maxEntries int) *lspClientPool {
	if maxEntries <= 0 {
		maxEntries = lspPoolMaxEntries
	}
	return &lspClientPool{
		entries:    make(map[string]*lspPoolEntry),
		maxEntries: maxEntries,
		start:      startPersistentLSPClient,
	}
}

// startPersistentLSPClient deliberately does not bind the language-server
// process lifetime to a single query context. The context only bounds the LSP
// initialize handshake; the pool owns the process lifetime and closes it on
// eviction, transport failure, or application shutdown.
func startPersistentLSPClient(ctx context.Context, project string, cfg Config, spec lspServerSpec) (*lspClient, error) {
	cmd := exec.Command(spec.Executable, spec.Args...)
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

func lspPoolConfigFingerprint(cfg Config) string {
	keys := make([]string, 0, len(cfg.EnvironmentVars))
	for key := range cfg.EnvironmentVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = io.WriteString(hash, key)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, cfg.EnvironmentVars[key])
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

func lspPoolKey(project string, cfg Config, spec lspServerSpec) string {
	project = filepath.Clean(project)
	executable := filepath.Clean(spec.Executable)
	if runtime.GOOS == "windows" {
		project = strings.ToLower(project)
		executable = strings.ToLower(executable)
	}
	return strings.Join([]string{
		project,
		executable,
		strings.ToLower(spec.Tool),
		strings.Join(spec.Args, "\x1f"),
		lspPoolConfigFingerprint(cfg),
	}, "\x00")
}

func (p *lspClientPool) entry(project string, cfg Config, spec lspServerSpec) *lspPoolEntry {
	key := lspPoolKey(project, cfg, spec)
	p.mu.Lock()
	if entry := p.entries[key]; entry != nil {
		entry.lastUsed = time.Now()
		p.mu.Unlock()
		return entry
	}
	entry := &lspPoolEntry{
		project:   filepath.Clean(project),
		spec:      spec,
		documents: make(map[string]lspDocumentState),
		lastUsed:  time.Now(),
	}
	p.entries[key] = entry
	var evicted *lspPoolEntry
	if len(p.entries) > p.maxEntries {
		evicted = p.evictIdleLocked(key)
	}
	p.mu.Unlock()
	if evicted != nil {
		if evicted.client != nil {
			evicted.client.close()
		}
		evicted.mu.Unlock()
	}
	return entry
}

// evictIdleLocked removes the least-recently-used entry whose operation lock
// can be acquired immediately. The returned entry stays locked so a caller can
// close its process after releasing the pool mutex without racing an operation.
func (p *lspClientPool) evictIdleLocked(exceptKey string) *lspPoolEntry {
	var selectedKey string
	var selected *lspPoolEntry
	for key, candidate := range p.entries {
		if key == exceptKey || candidate == nil || !candidate.mu.TryLock() {
			continue
		}
		if selected == nil || candidate.lastUsed.Before(selected.lastUsed) {
			if selected != nil {
				selected.mu.Unlock()
			}
			selectedKey = key
			selected = candidate
			continue
		}
		candidate.mu.Unlock()
	}
	if selected != nil {
		delete(p.entries, selectedKey)
	}
	return selected
}

func (p *lspClientPool) withDocumentClient(ctx context.Context, project string, cfg Config, spec lspServerSpec, path, text string, fn func(*lspClient) (string, error)) (string, error) {
	if p == nil || fn == nil {
		return "", errors.New("LSP pool is not initialized")
	}
	entry := p.entry(project, cfg, spec)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		if entry.client == nil {
			client, err := p.start(ctx, entry.project, cfg, entry.spec)
			if err != nil {
				return "", err
			}
			entry.client = client
			entry.documents = make(map[string]lspDocumentState)
		}
		entry.client.diagnostics = nil
		if err := entry.syncDocument(path, text); err != nil {
			entry.invalidate()
			if attempt == 0 && ctx.Err() == nil {
				continue
			}
			return "", err
		}
		result, err := fn(entry.client)
		if err == nil {
			p.touch(entry)
			return result, nil
		}
		if !lspShouldInvalidate(err) {
			return "", err
		}
		entry.invalidate()
		if attempt == 0 && ctx.Err() == nil {
			continue
		}
		return "", err
	}
	return "", errors.New("LSP request failed after restart")
}

func (entry *lspPoolEntry) invalidate() {
	if entry.client != nil {
		entry.client.close()
		entry.client = nil
	}
	entry.documents = make(map[string]lspDocumentState)
}

func (entry *lspPoolEntry) syncDocument(path, text string) error {
	path = filepath.Clean(path)
	hash := sha256.Sum256([]byte(text))
	state, opened := entry.documents[path]
	if !opened {
		if err := entry.client.openDocument(path, entry.spec.LanguageID, text); err != nil {
			return err
		}
		entry.documents[path] = lspDocumentState{Version: 1, Hash: hash}
		return nil
	}
	if state.Hash == hash {
		return nil
	}
	state.Version++
	if err := entry.client.notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     lspFileURI(path),
			"version": state.Version,
		},
		"contentChanges": []map[string]any{{"text": text}},
	}); err != nil {
		return err
	}
	state.Hash = hash
	entry.documents[path] = state
	return nil
}

func (p *lspClientPool) touch(entry *lspPoolEntry) {
	if p == nil || entry == nil {
		return
	}
	p.mu.Lock()
	entry.lastUsed = time.Now()
	p.mu.Unlock()
}

func lspShouldInvalidate(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.HasPrefix(text, "unsupported lsp operation") {
		return false
	}
	// JSON-RPC method/parameter errors are valid responses from a live server;
	// restarting cannot repair them. Transport, EOF and timeout errors invalidate
	// the client so stale responses cannot bleed into a later request.
	if strings.HasPrefix(text, "lsp ") && strings.Contains(text, " error ") {
		return false
	}
	return true
}

func (p *lspClientPool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	entries := make([]*lspPoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	p.entries = make(map[string]*lspPoolEntry)
	p.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
		entry.invalidate()
		entry.mu.Unlock()
	}
}

func (p *lspClientPool) size() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}
