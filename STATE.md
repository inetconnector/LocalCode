# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified functional master before this documentation-only state update:** `c576f27cf75b642987aa56c7227840a133d00e07`  
**Last merged feature:** PR #42 `feat: add deterministic native agent task DAG`  
**PR #42 merge:** `c576f27cf75b642987aa56c7227840a133d00e07`  
**Final tested PR #42 head:** `9bbd616d054c767030a6f6e7f0c89b8da005c545`  
**Quality on final PR #42 head:** #433 – success; total statement coverage **80.2%**  
**Documentation refresh carrier:** PR #43 `docs: refresh canonical state after task DAG merge`, branch `docs/state-after-task-dag`  
**Carrier content baseline before this self-resolving metadata update:** `47123d2a16e779ff3c4e13d97d0d4118d24d5707`  
**Open implementation PR:** none at this functional snapshot  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Immediate functional priority after this documentation refresh reaches `master`:** #32 Phase 5 – Scheduler / Resource Manager foundation  
**Canonical unfinished-work ledger:** `TODO.md`

**Self-resolving carrier rule:** when this file is read on `docs/state-after-task-dag`, PR #43 is the active documentation carrier. When this exact content is read from `master` after PR #43, the carrier is already merged by definition. Do not create another documentation PR merely to change “#43 carrier” into “#43 merged”. Always verify current GitHub reality before resuming. Functional branch/PR/CI changes still require normal STATE/TODO refreshes.

`STATE.md` is the authoritative self-contained bootstrap description of what is true now. `TODO.md` is the exhaustive list of unfinished functional work. Git history and closed PRs/issues are history; neither file may accumulate contradictory snapshots.

---

## 0. Permanent bootstrap + maintenance invariant

`STATE.md` and `TODO.md` MUST remain completely current and mutually consistent.

**Self-contained AI bootstrap invariant:** a newly started AI with no chat history, no memory and no prior context must be able to read `STATE.md` and understand the project well enough to resume implementation immediately, safely and correctly. `STATE.md` must therefore contain the current functional master baseline, relevant active branch/PR/CI reality, product goal, architecture, important files/entrypoints, safety/Quality invariants, implemented capabilities, known problems/failed approaches, open decisions and the exact next implementation step. Important facts from other files must be summarized here with those source files named; bare links are not a substitute for continuation context.

Blocking rules:

1. Before material work read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
2. Verify GitHub reality before resuming: current `master`, active implementation branch/PR, open issues, exact head, reviews/threads and latest required Quality run.
3. After every material functional branch/base/head change, implementation PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, refresh `STATE.md` and `TODO.md` in the same workstream or immediately afterward.
4. Documentation-only carrier PRs may use the self-resolving carrier convention above: record the verified functional master before the docs-only update and do not create an infinite chain solely to record the docs PR's own merge SHA.
5. `STATE.md` describes current implemented/verified reality; `TODO.md` contains only unfinished functional work, dependencies and acceptance gates.
6. Replace stale facts instead of appending contradictory snapshots.
7. A material functional change is not operationally complete while either file describes the previous repository reality.
8. Exact self-commit/merge SHAs cannot be predicted from inside the commit that records them. Record verified baselines honestly; never invent a SHA.

---

## 1. Product objective and architecture

LocalCode is a Windows-first local coding-agent/development platform centered on local models and controlled tool execution. Ollama is the default local inference path. LocalCode is the supervisor and safety boundary even when external coding engines are selected.

Selectable coding engines on current functional master:

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

Agent identity remains data-driven:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Logical task parallelism must be separated from model-inference parallelism so many logical tasks can exist without requiring equally many simultaneous local-model contexts. This separation is the core Phase-5 design constraint.

### Important continuation files / code map

- `AGENTS.md` – repository-wide coding, safety, localization and STATE/TODO rules.
- `STATE.md` – this self-contained current-state bootstrap.
- `TODO.md` – exhaustive unfinished functional roadmap and acceptance gates.
- `README.md` – user-facing DE/EN feature/usage contract; documents Native Agent Teams and the Task DAG.
- `docs/ARCHITECTURE.md` – architecture boundaries and Phase-4 DAG semantics.
- `docs/SECURITY.md` – security model; requested capabilities/dynamic role labels do not grant authority.
- `src/agent.go` – main Native agent action schema/tool loop and dispatch integration.
- `src/agent_supervisor.go` – intent/supervisor action eligibility and control logic.
- `src/edit_reliability.go` – deterministic edit preflight/reliability path.
- `src/subagent.go` – deterministic read-only repository handoff/fallback path.
- `src/agent_team_types.go` – reusable AgentTask/role/capability/budget/result contracts, graph states and requested-vs-granted capability separation.
- `src/subagent_model.go` – bounded model-backed read-only Explorer/Planner/Reviewer runtime and Planner DAG handoff.
- `src/subagent_model_test.go` – child-role/budget/fallback/Planner graph regression tests.
- `src/agent_task_graph.go` – deterministic DAG builder/validator, dependency reconciliation, readiness and state transitions. This is the primary backend input to Phase 5.
- `src/agent_task_graph_test.go` – Phase-4 DAG/state safety regression tests.
- `src/run_journal.go` – existing durable run/recovery authority; future mission persistence must integrate here rather than compete with it.
- `src/types.go` – AppState and shared runtime state; inspect carefully before adding scheduler ownership/lifetime.
- `src/server.go` and remote API files – Desktop/local HTTP and Mobile Remote routing/security boundaries.
- `src/static/ui_polish.js` – Desktop UI.
- `src/static/remote.html` – narrow Mobile Remote UI.
- `.github/workflows/quality.yml` – mandatory Windows merge gate.

