// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunJournalRedactsSecretsAndRecoversInterruptedRun(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	project := t.TempDir()
	state := RunRecoveryState{
		RunID:     "run-123",
		ThreadID:  "thread-1",
		Project:   project,
		Model:     "qwen-test",
		Task:      sanitizeRunJournalText("fix auth token=super-secret-value", 6000),
		Phase:     "executing:write_file",
		StartedAt: time.Now().Add(-time.Minute),
		Events: []RunJournalEvent{{
			At:      time.Now(),
			Type:    "agent_step",
			Action:  "write_file",
			Path:    "auth.go",
			Message: sanitizeRunJournalText("Authorization: Bearer abcdefghijklmnop", 1200),
		}},
	}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "super-secret-value") || strings.Contains(text, "abcdefghijklmnop") {
		t.Fatalf("journal leaked secret material:\n%s", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("journal did not mark redaction:\n%s", text)
	}
	recovered := loadRecoverableRun()
	if recovered == nil || recovered.RunID != "run-123" || recovered.Terminal {
		t.Fatalf("unexpected recovered state: %#v", recovered)
	}
}

func TestTerminalRunJournalIsNotRecoverable(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	state := RunRecoveryState{RunID: "done", Project: project, Phase: "idle", StartedAt: time.Now(), Terminal: true, Outcome: "done"}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}
	if recovered := loadRecoverableRun(); recovered != nil {
		t.Fatalf("terminal journal was offered for recovery: %#v", recovered)
	}
}

func TestRecoveryContextRequiresSameTaskOrExplicitResume(t *testing.T) {
	project := t.TempDir()
	state := &AppState{Recovery: &RunRecoveryState{
		RunID:     "crashed",
		Project:   project,
		Task:      "refactor parser",
		Phase:     "verifying",
		StartedAt: time.Now().Add(-time.Minute),
		Events: []RunJournalEvent{{Type: "tool_result", Action: "read_file", Path: "parser.go", Message: "Parser read"}},
	}}
	if context, task := state.recoveryContextForTask(project, "unrelated new task"); context != "" || task != "" {
		t.Fatalf("unrelated task inherited recovery context: %q %q", context, task)
	}
	context, task := state.recoveryContextForTask(project, "Weiter")
	if task != "refactor parser" || !strings.Contains(context, "Do NOT replay a mutating action") || !strings.Contains(context, "parser.go") {
		t.Fatalf("resume context incomplete:\n%s\nTask=%q", context, task)
	}
}

func TestRunJournalAtomicRewriteLeavesNoTempFiles(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	for i := 0; i < 12; i++ {
		state := RunRecoveryState{RunID: "active", Project: project, Phase: "step", StartedAt: time.Now()}
		if err := writeRunJournal(state); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(runJournalPath()), ".localcode-write-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("journal temp files leaked: %#v", matches)
	}
}
