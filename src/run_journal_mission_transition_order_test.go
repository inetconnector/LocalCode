// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestMissionRecoveryTransitionPlanPropagatesDependencyBlocksIndependentOfTaskOrder(t *testing.T) {
	now := time.Now()

	// Persisted order is deliberately reverse-topological: grandchild is seen
	// before middle, while middle later becomes blocked by foundation.
	grandchild := recoveryTransitionTask("grandchild", AgentTaskPending, "middle")
	middle := recoveryTransitionTask("middle", AgentTaskSucceeded, "foundation")
	middle.CompletionEvidence = verifiedRecoveryTransitionEvidence(t, now.Add(-time.Minute))
	foundation := recoveryTransitionTask("foundation", AgentTaskSucceeded)
	foundation.CompletionEvidence = missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, now.Add(-time.Minute))

	plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(grandchild, middle, foundation), now)
	if !plan.Valid {
		t.Fatalf("valid reverse-ordered DAG rejected: %#v", plan)
	}
	if plan.Tasks[2].Action != missionRecoveryTransitionVerifyPostconditions {
		t.Fatalf("unverified foundation should require verification: %#v", plan.Tasks[2])
	}
	if plan.Tasks[1].Action != missionRecoveryTransitionBlockedDependency || len(plan.Tasks[1].BlockedBy) != 1 || plan.Tasks[1].BlockedBy[0] != "foundation" {
		t.Fatalf("middle task not blocked by unverified foundation: %#v", plan.Tasks[1])
	}
	if plan.Tasks[0].Action != missionRecoveryTransitionBlockedDependency || len(plan.Tasks[0].BlockedBy) != 1 || plan.Tasks[0].BlockedBy[0] != "middle" {
		t.Fatalf("transitive dependency block did not reach earlier grandchild: %#v", plan.Tasks[0])
	}
	if plan.ReservedNewAttempts != 0 {
		t.Fatalf("transitively blocked graph reserved new attempts: %#v", plan)
	}
}
