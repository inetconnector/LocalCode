// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInstructionContextLoadsGlobalProjectRulesAndSkills(t *testing.T) {
	configHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("CODEX_HOME", codexHome)

	mustWrite(t, filepath.Join(configHome, productDirName, "AGENTS.md"), "global localcode base\n")
	mustWrite(t, filepath.Join(configHome, productDirName, "AGENTS.override.md"), "global localcode override\n")
	mustWrite(t, filepath.Join(codexHome, "AGENTS.md"), "global codex instruction\n")

	project := t.TempDir()
	mustWrite(t, filepath.Join(project, "AGENTS.md"), "project base should be hidden\n")
	mustWrite(t, filepath.Join(project, "AGENTS.override.md"), "project override instruction\n")
	mustWrite(t, filepath.Join(project, "README.md"), "# Readme\n\nsetup instructions\n")
	mustWrite(t, filepath.Join(project, "STATE.md"), "handoff state\n")
	mustWrite(t, filepath.Join(project, ".cursor", "rules", "always.mdc"), "---\nalwaysApply: true\n---\nalways cursor rule\n")
	mustWrite(t, filepath.Join(project, ".cursor", "rules", "python.mdc"), "---\ndescription: Python testing rule\n---\nrun pytest\n")
	mustWrite(t, filepath.Join(project, ".opencode", "skills", "python-test", "SKILL.md"), "---\ndescription: Python test workflow\n---\n# Python Test\n\nUse pytest and py_compile.\n")
	mustWrite(t, filepath.Join(project, ".opencode", "skills", "release", "SKILL.md"), "---\ndescription: Release workflow\n---\n# Release\n\nBuild changelog.\n")

	context := projectInstructionContext(project, "Bitte repariere die Python Tests.")

	for _, want := range []string{
		"global localcode override",
		"global codex instruction",
		"project override instruction",
		"setup instructions",
		"handoff state",
		"always cursor rule",
		"run pytest",
		"Use pytest and py_compile",
		"python-test [loaded]",
		"release [available]",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("context missing %q:\n%s", want, context)
		}
	}
	for _, forbidden := range []string{"global localcode base", "project base should be hidden", "# Release\n\nBuild changelog"} {
		if strings.Contains(context, forbidden) {
			t.Fatalf("context unexpectedly contains %q:\n%s", forbidden, context)
		}
	}
}

func TestProjectInstructionContextFallsBackWhenNoDocsExist(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	context := projectInstructionContext(t.TempDir(), "analyse")
	if !strings.Contains(context, "Keine Projektdokumente") {
		t.Fatalf("unexpected empty context: %s", context)
	}
}

func TestFrontmatterValueAndRelevance(t *testing.T) {
	content := "---\ndescription: API review workflow\nalwaysApply: false\n---\nbody\n"
	if got := frontmatterValue(content, "description"); got != "API review workflow" {
		t.Fatalf("description = %q", got)
	}
	if cursorRuleAlwaysApplies(content) {
		t.Fatal("rule should not always apply")
	}
	if !instructionTextRelevant("review the API", content) {
		t.Fatal("expected rule to be relevant")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
