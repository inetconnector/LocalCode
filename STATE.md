# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `21a00ee24c729b4c46653041941168c59a69a11f`  
**Last merged functional PR:** #65 `feat: plan safe mission recovery transitions`  
**Active work:** draft PR #66 `feat: add read-only mission recovery control boundary`, branch `feat/mission-recovery-control-boundary`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

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

## 3. Phase 6 recovery foundation merged through PR #65

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`; no second Mission journal exists.

PR #61 added restart reconciliation using bounded canonical project/Git identity evidence: hashed project/root identity, exact `HEAD`, hashed porcelain worktree state and timestamp. Interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always unknown/non-successful.

PR #62 added immutable successful-task completion evidence at scheduler-authoritative checkpoints. The journal stores result status, SHA-256 result digest, fixed structure counts, verification state and timestamps. Raw Child/model result text, findings, file paths, test details, risk text and suggested-task objectives are not copied into recovery evidence.

PR #63 added durable task lifecycle counters/timestamps and typed verification-state records. `AttemptCount` increments only on a genuine not-running -> running transition; repeated snapshots do not double-count. Failed/retryable legacy work without lifecycle evidence cannot prove remaining retry budget.

PR #64 added a deterministic read-only postcondition verifier. It freshly observes project/Git state and checks current matched reconciliation, non-running state, durable success, completion evidence, successful result status and canonical completion digest. Verification evidence binds Mission/task identity to current hashed/HEAD Git observation and fixed check results. Raw paths, Child/model output and raw verification output are excluded.

PR #65 added the deterministic recovery transition planner. It validates the durable DAG and lifecycle counters, enforces three attempts/task and 192 attempts/Mission, requires currently reusable verified dependencies, keeps crash-running work `interrupted_review_required`, and emits classification only: `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, terminal/blocking outcomes or fail-closed `invalid_recovery_state`.

Historical `verified` is not sufficient. Reuse requires successful result metadata, canonical result/verification SHA-256 values, exact six-check verification evidence, positive verification attempt evidence, nonnegative structure counts and monotonic timestamps. The persisted verification SHA must equal the canonical digest for the **current matched** reconciliation and the six successful fixed postcondition checks. A well-formed but semantically wrong SHA cannot unlock a task or dependency.

No automatic Mission resume, retry or replay is merged through PR #65.

## 4. Active PR #66 – read-only recovery-control snapshot boundary

PR #66 is building an explicit control/observation boundary on top of the merged recovery primitives. The current implementation in `src/run_journal_mission_control.go`:

- loads the exact requested nonterminal read-only Mission from the single durable run journal rather than trusting cached startup `AppState.Recovery`,
- fingerprints the authority-relevant journal state with SHA-256,
- freshly observes project/Git state and reconstructs current reconciliation,
- runs the #65 planner once to identify only tasks requiring `verify_postconditions`,
- evaluates those fixed postconditions read-only and applies successful verification **only to a cloned transient Mission snapshot**,
- recomputes the final transition plan from that fresh transient evidence,
- re-reads the journal and retries up to three times if its fingerprint changed during observation,
- returns an opaque snapshot SHA binding journal fingerprint, current reconciliation, transient verification summaries and final plan.

The control snapshot explicitly reports `read_only=true`, `execution_authorized=false`, `scheduler_lease_granted=false`, and `persistent_state_modified=false`. It does not write verification state, reconciliation or plan data back to `active-run.json`; tests compare the journal bytes before/after a stable snapshot.

Malformed historical `verified` evidence bridges safely into this boundary: #65 first classifies it as `verify_postconditions`; #66 freshly evaluates the six postconditions and may repair only the **transient clone** for the current snapshot. The durable malformed record remains unchanged. Structural evidence that still violates #65 invariants remains non-reusable even if the six direct postconditions pass.

The snapshot SHA is observation binding only. It is not an execution token, Scheduler lease, capability grant or permission to dispatch. Any later resume/retry slice must discard stale assumptions and recompute/revalidate immediately before Scheduler admission.

Focused tests cover transient verification without input mutation, malformed historical verified evidence, current Git drift, zero durable journal writes, retry after a concurrent journal change, and rejection of wrong/terminal run IDs.

Known active work in #66: expose the boundary through a narrow Desktop-only explicit control transport, reject use while an active agent run is in progress, update architecture/security documentation, then obtain one fully green exact Quality head. The first draft CI run #567 failed only because the new transient clone referenced a non-existent legacy field; that source error has been removed and subsequent CI must be evaluated on the final exact head.

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
- A crash-running task is never treated as successful or directly resumable.
- Repeated scheduler snapshots must not double-count task attempts.
- Verification records must not persist raw verification output; durable `verified` may not silently regress.
- A malformed historical `verified` record must never authorize reuse or satisfy a dependency without fresh postcondition verification.
- Recovery transition plans and #66 control snapshots are observation/classification only, never Scheduler admission or execution authority.
- A #66 snapshot must never mutate `active-run.json`; transient verification exists only on a clone.
- Reusable/executable candidates require currently verified reusable dependencies.
- Malformed recovery graphs/counters fail closed to `invalid_recovery_state`.
- Raw Git porcelain paths and raw Child/model result content are not persisted in recovery evidence.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Startup remains passive: no automatic Mission resume/retry/replay.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, their focused tests, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

Desktop/Mobile boundary: `src/server.go`, `src/remote_server.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #66 as a strictly read-only Desktop recovery-control boundary: explicit transport, active-run exclusion, docs and one fully green exact Quality head.
2. After #66 is merged, define controlled pause/resume for eligible nonterminal read-only tasks, but only after a fresh control snapshot is recomputed at the dispatch boundary.
3. Add controlled retry for eligible failed/retryable tasks while preserving durable task/Mission attempt limits, Scheduler admission/resource limits, cancellation semantics and historical accepted usage/accounting without double-counting.
4. Expand crash/restart tests for partially completed Missions, repeated restarts, drift between plan and dispatch, cancel-vs-resume/retry races and budget/accounting continuity.
5. Only after durable Mission continuation is sound, proceed to mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; until then, stale refs must never be treated as active development.
