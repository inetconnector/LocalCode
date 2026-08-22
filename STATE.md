# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `fd5c7e38788b0485f0548f01de89fe25f320ec7f`  
**Last merged functional PR:** #68 `feat: atomically admit mission recovery continuation`  
**Active work:** draft PR #69 `feat: add explicit desktop mission recovery controls`, branch `feat/desktop-mission-recovery-control`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point. Only merged `master` is authoritative product behavior; PR #69 remains candidate behavior until its exact head passes all gates and is merged. `TODO.md` contains unfinished work only.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution.

Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Merged runtime and Agent-Team state

Merged runtime includes the Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, persistent project/task history, controlled file/Git/build/test/tool/web/MCP/attachment/asset operations, context compaction, local memory boundaries and durable normal-run recovery.

Executable Child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Their schemas allow project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration includes structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, Desktop/Mobile observation, diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current Child dispatch remains synchronous; configured model-slot capacity alone is not proof of parallel inference.

## 3. Phase 6 recovery foundation merged through PR #68

`run_journal.go` remains the **single durable recovery authority**. Read-only Missions use bounded structured metadata in the existing `active-run.json`; no second Mission journal exists.

- **#61**: restart reconciliation from canonical project/Git identity, exact HEAD and hashed worktree evidence. Crash-running work is never treated as success.
- **#62**: immutable successful-task completion evidence without copying raw Child/model output into recovery authority.
- **#63**: durable lifecycle counters/timestamps. Repeated running snapshots cannot double-count attempts.
- **#64**: deterministic read-only postcondition verification against fresh project/Git state and canonical completion evidence.
- **#65**: deterministic recovery transition planner with three attempts/task and 192 attempts/Mission, verified dependency requirements and fail-closed invalid-state handling.
- **#66**: trusted read-only `MissionRecoveryControlSnapshot`; fresh observation, transient verification and bounded journal-stability retry. Snapshot/control data is observation, not a Scheduler lease or execution token.
- **#67**: bounded continuation materialization for one explicit current `resume_candidate` or `retry_candidate`, including only the candidate plus transitively verified dependencies. Trusted role capabilities are regenerated, model identity and historical Usage are revalidated, and offline/crash downtime is excluded from active Mission time.
- **#68**: atomic execution-capable continuation admission. The #67 materialization is recomputed while holding the AppState run gate; the exact journal fingerprint/file version is checked; a fresh execution `RunID` is reserved before any Scheduler exists; historical task/Mission budgets and accepted Usage remain cumulative; the full durable Mission is preserved while the narrow continuation graph is merged back.

### #68 crash/attempt semantics

A reservation is not an attempt. `AttemptReserved` is written before Scheduler admission, but `AttemptCount` advances only when a durable Scheduler `Running` checkpoint proves execution actually started. A crash after reservation but before Scheduler admission therefore does not consume retry budget. Explicit cancellation uses the existing AppState/Scheduler cancellation owner and terminalizes the Mission consistently.

The authoritative #68 squash commit on `master` is `fd5c7e38788b0485f0548f01de89fe25f320ec7f`.

## 4. Active PR #69 – explicit Desktop Mission recovery control

PR #69 adds the first product-facing recovery control surface, **Desktop loopback only**.

### 4.1 Inspection

`GET /api/mission-recovery` returns a deliberately bounded DTO derived from the trusted #66 control snapshot. It exposes only the interrupted Run/Mission identity, observation/fingerprint hashes, reconciliation state, plan validity/candidate availability and per-task durable state/action flags required to render an explicit choice.

It does **not** expose project paths, objectives, capabilities, raw Child/model output, findings, Usage or Mission accounting.

The #66 control snapshot now exposes its already-computed `JournalSHA256` as a bounded field. The hash was already part of the snapshot digest; exposing it does not add execution authority.

### 4.2 Explicit continuation

`POST /api/mission-recovery/continue` accepts exactly one explicit task and its inspected stale-state preconditions. The request is strictly decoded, body-bounded and permits only current `resume_candidate` / `retry_candidate` intent.

