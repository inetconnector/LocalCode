# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current functional master before this feature branch:** `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`  
**Last merged feature:** PR #40 `feat: add bounded native agent team roles`  
**Last merged bootstrap/state refresh:** PR #41, merge `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`  
**Active feature branch:** `feat/native-agent-task-dag`  
**Active draft PR:** #42 `feat: add deterministic native agent task DAG`  
**Permanent PR head before this final STATE-only refresh:** `18fff83fa6c020f48da7042a39eee45d613f543f`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Issue #23:** completed after PR #40 verification  
**Current implementation slice:** #32 Phase 4 – deterministic Task DAG + Planner graph validation/state semantics; implementation complete, final exact-head Quality/merge gate pending  
**Canonical unfinished-work ledger:** `TODO.md`

`STATE.md` is the authoritative self-contained bootstrap description of what is true now. `TODO.md` is the exhaustive list of unfinished functional work. Git history and closed PRs/issues are history; neither file may accumulate contradictory snapshots.

---

## 0. Permanent bootstrap + maintenance invariant

`STATE.md` and `TODO.md` MUST remain completely current and mutually consistent.

**Self-contained AI bootstrap invariant:** a newly started AI with no chat history, no memory and no prior context must be able to read `STATE.md` and understand the project well enough to resume implementation immediately, safely and correctly. `STATE.md` must therefore contain the current master/branch/PR/head/CI/review reality, product goal, architecture, important files/entrypoints, safety/Quality invariants, relevant implemented capabilities, active work, known problems/failed approaches, open decisions and the exact next implementation step. Important facts from other files must be summarized here with those source files named; bare links are not a substitute for continuation context.

Blocking rules:

1. Before material work read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
2. Verify GitHub reality before resuming: `master`, active branch/PR, open issues, exact head, reviews/threads and latest required Quality run.
3. After every material branch/base/head change, PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, refresh `STATE.md` and `TODO.md` in the same workstream or immediately afterward.
4. `STATE.md` describes current implemented/verified reality; `TODO.md` contains only unfinished functional work, dependencies and acceptance gates.
5. Replace stale facts instead of appending contradictory snapshots.
6. A change is not operationally complete while either file describes the previous repository reality.
7. Exact self-commit/merge SHAs cannot be predicted from inside the commit that records them. Record the verified functional/documentation baseline honestly; do not create recursive STATE-only updates merely to record the SHA of this final STATE refresh itself.

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

Target UMAF-LC orchestration architecture:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> Worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Agent identity should remain data-driven rather than a large rigid class hierarchy:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Logical task parallelism must be separated from model-inference parallelism so many logical tasks can exist without requiring equally many simultaneous local-model contexts.

### Important continuation files / code map

- `AGENTS.md` – repository-wide coding, safety, localization and STATE/TODO maintenance rules.
- `STATE.md` – this self-contained current-state bootstrap.
- `TODO.md` – exhaustive unfinished functional work and acceptance gates; Phase 4 now contains only final Quality/merge work, with Phase 5 as the next feature.
- `README.md` – user-facing DE/EN feature/usage contract; documents the Native Agent Task DAG.
- `docs/ARCHITECTURE.md` – architecture boundaries; documents Phase-4 DAG semantics and later-layer boundaries.
- `docs/SECURITY.md` – security model; explicitly states that requested capabilities/dynamic role labels do not grant authority.
- `src/agent.go` – main Native agent action schema/tool loop and dispatch integration.
- `src/agent_supervisor.go` – intent/supervisor action eligibility and control logic.
- `src/edit_reliability.go` – deterministic edit preflight/reliability path.
- `src/subagent.go` – deterministic read-only repository handoff/fallback path.
- `src/agent_team_types.go` – AgentTask/role/capability/budget/result contracts; Phase 4 adds graph states, proposal IDs, state reasons and requested-vs-granted capabilities.
- `src/subagent_model.go` – bounded model-backed read-only Explorer/Planner/Reviewer runtime; Planner outputs are validated into machine-readable DAGs before acceptance.
- `src/subagent_model_test.go` – child-role/budget/fallback/Planner graph regression tests.
- `src/agent_task_graph.go` – Phase-4 deterministic DAG builder, validator, dependency reconciliation and transition logic.
- `src/agent_task_graph_test.go` – DAG/state safety regression tests.
- `src/server.go` and project/remote API files – Desktop/local HTTP and Mobile Remote routing/security boundaries.
- `src/static/ui_polish.js` – Desktop UI behavior/polish and project UX.
- `src/static/remote.html` – narrow Mobile Remote UI.
- `.github/workflows/quality.yml` – mandatory Windows merge gate; restored to the same permanent content as `master`.

