// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"sync"
)

type AgentResourceClass string

const (
	AgentResourceModelInference AgentResourceClass = "model-inference"
	AgentResourceReadCPU        AgentResourceClass = "read-cpu"
	AgentResourceBuild          AgentResourceClass = "build"
	AgentResourceIntegration    AgentResourceClass = "exclusive-integration"
)

type AgentResourceLimits struct {
	MaxQueued            int `json:"max_queued"`
	ModelInference       int `json:"model_inference"`
	ReadCPU              int `json:"read_cpu"`
	Build                int `json:"build"`
	ExclusiveIntegration int `json:"exclusive_integration"`
}

type AgentResourceSnapshot struct {
	Class     AgentResourceClass `json:"class"`
	Limit     int                `json:"limit"`
	InUse     int                `json:"in_use"`
	Available int                `json:"available"`
}

type AgentBudgetSnapshot struct {
	Limit       AgentBudget `json:"limit"`
	Usage       AgentUsage  `json:"usage"`
	Remaining   AgentBudget `json:"remaining"`
	Exhausted   bool        `json:"exhausted"`
	ExhaustedBy string      `json:"exhausted_by,omitempty"`
}

type AgentTaskScheduleSnapshot struct {
	TaskID                 string              `json:"task_id"`
	State                  AgentTaskState      `json:"state"`
	ResourceClass          AgentResourceClass  `json:"resource_class,omitempty"`
	QueuePosition          int                 `json:"queue_position,omitempty"`
	Running                bool                `json:"running"`
	AdmissionBlockedReason string              `json:"admission_blocked_reason,omitempty"`
	Budget                 AgentBudgetSnapshot `json:"budget"`
}

type AgentSchedulerSnapshot struct {
	MissionID string                      `json:"mission_id"`
	Cancelled bool                        `json:"cancelled"`
	Queued    int                         `json:"queued"`
	Running   int                         `json:"running"`
	Resources []AgentResourceSnapshot     `json:"resources"`
	Tasks     []AgentTaskScheduleSnapshot `json:"tasks"`
}

type AgentResourceLease struct {
	TaskID        string             `json:"task_id"`
	ResourceClass AgentResourceClass `json:"resource_class"`
	Context       context.Context    `json:"-"`
	token         uint64
}

type agentQueuedTask struct {
	TaskID        string
	ResourceClass AgentResourceClass
	Sequence      uint64
}

type agentActiveLease struct {
	lease  AgentResourceLease
	cancel context.CancelFunc
}

type AgentScheduler struct {
	mu sync.Mutex

	limits        AgentResourceLimits
	missionCtx    context.Context
	missionCancel context.CancelFunc
	cancelled     bool
	nextSequence  uint64
	queue         []agentQueuedTask
	queued        map[string]struct{}
	active        map[string]agentActiveLease
	inUse         map[AgentResourceClass]int
}

func defaultAgentResourceLimits() AgentResourceLimits {
	return AgentResourceLimits{
		MaxQueued:            256,
		ModelInference:       1,
		ReadCPU:              4,
		Build:                1,
		ExclusiveIntegration: 1,
	}
}

func normalizeAgentResourceLimits(limits AgentResourceLimits) AgentResourceLimits {
	defaults := defaultAgentResourceLimits()
	if limits.MaxQueued <= 0 || limits.MaxQueued > 4096 {
		limits.MaxQueued = defaults.MaxQueued
	}
	if limits.ModelInference <= 0 || limits.ModelInference > 32 {
		limits.ModelInference = defaults.ModelInference
	}
	if limits.ReadCPU <= 0 || limits.ReadCPU > 64 {
		limits.ReadCPU = defaults.ReadCPU
	}
	if limits.Build <= 0 || limits.Build > 16 {
		limits.Build = defaults.Build
	}
	if limits.ExclusiveIntegration <= 0 || limits.ExclusiveIntegration > 1 {
		limits.ExclusiveIntegration = defaults.ExclusiveIntegration
	}
	return limits
}

func (limits AgentResourceLimits) limitFor(class AgentResourceClass) (int, error) {
	switch class {
	case AgentResourceModelInference:
		return limits.ModelInference, nil
	case AgentResourceReadCPU:
		return limits.ReadCPU, nil
	case AgentResourceBuild:
		return limits.Build, nil
	case AgentResourceIntegration:
		return limits.ExclusiveIntegration, nil
	default:
		return 0, fmt.Errorf("unsupported agent resource class %q", class)
	}
}

