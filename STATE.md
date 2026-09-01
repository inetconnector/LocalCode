# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-09-01 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `47d53e0`  
**Last merged functional PR:** #70 `docs: finalize desktop mission recovery state`  
**Active branch:** `feat/mission-recovery-atomic-admission`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point. Only merged `master` is authoritative product behavior. `TODO.md` contains unfinished work only.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution.

Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged runtime and Agent-Team state

Merged runtime includes the Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, persistent project/task history, controlled file/Git/build/test/tool/web/MCP/attachment/asset operations, context compaction, local memory boundaries and durable run recovery.

Executable Child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Their schemas allow project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration includes structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution-scoped `RunID`, Desktop/Mobile observation, diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current Child dispatch is synchronous. Higher configured model-slot capacity alone is not proof of parallel model inference.

## 3. Durable read-only Mission recovery – merged through PR #69

`run_journal.go` remains the **single durable recovery authority**. Read-only Missions use bounded structured metadata in the existing `active-run.json`; there is no second Mission journal.

Recovery layers now merged:

- **#61** restart reconciliation from canonical project/Git identity, exact `HEAD` and hashed worktree evidence. Crash-running work is never inferred successful.
- **#62** bounded completion evidence without copying raw Child/model output into recovery authority.
- **#63** durable lifecycle counters/timestamps; repeated running snapshots cannot double-count attempts.
- **#64** deterministic read-only postcondition verification against fresh project/Git state and canonical completion evidence.
- **#65** deterministic transition planner with three attempts/task and 192 attempts/Mission, verified dependency requirements and fail-closed invalid-state handling.
- **#66** trusted read-only `MissionRecoveryControlSnapshot`; fresh observation, transient verification and bounded journal-stability retry. Snapshot/control data is observation, not a Scheduler lease or execution token.
- **#67** bounded continuation materialization for one explicit current `resume_candidate` or `retry_candidate`, containing only that candidate plus transitively verified dependencies. Trusted role capabilities are regenerated, model identity and historical Usage are revalidated, and crash/offline downtime is excluded from active Mission time.
- **#68** atomic execution-capable continuation admission. Fresh materialization is recomputed while holding the AppState run gate; exact journal fingerprint/file version is checked; a fresh execution RunID is durably reserved before any Scheduler exists; historical task/Mission budgets and accepted Usage remain cumulative; subset execution merges back into the full durable Mission.
- **#69** explicit **Desktop-loopback-only** recovery inspection and continue transport plus bilingual Output-inspector controls. Startup remains passive and Remote/Mobile receives no recovery-control authority.

### 3.1 Attempt, budget and cancellation invariants

A durable reservation is not an attempt. `AttemptReserved` is written before Scheduler admission, while `AttemptCount` advances only after a durable Scheduler `Running` checkpoint proves execution actually started. A crash in the reservation/admission gap does not consume retry budget.

Recovery never silently mints a new budget. Historical scheduler-accepted Usage remains cumulative; task and Mission limits are restored conservatively and can only be narrowed. Offline/crash downtime is not counted as active Mission execution time.

Explicit cancellation uses the existing AppState/Scheduler cancellation owner. Late cancelled Child results are non-authoritative.

### 3.2 Desktop recovery surface merged in #69

`GET /api/mission-recovery` returns a deliberately bounded DTO derived from trusted recovery control state. It exposes only interrupted Run/Mission identity, observation/fingerprint hashes, reconciliation state, plan/candidate availability and per-task durable state/action flags required for an explicit Desktop choice.

It does **not** expose project paths, objectives, capabilities, raw Child/model output, findings, Usage or Mission accounting.

`POST /api/mission-recovery/continue` accepts one explicit task plus inspected stale-state preconditions. The body is size-bounded and strictly decoded. Only a current `resume_candidate` or `retry_candidate` can proceed.

