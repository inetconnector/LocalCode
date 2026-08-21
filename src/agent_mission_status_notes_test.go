// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

func TestMissionDesktopStatusDocumentsRecoveryBoundary(t *testing.T) {
	data, err := os.ReadFile("agent_mission_status_doc.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"ephemeral", "does not grant capabilities", "run_journal.go"} {
		if !strings.Contains(text, required) {
			t.Fatalf("mission status boundary documentation missing %q", required)
		}
	}
}
