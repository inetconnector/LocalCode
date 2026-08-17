// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newConfigTransactionTestState(t *testing.T) *AppState {
	t.Helper()
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))
	root := filepath.Join(base, "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.UIAccent = "initial"
	cfg.ApprovalRules = nil
	state := NewAppState(normalizeConfig(cfg), NewOllamaClient())
	t.Cleanup(state.Close)
	return state
}

func TestMutateConfigPreservesConcurrentDisjointUpdates(t *testing.T) {
	state := newConfigTransactionTestState(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)

	go func() {
		defer wg.Done()
		<-start
		_, err := state.mutateConfig(func(cfg *Config) error {
			cfg.UIAccent = "concurrent-accent"
			return nil
		})
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := state.mutateConfig(func(cfg *Config) error {
			cfg.ApprovalRules = append(cfg.ApprovalRules, ApprovalRule{
				ID: "concurrent-rule", Scope: "global", Decision: "allow", Pattern: []string{"git", "status"},
			})
			return nil
		})
		errCh <- err
	}()
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	state.mu.RLock()
	cfg := state.Config
	state.mu.RUnlock()
	if cfg.UIAccent != "concurrent-accent" {
		t.Fatalf("disjoint UI update lost: %q", cfg.UIAccent)
	}
	found := false
	for _, rule := range cfg.ApprovalRules {
		if rule.ID == "concurrent-rule" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("disjoint approval-rule update was lost")
	}
}

func TestMutateConfigRollsBackAliasedMapsOnMutationFailure(t *testing.T) {
	state := newConfigTransactionTestState(t)
	state.mu.Lock()
	state.Config.ProjectAliases = map[string]string{"before": "Before"}
	state.mu.Unlock()

	_, err := state.mutateConfig(func(cfg *Config) error {
		cfg.ProjectAliases["leak"] = "must-not-leak"
		return errors.New("reject mutation")
	})
	if err == nil || !isConfigMutationError(err) {
		t.Fatalf("expected config mutation error, got %v", err)
	}
	state.mu.RLock()
	_, leaked := state.Config.ProjectAliases["leak"]
	before := state.Config.ProjectAliases["before"]
	state.mu.RUnlock()
	if leaked || before != "Before" {
		t.Fatalf("failed mutation leaked through shallow aliases: %#v", state.Config.ProjectAliases)
	}
}
