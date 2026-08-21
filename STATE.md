# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `e330235ddbd7110fabe3f8cf4cb3936d6d974243`  
**Last merged functional PR:** #62 `feat: persist mission task completion evidence`  
**Active work:** PR #63 `feat: persist mission attempts and verification state`, branch `feat/mission-attempt-verification-state`  
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

Desktop Mission telemetry is bounded and ephemeral. `/api/status` attaches richer Mission payload only to the matching execution-scoped `RunID`; the Output inspector is observation-only and cannot start, authorize or resume work. Orchestration diagnostics distinguish backend/model availability, queue pressure and true resource saturation. Diagnostics never alter Scheduler policy or concurrency.

### Android / Mobile Remote

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, orchestration diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current scheduled Child dispatch is synchronous; higher configured model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Phase 6 recovery foundation merged through PR #62

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`, not in a second journal.

Durable Mission checkpoint data includes stable Mission identity, objective, direct project scope, bounded constraints/success criteria, Mission budget, DAG/task identity and state, requested/granted capabilities, model, task budget, scheduler resource/queue/running/budget snapshots, terminal Mission state/reason, Mission accounting and scheduler-accepted per-task usage.

Raw Child/model result text, findings and tool transcripts are deliberately excluded. Secret-like free text uses the existing run-journal sanitization and bounds. The normal chat `Weiter`/`Continue` path refuses Mission journal entries, so structured Mission work cannot accidentally replay as an ordinary prompt.

PR #61 added restart reconciliation before any future Mission resume. Mission start records bounded canonical project/Git identity evidence: hashed project/root identity, exact `HEAD`, hashed porcelain worktree state and timestamp. On restart, interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is always `interrupted_unknown`; durable successful work requires postcondition verification before reuse. Reconciliation is observation only.

PR #62 added bounded successful-task completion evidence at scheduler-authoritative checkpoints. For the first accepted successful terminal task, the journal stores result status, SHA-256 digest of the structured in-memory `AgentResult`, fixed structure counts, verification state `unverified` and completion timestamp. Raw Summary/Findings/Evidence/file paths/test details/risk text/suggested-task objectives are not copied into the journal. First accepted completion evidence is immutable and survives the terminal Mission graph rebuild.

No automatic Mission resume, retry, replay or postcondition verification execution is merged through PR #62.

## 4. Active PR #63 – durable attempts and verification-state records

PR #63 extends task recovery metadata without adding execution authority.

### Task lifecycle accounting

Each Mission task can persist a bounded lifecycle record:

- `AttemptCount`, incremented only when the Scheduler transitions a task from not-running to running,
- `RetryCount = max(AttemptCount - 1, 0)`,
- `StateUpdatedAt`,
- `LastStartedAt`,
- `LastFinishedAt`.

Repeated identical `running` snapshots do not increment attempts. A running-to-not-running transition records the attempt finish timestamp. Lifecycle state is deep-copied with recovery data and preserved across the final Mission graph rebuild. These counters do not themselves authorize a retry and do not change existing usage accounting.

### Verification-state records

Successful-task completion evidence now has a typed verification state. A completion record begins `unverified` and records the initial verification timestamp at completion. The internal outcome recorder accepts only `verified` or `failed` and requires:

- a canonical lowercase SHA-256 verification-evidence digest,
- a bounded verification check count from 1 through 32,
- a verification outcome timestamp.

The record also stores verification-attempt count, latest verification-evidence digest and latest check count. Raw verification output is not persisted. `verified` is terminal and cannot regress to `failed` or `unverified` through this state transition helper.

PR #63 intentionally does **not** include a verification executor. No current product path can automatically run postcondition checks, mark a task verified, resume a Mission or retry a task. The new fields are durable control/recovery state for the next reviewed slice.

Focused tests cover duplicate-running snapshots, retry counting, start/finish timestamps, deep-copy isolation, digest/check-count validation, failed-to-verified verification progression and terminal verified-state non-regression.

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
- Reconciliation, completion evidence, lifecycle counters and verification records are evidence/control state, not execution authority.
- No automatic Mission resume/retry/replay exists.
- A crash-running task is never treated as successful.
- Repeated scheduler snapshots must not double-count task attempts.
- Verification records must not persist raw verification output; `verified` may not silently regress.
- Raw Git porcelain paths and raw Child/model result content are not persisted in recovery evidence.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Diagnostics and benchmark output must not automatically alter Scheduler limits, admission or model concurrency.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle_verification_test.go`, `src/run_journal_mission_evidence_test.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_reconcile_test.go`, `src/run_journal_mission_test.go`, `src/run_journal.go`, `src/run_journal_test.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`.

Benchmarks/diagnostics/UI: `src/agent_orchestration_parallelism_benchmark_test.go`, `src/agent_orchestration_diagnostics_test.go`, `src/static/mission_status.js`, `docs/ORCHESTRATION_BENCHMARKS.md`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #63 on one exact head: require complete Quality success, inspect reviews/threads, mark Ready and merge automatically.
2. Add a controlled read-only postcondition-verification executor that produces bounded verification evidence before a task may become `verified`.
3. Define deterministic restart transitions for queued, ready, blocked, running-at-crash, failed, cancelled, succeeded-but-unverified, verification-failed and verified work.
4. Add controlled Mission/task pause, resume and retry only after reconciliation and required verification; enforce explicit attempt limits using durable counters without double-counting usage.
5. Expand crash/restart coverage before any mutation-capable Builder/worktree phase.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