When adding orchestration code, prefer new focused files rather than expanding `agent.go` into a monolith. Preserve the existing single-agent path as a compatibility path while orchestration is added above it.

---

## 2. Permanent safety / correctness baseline

Current `master` and PR #42 preserve:

- project-root containment and symlink/path-escape protections
- SHA-256 file-version preconditions
- approval bound to the previewed file version
- per-path locking
- same-directory staging and atomic replacement/move behavior
- conflict instead of silent overwrite after external modification
- checked mutation postconditions and backup/Git recovery paths
- process-tree timeout/cancellation for owned external processes
- no default or persistent `danger-full-access` equivalent
- Mobile permissions narrower than Desktop
- no silent provider/model drift
- secrets not intentionally persisted/logged/displayed
- durable redacted active-run journal and interrupted-run recovery without blind mutation replay
- session-wide repeated-action/result and short-cycle stagnation detection
- explicit no-op mutation rejection
- read-only LSP navigation without implicit mutation authority

Never bypass these through orchestration, dynamic roles, capability requests or future worktrees.

Future mission persistence must extend/integrate with the existing durable run journal rather than create a competing recovery authority.

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
- Reviewer independent from Builder reasoning
- deterministic read-only fallback when model unavailable/fails or budget is exhausted
- deterministic mandatory edit-reliability preflight
- visible `subagent:<role>:<action>` traces

Issue #23 is closed/completed because #40 satisfied its bounded read-only model-subagent requirements without stale-merging historical PR #14.

---

## 4. PR #42 – deterministic Task DAG foundation

Branch: `feat/native-agent-task-dag`, based on master `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`.

Implementation present on PR #42:

- `AgentTaskProposal` has a stable explicit `ID`.
- `AgentTask` has graph-specific `StateReason` and `RequestedCapabilities` separate from actually granted `Capabilities`.
- Graph states include `proposed`, `ready`, `succeeded`, `cancelled`, `retryable`; legacy `pending/running/completed/blocked/failed` remain valid for compatibility.
- `AgentTaskGraph` carries a mission ID and task nodes.
- Validation rejects invalid IDs/role labels, duplicate task IDs/dependencies, missing dependencies, self-dependencies, dependency cycles, mission mismatch, invalid states and invalid parent semantics.
- Dynamic Planner role labels such as `kernel-memory-specialist` remain inert plan data; executable Native child roles remain restricted by the existing runtime.
- Planner-requested capabilities are copied only into `RequestedCapabilities`; DAG construction leaves granted `Capabilities` empty.
- Dependency reconciliation deterministically derives ready/blocked state.
- Successful dependencies release dependents; failed/cancelled dependencies block them with structured state reasons.
- Controlled transitions cover ready -> running -> succeeded/completed/failed/cancelled/retryable and reject unsafe/terminal restarts.
- Planner structured schema requires `id`, `role`, and `objective` for every suggested task.
- Planner dependencies refer to stable task IDs rather than role prose.
- On Planner `finish`, a non-empty proposal set is converted to `AgentTaskGraph`; invalid/missing/cyclic graphs are rejected inside the existing bounded child loop and a corrected structured finish result is requested. Existing model/time/token budgets still apply.
- Formatted Planner results include validated machine-readable `task_graph` data.
- Focused tests cover invalid identifiers, duplicate/missing/self/cyclic dependencies, inert dynamic roles/requested capabilities, deterministic parallel readiness, dependency release, legacy completed compatibility, failure/cancellation propagation, retry, unsafe transition rejection and invalid-Planner-graph correction.
- Focused DAG/Planner tests passed before the integration commit was pushed.
- `README.md`, `docs/ARCHITECTURE.md`, and `docs/SECURITY.md` contain DE/EN documentation of the DAG and its non-escalating security boundary.
- All temporary integration workflows/scripts have been removed. `.github/workflows/quality.yml` is standard again.
- `TODO.md` was refreshed on commit `18fff83fa6c020f48da7042a39eee45d613f543f` so implemented Phase-4 work is no longer listed as unfinished.

