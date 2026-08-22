# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `8e779852b98d83a8315a798c4c319de751fbe344`  
**Last merged functional PR:** #67 `feat: materialize safe mission recovery continuation`  
**Active work:** draft PR #68 `feat: atomically admit mission recovery continuation`, branch `feat/mission-recovery-atomic-admission`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record. Only merged `master` is authoritative product state; PR #68 remains candidate behavior until its exact head passes all gates and is merged.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, context compaction, local memory boundaries and durable normal-run recovery through `run_journal.go`.

Desktop Mission telemetry is bounded and ephemeral. `/api/status` attaches richer Mission payload only to the matching execution-scoped `RunID`; the Output inspector is observation-only and cannot start, authorize or resume work. Orchestration diagnostics never alter Scheduler policy or concurrency.

### Android / Mobile Remote

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or Mission-recovery control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current scheduled Child dispatch is synchronous; higher configured model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Phase 6 recovery foundation merged through PR #67

`run_journal.go` remains the **single durable recovery authority**. A read-only Mission stores bounded structured metadata in the existing `active-run.json`; no second Mission journal exists.

PR #61 added restart reconciliation using bounded canonical project/Git identity evidence. Interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always unknown/non-successful.

PR #62 added immutable successful-task completion evidence at scheduler-authoritative checkpoints. Raw Child/model result text, findings, paths and other free-form result details are not duplicated into recovery evidence.

PR #63 added durable task lifecycle counters/timestamps and typed verification state. `AttemptCount` increments only on a genuine not-running -> running transition; repeated snapshots do not double-count.

PR #64 added deterministic read-only postcondition verification against fresh project/Git state and canonical completion evidence.

PR #65 added the deterministic recovery transition planner. It validates the durable DAG/lifecycle evidence, enforces three attempts/task and 192 attempts/Mission, requires currently verified reusable dependencies, keeps crash-running work `interrupted_review_required`, and emits classification only: `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, terminal/blocking outcomes or fail-closed `invalid_recovery_state`.

PR #66 added the trusted read-only recovery-control boundary. `AppState.MissionRecoveryControlSnapshot(runID)` reloads the exact nonterminal Mission, freshly observes project/Git, transiently re-verifies only required postconditions, recomputes the plan and re-reads the journal. It retries boundedly if the authority-relevant fingerprint changes. The snapshot is read-only observation, not an execution token or Scheduler lease.

PR #67 added `MissionRecoveryContinuationMaterialization`. For one explicit current `resume_candidate` or `retry_candidate` it reconstructs only the candidate plus its transitively `reuse_verified` dependency closure, regenerates executable capabilities from trusted role governance, revalidates requested capabilities/model identity, carries accepted historical Usage, checks Mission budget against durable active time, and requires a stable journal fingerprint. Offline/crash downtime does not consume Mission execution-time budget. The returned object still has no execution authority and never mutates the journal.

The final #67 head was merged to `master` as `8e779852b98d83a8315a798c4c319de751fbe344`.

## 4. Draft PR #68 – atomic continuation admission and execution

PR #68 is the first execution-capable recovery boundary. Its implementation is centered in `src/run_journal_mission_admission.go`, with cumulative Scheduler accounting in `src/agent_scheduler_dispatch.go` and reservation-aware lifecycle bookkeeping in `src/run_journal_mission_lifecycle.go`.

### 4.1 Atomic admission boundary

`AppState.RunMissionRecoveryContinuation(ctx, runID, taskID)` remains an explicit internal control action. The implementation:

- holds the AppState run mutex while recomputing the fresh #67 materialization, so a normal LocalCode run cannot start between recovery validation and reservation;
- never trusts a previously returned #66/#67 object as authorization;
- restores/caps the recovered graph against current configuration and durable budget evidence;
- captures the exact `active-run.json` content version, reloads the exact interrupted Run/Mission, verifies that file version and recomputes the authority-relevant journal fingerprint;
- fails closed if the journal/fingerprint changed;
- rotates to a fresh execution-scoped `RunID` while preserving the stable `MissionID` and records the parent/interrupted RunID in the bounded reservation event;
- writes the reservation through `atomicWriteFileIfVersion` **before any Scheduler exists**;
- only after that durable reservation succeeds sets `AppState.Running=true` and creates the continuation Scheduler.

A stale admission cannot call the Child executor or reserve a Scheduler lease.

### 4.2 Crash-safe attempt reservation

A durable continuation reservation is admission intent, not proof that execution began.

`MissionTaskLifecycle` therefore has bounded `AttemptReserved` / `AttemptReservedAt` evidence. Reservation sets that marker but does **not** increment `AttemptCount`. The first durable Scheduler `Running` checkpoint consumes the marker, increments `AttemptCount` exactly once, updates `RetryCount`, and records `LastStartedAt`.

This distinction is deliberate: if the process crashes after durable reservation but before Scheduler admission, restart sees a non-running task with the same unconsumed reservation. It can rebuild the fresh recovery materialization and rotate to another execution RunID without consuming another retry. Repeated running snapshots still cannot double-count the attempt.

The fixed maximums remain three actual started attempts per task and 192 actual started attempts per Mission.

### 4.3 Historical task and Mission budgets

Recovery must not silently mint a fresh budget.

For an attempted task, the prior `BudgetSnapshot.Limit` is durable evidence of the normalized total Scheduler budget actually governing prior execution. PR #68 restores a conservative total limit from durable task budget plus that snapshot, then caps it again through current canonical role/configuration defaults so recovery can never widen the trusted runtime envelope.

