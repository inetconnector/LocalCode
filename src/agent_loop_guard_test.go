// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestAgentActionFingerprintIncludesEditPayload(t *testing.T) {
	a := AgentAction{Action: "write_file", Path: "main.go", Content: "package main\n"}
	b := AgentAction{Action: "write_file", Path: "main.go", Content: "package other\n"}
	if actionSignature(a) != actionSignature(b) {
		t.Fatal("baseline actionSignature unexpectedly includes edit payload; regression premise changed")
	}
	if agentActionFingerprint(a) == agentActionFingerprint(b) {
		t.Fatal("complete action fingerprint must distinguish different write payloads")
	}
}

func TestAgentLoopGuardBlocksThirdRepeatedFailureAcrossInterveningActions(t *testing.T) {
	guard := newAgentLoopGuard()
	failing := AgentAction{Action: "run_tool", Tool: "go", Args: []string{"test", "./..."}}
	read := AgentAction{Action: "read_file", Path: "go.mod"}

	guard.Observe(failing, "ERROR: exit status 1", true, "fix tests")
	guard.Observe(read, "module example", false, "fix tests")
	guard.Observe(failing, "ERROR: exit status 1", true, "fix tests")

	reason := guard.ShouldBlock(failing)
	if !strings.Contains(reason, "zweimal fehlgeschlagen") {
		t.Fatalf("expected repeated failure block, got %q", reason)
	}
}

func TestAgentLoopGuardBlocksSameOutcomeAcrossTurns(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	search := AgentAction{Action: "search_text", Query: "TODO"}

	guard.Observe(read, "same content", false, "inspect project")
	guard.Observe(search, "No matches.", false, "inspect project")
	guard.Observe(read, "same content", false, "inspect project")

	reason := guard.ShouldBlock(read)
	if !strings.Contains(reason, "dasselbe Ergebnis") {
		t.Fatalf("expected identical-outcome block, got %q", reason)
	}
}

func TestAgentLoopGuardAllowsChangedOutcome(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	guard.Observe(read, "version one", false, "inspect project")
	guard.Observe(read, "version two", false, "inspect project")
	if reason := guard.ShouldBlock(read); reason != "" {
		t.Fatalf("changed tool output is new information and must reset stagnation: %s", reason)
	}
}

func TestAgentLoopGuardSuccessfulMutationResetsHistory(t *testing.T) {
	guard := newAgentLoopGuard()
	read := AgentAction{Action: "read_file", Path: "main.go"}
	guard.Observe(read, "same", false, "edit project")
	guard.Observe(read, "same", false, "edit project")

	write := AgentAction{Action: "write_file", Path: "main.go", Content: "changed\n"}
	guard.Observe(write, "--- before\n+++ after\n-old\n+changed", false, "edit project")

	if reason := guard.ShouldBlock(read); reason != "" {
		t.Fatalf("real mutation must reset prior loop history: %s", reason)
	}
}

func TestAgentLoopGuardDetectsTwoStepCycle(t *testing.T) {
	guard := newAgentLoopGuard()
	a := AgentAction{Action: "read_file", Path: "a.go"}
	b := AgentAction{Action: "search_text", Query: "Thing"}
	guard.Observe(a, "A", false, "analyze")
	guard.Observe(b, "B", false, "analyze")
	guard.Observe(a, "A", false, "analyze")
	guard.Observe(b, "B", false, "analyze")

	reason := guard.ShouldBlock(a)
	if !strings.Contains(reason, "Werkzeugzyklus") {
		t.Fatalf("expected two-step cycle block, got %q", reason)
	}
}

func TestAgentLoopGuardDetectsThreeStepCycle(t *testing.T) {
	guard := newAgentLoopGuard()
	a := AgentAction{Action: "read_file", Path: "a.go"}
	b := AgentAction{Action: "read_file", Path: "b.go"}
	c := AgentAction{Action: "search_text", Query: "Thing"}
	for i := 0; i < 2; i++ {
		guard.Observe(a, "A", false, "analyze")
		guard.Observe(b, "B", false, "analyze")
		guard.Observe(c, "C", false, "analyze")
	}

	if reason := guard.ShouldBlock(a); !strings.Contains(reason, "Werkzeugzyklus") {
		t.Fatalf("expected three-step cycle block, got %q", reason)
	}
}

func TestAgentLoopGuardDetectsFourStepCycle(t *testing.T) {
	guard := newAgentLoopGuard()
	a := AgentAction{Action: "read_file", Path: "a.go"}
	b := AgentAction{Action: "read_file", Path: "b.go"}
	c := AgentAction{Action: "search_text", Query: "Thing"}
	d := AgentAction{Action: "project_tree", Path: "src", MaxDepth: 2}
	for i := 0; i < 2; i++ {
		guard.Observe(a, "A", false, "analyze")
		guard.Observe(b, "B", false, "analyze")
		guard.Observe(c, "C", false, "analyze")
		guard.Observe(d, "D", false, "analyze")
	}

	if reason := guard.ShouldBlock(a); !strings.Contains(reason, "Werkzeugzyklus") {
		t.Fatalf("expected four-step cycle block, got %q", reason)
	}
}

func TestAgentLoopGuardDifferentWritePayloadIsNotSameAction(t *testing.T) {
	guard := newAgentLoopGuard()
	first := AgentAction{Action: "write_file", Path: "main.go", Content: "one\n"}
	second := AgentAction{Action: "write_file", Path: "main.go", Content: "two\n"}
	guard.Observe(first, "ERROR: blocked", true, "edit")
	guard.Observe(first, "ERROR: blocked", true, "edit")
	if reason := guard.ShouldBlock(second); reason != "" {
		t.Fatalf("different edit payload must not inherit a prior action loop: %s", reason)
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
	if reason := guard.ShouldBlock(finish); reason != "" {
		t.Fatalf("finish must never be blocked by the loop guard: %s", reason)
	}
	if reason := guard.ShouldBlock(question); reason != "" {
		t.Fatalf("ask_user must never be blocked by the loop guard: %s", reason)
	}
	if len(guard.history) != 0 {
		t.Fatalf("control actions must not pollute loop history: %+v", guard.history)
	}
}