When adding orchestration code, prefer new focused files rather than expanding `agent.go` into a monolith. Preserve the existing single-agent/read-only-child paths as compatibility paths while orchestration is added above them.

---

## 2. Permanent safety / correctness baseline

Current functional master preserves:

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

Never bypass these through scheduling, dynamic roles, capability requests, resource queues or future worktrees.

No Phase-5 scheduler/resource abstraction may implicitly grant capabilities. `RequestedCapabilities` remains planning/request data; executable `Capabilities` must still come from LocalCode governance. No concurrent unsupervised mutation of the same workspace is allowed. Phase 5 must remain mutation-free for child tasks.

---

## 3. Merged Native Agent Teams foundation – PR #40

PR #40 merged as `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`. Final feature head `9c3b25b1b070d80c075e9b697a9fffe86f0d3184` passed Quality #406.

Merged capability:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- model-backed read-only Explorer, Planner and Reviewer roles
- separate curated child model contexts
- hard model-call/tool-call/time/estimated-token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- mutation, shell, Git, network/web, MCP, installation, memory, approval requests and recursive spawning absent from child schema
- Planner may emit structured follow-up proposals but cannot execute mutation-capable roles
- Reviewer independent from Builder reasoning
- deterministic read-only fallback when model unavailable/fails or budget is exhausted
- deterministic mandatory edit-reliability preflight
- visible `subagent:<role>:<action>` traces

Issue #23 is closed/completed because #40 satisfied its bounded read-only model-subagent requirements without stale-merging historical PR #14.

---

## 4. PR #42 merged – deterministic Native Agent Task DAG

PR #42 `feat: add deterministic native agent task DAG` is complete and merged into `master` as `c576f27cf75b642987aa56c7227840a133d00e07`.

Final tested feature head: `9bbd616d054c767030a6f6e7f0c89b8da005c545`.

Quality #433 passed on that exact final head. The complete Windows gate was green: Go setup/version, gofmt, `go vet ./...`, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, `govulncheck` with no vulnerabilities found, full-stack loopback HTTP integration, complete tests, race detector, coverage, native Windows builds and `git diff --check`. Total statement coverage was **80.2%**. Before merge the PR was 0 behind `master`, mergeable, with no review submissions or unresolved review threads, and it was merged using `expected_head_sha`.

Merged Phase-4 capability:

- `AgentTaskProposal` has stable explicit machine-readable `ID`.
- `AgentTask` has graph-specific `StateReason` and `RequestedCapabilities` separate from actually granted `Capabilities`.
- Graph states include `proposed`, `ready`, `succeeded`, `cancelled`, `retryable`; legacy `pending/running/completed/blocked/failed` remain valid for compatibility.
- `AgentTaskGraph` carries mission ID and task nodes.
- deterministic validation rejects invalid IDs/role labels, duplicate task IDs/dependencies, missing dependencies, self-dependencies, dependency cycles, mission mismatch, invalid states and invalid parent semantics.
- dynamic Planner role labels remain inert plan data; they do not become executable Native roles merely by being proposed.
- requested capabilities are copied only to `RequestedCapabilities`; DAG construction grants no executable capabilities.
- dependency reconciliation derives ready/blocked state deterministically.
- successful dependencies release dependents; failed/cancelled dependencies block them with structured state reasons.
- controlled transitions cover ready -> running -> succeeded/completed/failed/cancelled/retryable and reject unsafe/terminal restarts.
- Planner structured schema requires stable `id`, `role` and `objective` for each suggested task; dependencies reference task IDs rather than prose labels.
- Planner `finish` with proposals builds a validated `AgentTaskGraph`; invalid/missing/cyclic graphs are rejected inside the existing bounded child loop and corrected within the same model/time/token budgets.
- formatted Planner output includes machine-readable `task_graph` data.
- focused tests cover invalid identifiers, duplicate/missing/self/cyclic dependencies, inert dynamic roles/requested capabilities, multiple independent ready tasks, dependency release, legacy completed compatibility, failure/cancellation propagation, retry, unsafe transitions and invalid-Planner-graph correction.
- DE/EN README, architecture and security documentation describe the DAG and its non-escalating security boundary.