Intentionally NOT implemented by PR #42:

- asynchronous scheduler/resource queues
- model-inference concurrency management
- persistent Mission Manager / durable task graph storage
- mutation-capable Builder agents
- Git-worktree isolation
- Integrator/Test-Agent mutation orchestration
- Mobile permission expansion
- QEMU/OS mission execution

### Recent implementation notes / failed approaches

During development, temporary PR-specific workflow helpers were attempted. Early transient runs failed safely before product-code commits because of a wrong `go.mod` path and then overly brittle multiline PowerShell anchors. A later transient Quality run #417 exposed the real `gofmt` requirement in `src/agent_task_graph.go`. The final integration used exact branch-bound assertions plus `gofmt`, `git diff --check` and focused tests before push. All temporary workflows/scripts were subsequently removed. Do not recreate them; they are not part of the product architecture.

The permanent implementation/documentation/TODO baseline immediately before this final STATE-only refresh is `18fff83fa6c020f48da7042a39eee45d613f543f`. This STATE refresh necessarily creates one later documentation-only head. Use that resulting head for the final Quality run and merge checks; do not create another STATE-only commit merely to record its own SHA.

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

## 6. Remaining roadmap

### Issue #32 – UMAF-LC / Native orchestration

Issue #32 remains open. Phase 1–3 foundation is merged via #40. Phase 4 is PR #42 and is functionally implemented; only final exact-head Quality/merge remains.

After #42 the intended sequence is:

1. Phase 5 scheduler/resource manager separating logical parallelism from model-inference parallelism, with deterministic bounded queues, resource classes and mission/task budgets.
2. Phase 6 durable missions + recovery integrated with the existing run journal.
3. Phase 7 isolated Git-worktree mutation agents under normal LocalCode approvals/preconditions.
4. Phase 8 Integrator + Test Agent + independent Reviewer loop.
5. Phase 9 dynamic Agent Factory, constrained spawning/replanning and mission-level stagnation controls.
6. Deferred tool discovery/context economy, typed project commands, broader safe MCP transports and Doctor diagnostics.
7. Reproducible multi-agent benchmark expansion against Aider/OpenCode/Claw.
8. OS/QEMU challenge only after the underlying primitives are stable.

### Issue #30 – llama.cpp / DMC backend

Issue #30 remains open and follows the main orchestration foundation. Ollama stays default. A llama.cpp backend must live below the supervisor, remain local/loopback by default, make provider/model selection explicit, and must not be labeled DMC-enabled until runtime markers/self-tests prove real DMC KV selection/rehydration.

### Repository hygiene

Issue #23 is completed. Issues #22 and #25 still require reconciliation against merged work and may be closed only after their acceptance criteria are verified.

---

## 7. Exact next action

1. Treat the head created by this final STATE-only refresh as the only PR #42 merge candidate.
2. Run the complete standard Windows Quality workflow on that exact head.
3. If any gate fails, fix only the concrete failure and refresh STATE/TODO only if repository reality changes materially.
4. When Quality is fully green, verify PR #42 is 0 behind current `master`, mergeable, exact head unchanged, with no blocking reviews or unresolved review threads.
5. Mark #42 ready and merge only with `expected_head_sha`.
6. Immediately refresh `STATE.md` and `TODO.md` for the resulting master using the functional-baseline/self-SHA convention.
7. Then create a fresh current-master branch for Phase 5 scheduler/resource-manager foundation. Do not add worktree mutation in that same slice.
