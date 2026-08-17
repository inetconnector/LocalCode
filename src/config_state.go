// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// configMutationError distinguishes invalid caller-provided mutations from
// persistence failures. HTTP handlers can return 4xx for the former while
// keeping disk/config-store errors as 5xx.
type configMutationError struct {
	err error
}

func (e *configMutationError) Error() string {
	if e == nil || e.err == nil {
		return "invalid config mutation"
	}
	return e.err.Error()
}

func (e *configMutationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func isConfigMutationError(err error) bool {
	var target *configMutationError
	return errors.As(err, &target)
}

func cloneConfig(cfg Config) (Config, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("clone config: %w", err)
	}
	var clone Config
	if err := json.Unmarshal(data, &clone); err != nil {
		return Config{}, fmt.Errorf("clone config: %w", err)
	}
	return clone, nil
}

// mutateConfig serializes the complete state transition: read current state,
// mutate a detached copy, normalize it, persist it, then publish it in memory.
// Holding AppState.mu through persistence is intentional. Configuration writes
// are rare, and releasing the lock between snapshot and save allows a stale
// whole-Config snapshot to overwrite a newer unrelated setting.
func (s *AppState) mutateConfig(mutator func(*Config) error) (Config, error) {
	if s == nil {
		return Config{}, errors.New("app state is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	next, err := cloneConfig(s.Config)
	if err != nil {
		return s.Config, err
	}
	if mutator != nil {
		if err := mutator(&next); err != nil {
			return s.Config, &configMutationError{err: err}
		}
	}
	next = normalizeConfig(next)
	if err := saveConfig(next); err != nil {
		return s.Config, err
	}
	s.Config = next
	return next, nil
}
