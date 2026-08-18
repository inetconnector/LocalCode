// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const lspPoolMaxEntries = 8

type lspPoolEntry struct {
	mu       sync.Mutex
	client   *lspClient
	project  string
	spec     lspServerSpec
	lastUsed time.Time
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
		start:      startLSPClient,
	}
}

func lspPoolKey(project string, spec lspServerSpec) string {
	project = filepath.Clean(project)
	return strings.Join([]string{
		strings.ToLower(project),
		strings.ToLower(filepath.Clean(spec.Executable)),
		spec.Tool,
		strings.Join(spec.Args, "\x1f"),
	}, "\x00")
}

func (p *lspClientPool) entry(project string, spec lspServerSpec) *lspPoolEntry {
	key := lspPoolKey(project, spec)
	p.mu.Lock()
	if entry := p.entries[key]; entry != nil {
		entry.lastUsed = time.Now()
		p.mu.Unlock()
		return entry
	}
	entry := &lspPoolEntry{project: filepath.Clean(project), spec: spec, lastUsed: time.Now()}
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

func (p *lspClientPool) withClient(ctx context.Context, project string, cfg Config, spec lspServerSpec, fn func(*lspClient) (string, error)) (string, error) {
	if p == nil || fn == nil {
		return "", errors.New("LSP pool is not initialized")
	}
	entry := p.entry(project, spec)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		if entry.client == nil {
			client, err := p.start(ctx, entry.project, cfg, entry.spec)
			if err != nil {
				return "", err
			}
			entry.client = client
		}
		entry.client.actionMu.Lock()
		result, err := fn(entry.client)
		entry.client.actionMu.Unlock()
		if err == nil {
			p.touch(entry)
			return result, nil
		}
		if !lspShouldRestart(err, ctx) {
			return "", err
		}
		entry.client.close()
		entry.client = nil
		if attempt == 1 {
			return "", err
		}
	}
	return "", fmt.Errorf("LSP request failed after restart")
}

func (p *lspClientPool) touch(entry *lspPoolEntry) {
	if p == nil || entry == nil {
		return
	}
	p.mu.Lock()
	entry.lastUsed = time.Now()
	p.mu.Unlock()
}

func lspShouldRestart(err error, ctx context.Context) bool {
	if err == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	var responseErr *lspServerResponseError
	if errors.As(err, &responseErr) {
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
		if entry.client != nil {
			entry.client.close()
			entry.client = nil
		}
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
