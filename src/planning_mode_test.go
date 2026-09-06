// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSystemPromptContainsPlanningModeInstructions(t *testing.T) {
	for _, marker := range []string{
		"implementation_plan.md",
		"Plan genehmigen & ausführen (Proceed)",
		"walkthrough.md",
		"subagent_analyze",
	} {
		if !strings.Contains(agentSystemPrompt, marker) {
			t.Fatalf("agentSystemPrompt missing planning mode instruction: %q", marker)
		}
	}
}

func TestExtensionAndWebLocalizationContainsPlanApprovalKeys(t *testing.T) {
	deExtData, err := os.ReadFile(filepath.Join("..", "extensions", "localcode", "media", "de.json"))
	if err != nil {
		t.Fatal(err)
	}
	enExtData, err := os.ReadFile(filepath.Join("..", "extensions", "localcode", "media", "en.json"))
	if err != nil {
		t.Fatal(err)
	}

	var deExt, enExt map[string]string
	if err := json.Unmarshal(deExtData, &deExt); err != nil {
		t.Fatalf("unmarshal de.json: %v", err)
	}
	if err := json.Unmarshal(enExtData, &enExt); err != nil {
		t.Fatalf("unmarshal en.json: %v", err)
	}

	requiredExtKeys := []string{"proceedPlan", "proceedMessage", "openPlan", "planCard"}
	for _, key := range requiredExtKeys {
		if _, ok := deExt[key]; !ok {
			t.Fatalf("de.json missing extension key: %s", key)
		}
		if _, ok := enExt[key]; !ok {
			t.Fatalf("en.json missing extension key: %s", key)
		}
	}

	// Verify static ui_polish.js contains the web keys
	polishData, err := fs.ReadFile(staticFS, "static/ui_polish.js")
	if err != nil {
		t.Fatal(err)
	}
	polishText := string(polishData)

	for _, key := range []string{
		"Plan genehmigen & ausführen",
		"Plan im Editor öffnen",
		"Implementierungsplan",
		"Genehmigt, bitte gemäß Plan ausführen.",
	} {
		if !strings.Contains(polishText, key) {
			t.Fatalf("ui_polish.js missing key: %s", key)
		}
	}
}

func TestDesktopHTMLContainsPlanCardAndProceedHandlers(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	for _, fragment := range []string{
		"proceed-plan-btn",
		"open-plan-btn",
		"implementation_plan.md",
		"Plan genehmigen & ausführen",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("index.html missing plan fragment: %q", fragment)
		}
	}
}
