// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	errReplanningMaxTasksExceeded    = errors.New("mission replanning exceeded maximum allowed tasks (32)")
	errReplanningMaxAttemptsExceeded = errors.New("mission replanning exceeded maximum repair cycles per task (3)")
	errReplanningMaxDepthExceeded    = errors.New("mission replanning exceeded maximum DAG depth (5)")
	errReplanningStagnationDetected  = errors.New("mission replanning halted due to stagnation (repeated identical failure symptoms)")
	errFailedTaskNotFound            = errors.New("failed task not found in mission DAG")
)

const (
	maxMissionTasksLimit = 32
	maxTaskReplanCycles  = 3
	maxDAGDepthLimit     = 5
)

type ReplanRecord struct {
	OriginalTaskID string          `json:"original_task_id"`
	CycleNumber    int             `json:"cycle_number"`
	FailureReason  string          `json:"failure_reason"`
	SymptomsHash   string          `json:"symptoms_hash"`
	RepairProposal *RepairProposal `json:"repair_proposal,omitempty"`
	GeneratedTasks []string        `json:"generated_tasks"`
	CreatedAt      time.Time       `json:"created_at"`
}

type MissionReplanner struct {
	mu            sync.RWMutex
	factory       *AgentFactory
	replanCycles  map[string]int      // taskID -> count
	failureHashes map[string][]string // taskID -> list of symptom hashes
}

func NewMissionReplanner(factory *AgentFactory) *MissionReplanner {
	if factory == nil {
		factory = NewAgentFactory()
	}
	return &MissionReplanner{
		factory:       factory,
		replanCycles:  make(map[string]int),
		failureHashes: make(map[string][]string),
	}
}

