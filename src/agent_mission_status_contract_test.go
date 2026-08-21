// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

func TestDesktopMissionStatusAssetsExposeReadOnlyContract(t *testing.T) {
	i18nLoader, err := os.ReadFile("static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(i18nLoader), "/mission_status.js") {
		t.Fatal("Desktop i18n loader does not load mission_status.js")
	}

	asset, err := os.ReadFile("static/mission_status.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(asset)
	for _, required := range []string{
		"mission-status-card",
		"mission.scheduler",
		"scheduler.tasks",
		"resource_class",
		"mission.budget",
		"orchestration-diagnostics-card",
		"state.status?.orchestration",
		"waiting_for_model_inference",
		"resource.saturated",
		"resource.at_capacity",
		"dictionaries.de",
		"dictionaries.en",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("mission/diagnostics status UI missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"/api/chat",
		"/api/approve",
		"/api/project-action",
		"/api/terminal-command",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("read-only mission/diagnostics UI unexpectedly references mutating endpoint %q", forbidden)
		}
	}
}
