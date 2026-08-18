// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestRunJournalPersistsOnlyRecoveryRelevantUIEvents(t *testing.T) {
	for _, eventType := range []string{"approval_required", "approval", "action_running", "action_done", "tool_error", "error", "warning", "final", "verification", "recovery"} {
		if !shouldJournalRunEvent(UIEvent{Type: eventType}) {
			t.Fatalf("recovery-relevant event %q was not journaled", eventType)
		}
	}
	for _, eventType := range []string{"user", "agent_step", "tool_result", "status", "debug"} {
		if shouldJournalRunEvent(UIEvent{Type: eventType}) {
			t.Fatalf("redundant/free-text event %q should not force a durable journal write", eventType)
		}
	}
}
