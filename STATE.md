# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `d72d2d1b14dc50bb6e5fce4f3d2fe1174d51c5ed`  
**Last merged functional PR:** #60 `feat: persist durable Mission metadata in run journal`  
**Active work:** PR #61 `feat: reconcile interrupted missions on restart`, branch `feat/mission-restart-reconciliation`  
**Active head:** the commit containing this `STATE.md` refresh; its immediate source/documentation parent before this self-referential refresh was `784b15ac9af8dfa24a82ff1c97ee2c125890c121`. The exact resulting PR head is verified from GitHub immediately before Quality/merge.  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, context compaction, local memory boundaries and durable normal-run recovery through `run_journal.go`.

Desktop Mission telemetry remains bounded and ephemeral. `/api/status` attaches richer Mission payload only to the matching execution-scoped `RunID`; the Output inspector is observation-only and cannot start, authorize or resume work.

Orchestration diagnostics distinguish backend/model availability, queue pressure and true resource saturation. `at_capacity` means a resource is full; `saturated` additionally requires matching waiting work. Diagnostics never alter Scheduler policy or concurrency.

### Android / Mobile Remote

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, orchestration diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current scheduled Child dispatch is synchronous; higher configured model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Merged Phase 6 foundation from PR #60

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`, not in a second journal.

Durable Mission checkpoint data includes stable Mission identity, objective, direct project scope, bounded constraints/success criteria, Mission budget, DAG/task identity and state, requested/granted capabilities, model, task budget, scheduler resource/queue/running/budget snapshots, terminal Mission state/reason, Mission accounting and scheduler-accepted per-task usage.

Raw Child/model result text, findings and tool transcripts are deliberately excluded. Secret-like free text uses the existing run-journal sanitization and bounds. The normal chat `Weiter`/`Continue` path refuses Mission journal entries, so structured Mission work cannot accidentally replay as an ordinary prompt.

## 4. Active PR #61 – restart reconciliation before any future Mission resume

PR #61 adds observation and classification only. It does **not** add resume, retry, replay, new capabilities or Scheduler authority.

Baseline captured when a read-only Mission begins:

- canonical project identity as SHA-256,
- Git observation state (`observed`, `not_repository`, `unavailable`),
- SHA-256 of Git repository-root identity rather than a new raw path,
- exact Git `HEAD`,
- SHA-256 of `git status --porcelain=v1 -z --untracked-files=all`, never the raw status/path list,
- capture timestamp.

The Git observer is private and fixed-function. It executes only three hard-coded read-only commands with a three-second timeout: `rev-parse --show-toplevel`, `rev-parse --verify HEAD`, and porcelain status. It does not accept arbitrary command text and does not mutate Git state.

On startup, an interrupted Mission is compared against the current project/Git observation and receives a structured reconciliation state:

- `matched` – project identity, Git root, HEAD and worktree-status hash still match,
- `project_unavailable` – the original project directory is no longer available,
- `project_mismatch` – project/Git repository identity changed,
- `git_changed` – HEAD or worktree state changed,
- `git_unavailable` – current Git evidence cannot be obtained although the baseline had it,
- `insufficient_evidence` – for example an older journal without the new baseline or a non-Git/unobservable baseline.

Task dispositions are conservative:

- durable `failed`/`cancelled` tasks stay terminal,
- a task whose durable `Running` flag is true or whose state is `running` at interruption is always `interrupted_unknown` and is never inferred successful,
- durable `succeeded`/legacy `completed` requires `verify_postconditions` even when project/Git match,
- pending/not-started work is only classified `pending` when the overall project/Git reconciliation matched,
- otherwise potentially reusable/pending work is `blocked_reconciliation`.

Interrupted Mission recovery remains visible even if the project path disappeared; ordinary non-Mission recovery retains the old requirement that the project directory still exist. The startup card exposes the reconciliation state in synchronized DE/EN wording and reiterates that no automatic resume occurs.

Focused tests cover exact task dispositions including a stale/inconsistent durable `Running` flag, Git drift, legacy/missing baseline, privacy of raw porcelain paths, the fixed read-only Git command set, and missing-project recovery visibility.

Canonical documentation has been synchronized in `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `STATE.md` and `TODO.md`. The final PR head must pass the complete Quality workflow before Ready/merge.

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
- Reconciliation is evidence, not execution authority. No automatic Mission resume/retry/replay exists.
- A crash-running task is never treated as successful, including when a stale durable `Running` flag conflicts with its state field.
- Raw Git porcelain paths are not persisted in the Mission baseline; only bounded hashes/HEAD evidence is stored.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Diagnostics and benchmark output must not automatically alter Scheduler limits, admission or model concurrency.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_reconcile_test.go`, `src/run_journal_mission_test.go`, `src/run_journal.go`, `src/run_journal_test.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`.

Benchmarks/diagnostics/UI: `src/agent_orchestration_parallelism_benchmark_test.go`, `src/agent_orchestration_diagnostics_test.go`, `src/static/mission_status.js`, `docs/ORCHESTRATION_BENCHMARKS.md`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #61 on this frozen source/documentation scope: require complete Quality success for the exact final GitHub head, inspect reviews/threads, mark Ready and merge automatically.
2. Persist remaining recovery semantics needed for safe continuation: bounded task result/postcondition evidence, attempts/retry counters, verification state and relevant timestamps.
3. Add controlled Mission/task pause, resume and retry only on top of reconciled state; preserve cancel semantics, resource limits and usage accounting without double-counting.
4. Expand crash/restart cases for queued, ready, running, failed and partially completed Mission work.
5. Only after durable Mission recovery is sound, implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