func NewAgentScheduler(parent context.Context, limits AgentResourceLimits) *AgentScheduler {
	if parent == nil {
		parent = context.Background()
	}
	missionCtx, missionCancel := context.WithCancel(parent)
	return &AgentScheduler{
		limits:        normalizeAgentResourceLimits(limits),
		missionCtx:    missionCtx,
		missionCancel: missionCancel,
		queued:        map[string]struct{}{},
		active:        map[string]agentActiveLease{},
		inUse:         map[AgentResourceClass]int{},
	}
}

func agentTaskByID(graph *AgentTaskGraph, id string) *AgentTask {
	if graph == nil {
		return nil
	}
	for i := range graph.Tasks {
		if graph.Tasks[i].ID == id {
			return &graph.Tasks[i]
		}
	}
	return nil
}

func resourceClassForAgentTask(task AgentTask) AgentResourceClass {
	return AgentResourceModelInference
}

func agentTaskAdmissionReason(task AgentTask) string {
	role, err := normalizeAgentRole(string(task.Role))
	if err != nil {
		return "task role is planning data and has no executable Native runtime yet"
	}
	for _, required := range capabilitiesForAgentRole(role) {
		granted := false
		for _, capability := range task.Capabilities {
			if capability == required {
				granted = true
				break
			}
		}
		if !granted {
			return fmt.Sprintf("missing granted capability %s", required)
		}
	}
	return ""
}

func (s *AgentScheduler) QueueReady(graph *AgentTaskGraph, resourceOverrides map[string]AgentResourceClass) error {
	if s == nil {
		return fmt.Errorf("agent scheduler is nil")
	}
	readyIDs, err := readyAgentTaskIDs(graph)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled || s.missionCtx.Err() != nil {
		return context.Canceled
	}
	s.pruneQueueLocked(graph)

	pending := make([]agentQueuedTask, 0, len(readyIDs))
	for _, id := range readyIDs {
		if _, exists := s.queued[id]; exists {
			continue
		}
		if _, exists := s.active[id]; exists {
			continue
		}
		task := agentTaskByID(graph, id)
		if task == nil {
			continue
		}
		resourceClass := resourceClassForAgentTask(*task)
		if override := resourceOverrides[id]; override != "" {
			resourceClass = override
		}
		if _, err := s.limits.limitFor(resourceClass); err != nil {
			return fmt.Errorf("task %q: %w", id, err)
		}
		pending = append(pending, agentQueuedTask{TaskID: id, ResourceClass: resourceClass})
	}
	if len(s.queue)+len(pending) > s.limits.MaxQueued {
		return fmt.Errorf("agent scheduler queue limit %d exceeded", s.limits.MaxQueued)
	}
	for i := range pending {
		s.nextSequence++
		pending[i].Sequence = s.nextSequence
		s.queue = append(s.queue, pending[i])
		s.queued[pending[i].TaskID] = struct{}{}
	}
	return nil
}

func (s *AgentScheduler) pruneQueueLocked(graph *AgentTaskGraph) {
	if len(s.queue) == 0 {
		return
	}
	kept := s.queue[:0]
	for _, entry := range s.queue {
		task := agentTaskByID(graph, entry.TaskID)
		if task == nil || task.State != AgentTaskReady {
			delete(s.queued, entry.TaskID)
			continue
		}
		kept = append(kept, entry)
	}
	s.queue = kept
}

