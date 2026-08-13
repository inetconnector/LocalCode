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
	mustWrite(t, filepath.Join(codexHome, "skills", "global-python", "SKILL.md"), "---\ndescription: Global Python workflow\n---\n# Global Python\n\nUse global pytest guidance.\n")

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
		"Use global pytest guidance",
		"global-python [loaded]",
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

func TestInstructionContextMatchesGlobs(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, ".cursor", "rules", "go.mdc"), "---\ndescription: unrelated\nglobs: src/**/*.go\n---\nuse go vet for matched files\n")
	mustWrite(t, filepath.Join(project, ".codex", "skills", "python-skill", "SKILL.md"), "---\ndescription: unrelated\nglobs: \"*.py\"\n---\n# Python Glob\n\nUse py_compile for matched files.\n")

	context := projectInstructionContext(project, "Passe src/server/main.go und scripts/check.py an.")
	if !strings.Contains(context, "use go vet for matched files") {
		t.Fatalf("cursor glob rule was not loaded:\n%s", context)
	}
	if !strings.Contains(context, "Use py_compile for matched files.") {
		t.Fatalf("skill glob match was not loaded:\n%s", context)
	}
	if !instructionGlobMatch(project, "src/server/main.go", "src/**/*.go") {
		t.Fatal("expected recursive go glob to match")
	}
	if !instructionGlobMatch(project, "scripts/check.py", "*.py") {
		t.Fatal("expected basename glob to match")
	}
}

func TestSkillListAndRead(t *testing.T) {
	configHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("CODEX_HOME", codexHome)
	project := t.TempDir()
	mustWrite(t, filepath.Join(configHome, productDirName, "skills", "global-design", "SKILL.md"), "---\ndescription: Design asset workflow\n---\n# Design\n\nCreate concrete visual assets.\n")

	list := formatSkillList(project, "design asset")
	if !strings.Contains(list, "global-design [loaded]") {
		t.Fatalf("global skill missing from list:\n%s", list)
	}
	read, err := readSkillByName(project, "global-design")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read, "Create concrete visual assets.") || !strings.Contains(read, "Skill: global-design") {
		t.Fatalf("unexpected skill read:\n%s", read)
	}
	if _, err := readSkillByName(project, "missing-skill"); err == nil {
		t.Fatal("missing skill should fail")
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
