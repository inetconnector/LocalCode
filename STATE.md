# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `bcd17f6975ce63dd28b23dbdeea34d42e1d53ad4`  
**Last merged functional PR:** #66 `feat: add read-only mission recovery control boundary`  
**Active work:** draft PR #67 `feat: materialize safe mission recovery continuation`, branch `feat/mission-recovery-dispatch-gate`  
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

## 3. Phase 6 recovery foundation merged through PR #66

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`; no second Mission journal exists.

PR #61 added restart reconciliation using bounded canonical project/Git identity evidence: hashed project/root identity, exact `HEAD`, hashed porcelain worktree state and timestamp. Interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always unknown/non-successful.

PR #62 added immutable successful-task completion evidence at scheduler-authoritative checkpoints. The journal stores result status, SHA-256 result digest, fixed structure counts, verification state and timestamps. Raw Child/model result text, findings, file paths, test details, risk text and suggested-task objectives are not copied into recovery evidence.

PR #63 added durable task lifecycle counters/timestamps and typed verification-state records. `AttemptCount` increments only on a genuine not-running -> running transition; repeated snapshots do not double-count. Failed/retryable legacy work without lifecycle evidence cannot prove remaining retry budget.

PR #64 added a deterministic read-only postcondition verifier. It freshly observes project/Git state and checks current matched reconciliation, non-running state, durable success, completion evidence, successful result status and canonical completion digest. Verification evidence binds Mission/task identity to current hashed/HEAD Git observation and fixed check results. Raw paths, Child/model output and raw verification output are excluded.

PR #65 added the deterministic recovery transition planner. It validates the durable DAG and lifecycle counters, enforces three attempts/task and 192 attempts/Mission, requires currently reusable verified dependencies, keeps crash-running work `interrupted_review_required`, and emits classification only: `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, terminal/blocking outcomes or fail-closed `invalid_recovery_state`.

Historical `verified` is not sufficient. Reuse requires successful result metadata, canonical result/verification SHA-256 values, exact six-check verification evidence, positive verification attempt evidence, nonnegative structure counts and monotonic timestamps. The persisted verification SHA must equal the canonical digest for the **current matched** reconciliation and the six successful fixed postcondition checks. A well-formed but semantically wrong SHA cannot unlock a task or dependency.

PR #66 added the trusted read-only recovery-control boundary. `AppState.MissionRecoveryControlSnapshot(runID)` loads the exact nonterminal Mission from the single journal, freshly observes project/Git, transiently re-verifies only tasks that require postcondition checks, recomputes the transition plan, and re-reads the journal. It retries up to three times if the authority-relevant journal fingerprint changes and fails closed when a stable observation cannot be obtained.

The #66 snapshot reports `read_only=true`, `execution_authorized=false`, `scheduler_lease_granted=false`, and `persistent_state_modified=false`. It writes no verification, reconciliation or plan data back to `active-run.json`. A cached startup `AppState.Recovery` object is never the control authority. Unknown verification states and malformed historical `verified` metadata are not normalized into durable reusable evidence.

The trusted AppState boundary rejects snapshots while another agent run is active and checks `Running` again after construction. The snapshot SHA binds the current observation but is **not** an execution token, Scheduler lease, capability grant or permission to dispatch.

No automatic Mission resume, retry or replay is merged through PR #66.

## 4. Active PR #67 – bounded continuation materialization

PR #67 adds the next non-executing boundary in `src/run_journal_mission_continuation.go`. It does **not** run a Child, call a model, mutate the journal or request Scheduler admission.

For one explicit task ID it:

- starts from the same fresh #66 reconciliation/verification/transition machinery and the same stable journal fingerprint,
- accepts only a current `resume_candidate` or `retry_candidate` with a prospective new attempt already allowed by the #65 fixed attempt limits,
- includes only that selected task plus the transitive dependency closure whose transitions are currently `reuse_verified`, excluding unrelated ready work from the reconstructed graph,
- regenerates executable read-only `Capabilities` from the canonical Explorer/Planner/Reviewer role envelope instead of trusting persisted granted capability data,
- revalidates persisted `RequestedCapabilities` against that role envelope,
- requires Run/Mission/task model identity to remain consistent and does not silently select a new model,
- carries historical scheduler-adjacent usage forward from task BudgetSnapshot evidence and rejects negative or conflicting usage facts,
- fails closed when a task with a recorded execution attempt lacks BudgetSnapshot usage evidence rather than interpreting missing accounting evidence as zero usage,
- evaluates the recovered Mission budget against historical accepted usage and elapsed Mission wall time; an already-exhausted budget cannot produce a continuation materialization,
- constructs an in-memory DAG where verified dependencies remain successful and only the selected candidate becomes ready,
- re-reads the journal after observation and requires the fingerprint to remain unchanged.

The returned `MissionRecoveryContinuationMaterialization` includes the stable journal/snapshot SHA evidence, bounded graph, historical usage and Mission budget snapshot, but explicitly remains `read_only=true`, `execution_authorized=false`, `scheduler_lease_granted=false`, and `persistent_state_modified=false`.

`AppState.MissionRecoveryContinuationMaterialization(runID, taskID)` also rejects an already-active agent run and rejects a result if a run becomes active during materialization. Because an active run could theoretically start and finish after the pre-check but before the post-check, this object is intentionally **not** a reusable authorization token. The later execution slice must recompute this materialization and couple final revalidation to actual Scheduler admission under a separately reviewed atomic boundary.

Current focused #67 tests cover bounded dependency closure, exclusion of unrelated work, canonical capability regeneration, resume and retry candidates, capability escalation rejection, conflicting historical usage, missing usage evidence for attempted tasks, Mission-budget exhaustion, zero journal writes and AppState active-run races.

The first #67 CI attempt (#578) stopped only at `gofmt` on the new production file before Vet/tests. Formatting was corrected and the missing-usage-evidence rule was added afterwards; the final exact head must pass the complete Quality workflow before merge.

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
- Recovery transition plans, #66 snapshots and #67 materializations are observation/preparation only, never Scheduler admission or execution authority.
- #66/#67 read-only boundaries must never mutate `active-run.json`.
- Persisted granted capabilities never become executable merely because they are present in recovery metadata; executable continuation capabilities must be regenerated from trusted role governance.
- Missing/conflicting historical usage evidence for attempted work must fail closed rather than widen Mission budget.
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

Mission/recovery: `src/agent_mission.go`, `src/agent_mission_recovery_control.go`, `src/run_journal.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, `src/run_journal_mission_continuation.go`, their focused tests, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_accounting.go`.

Desktop/Mobile boundary: `src/server.go`, `src/remote_server.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #67 on one exact fully green Quality head; inspect reviews/threads, mark Ready and merge.
2. After #67 is merged, implement the separately governed **atomic continuation admission/execution boundary**. It must recompute the #67 materialization immediately before execution and couple stale-state/active-run/journal checks to Scheduler admission; a previously returned materialization may never authorize dispatch.
3. On actual continuation, persist the new execution attempt without losing prior lifecycle counters, carry historical scheduler-accepted Usage/Accounting forward without double-counting, and preserve Mission/task attempt limits and cancellation semantics.
4. Add crash/restart tests for repeated restarts, drift between materialization and admission, cancel-vs-resume/retry races, attempt-counter persistence and budget/accounting continuity.
5. Add a narrow Desktop-only recovery inspection/control transport only when the execution boundary is sound; it must call trusted AppState governance and must not create a Mobile recovery-control surface.
6. Only after durable Mission continuation is sound, proceed to mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; until then, stale refs must never be treated as active development.
