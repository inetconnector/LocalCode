# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `f54e69bdfee270314aa32f53f3a9c7d5cbca96c9`  
**Last merged functional PR:** #63 `feat: persist mission attempts and verification state`  
**Active work:** PR #64 `feat: verify recovered mission postconditions`, branch `feat/mission-recovery-postcondition-verifier`  
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

## 3. Phase 6 recovery foundation merged through PR #63

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`, not in a second journal.

PR #61 added restart reconciliation using bounded canonical project/Git identity evidence: hashed project/root identity, exact `HEAD`, hashed porcelain worktree state and timestamp. Interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always `interrupted_unknown`.

PR #62 added immutable successful-task completion evidence at scheduler-authoritative checkpoints. The journal stores only result status, SHA-256 result digest, fixed structure counts, verification state and timestamps. Raw Child/model result text, findings, file paths, test details, risk text and suggested-task objectives are not copied into recovery evidence.

PR #63 added durable task lifecycle counters/timestamps and typed verification-state records. `AttemptCount` increments only on a genuine not-running -> running transition; repeated running snapshots do not double-count. `RetryCount = max(AttemptCount-1, 0)`. Completion evidence starts `unverified`; bounded verification outcome records may become `failed` or terminal `verified` only with a canonical SHA-256 evidence digest and 1–32 checks. Raw verification output is not persisted.

No automatic Mission resume, retry or replay is merged through PR #63.

## 4. Active PR #64 – deterministic recovery postcondition verifier

PR #64 adds an internal deterministic **read-only** verifier for durable successful Mission tasks after restart. It does not call a model, rerun a Child result, mutate project files, grant capabilities or resume/retry a Mission.

Before a task can be marked `verified`, the verifier obtains a fresh project/Git reconciliation through the existing fixed read-only observer and evaluates six fixed checks:

1. current project/Git reconciliation is `matched`,
2. task is not running,
3. durable task state is `succeeded`/legacy `completed`,
4. completion evidence exists,
5. completion result status is `completed` or `fallback`,
6. the durable completion-result digest is a canonical SHA-256 value.

The verification evidence digest binds Mission ID, Task ID, completion-result digest, reconciliation state/reason, current hashed project/Git-root identity, exact current `HEAD`, current hashed porcelain status and the fixed check booleans. Raw project paths, porcelain paths, Child/model output and raw verification output are not included.

Verification uses an optimistic journal precondition: after the fresh filesystem/Git observation, LocalCode reloads `active-run.json` and refuses to write if the Mission recovery state changed meanwhile. A passing verification records `verified`; a failing check records `failed` for non-terminal verification state. The freshly observed reconciliation is persisted in the same journal.

A historical `verified` state never overrides current drift. If the task was previously verified but a later check sees changed `HEAD`/worktree/project identity, the verification record remains terminal `verified`, while the newly persisted current reconciliation blocks the task from reuse. `verified` is therefore reusable only when **current** reconciliation is still `matched`.

Restart task reconciliation now also exposes durable attempt/retry counts and verification state. A verified successful task with current `matched` project/Git state is classified terminal/reusable; verification-failed or unverified successful work requires postcondition verification; any current project/Git mismatch remains blocked; crash-running remains `interrupted_unknown`.

Focused tests cover matched success, previously verified idempotence, current drift overriding reuse eligibility, running/missing/invalid completion evidence, lifecycle/verification fields in reconciliation, an actual temporary Git repository, durable verification writes, and drift observed after a prior verification.

There is still no automatic invocation from startup, no Mission-control UI/API for this verifier, and no Mission resume/retry/replay in PR #64.

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
- A crash-running task is never treated as successful.
- Repeated scheduler snapshots must not double-count task attempts.
- Verification records must not persist raw verification output; `verified` may not silently regress.
- Raw Git porcelain paths and raw Child/model result content are not persisted in recovery evidence.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- No automatic Mission resume/retry/replay exists.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_postcondition_verify_test.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_reconcile_test.go`, `src/run_journal_mission_test.go`, `src/run_journal.go`, `src/run_journal_test.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

## 7. Exact next development direction

1. Finish PR #64 on one exact head: require complete Quality success, inspect reviews/threads, mark Ready and merge automatically.
2. Define a deterministic recovery transition planner/state machine for queued, ready, blocked, running-at-crash, failed, cancelled, succeeded-but-unverified, verification-failed and verified work.
3. Add explicit controlled Mission/task pause/resume/retry only on top of that transition model; enforce attempt limits from durable lifecycle counters and never double-count accepted usage.
4. Expand crash/restart coverage before any mutation-capable Builder/worktree phase.
5. Only after durable Mission continuation is sound, proceed to Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