func (s *AgentScheduler) AdmitNext(graph *AgentTaskGraph) (AgentResourceLease, bool, error) {
	if s == nil {
		return AgentResourceLease{}, false, fmt.Errorf("agent scheduler is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled || s.missionCtx.Err() != nil {
		return AgentResourceLease{}, false, context.Canceled
	}
	s.pruneQueueLocked(graph)

	for index, entry := range s.queue {
		task := agentTaskByID(graph, entry.TaskID)
		if task == nil || task.State != AgentTaskReady {
			continue
		}
		if agentTaskAdmissionReason(*task) != "" {
			continue
		}
		limit, err := s.limits.limitFor(entry.ResourceClass)
		if err != nil {
			return AgentResourceLease{}, false, err
		}
		if s.inUse[entry.ResourceClass] >= limit {
			continue
		}
		if err := transitionAgentTask(graph, entry.TaskID, AgentTaskRunning); err != nil {
			return AgentResourceLease{}, false, err
		}

		s.nextSequence++
		taskCtx, cancel := context.WithCancel(s.missionCtx)
		lease := AgentResourceLease{
			TaskID:        entry.TaskID,
			ResourceClass: entry.ResourceClass,
			Context:       taskCtx,
			token:         s.nextSequence,
		}
		s.active[entry.TaskID] = agentActiveLease{lease: lease, cancel: cancel}
		s.inUse[entry.ResourceClass]++
		delete(s.queued, entry.TaskID)
		s.queue = append(s.queue[:index], s.queue[index+1:]...)
		return lease, true, nil
	}
	return AgentResourceLease{}, false, nil
}

func (s *AgentScheduler) Release(graph *AgentTaskGraph, lease AgentResourceLease, next AgentTaskState) error {
	if s == nil {
		return fmt.Errorf("agent scheduler is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	active, exists := s.active[lease.TaskID]
	if !exists || active.lease.token != lease.token || active.lease.ResourceClass != lease.ResourceClass {
		return fmt.Errorf("task %q does not hold this scheduler lease", lease.TaskID)
	}
	if err := transitionAgentTask(graph, lease.TaskID, next); err != nil {
		return err
	}
	active.cancel()
	delete(s.active, lease.TaskID)
	if s.inUse[lease.ResourceClass] > 0 {
		s.inUse[lease.ResourceClass]--
	}
	return nil
}

func (s *AgentScheduler) CancelTask(graph *AgentTaskGraph, taskID string) error {
	if s == nil {
		return fmt.Errorf("agent scheduler is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if active, exists := s.active[taskID]; exists {
		active.cancel()
		delete(s.active, taskID)
		if s.inUse[active.lease.ResourceClass] > 0 {
			s.inUse[active.lease.ResourceClass]--
		}
	}
	if _, exists := s.queued[taskID]; exists {
		delete(s.queued, taskID)
		kept := s.queue[:0]
		for _, entry := range s.queue {
			if entry.TaskID != taskID {
				kept = append(kept, entry)
			}
		}
		s.queue = kept
	}
	task := agentTaskByID(graph, taskID)
	if task == nil {
		return fmt.Errorf("task %q not found", taskID)
	}
	if task.State == AgentTaskCancelled || isSuccessfulAgentTaskState(task.State) {
		return nil
	}
	if !agentTaskTransitionAllowed(task.State, AgentTaskCancelled) {
		return fmt.Errorf("task %q cannot be cancelled from state %q", taskID, task.State)
	}
	return transitionAgentTask(graph, taskID, AgentTaskCancelled)
}

func (s *AgentScheduler) CancelMission(graph *AgentTaskGraph) error {
	if s == nil {
		return fmt.Errorf("agent scheduler is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled {
		return nil
	}
	s.cancelled = true
	s.missionCancel()

	cancelIDs := make(map[string]struct{}, len(s.queue)+len(s.active))
	for _, entry := range s.queue {
		cancelIDs[entry.TaskID] = struct{}{}
	}
	for taskID, active := range s.active {
		active.cancel()
		cancelIDs[taskID] = struct{}{}
	}
	s.queue = nil
	s.queued = map[string]struct{}{}
	s.active = map[string]agentActiveLease{}
	s.inUse = map[AgentResourceClass]int{}

	for i := range graph.Tasks {
		task := &graph.Tasks[i]
		if _, shouldCancel := cancelIDs[task.ID]; !shouldCancel {
			continue
		}
		if task.State == AgentTaskCancelled || isSuccessfulAgentTaskState(task.State) {
			continue
		}
		if agentTaskTransitionAllowed(task.State, AgentTaskCancelled) {
			if err := transitionAgentTask(graph, task.ID, AgentTaskCancelled); err != nil {
				return err
			}
		}
	}
	return nil
}

func agentBudgetSnapshot(budget AgentBudget, usage AgentUsage) AgentBudgetSnapshot {
	remaining := AgentBudget{
		ModelCalls:           remainingIntBudget(budget.ModelCalls, usage.ModelCalls),
		ToolCalls:            remainingIntBudget(budget.ToolCalls, usage.ToolCalls),
		EstimatedTokenBudget: remainingInt64Budget(budget.EstimatedTokenBudget, usage.EstimatedTokens),
		TimeSeconds:          remainingTimeBudgetSeconds(budget.TimeSeconds, usage.ElapsedMillis),
	}
	snapshot := AgentBudgetSnapshot{Limit: budget, Usage: usage, Remaining: remaining}
	switch {
	case budget.ModelCalls > 0 && usage.ModelCalls >= budget.ModelCalls:
		snapshot.Exhausted, snapshot.ExhaustedBy = true, "model_calls"
	case budget.ToolCalls > 0 && usage.ToolCalls >= budget.ToolCalls:
		snapshot.Exhausted, snapshot.ExhaustedBy = true, "tool_calls"
	case budget.EstimatedTokenBudget > 0 && usage.EstimatedTokens >= budget.EstimatedTokenBudget:
		snapshot.Exhausted, snapshot.ExhaustedBy = true, "estimated_tokens"
	case budget.TimeSeconds > 0 && usage.ElapsedMillis >= int64(budget.TimeSeconds)*1000:
		snapshot.Exhausted, snapshot.ExhaustedBy = true, "time"
	}
	return snapshot
}

func remainingIntBudget(limit, used int) int {
	if limit <= 0 {
		return 0
	}
	remaining := limit - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func remainingInt64Budget(limit, used int64) int64 {
	if limit <= 0 {
		return 0
	}
	remaining := limit - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func remainingTimeBudgetSeconds(limitSeconds int, elapsedMillis int64) int {
	if limitSeconds <= 0 {
		return 0
	}
	remainingMillis := int64(limitSeconds)*1000 - elapsedMillis
	if remainingMillis <= 0 {
		return 0
	}
	return int((remainingMillis + 999) / 1000)
}

func agentBudgetHardStop(budget AgentBudget, usage AgentUsage) (AgentResult, bool) {
	snapshot := agentBudgetSnapshot(budget, usage)
	if !snapshot.Exhausted {
		return AgentResult{}, false
	}
	return AgentResult{
		Status:  AgentResultBudgetExhausted,
		Summary: fmt.Sprintf("Agent budget exhausted: %s", snapshot.ExhaustedBy),
		Usage:   usage,
	}, true
}

func (s *AgentScheduler) Snapshot(graph *AgentTaskGraph, usageByTask map[string]AgentUsage) AgentSchedulerSnapshot {
	if s == nil {
		return AgentSchedulerSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneQueueLocked(graph)

	snapshot := AgentSchedulerSnapshot{
		Cancelled: s.cancelled || s.missionCtx.Err() != nil,
		Queued:    len(s.queue),
		Running:   len(s.active),
	}
	if graph != nil {
		snapshot.MissionID = graph.MissionID
	}
	classes := []AgentResourceClass{AgentResourceModelInference, AgentResourceReadCPU, AgentResourceBuild, AgentResourceIntegration}
	for _, class := range classes {
		limit, _ := s.limits.limitFor(class)
		inUse := s.inUse[class]
		available := limit - inUse
		if available < 0 {
			available = 0
		}
		snapshot.Resources = append(snapshot.Resources, AgentResourceSnapshot{Class: class, Limit: limit, InUse: inUse, Available: available})
	}

	queuePosition := make(map[string]int, len(s.queue))
	queueResource := make(map[string]AgentResourceClass, len(s.queue))
	for index, entry := range s.queue {
		queuePosition[entry.TaskID] = index + 1
		queueResource[entry.TaskID] = entry.ResourceClass
	}
	for _, task := range graph.Tasks {
		taskSnapshot := AgentTaskScheduleSnapshot{
			TaskID:        task.ID,
			State:         task.State,
			QueuePosition: queuePosition[task.ID],
			Budget:        agentBudgetSnapshot(task.Budget, usageByTask[task.ID]),
		}
		if active, exists := s.active[task.ID]; exists {
			taskSnapshot.ResourceClass = active.lease.ResourceClass
			taskSnapshot.Running = true
		} else if resourceClass := queueResource[task.ID]; resourceClass != "" {
			taskSnapshot.ResourceClass = resourceClass
			taskSnapshot.AdmissionBlockedReason = agentTaskAdmissionReason(task)
		}
		snapshot.Tasks = append(snapshot.Tasks, taskSnapshot)
	}
	return snapshot
}
