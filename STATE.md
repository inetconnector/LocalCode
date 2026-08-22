# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-22 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `74f0a50ca8d62696340677479e5d3e8e44fd99bb`  
**Last merged functional PR:** #64 `feat: verify recovered mission postconditions`  
**Active work:** PR #65 `feat: plan safe mission recovery transitions`, branch `feat/mission-recovery-transition-planner`  
**Active head:** the exact PR head is verified from GitHub immediately before Quality/merge.  
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

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, orchestration diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current scheduled Child dispatch is synchronous; higher configured model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Phase 6 recovery foundation merged through PR #64

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`, not in a second journal.

PR #61 added restart reconciliation using bounded canonical project/Git identity evidence: hashed project/root identity, exact `HEAD`, hashed porcelain worktree state and timestamp. Interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always `interrupted_unknown`.

PR #62 added immutable successful-task completion evidence at scheduler-authoritative checkpoints. The journal stores only result status, SHA-256 result digest, fixed structure counts, verification state and timestamps. Raw Child/model result text, findings, file paths, test details, risk text and suggested-task objectives are not copied into recovery evidence.

PR #63 added durable task lifecycle counters/timestamps and typed verification-state records. `AttemptCount` increments only on a genuine not-running -> running transition; repeated running snapshots do not double-count. `RetryCount = max(AttemptCount-1, 0)`. Completion evidence starts `unverified`; bounded verification outcomes may become `failed` or terminal `verified` only with canonical SHA-256 evidence and bounded check counts.

PR #64 added an internal deterministic **read-only** postcondition verifier. It calls no model and reruns no Child. Before successful durable work can be marked `verified`, it freshly observes current project/Git state through the existing fixed read-only observer and checks current `matched` reconciliation, non-running state, durable success, completion evidence, successful result status and a canonical completion digest. Verification evidence binds Mission/task identity and current hashed/HEAD Git observation. Raw paths, Child/model output and raw verification output are not persisted.

A historical `verified` state never overrides current drift. `verified` work is reusable only while the **current** project/Git reconciliation remains `matched`. Verification uses an optimistic journal precondition so a concurrent Mission-recovery change prevents a stale verification write.

No automatic Mission resume, retry or replay is merged through PR #64.

## 4. Active PR #65 – deterministic recovery transition planner

PR #65 adds a pure recovery planner. It classifies possible next recovery actions but executes **nothing**, grants no capabilities and performs no Scheduler admission.

The planner emits per-task actions such as:

- `reuse_verified` for currently matched, verified durable success,
- `verify_postconditions` for successful but unverified/verification-failed work,
- `resume_candidate` for eligible nonterminal work,
- `retry_candidate` for eligible failed/retryable work,
- `interrupted_review_required` for crash-running work,
- `preserve_terminal` for states that must remain terminal,
- blocking outcomes for reconciliation mismatch, unsatisfied dependencies, missing lifecycle evidence or attempt limits,
- `invalid_recovery_state` for malformed durable recovery structures.

Dependency safety is deliberately stricter than normal DAG readiness: any task that could later be reused or executed requires every dependency to be a **currently matched, verified durable success**. An unverified or drift-blocked dependency prevents downstream continuation.

A historical `verified` flag is not sufficient for reuse. `reuse_verified` additionally requires coherent durable evidence: a successful persisted result status, canonical result and verification SHA-256 digests, a positive bounded verification check count and attempt record, nonnegative structure counts, and non-zero monotonic completion/verification timestamps. Malformed historical verification evidence is downgraded to `verify_postconditions` and cannot satisfy downstream dependencies until freshly re-verified.

Fixed planning bounds are `3` execution attempts per task and `192` aggregate Mission attempts (`64` maximum read-only Mission tasks × `3`). Existing lifecycle counters are authoritative evidence for these bounds; repeated snapshots never create attempts. Failed/retryable legacy work without lifecycle evidence is not made retryable because the remaining attempt budget cannot be proven.

Before producing any candidate action, #65 reconstructs and validates the durable task graph using the existing DAG validator. Duplicate IDs, missing dependencies, cycles, unsupported states, invalid task metadata, more than 64 tasks, negative/inconsistent lifecycle counters or counters above the fixed per-task limit make the plan invalid. Invalid plans produce only `invalid_recovery_state`, never executable candidates.

The aggregate Mission attempt bound is represented explicitly in the plan together with observed attempts and bounded prospective reservations. Reservations are planning facts only: they are not execution leases and cannot authorize a task.

Focused tests cover verified/unverified dependencies, malformed historical `verified` evidence, current Git drift, stale crash-running flags, retry eligibility and task limits, missing legacy lifecycle evidence, malformed/cyclic recovery DAGs, inconsistent lifecycle counters and the exact 64×3 Mission bound.

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
- Durable Mission metadata is bounded operational state, not a second transcript or authority source.
- Current project/Git reconciliation always outranks historical verification for reuse eligibility.
- A crash-running task is never treated as successful or directly resumable.
- Repeated scheduler snapshots must not double-count task attempts.
- Verification records must not persist raw verification output; `verified` may not silently regress.
- A malformed historical `verified` record must never authorize reuse or satisfy a dependency without fresh postcondition verification.
- Recovery transition plans are classification only, never Scheduler admission or execution authority.
- Reusable/executable candidates require currently verified reusable dependencies.
- Malformed recovery graphs/counters must fail closed to `invalid_recovery_state`.
- Raw Git porcelain paths and raw Child/model result content are not persisted in recovery evidence.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- No automatic Mission resume/retry/replay exists.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_transition_plan_test.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

## 7. Exact next development direction

1. Finish PR #65 on one exact head: require complete Quality success, inspect reviews/threads, mark Ready and merge automatically.
2. Add an explicit controlled read-only Mission recovery-control boundary that recomputes fresh reconciliation/verification/transition state before any pause/resume/retry decision; no automatic startup continuation.
3. Preserve Scheduler admission/resource limits, cancellation semantics, durable attempt limits and historical accepted usage without double-counting across controlled continuation.
4. Expand crash/restart coverage for partially completed Missions and controlled resume/retry before any mutation-capable Builder/worktree phase.
5. Only after durable Mission continuation is sound, proceed to Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
