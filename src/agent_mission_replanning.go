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
	replanCycles  map[string]int      // originTaskID -> count
	failureHashes map[string][]string // originTaskID -> list of symptom hashes
	taskOrigin    map[string]string   // generated repair taskID -> originTaskID
	originDeps    map[string][]string // originTaskID -> original dependencies
	latestChain   map[string][]string // originTaskID -> [builderID, testID, reviewID]
}

func NewMissionReplanner(factory *AgentFactory) *MissionReplanner {
	if factory == nil {
		factory = NewAgentFactory()
	}
	return &MissionReplanner{
		factory:       factory,
		replanCycles:  make(map[string]int),
		failureHashes: make(map[string][]string),
		taskOrigin:    make(map[string]string),
		originDeps:    make(map[string][]string),
		latestChain:   make(map[string][]string),
	}
}

func (r *MissionReplanner) originForLocked(taskID string) string {
	if r.taskOrigin != nil {
		if origin, ok := r.taskOrigin[taskID]; ok && origin != "" {
			return origin
		}
	}
	return taskID
}

func buildReplanTaskID(prefix, originID string, cycle int, now int64) string {
	base := strings.TrimSpace(originID)
	if len(base) > 36 {
		base = base[:36]
	}
	base = strings.TrimRight(base, "-._")
	if base == "" {
		base = "task"
	}
	suffix := fmt.Sprintf("c%d-%d", cycle, now%1_000_000_000)
	id := fmt.Sprintf("%s-%s-%s", prefix, base, suffix)
	if len(id) > maxAgentTaskIDLength {
		id = id[:maxAgentTaskIDLength]
		id = strings.TrimRight(id, "-._")
	}
	return id
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

	// 2. Resolve root lineage and check replan cycle count (max 3 per origin task)
	originID := r.originForLocked(failedTaskID)
	cycles := r.replanCycles[originID] + 1
	if cycles > maxTaskReplanCycles {
		return nil, nil, fmt.Errorf("%w: task %s reached %d repair attempts", errReplanningMaxAttemptsExceeded, originID, cycles-1)
	}

	// 3. Stagnation detection via symptom hashing
	symptomHash := hashRepairSymptoms(reason, repair)
	history := r.failureHashes[originID]
	consecutiveSame := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] == symptomHash {
			consecutiveSame++
		} else {
			break
		}
	}
	if consecutiveSame >= 2 { // 3rd identical consecutive failure
		return nil, nil, fmt.Errorf("%w: identical failure fingerprint %s observed %d times for task %s", errReplanningStagnationDetected, symptomHash, consecutiveSame+1, originID)
	}

	// 4. Check total task count limit (max 32 tasks per mission)
	newTasksCount := 3 // Builder -> TestAgent -> Reviewer
	if len(currentGraph.Tasks)+newTasksCount > maxMissionTasksLimit {
		return nil, nil, fmt.Errorf("%w: adding %d tasks would exceed limit %d (current: %d)", errReplanningMaxTasksExceeded, newTasksCount, maxMissionTasksLimit, len(currentGraph.Tasks))
	}

	// 5. Construct Repair Pipeline Tasks
	workspace := failedTask.Workspace
	missionID := currentGraph.MissionID
	model := failedTask.Model
	now := time.Now().UnixNano()

	if r.originDeps == nil {
		r.originDeps = make(map[string][]string)
	}
	if _, recorded := r.originDeps[originID]; !recorded {
		r.originDeps[originID] = append([]string(nil), failedTask.Dependencies...)
	}
	baseDeps := append([]string(nil), r.originDeps[originID]...)

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
	repairBuilder.ID = buildReplanTaskID("repair-builder", originID, cycles, now)
	repairBuilder.Dependencies = baseDeps
	repairBuilder.Model = model

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
	testAgent.ID = buildReplanTaskID("repair-test", originID, cycles, now)
	testAgent.Dependencies = []string{repairBuilder.ID}
	testAgent.Model = model

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
	reviewer.ID = buildReplanTaskID("repair-review", originID, cycles, now)
	reviewer.Dependencies = []string{testAgent.ID}
	reviewer.Model = model

	// 6. Build new task graph
	newGraph := AgentTaskGraph{
		MissionID: currentGraph.MissionID,
		Tasks:     make([]AgentTask, 0, len(currentGraph.Tasks)+3),
	}

	// Mark failed task state
	currentGraph.Tasks[failedIndex].State = AgentTaskFailed
	currentGraph.Tasks[failedIndex].StateReason = fmt.Sprintf("Failed with replanning cycle %d: %s", cycles, reason)

	// Collect IDs whose downstream dependents should be rerouted to the new reviewer.
	rerouteTargets := map[string]bool{failedTaskID: true}
	supersededChainIDs := map[string]bool{}
	if prevChain := r.latestChain[originID]; len(prevChain) > 0 && failedTaskID != originID {
		for _, prevID := range prevChain {
			rerouteTargets[prevID] = true
			if prevID != failedTaskID {
				supersededChainIDs[prevID] = true
			}
		}
	}

	for _, existing := range currentGraph.Tasks {
		copied := existing
		copied.Dependencies = append([]string(nil), existing.Dependencies...)
		copied.RequestedCapabilities = append([]AgentCapability(nil), existing.RequestedCapabilities...)
		copied.Capabilities = append([]AgentCapability(nil), existing.Capabilities...)
		if supersededChainIDs[copied.ID] && !isSuccessfulAgentTaskState(copied.State) && copied.State != AgentTaskFailed {
			copied.State = AgentTaskCancelled
			copied.StateReason = fmt.Sprintf("Superseded by repair cycle %d", cycles)
		}
		newGraph.Tasks = append(newGraph.Tasks, copied)
	}
	newGraph.Tasks = append(newGraph.Tasks, repairBuilder, testAgent, reviewer)

	// Re-route any downstream tasks that depended on failedTask (or superseded previous repair tail) to depend on reviewer
	for i := range newGraph.Tasks {
		taskID := newGraph.Tasks[i].ID
		if taskID == failedTaskID || supersededChainIDs[taskID] || taskID == repairBuilder.ID || taskID == testAgent.ID || taskID == reviewer.ID {
			continue
		}
		seenDeps := make(map[string]bool, len(newGraph.Tasks[i].Dependencies))
		updatedDeps := make([]string, 0, len(newGraph.Tasks[i].Dependencies))
		for _, dep := range newGraph.Tasks[i].Dependencies {
			nextDep := dep
			if rerouteTargets[dep] {
				nextDep = reviewer.ID
			}
			if !seenDeps[nextDep] {
				seenDeps[nextDep] = true
				updatedDeps = append(updatedDeps, nextDep)
			}
		}
		newGraph.Tasks[i].Dependencies = updatedDeps
	}

	// 7. Check DAG Depth (max 5)
	depth := ComputeDAGDepth(newGraph.Tasks)
	if depth > maxDAGDepthLimit {
		return nil, nil, fmt.Errorf("%w: resulting DAG depth %d exceeds limit %d", errReplanningMaxDepthExceeded, depth, maxDAGDepthLimit)
	}

	// 8. Record replan state and lineage
	if r.taskOrigin == nil {
		r.taskOrigin = make(map[string]string)
	}
	if r.latestChain == nil {
		r.latestChain = make(map[string][]string)
	}
	r.replanCycles[originID] = cycles
	r.failureHashes[originID] = append(r.failureHashes[originID], symptomHash)
	r.taskOrigin[repairBuilder.ID] = originID
	r.taskOrigin[testAgent.ID] = originID
	r.taskOrigin[reviewer.ID] = originID
	r.latestChain[originID] = []string{repairBuilder.ID, testAgent.ID, reviewer.ID}

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