UI-provided hashes are preconditions, never authority. Admission recomputes fresh #67 governance under the AppState run gate, rechecks Mission/action identity and the durable journal fingerprint, restores/caps budgets, and then executes the #68 exact journal CAS. `202 Accepted` is returned only after durable reservation and AppState ownership succeed. Accepted execution then runs asynchronously under the existing Scheduler and remains cancellable through `StopAgent`.

`SnapshotSHA256` records the observed control snapshot but is not reused as an authorization token because that digest includes observation time. Durable staleness is bound by `JournalSHA256`, while current project/Git reconciliation and transition planning are recomputed immediately before admission.

### 3.3 Desktop/Remote trust boundary

`src/static/mission_status.js` renders the bilingual **Interrupted Mission / Unterbrochene Mission** card in the Desktop Output inspector. Only current candidates get explicit **Resume task / Retry task** controls; nothing auto-posts or auto-resumes.

The two recovery routes live only on the Desktop `Server` and inherit its loopback Host, Origin and `Sec-Fetch-Site` checks.

`RemoteServer` has no recovery route. Regression tests require `/remote/api/mission-recovery` and `/remote/api/mission-recovery/continue` to remain `404`. Mobile continues to receive only the narrow active-Mission indicator and existing stop behavior, with no recovery IDs, plan, Scheduler, budget or continuation authority.

## 4. Verification status

PR #69 was squash-merged to `master` as `a4fad2494cd4b6b509647a8c514807e09f5736aa`.

The final #69 feature-branch head `2102b0445c1d199d3f4fbe5d077735f40f096254` has the **same tree SHA** (`566fdf3d45962444cfa1c9e9950fe5db9f307177`) as the merged master commit. Quality run **#608** / run ID `32573085942` completed successfully on that exact tree. Every gate passed: Go version/setup, format, Vet, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, vulnerability scan, full-stack loopback HTTP integration, Go tests, Race Detector, >=80% coverage, native Windows builds and Git diff check.

<<<<<<< HEAD
The older #69 runs that stopped at format/Vet are historical failures only and are not current evidence.
=======
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

### 4.6 Focused tests and active branch subsystems

Focused active-branch tests in `src/run_journal_mission_admission_test.go`, `src/mission_recovery_transport_test.go`, `src/agent_mission_knowledge_test.go`, `src/agent_worktree_test.go`, `src/agent_integrator_test.go`, `src/computemesh_test.go`, and `src/agent_scheduler_recovery_usage_test.go` cover:

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
- rejection of an already active AppState before the executor can run;
- Desktop-only loopback/CSRF-protected recovery transport (`/api/mission/recovery` and `/api/mission/recovery/continuation`) with strict Mobile Remote isolation;
- bounded Mission Memory/Knowledge (`src/agent_mission_knowledge.go` and `/api/mission/knowledge`) for architecture decisions, subsystem contracts, known failures, and test evidence;
- Phase 7 LocalCode-managed Git worktrees (`src/agent_worktree.go`) for isolated mutation-capable Builder agents with directory containment, single-lease concurrency, non-colliding branch allocation, and non-destructive cleanup;
- Phase 8 Integrator, Test Agent, and Independent Reviewer (`src/agent_integrator.go`) with single-authority merge, objective evidence isolation, structured PASS/FAIL/REPAIR decision lifecycle, conflict recovery, and stagnation protection;
- Phase 9 Constrained Agent Factory & Bounded Replanning (`src/agent_factory.go`, `src/agent_mission_replanning.go`, and their focused tests) for type-safe role mapping, inert dynamic role quarantine, deferred tool resolution, and bounded DAG replanning with hard task caps (32 tasks), cycle limits (3 per task), depth caps (5), and symptom-hash stagnation protection;
- Backend-Neutral Local Inference Runner (`src/inference_backend.go`, `src/inference_backend_test.go`) supporting ComputeMesh, Ollama, and llama.cpp/OpenAI-compatible endpoints (`/v1/chat/completions`) without silent provider drift;
- LocalCode Doctor Subsystem (`src/doctor.go`, `src/doctor_test.go`, `GET /api/doctor`) for structured full-system health diagnostics covering ComputeMesh cluster connectivity, GPU/VRAM live specs, local Ollama daemon, Git worktrees, MCP stdio processes, and coding engines;
- ComputeMesh decentralized cluster subsystem (`src/computemesh.go`, `src/computemesh_test.go`, `src/static/computemesh.js`) for zero-config provider self-compute (0% platform fee) with auto-discovery of keys from `.computemesh/provider_config.json`, live workstation node probing, bearer token injection in `OllamaClient`, live hardware/cluster latency and status probing (`https://computemesh.inetconnector.com`), model discovery, REST endpoints (`/api/computemesh/status`, `/api/computemesh/autodetect`, `/api/computemesh/test`), and dual German/English localization;
- E2E Automation & Remote Control Service Harness (`scripts/test-automation-service.ps1`, `scripts/test-android-remote-full.ps1`) validating 10 vital system subsystems: Desktop REST API, Projects, Settings roundtrip, Engines, MCP status, System Doctor / Diagnostics, Remote Pairing, Token Authentication, Android Companion verification via ADB, Thread/Chat lifecycle;
- HTTP 429 Rate-Limit Exponential Backoff in `OllamaClient` (`src/ollama.go`) for resilient inference over ComputeMesh cluster nodes;
- Streamlined & Decluttered UI (`src/static/remote.html`, `src/static/index.html`) with collapsible tool accordions and high-level progress indicators;
- Camera QR Scanner Button on Pairing Screen (`src/static/remote.html`, `MainActivity.java`), top-right Header Gear Settings Menu (`⚙️`) with coding engine selection modal, uncluttered composer dock, and safety confirmation dialog on project switches.