The request never authorizes execution by itself. AppState recomputes the trusted #67 materialization immediately at admission, rechecks Mission/action identity and the durable journal fingerprint, restores/caps task budgets, then calls the #68 exact journal CAS. Only after that durable reservation succeeds does AppState become `Running` under a fresh execution-scoped `RunID`.

`SnapshotSHA256` records the UI observation shape but is not reused as an authorization token because the trusted snapshot digest deliberately includes its observation time. Durable staleness is bound by `JournalSHA256`; fresh project/Git reconciliation and transition planning are recomputed before admission.

The HTTP endpoint returns `202 Accepted` only after durable reservation plus AppState ownership have succeeded. Execution then proceeds asynchronously under the existing Scheduler. Request cancellation after acceptance does not orphan the run; `StopAgent` owns the accepted continuation cancellation exactly like other active work.

### 4.3 Desktop UI and Remote boundary

`src/static/mission_status.js` renders a bilingual **Interrupted Mission / Unterbrochene Mission** card in the Desktop Output inspector. Only current candidates receive explicit **Resume task / Retry task** buttons. The copy states that recovery never starts automatically and that the selected task is revalidated immediately before execution.

The two routes are registered on the existing Desktop `Server` and inherit its loopback Host, Origin and `Sec-Fetch-Site` checks. The server diff is limited to the two route registrations.

`RemoteServer` gains **no** recovery route. Focused tests require `/remote/api/mission-recovery` and `/remote/api/mission-recovery/continue` to remain `404`. Mobile continues to receive only the narrow active-Mission indicator and existing stop behavior; it receives no recovery IDs, plan, Scheduler, budget or control authority.

Startup remains passive. There is no automatic resume, retry or replay.

### 4.4 Focused #69 regression coverage

Current branch tests cover:

- bounded Desktop recovery DTO and candidate-only executable flags;
- no-recovery `204` and bounded inspection JSON;
- strict request decoding, unknown-field rejection and stale-precondition `409`;
- `202 Accepted` admission response;
- stale preconditions cannot rotate RunID, set AppState running or invoke the executor;
- successful admission is visible in AppState before return and `StopAgent` owns cancellation;
- terminal cancellation is persisted through the single run journal;
- Desktop routes inherit loopback/Origin protection;
- Remote recovery paths remain absent/`404`.

Full Quality, reviews and review threads remain the merge gates.

## 5. Safety and correctness invariants

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions and atomic conflict-aware writes.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent unrestricted capability equivalent.
- Planner-requested or persisted capabilities never become executable authority by presence alone.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- `run_journal.go` remains the single durable recovery authority.
- Current project/Git reconciliation outranks historical verification.
- Crash-running work is never successful by inference.
- Reservation is not an attempt; attempts count only at durable Scheduler Running.
- Historical Usage/budget evidence is cumulative and must never be silently reset or widened.
- Offline/crash downtime is not active Mission execution time.
- Recovery plan/snapshot/materialization objects are not reusable execution tokens.
- Desktop recovery remains explicit and loopback-only; Mobile/Remote has no recovery-control authority.
- Stable Mission identity remains separate from execution-scoped RunID.
- Startup remains passive: no automatic Mission resume/retry/replay.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened to make CI pass.

## 6. Important files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Recovery core: `src/run_journal.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, `src/run_journal_mission_continuation.go`, `src/run_journal_mission_admission.go` and focused tests.

Desktop recovery surface: `src/desktop_mission_recovery.go`, `src/server.go`, `src/static/mission_status.js`, `src/desktop_mission_recovery_test.go`, `src/desktop_mission_recovery_server_test.go`, `src/desktop_mission_recovery_remote_test.go`.

Mobile boundary: `src/remote_server.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next direction

1. Finish PR #69 on one exact fully green Quality head; inspect reviews/threads, mark Ready and squash-merge with expected-head guard.
2. Verify the resulting authoritative `master` SHA.
3. After the read-only recovery product surface is sound, add bounded Mission Memory/Knowledge without creating a second recovery authority.
4. Then proceed to mutation-capable Builder agents in isolated Git worktrees, followed by Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers and historical workflow runs must never be treated as active development. Delete obsolete feature branches/runs when supported; never claim cleanup that was not actually performed.
