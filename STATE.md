# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current functional master before this feature branch:** `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`  
**Last merged feature:** PR #40 `feat: add bounded native agent team roles`  
**Last merged bootstrap/state refresh:** PR #41, merge `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`  
**Active feature branch:** `feat/native-agent-task-dag`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Issue #23:** completed after verification that PR #40 fully superseded its bounded read-only model-subagent acceptance criteria  
**Current implementation slice:** #32 Phase 4 – deterministic Task DAG + dependency validation/state semantics only  
**Canonical unfinished-work ledger:** `TODO.md`

`STATE.md` is the authoritative self-contained bootstrap description of what is true now. `TODO.md` is the exhaustive list of unfinished work. Git history and closed PRs/issues are history; neither file may accumulate contradictory snapshots.

---

## 0. Permanent bootstrap + maintenance invariant

`STATE.md` and `TODO.md` MUST remain completely current and mutually consistent.

**Self-contained AI bootstrap invariant:** a newly started AI with no chat history, no memory and no prior context must be able to read `STATE.md` and understand the project well enough to resume implementation immediately, safely and correctly. `STATE.md` must therefore contain the current master/branch/PR/head/CI/review reality, product goal, architecture, important files/entrypoints, safety/Quality invariants, relevant implemented capabilities, active work, known problems/failed approaches, open decisions and the exact next implementation step. Important facts from other files must be summarized here with those source files named; bare links are not a substitute for continuation context.

Blocking rules:

1. Before material work read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
2. Verify GitHub reality before resuming: `master`, active branch/PR, open issues, exact head, reviews/threads and latest required Quality run.
3. After every material branch/base/head change, PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, refresh `STATE.md` and `TODO.md` in the same workstream or immediately afterward.
4. `STATE.md` describes current implemented/verified reality; `TODO.md` contains only unfinished work, dependencies and acceptance gates.
5. Replace stale facts instead of appending contradictory snapshots.
6. A change is not operationally complete while either file describes the previous repository reality.
7. Exact self-commit/merge SHAs cannot be predicted from inside the commit that records them. Record verified baselines honestly and update resulting SHAs in the next current-state refresh; never invent a SHA.

---

## 1. Product objective and architecture

LocalCode is a Windows-first local coding-agent/development platform centered on local models and controlled tool execution. Ollama is the default local inference path. LocalCode is the supervisor and safety boundary even when external coding engines are selected.

Selectable coding engines on current `master`:

- LocalCode Native
- Aider
- Claude Code
- OpenCode
- Claw Code

Long-term objective: make LocalCode Native objectively stronger than Aider/OpenCode/Claw in repository intelligence, edit reliability, safety, recovery, orchestration, context efficiency, verification and reproducible benchmark success. Do not claim parity/superiority without measured evidence.

High-level runtime:

`User/UI -> LocalCode supervisor -> Native agent or selected external engine -> controlled tools -> project/Git/build/test/network/MCP boundaries -> verification/recovery`

The application is a Go program with embedded web UI, local HTTP/SSE backend, Ollama/model integration, project management, agent loop, structured tools, Git/build/deploy, repository intelligence, LSP, MCP, skills/commands, persistent memory/history, durable run recovery and a paired Mobile Remote surface.

### Important continuation files / code map

- `AGENTS.md` – repository-wide coding, safety, localization and STATE/TODO maintenance rules.
- `STATE.md` – this self-contained current-state bootstrap.
- `TODO.md` – exhaustive unfinished work and acceptance gates.
- `README.md` – user-facing feature/usage contract in German and English.
- `docs/ARCHITECTURE.md` – architecture boundaries and component behavior.
- `docs/SECURITY.md` – security model and privilege boundaries.
- `src/agent.go` – main Native agent action schema/tool loop and dispatch integration.
- `src/agent_supervisor.go` – intent/supervisor action eligibility and control logic.
- `src/edit_reliability.go` – deterministic edit preflight/reliability path.
- `src/subagent.go` – deterministic read-only repository handoff/fallback path.
- `src/agent_team_types.go` – AgentTask/role/capability/budget/result contracts from #40; now extended on the active branch with graph-specific task states, stable proposal IDs, state reasons and requested-vs-granted capability separation.
- `src/subagent_model.go` – isolated model-backed read-only Explorer/Planner/Reviewer runtime; active branch is integrating Planner `SuggestedTasks` into validated machine-readable DAGs.
- `src/subagent_model_test.go` – child-role/budget/fallback/structured-result tests.
- `src/agent_task_graph.go` – new Phase-4 deterministic DAG validator/builder/state reconciler on the active branch.
- `src/agent_task_graph_test.go` – new DAG/state safety regression tests on the active branch.
- `src/server.go` and project/remote API files – Desktop/local HTTP and Mobile Remote routing/security boundaries.
- `src/static/ui_polish.js` – Desktop UI behavior/polish and project UX.
- `src/static/remote.html` – narrow Mobile Remote UI.
- `.github/workflows/quality.yml` – mandatory Windows merge gate.

