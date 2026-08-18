// SPDX-License-Identifier: Apache-2.0

package benchharness

import "testing"

func TestManifestValidationContractBranches(t *testing.T) {
	valid := Manifest{
		Version:       ManifestVersion,
		Name:          "benchmark",
		Repository:    t.TempDir(),
		BaseRef:       "HEAD",
		Task:          "implement task",
		Engine:        "native",
		Model:         "model",
		EngineCommand: []string{"engine"},
		Checks: []Check{{
			Name:     "tests",
			Kind:     "test",
			Command:  []string{"go", "test", "./..."},
			Required: true,
		}},
	}

	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"missing repository", func(m *Manifest) { m.Repository = "" }},
		{"missing base ref", func(m *Manifest) { m.BaseRef = "" }},
		{"missing task", func(m *Manifest) { m.Task = "" }},
		{"missing engine", func(m *Manifest) { m.Engine = "" }},
		{"missing model", func(m *Manifest) { m.Model = "" }},
		{"negative timeout", func(m *Manifest) { m.TimeoutSeconds = -1 }},
		{"empty setup command", func(m *Manifest) { m.SetupCommands = []Command{{Name: "setup"}} }},
		{"check missing name", func(m *Manifest) { m.Checks = []Check{{Kind: "test", Command: []string{"go", "test"}, Required: true}} }},
		{"check missing command", func(m *Manifest) { m.Checks = []Check{{Name: "tests", Kind: "test", Required: true}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid manifest accepted: %s", tc.name)
			}
		})
	}

	for _, kind := range []string{"build", "test", "hidden", "lint", "syntax", "custom"} {
		candidate := valid
		candidate.Checks = []Check{{Name: kind, Kind: kind, Command: []string{"check"}, Required: true}}
		if err := candidate.Validate(); err != nil {
			t.Fatalf("supported check kind %q rejected: %v", kind, err)
		}
	}

	candidate := valid
	candidate.MetricsFile = "bench/results/metrics.json"
	candidate.AllowedPaths = []string{"src", "tests/fixture.txt"}
	if err := candidate.Validate(); err != nil {
		t.Fatalf("safe manifest rejected: %v", err)
	}
}
