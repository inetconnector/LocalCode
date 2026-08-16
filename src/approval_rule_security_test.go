// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"
)

func TestPersistentGitApprovalPatternKeepsCompleteArguments(t *testing.T) {
	action := AgentAction{Action: "git", Args: []string{"branch", "new-feature"}}
	pattern, ok := persistentApprovalPattern(action)
	if !ok {
		t.Fatal("structured Git action should support persistent approval")
	}
	want := []string{"git", "branch", "new-feature"}
	if !reflect.DeepEqual(pattern, want) {
		t.Fatalf("pattern = %#v, want exact %#v", pattern, want)
	}
	if rulePatternMatches(pattern, []string{"git", "branch", "different-feature"}) {
		t.Fatal("approval for one branch mutation must not authorize another")
	}
}

func TestPersistentShellApprovalRemainsExactCommandToken(t *testing.T) {
	action := AgentAction{Action: "run_command", Command: "go test ./... && echo done"}
	pattern, ok := persistentApprovalPattern(action)
	if !ok || len(pattern) != 2 {
		t.Fatalf("unexpected shell pattern: %#v ok=%v", pattern, ok)
	}
	if rulePatternMatches(pattern, []string{"shell", "go test ./... && echo changed"}) {
		t.Fatal("persistent shell approval must not act as a wildcard")
	}
}