When adding orchestration code, prefer new focused files rather than expanding `agent.go` into a monolith. Preserve the existing single-agent path as a compatibility path while orchestration is added above it.

---

## 2. Current merged capability baseline

### Safety / mutation correctness

Current master preserves:

- project-root containment and symlink/path-escape protections
- SHA-256 file-version preconditions
- approval bound to the previewed file version
- per-path locking
- same-directory staging and atomic replacement/move behavior
- conflict instead of silent overwrite after external modification
- checked mutation postconditions
- backup/Git recovery paths
- process-tree timeout/cancellation for owned external processes
- no default or persistent `danger-full-access` equivalent
- Mobile permissions narrower than Desktop
- no silent provider/model drift
- secrets not intentionally persisted/logged/displayed

Never bypass these through orchestration or worktrees.

### Repository intelligence / navigation

Current master includes import-aware repository intelligence, typed/weighted graph relations, Go AST plus Tree-sitter-backed parsing where supported, deterministic fallback, task-ranked context, bounded parallel independent reads, native read-only LSP navigation and persistent/recovering LSP sessions.

LSP is navigation/diagnostic authority only; it is not mutation permission.

### Reliability / recovery

Current master includes:

- durable redacted active-run journal under LocalCode app data
- atomic/mutex-protected journal persistence
- interrupted-run detection and recovery handoff
- no blind mutation replay after crash; project/Git/postconditions must be re-read
- session-wide repeated-action/result and short-cycle stagnation detection
- changed evidence treated as progress
- reset on real mutation/verification
- explicit no-op rejection for identical `write_file`/`replace_text` before pointless approval/backup/write

Future mission persistence must extend/integrate with this recovery authority rather than create a competing journal.

### Project lifecycle / Mobile Remote

Current master has safe project creation/folder creation, reversible same-volume quarantine for confirmed non-empty project deletion, server-generated delete previews, exact confirmations, Desktop/Mobile Trash + Restore, permanent purge with exact `PURGE <project>`, occupied-destination refusal, symlink/path protections, deterministic thread restore, and a Mobile API narrower than Desktop.

### External engine / benchmark baseline

Claw Code is a managed optional engine with pinned/version-verified executable, process-scoped Ollama host, cloud credential stripping, read-only/workspace-write boundaries and no default danger-full-access.

The repository also contains a cross-engine benchmark harness that isolates runs in fresh worktrees and records timings/checks/diff metrics. Same commit/model/task/context/hidden tests are required for fair comparisons.

---

## 3. Merged Native Agent Teams foundation – PR #40

PR #40 merged as `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`. Final feature head `9c3b25b1b070d80c075e9b697a9fffe86f0d3184` passed Quality #406.

Merged capability:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- model-backed read-only Explorer, Planner and Reviewer roles
- separate curated child model contexts
- hard model-call/tool-call/time/estimated-token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- mutation, shell, Git, network/web, MCP, installation, memory, approval requests and recursive spawning absent from the child schema
- Planner may emit structured follow-up task proposals but cannot execute mutation-capable roles
- Reviewer is independent from Builder reasoning
- deterministic read-only fallback when model unavailable/fails or budget is exhausted
- deterministic mandatory edit-reliability preflight
- visible `subagent:<role>:<action>` traces

Issue #23 is closed/completed because #40 satisfied its bounded read-only model-subagent requirements without stale-merging historical PR #14.

---

## 4. Active Phase-4 branch – deterministic Task DAG foundation

Branch: `feat/native-agent-task-dag`, created from master `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`.

Current implemented branch delta before final integration/Quality:

- `AgentTaskProposal` now has a stable explicit `ID`.
- `AgentTask` has graph-specific `StateReason` and `RequestedCapabilities` separate from actually granted `Capabilities`.
- Added graph states: `proposed`, `ready`, `succeeded`, `cancelled`, `retryable`; existing `pending/running/completed/blocked/failed` remain valid for compatibility.
- New `AgentTaskGraph` carries a mission ID and task nodes.
- Deterministic validation rejects invalid IDs/roles, duplicate task IDs, duplicate dependencies, missing dependencies, self-dependencies, dependency cycles, mission mismatch, invalid state and invalid parent semantics.
- Dynamic planner role labels such as `kernel-memory-specialist` remain inert plan data. They do not become executable Native roles merely by being proposed.
- Planner-requested capabilities are recorded as `RequestedCapabilities`; DAG construction grants no executable capabilities.
- Deterministic graph reconciliation derives ready vs blocked state from dependency status.
- Successful dependencies release dependents; failed/cancelled dependencies block them with machine-readable state/reason data.
- Controlled transitions support ready->running->succeeded/completed/failed/cancelled/retryable while terminal/unsafe transitions fail closed.
- Focused tests cover invalid identifiers, duplicate/missing/self/cyclic dependencies, inert dynamic roles/capabilities, deterministic parallel readiness, success release, legacy completed compatibility, failure propagation, retry and terminal-transition rejection.

Integration currently being completed:

- Planner structured output schema must require stable `id` for each suggested task.
- Invalid Planner DAGs must be rejected inside the bounded child-model loop and corrected rather than accepted as prose/invalid data.
- Formatted Planner results must include a validated machine-readable `task_graph` when proposals exist.
- Existing Planner tests must be updated and a correction/retry regression test added.
- Temporary CI patch helper must be removed before the final PR head.
- `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `STATE.md`, and `TODO.md` must describe the final slice accurately.

Intentionally out of scope for this slice:

- no asynchronous execution scheduler
- no model-inference parallel queue/resource manager
- no persistent Mission Manager
- no Builder mutation role
- no Git worktrees
- no Integrator/Test-Agent mutation orchestration
- no Mobile permission expansion
- no QEMU/OS execution

---

## 5. Permanent Quality contract

Every material implementation PR must pass the exact final-head Windows Quality workflow, including at least:

- Go setup/version
- gofmt
- `go vet ./...`
- frontend JavaScript syntax
- PowerShell syntax
- native Android Remote APK build
- `govulncheck`
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- statement coverage >=80.0%
- native Windows builds including GUI path
- `git diff --check`
- exact head/behind/mergeability/review-thread checks before merge

Never lower the 80% threshold or weaken sandbox, approvals, atomic writes, path/symlink protection, Mobile restrictions, cancellation, secret handling or loop guards to rescue CI.

---

## 6. Roadmap state

### Issue #32 – UMAF-LC / Native orchestration

Issue #32 remains open. Phase 1–3 foundation is merged via #40. Phase 4 deterministic DAG foundation is active on `feat/native-agent-task-dag`.

Target architecture remains:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> Worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Do not build dozens of hard-coded agent classes. Agent identity should remain data-driven:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Logical task parallelism must be separated from model-inference parallelism so many logical tasks can exist without loading many local model contexts simultaneously.

### Issue #30 – llama.cpp / DMC backend

Issue #30 remains open and follows the main orchestration foundation. Ollama stays default. A llama.cpp backend must live below the supervisor, be local/loopback by default, explicit about provider/model selection and process lifecycle, and must not be labeled DMC-enabled until runtime markers/self-tests prove real DMC KV selection/rehydration.

### Repository hygiene

Issue #23 is completed. Issues #22 and #25 still require reconciliation against merged work and must only be closed after their acceptance criteria are verified.

---

## 7. Exact next implementation step

From the active branch:

1. Finish Planner schema/loop integration so every suggested task has a stable ID and invalid/missing/cyclic plans are rejected inside the bounded child loop.
2. Include the validated task graph in structured Planner output without granting any requested capability.
3. Run focused DAG/Planner tests and fix only concrete failures.
4. Remove the temporary integration workflow/helper from the branch.
5. Update architecture/security/README documentation in DE/EN as appropriate.
6. Ensure `STATE.md` and `TODO.md` describe the exact final branch/PR/CI state.
7. Open/reuse exactly one draft PR for this Phase-4 slice.
8. Run full required Quality on the exact final permanent head.
9. Verify 0 behind master, exact head, mergeability, reviews and unresolved threads.
10. Merge only with `expected_head_sha`, then refresh `STATE.md`/`TODO.md` for resulting master.

After this slice, the next separate PR is the scheduler/resource-manager foundation separating logical task queues from local-model inference concurrency and enforcing mission/task budgets. Worktree mutation remains later.