Intentionally still absent after #42:

- asynchronous scheduler/resource queues
- model-inference concurrency management
- persistent Mission Manager / durable task graph storage
- mutation-capable Builder agents
- Git-worktree isolation
- Integrator/Test-Agent mutation orchestration
- Mobile permission expansion
- QEMU/OS mission execution

### Historical development note

During #42 development, temporary PR-specific workflow helpers were used to overcome connector-only editing limitations. Early transient runs failed safely due a wrong `go.mod` path and brittle multiline PowerShell anchors; transient Quality #417 exposed a real `gofmt` requirement. The eventual integration used exact assertions, gofmt, diff checks and focused tests before push. All temporary helpers were removed before the final tested head. Do not recreate them as product architecture.

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

Issue #32 remains open. Phase 1–3 foundation is merged via #40. Phase 4 Task DAG is merged via #42. The immediate next feature is Phase 5 Scheduler / Resource Manager.

Phase-5 objective: represent many logically ready tasks while bounding actual executor resources — especially local-model inference — independently. A large DAG must not imply many simultaneous model contexts on local GPU/CPU resources.

Preferred first Phase-5 slice:

- backend-only scheduler/resource contracts over the merged `AgentTaskGraph`
- deterministic bounded logical ready queue
- explicit resource classes and concurrency limits, with model inference separate from cheap CPU/read/search work
- conservative model-inference default (single active model slot unless explicitly configured later)
- mission/task budget accounting and structured remaining/exhausted state
- cancellation propagation and deterministic queue ordering/fairness tests
- failed/cancelled/blocked DAG nodes consume no executor slot until genuinely ready/retried
- preserve current single-agent/read-only child execution compatibility
- no Builder mutation, worktrees, Integrator mutation or Mobile permission widening in this first scheduler slice

Later #32 sequence:

1. Phase 5 scheduler/resource manager.
2. Phase 6 durable missions + recovery integrated with existing run journal.
3. Phase 7 isolated Git-worktree mutation agents under normal LocalCode approvals/preconditions.
4. Phase 8 Integrator + Test Agent + independent Reviewer loop.
5. Phase 9 constrained dynamic Agent Factory, replanning and mission-level stagnation controls.
6. Deferred tool discovery/context economy, typed project commands, broader safe MCP transports and Doctor diagnostics.
7. Reproducible multi-agent benchmark expansion against Aider/OpenCode/Claw.
8. OS/QEMU challenge only after underlying orchestration/recovery/integration primitives are stable.

### Issue #30 – llama.cpp / DMC backend

Issue #30 remains open and follows the main orchestration foundation. Ollama stays default. A llama.cpp backend must live below the supervisor, remain local/loopback by default, make provider/model selection explicit, and must not be labeled DMC-enabled until runtime markers/self-tests prove real DMC KV selection/rehydration.

### Repository hygiene

Issue #23 is completed. Issues #22 and #25 still require reconciliation against merged work and may be closed only after their acceptance criteria are verified.

---

## 7. Exact next functional implementation step

After this documentation refresh is merged to `master`:

1. Verify current `master` contains #42 merge `c576f27cf75b642987aa56c7227840a133d00e07` plus the self-resolving PR #43 docs carrier and that no competing implementation PR appeared.
2. Read current `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, then inspect `src/agent_task_graph.go`, `src/agent_team_types.go`, `src/subagent_model.go`, `src/types.go` and existing cancellation/budget helpers.
3. Create a fresh branch from the then-current `master` for #32 Phase 5; do not reuse `feat/native-agent-task-dag`.
4. Implement the smallest safe scheduler/resource-manager foundation in focused new files. Start backend-only; do not add worktrees/mutation.
5. Deterministically separate logical ready-task queueing from resource admission. Multiple graph tasks may be logically ready while only the configured number of model-inference tasks can execute.
6. Add explicit bounded resource classes/limits and conservative defaults; model inference should default to one slot.
7. Add structured mission/task resource/budget snapshots, cancellation propagation, deterministic ordering and fairness/starvation tests.
8. Preserve all existing single-agent/read-only-child behavior and safety boundaries.
9. Update DE/EN docs and STATE/TODO with the exact implemented slice.
10. Full exact-head Quality, pre-merge review/behind checks, SHA-bound merge, then immediate STATE/TODO refresh before the next Phase-5 increment.

`TODO.md` contains the exhaustive remaining functional roadmap and acceptance criteria.