func hashRepairSymptoms(reason string, repair *RepairProposal) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(reason)))
	if repair != nil {
		h.Write([]byte(strings.TrimSpace(repair.Summary)))
		for _, t := range repair.FailingTests {
			h.Write([]byte(strings.TrimSpace(t)))
		}
		for _, p := range repair.AffectedPaths {
			h.Write([]byte(strings.TrimSpace(p)))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ComputeDAGDepth returns the maximum topological depth of the task graph.
func ComputeDAGDepth(tasks []AgentTask) int {
	if len(tasks) == 0 {
		return 0
	}
	taskMap := make(map[string]AgentTask, len(tasks))
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	depthMap := make(map[string]int)
	var getDepth func(id string, visited map[string]bool) int
	getDepth = func(id string, visited map[string]bool) int {
		if visited[id] {
			return 999 // Cycle detected
		}
		if d, ok := depthMap[id]; ok {
			return d
		}
		task, ok := taskMap[id]
		if !ok || len(task.Dependencies) == 0 {
			depthMap[id] = 1
			return 1
		}
		visited[id] = true
		maxDep := 0
		for _, dep := range task.Dependencies {
			d := getDepth(dep, visited)
			if d > maxDep {
				maxDep = d
			}
		}
		delete(visited, id)
		depthMap[id] = maxDep + 1
		return maxDep + 1
	}

	maxOverall := 1
	for _, t := range tasks {
		d := getDepth(t.ID, make(map[string]bool))
		if d > maxOverall {
			maxOverall = d
		}
	}
	return maxOverall
}

// ReplanFailedTask generates a bounded repair sub-DAG for a failed task and updates the mission task graph.
func (r *MissionReplanner) ReplanFailedTask(
	ctx context.Context,
	currentGraph *AgentTaskGraph,
	failedTaskID string,
	reason string,
	repair *RepairProposal,
	cfg Config,
) (*AgentTaskGraph, *ReplanRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if currentGraph == nil {
		return nil, nil, errors.New("current task graph is nil")
	}

	// 1. Find failed task
	var failedTask *AgentTask
	var failedIndex int
	for i, t := range currentGraph.Tasks {
		if t.ID == failedTaskID {
			failedTask = &currentGraph.Tasks[i]
			failedIndex = i
			break
		}
	}
	if failedTask == nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedTaskNotFound, failedTaskID)
	}

	// 2. Check replan cycle count (max 3 per task)
	cycles := r.replanCycles[failedTaskID] + 1
	if cycles > maxTaskReplanCycles {
		return nil, nil, fmt.Errorf("%w: task %s reached %d repair attempts", errReplanningMaxAttemptsExceeded, failedTaskID, cycles-1)
	}

	// 3. Stagnation detection via symptom hashing
	symptomHash := hashRepairSymptoms(reason, repair)
	history := r.failureHashes[failedTaskID]
	consecutiveSame := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] == symptomHash {
			consecutiveSame++
		} else {
			break
		}
	}
	if consecutiveSame >= 2 { // 3rd identical consecutive failure
		return nil, nil, fmt.Errorf("%w: identical failure fingerprint %s observed %d times for task %s", errReplanningStagnationDetected, symptomHash, consecutiveSame+1, failedTaskID)
	}

	// 4. Check total task count limit (max 32 tasks per mission)
	newTasksCount := 3 // Builder -> TestAgent -> Reviewer
	if len(currentGraph.Tasks)+newTasksCount > maxMissionTasksLimit {
		return nil, nil, fmt.Errorf("%w: adding %d tasks would exceed limit %d (current: %d)", errReplanningMaxTasksExceeded, newTasksCount, maxMissionTasksLimit, len(currentGraph.Tasks))
	}

	// 5. Construct Repair Pipeline Tasks
	workspace := failedTask.Workspace
	missionID := currentGraph.MissionID
	now := time.Now().UnixNano()

	repairBuilderObjective := fmt.Sprintf("Repair defect in task %s: %s", failedTaskID, reason)
	if repair != nil && repair.Summary != "" {
		repairBuilderObjective = fmt.Sprintf("Repair defect in task %s: %s", failedTaskID, repair.Summary)
	}

	repairBuilder, err := r.factory.CreateGovernedTask(
		missionID,
		string(AgentRoleBuilder),
		repairBuilderObjective,
		workspace,
		[]AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP, AgentCapabilityBuilderWorktree},
		cfg,
	)
	if err != nil {
		return nil, nil, err
	}
	repairBuilder.ID = fmt.Sprintf("repair-builder-%s-c%d-%d", failedTaskID, cycles, now)
	repairBuilder.Dependencies = append([]string{}, failedTask.Dependencies...)

	// Test Agent task
	testAgent, err := r.factory.CreateGovernedTask(
		missionID,
		string(AgentRoleTestAgent),
		fmt.Sprintf("Verify repair fix for task %s", failedTaskID),
		workspace,
		[]AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityTesting},
		cfg,
	)
	if err != nil {
		return nil, nil, err
	}
	testAgent.ID = fmt.Sprintf("repair-test-%s-c%d-%d", failedTaskID, cycles, now)
	testAgent.Dependencies = []string{repairBuilder.ID}

	// Reviewer task
	reviewer, err := r.factory.CreateGovernedTask(
		missionID,
		string(AgentRoleReviewer),
		fmt.Sprintf("Review sanitized evidence and acceptance criteria for repaired task %s", failedTaskID),
		workspace,
		[]AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP, AgentCapabilityReview},
		cfg,
	)
	if err != nil {
		return nil, nil, err
	}
	reviewer.ID = fmt.Sprintf("repair-review-%s-c%d-%d", failedTaskID, cycles, now)
	reviewer.Dependencies = []string{testAgent.ID}

	// 6. Build new task graph
	newGraph := AgentTaskGraph{
		MissionID: currentGraph.MissionID,
		Tasks:     make([]AgentTask, 0, len(currentGraph.Tasks)+3),
	}

	// Mark failed task state
	currentGraph.Tasks[failedIndex].State = AgentTaskFailed
	currentGraph.Tasks[failedIndex].StateReason = fmt.Sprintf("Failed with replanning cycle %d: %s", cycles, reason)

	newGraph.Tasks = append(newGraph.Tasks, currentGraph.Tasks...)
	newGraph.Tasks = append(newGraph.Tasks, repairBuilder, testAgent, reviewer)

	// Re-route any downstream tasks that depended on failedTask to depend on reviewer
	for i := range newGraph.Tasks {
		if newGraph.Tasks[i].ID == failedTaskID || newGraph.Tasks[i].ID == repairBuilder.ID || newGraph.Tasks[i].ID == testAgent.ID || newGraph.Tasks[i].ID == reviewer.ID {
			continue
		}
		for j, dep := range newGraph.Tasks[i].Dependencies {
			if dep == failedTaskID {
				newGraph.Tasks[i].Dependencies[j] = reviewer.ID
			}
		}
	}

	// 7. Check DAG Depth (max 5)
	depth := ComputeDAGDepth(newGraph.Tasks)
	if depth > maxDAGDepthLimit {
		return nil, nil, fmt.Errorf("%w: resulting DAG depth %d exceeds limit %d", errReplanningMaxDepthExceeded, depth, maxDAGDepthLimit)
	}

	// 8. Record replan state
	r.replanCycles[failedTaskID] = cycles
	r.failureHashes[failedTaskID] = append(r.failureHashes[failedTaskID], symptomHash)

	record := &ReplanRecord{
		OriginalTaskID: failedTaskID,
		CycleNumber:    cycles,
		FailureReason:  reason,
		SymptomsHash:   symptomHash,
		RepairProposal: repair,
		GeneratedTasks: []string{repairBuilder.ID, testAgent.ID, reviewer.ID},
		CreatedAt:      time.Now(),
	}

	return &newGraph, record, nil
}
