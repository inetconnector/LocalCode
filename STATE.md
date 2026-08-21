# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `6feb2e981bc290f0fd08c642488fc46b1d351c2e`  
**Last merged functional PR:** #61 `feat: reconcile interrupted missions on restart`  
**Active work:** PR #62 `feat: persist mission task completion evidence`, branch `feat/mission-postcondition-evidence`  
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

Desktop Mission telemetry is bounded and ephemeral. `/api/status` attaches richer Mission payload only to the matching execution-scoped `RunID`; the Output inspector is observation-only and cannot start, authorize or resume work. Orchestration diagnostics distinguish backend/model availability, queue pressure and true resource saturation. `at_capacity` means a resource is full; `saturated` additionally requires matching waiting work. Diagnostics never alter Scheduler policy or concurrency.

### Android / Mobile Remote

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, orchestration diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current scheduled Child dispatch is synchronous; higher configured model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Phase 6 recovery foundation merged through PR #61

`run_journal.go` remains the single durable recovery authority. A read-only Mission stores bounded structured metadata in the existing `active-run.json`, not in a second journal.

Durable Mission checkpoint data includes stable Mission identity, objective, direct project scope, bounded constraints/success criteria, Mission budget, DAG/task identity and state, requested/granted capabilities, model, task budget, scheduler resource/queue/running/budget snapshots, terminal Mission state/reason, Mission accounting and scheduler-accepted per-task usage.

Raw Child/model result text, findings and tool transcripts are deliberately excluded. Secret-like free text uses the existing run-journal sanitization and bounds. The normal chat `Weiter`/`Continue` path refuses Mission journal entries, so structured Mission work cannot accidentally replay as an ordinary prompt.

PR #61 added restart reconciliation before any future Mission resume. At Mission start LocalCode records a bounded baseline containing SHA-256 canonical project identity, Git observation state, SHA-256 Git-root identity, exact `HEAD`, SHA-256 of `git status --porcelain=v1 -z --untracked-files=all`, and capture timestamp. Raw porcelain paths are never persisted.

The private Git observer accepts no arbitrary command text and runs only three fixed read-only operations with a three-second timeout: `rev-parse --show-toplevel`, `rev-parse --verify HEAD`, and porcelain status.

On restart, interrupted Missions are classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Task dispositions are conservative: failed/cancelled stay terminal; `Running == true` or state `running` becomes `interrupted_unknown`; durable succeeded/completed work becomes `verify_postconditions` only when project/Git reconciliation matches; pending work becomes `pending` only on a match; otherwise reusable/pending work is `blocked_reconciliation`. A crash-running task is never inferred successful, including stale/inconsistent durable `Running` flags.

Interrupted Mission evidence remains visible even if the project directory disappears. Normal non-Mission recovery retains its existing project-existence requirement. Reconciliation is observation only: no automatic resume, retry or replay exists.

## 4. Active PR #62 – bounded successful-task completion evidence

PR #62 extends the existing Mission task checkpoint with **bounded completion evidence** for scheduler-accepted successful read-only tasks. It does not add resume, retry, replay, postcondition execution, new capabilities, or Scheduler authority.

For the first accepted successful terminal task state (`succeeded`/legacy `completed` with result status `completed` or `fallback`), the journal stores only:

- result status,
- SHA-256 digest of the canonical JSON representation of the in-memory structured `AgentResult`,
- counts of findings, changed files, commits, tests, risks and suggested tasks,
- verification state `unverified`,
- completion checkpoint timestamp.

The raw `AgentResult` is used only transiently to compute the digest/counts. Summary text, Findings/Evidence text, changed-file paths, test details, risk text, suggested-task objectives and other Child/model output are **not** copied into `active-run.json`.

Evidence is checkpointed immediately after scheduler-authoritative task finalization, not only at whole-Mission shutdown. Therefore a later process crash does not erase evidence for an already accepted successful task. Once first captured, the completion evidence is immutable for that task so a stale late result cannot overwrite it. The terminal Mission graph rebuild preserves existing evidence and only fills a missing successful-task evidence record defensively.

`unverified` is explicit: a digest/count record is recovery evidence, not proof that postconditions currently hold. PR #62 intentionally does not change the PR #61 reconciliation disposition `verify_postconditions` and does not create a continuation command.

Focused tests cover raw-result/path non-persistence, structural evidence fields, immutable first accepted evidence, refusal to create evidence for running/failed/non-success result states, and evidence survival across the final graph rebuild.

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
- Reconciliation and completion evidence are evidence, not execution authority. No automatic Mission resume/retry/replay exists.
- Completion-evidence `unverified` must never be interpreted as postcondition success.
- A crash-running task is never treated as successful, including when a stale durable `Running` flag conflicts with its state field.
- Raw Git porcelain paths and raw Child/model result content are not persisted in recovery evidence.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Diagnostics and benchmark output must not automatically alter Scheduler limits, admission or model concurrency.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 6. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_evidence_test.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_reconcile_test.go`, `src/run_journal_mission_test.go`, `src/run_journal.go`, `src/run_journal_test.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`.

Benchmarks/diagnostics/UI: `src/agent_orchestration_parallelism_benchmark_test.go`, `src/agent_orchestration_diagnostics_test.go`, `src/static/mission_status.js`, `docs/ORCHESTRATION_BENCHMARKS.md`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 7. Exact next development direction

1. Finish PR #62 on one exact head: synchronize canonical docs, require complete Quality success, inspect reviews/threads, mark Ready and merge automatically.
2. Persist task attempt/retry counters, explicit verification state transitions and relevant timestamps without turning journal data into executable authority.
3. Define deterministic recovery transitions for queued, ready, blocked, running-at-crash, failed, cancelled, succeeded-but-unverified and verified work.
4. Add controlled Mission/task pause, resume and retry only after reconciliation and required postcondition verification; preserve cancel semantics, resource limits and usage accounting without double-counting.
5. Expand crash/restart coverage for queued, ready, running, failed and partially completed Mission work.
6. Only after durable Mission recovery is sound, implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
