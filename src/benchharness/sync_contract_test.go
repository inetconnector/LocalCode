// SPDX-License-Identifier: Apache-2.0

package benchharness

import "testing"

func TestBenchmarkFairnessContractRequiresPinnedExecutionInputs(t *testing.T) {
	valid := Manifest{
		Version:       ManifestVersion,
		Name:          "fairness-sync",
		Repository:    t.TempDir(),
		BaseRef:       "HEAD",
		Task:          "same task",
		Engine:        "localcode",
		Model:         "same-model",
		EngineCommand: []string{"engine"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid fairness manifest rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"missing base", func(m *Manifest) { m.BaseRef = "" }},
		{"missing task", func(m *Manifest) { m.Task = "" }},
		{"missing model", func(m *Manifest) { m.Model = "" }},
		{"missing engine", func(m *Manifest) { m.Engine = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("benchmark manifest accepted %s", tc.name)
			}
		})
	}
}
