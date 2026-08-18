// SPDX-License-Identifier: Apache-2.0

package benchharness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ManifestVersion = 1

type Manifest struct {
	Version        int               `json:"version"`
	Name           string            `json:"name"`
	Repository     string            `json:"repository"`
	BaseRef        string            `json:"base_ref"`
	Task           string            `json:"task"`
	Engine         string            `json:"engine"`
	Model          string            `json:"model"`
	EngineCommand  []string          `json:"engine_command"`
	SetupCommands  []Command         `json:"setup_commands,omitempty"`
	Checks         []Check           `json:"checks"`
	AllowedPaths   []string          `json:"allowed_paths,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	MetricsFile    string            `json:"metrics_file,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	KeepWorktree   bool              `json:"keep_worktree,omitempty"`
}

type Command struct {
	Name    string   `json:"name,omitempty"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout_seconds,omitempty"`
}

type Check struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Command  []string `json:"command"`
	Required bool     `json:"required"`
	Timeout  int      `json:"timeout_seconds,omitempty"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse benchmark manifest: %w", err)
	}
	if !filepath.IsAbs(manifest.Repository) {
		manifest.Repository = filepath.Join(filepath.Dir(path), manifest.Repository)
	}
	manifest.Repository = filepath.Clean(manifest.Repository)
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.Version != ManifestVersion {
		return fmt.Errorf("unsupported benchmark manifest version %d", m.Version)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("benchmark name is required")
	}
	if strings.TrimSpace(m.Repository) == "" {
		return errors.New("benchmark repository is required")
	}
	if strings.TrimSpace(m.BaseRef) == "" {
		return errors.New("benchmark base_ref is required")
	}
	if strings.TrimSpace(m.Task) == "" {
		return errors.New("benchmark task is required")
	}
	if strings.TrimSpace(m.Engine) == "" {
		return errors.New("benchmark engine is required")
	}
	if strings.TrimSpace(m.Model) == "" {
		return errors.New("benchmark model is required so engines can be compared fairly")
	}
	if len(m.EngineCommand) == 0 || strings.TrimSpace(m.EngineCommand[0]) == "" {
		return errors.New("benchmark engine_command is required")
	}
	if m.TimeoutSeconds < 0 {
		return errors.New("timeout_seconds cannot be negative")
	}
	for index, command := range m.SetupCommands {
		if len(command.Args) == 0 || strings.TrimSpace(command.Args[0]) == "" {
			return fmt.Errorf("setup command %d is empty", index)
		}
	}
	for index, check := range m.Checks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("check %d has no name", index)
		}
		switch strings.ToLower(strings.TrimSpace(check.Kind)) {
		case "build", "test", "hidden", "lint", "syntax", "custom":
		default:
			return fmt.Errorf("check %q has unsupported kind %q", check.Name, check.Kind)
		}
		if len(check.Command) == 0 || strings.TrimSpace(check.Command[0]) == "" {
			return fmt.Errorf("check %q has no command", check.Name)
		}
	}
	for _, allowed := range m.AllowedPaths {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(allowed)))
		if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(allowed) {
			return fmt.Errorf("invalid allowed path %q", allowed)
		}
	}
	if m.MetricsFile != "" {
		clean := filepath.Clean(m.MetricsFile)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("metrics_file must remain inside benchmark worktree: %q", m.MetricsFile)
		}
	}
	return nil
}