This branch is fully tested, green across all packages, and ready for merge.

## 5. Safety and correctness invariants

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions and atomic conflict-aware writes.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent unrestricted capability equivalent.
- Planner-requested or persisted capabilities never become executable authority merely by being present.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- `run_journal.go` remains the single durable Mission recovery authority.
- Current project/Git reconciliation outranks historical verification.
- Crash-running work is never successful by inference.
- Reservation is not an attempt; attempts count only at durable Scheduler `Running`.
- Historical Usage/budget evidence is cumulative and must never be silently reset or widened.
- Offline/crash downtime is not active Mission execution time.
- Recovery plan/snapshot/materialization objects are not reusable execution tokens.
- Desktop recovery remains explicit and loopback-only; Mobile/Remote has no recovery-control authority.
- Stable Mission identity remains separate from execution-scoped RunID.
- Startup remains passive: no automatic Mission resume/retry/replay.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened to make CI pass.

## 6. Important files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery core: `src/agent_mission.go`, `src/run_journal.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, `src/run_journal_mission_continuation.go`, `src/run_journal_mission_admission.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go` and focused tests.

Desktop recovery surface: `src/desktop_mission_recovery.go`, `src/desktop_mission_recovery_observer.go`, `src/server.go`, `src/static/mission_status.js`, `src/desktop_mission_recovery_test.go`, `src/desktop_mission_recovery_server_test.go`, `src/desktop_mission_recovery_remote_test.go`.

Mobile boundary: `src/remote_server.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #70 on one exact fully green Quality head, inspect reviews/threads, mark Ready and squash-merge with an expected-head guard.
2. Verify the resulting authoritative `master` SHA.
3. Add **bounded Mission Memory/Knowledge** for architecture decisions, subsystem contracts, known failures and test evidence without creating a second recovery authority.
4. Define retention, size, schema-version and privacy/redaction limits before Mission Memory persistence is enabled.
5. Only after bounded read-only Mission memory/recovery remains sound, proceed to mutation-capable Builder agents in isolated Git worktrees, followed by Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded feature refs and historical workflow runs must never be treated as active development. Delete obsolete branches/runs when the available GitHub integration supports the operation; never claim cleanup that was not actually performed.