The graph/Scheduler retains this **total** task limit. Historical accepted Usage stays cumulative. Immediately before Child execution, only the detached execution-task copy is capped to the remaining per-task budget. This prevents the erroneous state `remaining limit + cumulative Usage`, which would shrink the same historical usage twice on the next checkpoint.

Mission budget tracking is similarly seeded with historical accepted Model/Tool/Token usage and a synthetic active-time anchor. Prior durable active Mission time remains charged; crash/offline downtime remains excluded; current continuation active time is added normally.

### 4.4 Cumulative Scheduler accounting

`runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded` copies the historical `UsageByTask` map before admission. Accepted new result usage is added to that seed instead of overwriting it. Untouched historical siblings remain in the map. Negative/corrupt seed usage fails before Scheduler admission.

Late cancelled Child results remain non-authoritative because only Scheduler-finalized `Applied` results are accumulated.

### 4.5 Continuation checkpoint/finalization

PR #68 does **not** call the ordinary `finishMissionRunJournal`, because that routine rebuilds `Mission.Tasks` from the supplied graph and is only correct for a whole fresh Mission.

`finishMissionRecoveryContinuation` instead:

- captures and verifies the exact journal file version before modifying it and commits through the same version-bound atomic write path;
- requires the current execution RunID/MissionID;
- merges Scheduler state/completion evidence only for tasks present in the bounded continuation graph;
- preserves all unrelated durable Mission tasks;
- uses cumulative Scheduler Usage for continued tasks and canonical accepted historical Usage for untouched tasks, including older records whose accepted usage is present only in `BudgetSnapshot.Usage`;
- rebuilds Mission-wide accounting across the full durable Mission;
- terminalizes `succeeded/completed` only when **all durable Mission tasks** are successful;
- terminalizes an explicit context cancellation/deadline as Mission `cancelled`, cancelling any remaining unfinished durable tasks;
- otherwise leaves the Mission nonterminal in `mission-read-only` for a later explicit fresh recovery decision.

After the execution returns, AppState is reset only if its RunID still matches that continuation execution, and cached `Recovery` is refreshed from the single durable journal.

### 4.6 Focused PR #68 tests

Current focused tests cover:

- historical Scheduler Usage seeding, accumulation, defensive copy and fail-closed invalid seed;
- restoration of a previously normalized total task budget when the durable task budget is zero;
- detached remaining-budget capping without mutating the total graph budget;
- stale journal fingerprint rejection without RunID/lifecycle mutation;
- RunID rotation plus explicit parent-run lineage;
- crash after durable reservation but before Scheduler admission, reservation reuse after restart and AttemptCount increment only on the later Running checkpoint;
- preservation of unrelated durable tasks by the continuation finalizer;
- terminal success only when the full durable Mission is successful;
- Mission-wide accounting that includes untouched historical BudgetSnapshot usage;
- terminal Mission cancellation including unrelated unfinished durable tasks;
- rejection of an already active AppState before the executor can run.

Full Quality, reviews and review threads remain the merge gates. Startup remains passive throughout; no automatic Mission resume/retry/replay and no new Desktop HTTP or Mobile recovery-control endpoint is introduced by PR #68.

## 5. Safety and correctness invariants

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions bind approval to previewed state.
- Atomic/conflict-aware writes and checked postconditions.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent `danger-full-access` equivalent.
- Planner `RequestedCapabilities` remain inert until trusted governance grants implemented runtime capabilities.
- Dynamic role labels remain inert until mapped to an implemented runtime.
- Mobile permissions and Mission observability remain narrower than Desktop.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- No unsupervised concurrent mutation of the same workspace.
- `run_journal.go` is the single durable recovery authority; no second Mission journal may be introduced.
- Current project/Git reconciliation always outranks historical verification for reuse eligibility.
- A crash-running task is never treated as successful.
- A reservation is not an attempt; an attempt is counted only when Scheduler Running is durably observed.
- Repeated scheduler snapshots must not double-count task attempts.
- A malformed historical `verified` record must never authorize reuse or satisfy a dependency without fresh postcondition verification.
- Recovery transition plans, #66 snapshots and #67 materializations remain observation/preparation; #68 must recompute them at its own admission boundary.
- Persisted granted capabilities never become executable merely because they are present in recovery metadata.
- Missing/conflicting historical usage or attempted-task budget evidence fails closed rather than widening budget.
- Offline/crash downtime is not active Mission execution time.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Startup remains passive: no automatic Mission resume/retry/replay.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/agent_mission_recovery_control.go`, `src/run_journal.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, `src/run_journal_mission_continuation.go`, `src/run_journal_mission_admission.go`, `src/run_journal_mission_admission_test.go`, `src/run_journal_mission_admission_finalizer_test.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_recovery_usage_test.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_accounting.go`.

Desktop/Mobile boundary: `src/server.go`, `src/remote_server.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #68 on one exact fully green Quality head; inspect reviews/threads, mark Ready and squash-merge with the exact-head guard.
2. Verify the new authoritative `master` SHA after merge.
3. Only after #68 is merged, consider a narrow Desktop-only explicit recovery inspection/control transport calling trusted AppState governance; do not add Mobile recovery authority and keep startup passive.
4. Add bounded Mission Memory/Knowledge without creating a second recovery authority.
5. Only after durable read-only Mission continuation is sound, proceed to mutation-capable Builder/worktree and later Integrator/Test-Agent phases.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers and historical workflow runs must never be treated as active development. Delete obsolete feature branches/runs when the available GitHub integration supports the required operation; never invent successful cleanup when the operation is unavailable.
