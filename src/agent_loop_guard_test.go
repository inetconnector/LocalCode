// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestAgentActionFingerprintIgnoresMessageButIncludesPayload(t *testing.T) {
	base := AgentAction{Action: "write_file", Message: "first explanation", Path: "main.go", Content: "package main\n"}
	reworded := base
	reworded.Message = "different explanation"
	if agentActionFingerprint(base) != agentActionFingerprint(reworded) {
		t.Fatal("human explanation text must not change the structured action fingerprint")
	}
	changed := base
	changed.Content = "package other\n"
	if agentActionFingerprint(base) == agentActionFingerprint(changed) {
		t.Fatal("different edit payload must change the structured action fingerprint")
	}
}

func TestAgentLoopGuardBlocksThirdRepeatedFailureAcrossInterveningAction(t *testing.T) {
	guard := newAgentLoopGuard()
	failing := AgentAction{Action: "run_tool", Tool: "go", Args: []string{"test", "./..."}}
	other := AgentAction{Action: "read_file", Path: "go.mod"}

	guard.Observe(failing, "ERROR: exit status 1", true, "fix tests")
	guard.Observe(other, "module example", false, "fix tests")
	guard.Observe(failing, "ERROR: exit status 1", true, "fix tests")

	if got := guard.ShouldBlock(failing); got != agentLoopBlockRepeatedFailure {
		t.Fatalf("block reason=%v want repeated failure", got)
	}
}

func TestAgentLoopGuardBlocksThirdSameOutcomeAcrossTurns(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	other := AgentAction{Action: "search_text", Query: "TODO"}

	guard.Observe(read, "same content", false, "inspect project")
	guard.Observe(other, "No matches.", false, "inspect project")
	guard.Observe(read, "same content", false, "inspect project")

	if got := guard.ShouldBlock(read); got != agentLoopBlockRepeatedOutcome {
		t.Fatalf("block reason=%v want repeated outcome", got)
	}
}

func TestAgentLoopGuardChangedOutcomeIsNewEvidence(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	guard.Observe(read, "version one", false, "inspect project")
	guard.Observe(read, "version two", false, "inspect project")
	if got := guard.ShouldBlock(read); got != agentLoopBlockNone {
		t.Fatalf("changed output must reset stagnation, got %v", got)
	}
	if len(guard.history) != 1 {
		t.Fatalf("changed output should reset stale history before recording new evidence: %+v", guard.history)
	}
}

func TestAgentLoopGuardSuccessfulMutationResetsHistory(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	guard.Observe(read, "same", false, "edit project")
	guard.Observe(read, "same", false, "edit project")

	write := AgentAction{Action: "write_file", Path: "main.go", Content: "changed\n"}
	guard.Observe(write, "--- before\n+++ after\n-old\n+changed", false, "edit project")

	if got := guard.ShouldBlock(read); got != agentLoopBlockNone {
		t.Fatalf("successful project mutation must reset prior stagnation, got %v", got)
	}
	if len(guard.history) != 0 {
		t.Fatalf("successful mutation should clear loop history: %+v", guard.history)
	}
}

func TestAgentLoopGuardSuccessfulVerificationResetsHistory(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	guard.Observe(read, "same", false, "build project")
	guard.Observe(read, "same", false, "build project")
	guard.Observe(AgentAction{Action: "build_project"}, "build passed", false, "build project")
	if got := guard.ShouldBlock(read); got != agentLoopBlockNone {
		t.Fatalf("successful verification must reset prior stagnation, got %v", got)
	}
	if len(guard.history) != 0 {
		t.Fatalf("successful verification should clear loop history: %+v", guard.history)
	}
}

