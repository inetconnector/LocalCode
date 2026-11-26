// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestOfflineJestWriteBlockedPreventsJestChurnLoop(t *testing.T) {
	project := t.TempDir()

	if msg := offlineJestWriteBlocked(project, AgentAction{Action: "write_file", Path: "jest.config.js", Content: "module.exports = {};"}); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected jest.config.js to be blocked, got %q", msg)
	}
	if msg := offlineJestWriteBlocked(project, AgentAction{Action: "write_file", Path: "setupTests.js", Content: "jest.mock('fs');"}); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected setupTests.js to be blocked, got %q", msg)
	}
	if msg := offlineJestWriteBlocked(project, AgentAction{Action: "write_file", Path: "package.json", Content: `{"devDependencies":{"jest":"^29.0.0"}}`}); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected package.json with jest to be blocked, got %q", msg)
	}
	if msg := offlineJestWriteBlocked(project, AgentAction{Action: "write_file", Path: "test.js", Content: `const fn = jest.fn();`}); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected test.js with jest.fn to be blocked, got %q", msg)
	}
	if msg := offlineJestWriteBlocked(project, AgentAction{Action: "write_file", Path: "test.js", Content: `const test = require('node:test'); const assert = require('node:assert/strict');`}); msg != "" {
		t.Fatalf("valid node:test file must not be blocked, got %q", msg)
	}
}

func TestPerFileWriteChurnBlockedStopsPackageJSONLoops(t *testing.T) {
	writes := map[string]int{"package.json": 2, "game.js": 6}
	if msg := perFileWriteChurnBlocked(AgentAction{Action: "write_file", Path: "package.json"}, writes); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected 3rd package.json write to be blocked, got %q", msg)
	}
	if msg := perFileWriteChurnBlocked(AgentAction{Action: "write_file", Path: "game.js"}, writes); !strings.Contains(msg, "BLOCKIERT") {
		t.Fatalf("expected 7th game.js write to be blocked, got %q", msg)
	}
	if msg := perFileWriteChurnBlocked(AgentAction{Action: "write_file", Path: "index.html"}, writes); msg != "" {
		t.Fatalf("first index.html write must not be blocked, got %q", msg)
	}
}

func TestResumePromptRecoversOriginalSubstantiveTask(t *testing.T) {
	if !isShortResumePrompt("weiter") || !isShortResumePrompt("Continue") || !isShortResumePrompt("mach da weiter") {
		t.Fatal("expected short resume prompts to be detected")
	}
	events := []UIEvent{
		{Type: "user", Message: "erstelle ein komplettes spielbares flappy birds"},
		{Type: "user", Message: "mach da weiter"},
	}
	if got := findSubstantiveThreadTask(events); got != "erstelle ein komplettes spielbares flappy birds" {
		t.Fatalf("recovered task = %q", got)
	}
}

func TestUnlimitedStepsDefaultAndStepLabelFormatting(t *testing.T) {
	cfg := defaultConfig()
	if cfg.MaxAgentSteps != 0 {
		t.Fatalf("expected default MaxAgentSteps=0 (unlimited with stagnation guard), got %d", cfg.MaxAgentSteps)
	}
	norm := normalizeConfig(cfg)
	if norm.MaxAgentSteps != 0 {
		t.Fatalf("normalizeConfig must preserve MaxAgentSteps=0, got %d", norm.MaxAgentSteps)
	}
	if got := formatAgentStepLabel(cfg, 25, 0); strings.Contains(got, "/0") || strings.Contains(got, "/60") || !strings.Contains(got, "25") {
		t.Fatalf("unexpected unlimited step label: %q", got)
	}
	if got := formatNomaStepBadge(25, 0, "Reparatur"); got != "NOMA × MICA × AgentOS · Schritt 25 · Reparatur" {
		t.Fatalf("unexpected unlimited noma badge: %q", got)
	}
}
