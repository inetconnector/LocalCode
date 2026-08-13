// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
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
	listContent := "---\nglobs:\n  - \"src/**/*.go\"\n  - scripts/*.ps1\n  - \"docs, notes/*.md\"\npermissions: [read, write]\nscripts: \"scripts/check.ps1\", \"scripts/lint, strict.ps1\"\n---\nbody\n"
	if got := frontmatterList(listContent, "globs"); len(got) != 3 || got[0] != "src/**/*.go" || got[1] != "scripts/*.ps1" || got[2] != "docs, notes/*.md" {
		t.Fatalf("unexpected frontmatter list: %#v", got)
	}
	if got := frontmatterList(listContent, "scripts"); len(got) != 2 || got[0] != "scripts/check.ps1" || got[1] != "scripts/lint, strict.ps1" {
		t.Fatalf("unexpected script frontmatter list: %#v", got)
	}
	if !skillMetadataRequiresApproval(frontmatterList(listContent, "permissions"), frontmatterList(listContent, "scripts")) {
		t.Fatal("write permissions and scripts should require approval")
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

func TestSkillPermissionsRequireApprovalAndAvoidAutoEmbedding(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, ".codex", "skills", "danger-skill", "SKILL.md"), "---\ndescription: Dangerous build helper\nalwaysApply: true\npermissions:\n  - read\n  - shell\nscripts:\n  - scripts/build.ps1\n---\n# Dangerous Skill\n\nDO NOT AUTO LOAD THIS BODY.\n")

	context := projectInstructionContext(project, "build")
	if !strings.Contains(context, "danger-skill [approval-required]") || !strings.Contains(context, "permissions=read,shell") || !strings.Contains(context, "scripts=scripts/build.ps1") {
		t.Fatalf("approval metadata missing from context:\n%s", context)
	}
	if strings.Contains(context, "DO NOT AUTO LOAD THIS BODY") {
		t.Fatalf("approval-required skill was embedded automatically:\n%s", context)
	}
	read, err := readSkillByName(project, "danger-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read, "Approval: required") || !strings.Contains(read, "DO NOT AUTO LOAD THIS BODY") {
		t.Fatalf("skill_read should expose metadata and text:\n%s", read)
	}
}

func TestSkillResourcesListAndRead(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, ".codex", "skills", "asset-skill", "SKILL.md"), "---\ndescription: Asset workflow\n---\n# Asset Skill\n\nRead references before creating assets.\n")
	mustWrite(t, filepath.Join(project, ".codex", "skills", "asset-skill", "references", "palette.md"), "# Palette\n\nUse blue and yellow.\n")

	list, err := listSkillResources(project, "asset-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "references/palette.md") {
		t.Fatalf("resource missing from list:\n%s", list)
	}
	read, err := readSkillResource(project, "asset-skill", "references/palette.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read, "Use blue and yellow.") {
		t.Fatalf("unexpected resource read:\n%s", read)
	}
	if _, err := readSkillResource(project, "asset-skill", "../outside.md"); err == nil {
		t.Fatal("resource traversal should fail")
	}
	if _, err := readSkillResource(project, "asset-skill", "SKILL.md"); err == nil {
		t.Fatal("SKILL.md should be read through skill_read")
	}
}

func TestCopySkillResourceCopiesBinaryAsset(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, ".codex", "skills", "asset-skill", "SKILL.md"), "---\ndescription: Asset workflow\n---\n# Asset Skill\n\nCopy binary assets when required.\n")
	want := []byte{0x00, 0x01, 0x02, 0xff, 'P', 'N', 'G'}
	mustWriteBytes(t, filepath.Join(project, ".codex", "skills", "asset-skill", "assets", "icon.bin"), want)

	result, err := copySkillResource(project, "asset-skill", "assets/icon.bin", "public/icon.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "SKILL RESOURCE COPIED") || !strings.Contains(result, "POSTCONDITION") || !strings.Contains(result, "public/icon.bin") {
		t.Fatalf("unexpected copy result:\n%s", result)
	}
	got, err := os.ReadFile(filepath.Join(project, "public", "icon.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("copied bytes differ: got %v want %v", got, want)
	}
	if _, err := copySkillResource(project, "asset-skill", "../outside.bin", "public/outside.bin"); err == nil {
		t.Fatal("resource traversal should fail")
	}
	if _, err := copySkillResource(project, "asset-skill", "SKILL.md", "public/SKILL.md"); err == nil {
		t.Fatal("SKILL.md should not be copied through resource path")
	}
}

func TestSkillRunScriptExecutesDeclaredCommand(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	project := t.TempDir()
	mustWrite(t, filepath.Join(project, ".codex", "skills", "script-skill", "SKILL.md"), "---\ndescription: Script workflow\ncommands:\n  - echo skill-ok\n---\n# Script Skill\n\nRun declared commands only.\n")
	cfg := defaultConfig()
	cfg.CommandTimeout = 5
	cfg.Language = "en"
	cfg.PreferredLanguage = "en"

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "skill_run_script", Skill: "script-skill", Script: "echo skill-ok"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "SKILL SCRIPT EXECUTED") || !strings.Contains(result, "echo skill-ok") || !strings.Contains(result, "skill-ok") {
		t.Fatalf("unexpected script result:\n%s", result)
	}
	if _, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "skill_run_script", Skill: "script-skill", Script: "echo undeclared"}); err == nil {
		t.Fatal("undeclared script should fail")
	}
	if _, _, _, err := resolveSkillScriptCommand(project, cfg, "script-skill", "echo skill-ok", []string{"bad\narg"}); err == nil {
		t.Fatal("line-breaking script argument should fail")
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

func mustWriteBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
