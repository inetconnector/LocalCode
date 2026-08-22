# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `a4fad2494cd4b6b509647a8c514807e09f5736aa`  
**Last merged functional PR:** #69 `feat: add explicit desktop mission recovery controls`  
**Active work:** documentation synchronization on branch `docs/finalize-desktop-mission-recovery`; PR not yet opened at this checkpoint  
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

The older #69 runs that stopped at format/Vet are historical failures only and are not current evidence.

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

1. Finish this documentation synchronization on a fully green exact head and merge it into `master`.
2. Add **bounded Mission Memory/Knowledge** for architecture decisions, subsystem contracts, known failures and test evidence without creating a second recovery authority.
3. Define retention, size, schema-version and privacy/redaction limits before Mission Memory persistence is enabled.
4. Only after bounded read-only Mission memory/recovery remains sound, proceed to mutation-capable Builder agents in isolated Git worktrees, followed by Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded feature refs and historical workflow runs must never be treated as active development. Delete obsolete branches/runs when the available GitHub integration supports the operation; never claim cleanup that was not actually performed.