func TestAgentLoopGuardDetectsShortCycles(t *testing.T) {
	cases := []struct {
		name    string
		actions []AgentAction
		results []string
	}{
		{
			name: "period two",
			actions: []AgentAction{{Action: "read_file", Path: "a.go"}, {Action: "search_text", Query: "Thing"}},
			results: []string{"A", "B"},
		},
		{
			name: "period three",
			actions: []AgentAction{{Action: "read_file", Path: "a.go"}, {Action: "read_file", Path: "b.go"}, {Action: "search_text", Query: "Thing"}},
			results: []string{"A", "B", "C"},
		},
		{
			name: "period four",
			actions: []AgentAction{{Action: "read_file", Path: "a.go"}, {Action: "read_file", Path: "b.go"}, {Action: "search_text", Query: "Thing"}, {Action: "list_files", Path: "src", MaxDepth: 2}},
			results: []string{"A", "B", "C", "D"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guard := newAgentLoopGuard()
			for repeat := 0; repeat < 2; repeat++ {
				for i, action := range tc.actions {
					guard.Observe(action, tc.results[i], false, "analyze")
				}
			}
			if got := guard.ShouldBlock(tc.actions[0]); got != agentLoopBlockCycle {
				t.Fatalf("block reason=%v want cycle", got)
			}
		})
	}
}

func TestAgentLoopGuardDoesNotBlockChangedCycleOutcome(t *testing.T) {
	guard := newAgentLoopGuard()
	a := AgentAction{Action: "read_file", Path: "a.go"}
	b := AgentAction{Action: "search_text", Query: "Thing"}
	guard.Observe(a, "A1", false, "analyze")
	guard.Observe(b, "B", false, "analyze")
	guard.Observe(a, "A2", false, "analyze")
	guard.Observe(b, "B", false, "analyze")
	if got := guard.ShouldBlock(a); got != agentLoopBlockNone {
		t.Fatalf("changed evidence must not be classified as a stagnant cycle: %v", got)
	}
}

func TestAgentLoopGuardDifferentWritePayloadDoesNotInheritFailure(t *testing.T) {
	guard := newAgentLoopGuard()
	first := AgentAction{Action: "write_file", Path: "main.go", Content: "one\n"}
	second := AgentAction{Action: "write_file", Path: "main.go", Content: "two\n"}
	guard.Observe(first, "ERROR: blocked", true, "edit")
	guard.Observe(first, "ERROR: blocked", true, "edit")
	if got := guard.ShouldBlock(second); got != agentLoopBlockNone {
		t.Fatalf("different edit payload must not inherit prior loop history: %v", got)
	}
}

func TestAgentLoopGuardIgnoresControlActions(t *testing.T) {
	guard := newAgentLoopGuard()
	finish := AgentAction{Action: "finish", Message: "done"}
	question := AgentAction{Action: "ask_user", Message: "continue?"}
	for i := 0; i < 4; i++ {
		guard.Observe(finish, "done", false, "task")
		guard.Observe(question, "continue?", false, "task")
	}
	if got := guard.ShouldBlock(finish); got != agentLoopBlockNone {
		t.Fatalf("finish must never be blocked: %v", got)
	}
	if got := guard.ShouldBlock(question); got != agentLoopBlockNone {
		t.Fatalf("ask_user must never be blocked: %v", got)
	}
	if len(guard.history) != 0 {
		t.Fatalf("control actions must not pollute loop history: %+v", guard.history)
	}
}

func TestAgentLoopBlockMessagesHaveEnglishAndGermanVariants(t *testing.T) {
	action := AgentAction{Action: "read_file", Path: "main.go"}
	de := defaultConfig()
	de.Language = "de"
	en := de
	en.Language = "en"
	deText := agentLoopBlockDetail(de, agentLoopBlockRepeatedOutcome, action)
	enText := agentLoopBlockDetail(en, agentLoopBlockRepeatedOutcome, action)
	if deText == enText {
		t.Fatalf("localized loop details should differ: %q", deText)
	}
	if agentLoopBlockHint(de, 1) == agentLoopBlockHint(en, 1) {
		t.Fatal("localized loop hints should differ")
	}
}
