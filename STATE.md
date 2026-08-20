# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current functional master:** `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`  
**Last merged feature:** PR #40 `feat: add bounded native agent team roles`  
**PR #40 merge:** `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`  
**Final tested PR #40 head:** `9c3b25b1b070d80c075e9b697a9fffe86f0d3184`  
**Quality on final PR #40 head:** #406 – success  
**Current documentation refresh branch:** `docs/state-todo-after-native-agent-teams`  
**Active documentation PR:** #41 `docs: refresh bootstrap state and TODO after native agent teams`  
**Documentation content baseline before this PR-metadata sync:** `143bd1abfba147434d42820f7f39b1fb0b58d81a`  
**Open implementation PR:** none at this snapshot  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Immediate implementation priority after this documentation refresh:** #32 Phase 4 – Task DAG + dependency validation/scheduling foundation  
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
- `src/subagent.go` – existing deterministic read-only repository handoff/fallback path.
- `src/agent_team_types.go` – merged #40 AgentTask/role/capability/budget/result contracts; primary starting point for Task-DAG work.
- `src/subagent_model.go` – merged #40 isolated model-backed read-only Explorer/Planner/Reviewer runtime.
- `src/subagent_model_test.go` – regression tests for child roles, budgets, fallback and structured results.
- `src/server.go` and project/remote API files – Desktop/local HTTP and Mobile Remote routing/security boundaries.
- `src/static/ui_polish.js` – Desktop UI behavior/polish and project UX.
- `src/static/remote.html` – narrow Mobile Remote UI.
- `.github/workflows/quality.yml` – mandatory Windows merge gate.

When adding Task-DAG/scheduler code, prefer new focused files rather than expanding `agent.go` into a monolith. Preserve the existing single-agent path as a compatibility path while orchestration is added above it.

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

PR #38 merge: `f84a38f25a09231fb973022867e9177e8de04974`.

### External engine / benchmark baseline

Claw Code is a managed optional engine (PR #28 merge `92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`) with pinned/version-verified executable, process-scoped Ollama host, cloud credential stripping, read-only/workspace-write boundaries and no default danger-full-access.

The repository also contains a cross-engine benchmark harness that isolates runs in fresh worktrees and records timings/checks/diff metrics. Same commit/model/task/context/hidden tests are required for fair comparisons.

---

## 3. PR #40 merged – Native Agent Teams foundation

PR #40 is complete and merged into `master` as `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`.

Final feature head: `9c3b25b1b070d80c075e9b697a9fffe86f0d3184`.

Quality #406 passed on that exact final head with all required gates: gofmt, vet, JS syntax, PowerShell syntax, Android Remote APK, vulnerability scan, full-stack loopback integration, complete tests, race detector, statement coverage >=80%, native Windows builds and final diff check. Before merge the branch was 0 behind `master`, mergeable and had no review submissions or unresolved review threads.

Merged capability:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- model-backed read-only Explorer, Planner and Reviewer roles
- separate curated child model contexts
- hard model-call/tool-call/time/estimated-token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- mutation, shell, Git, network/web, MCP, installation, memory, approval requests and recursive spawning absent from the child schema
- Planner may emit structured follow-up task proposals but cannot execute mutation-capable roles
- Reviewer is independent and intended to receive explicit task/evidence, not builder self-justification
- deterministic read-only fallback when model unavailable/fails or budget exhausted
- mandatory edit-reliability preflight remains deterministic and consumes no child-model calls
- child tool steps surface as `subagent:<role>:<action>` events
- DE/EN README, architecture and security documentation updated

Intentionally not implemented yet: Task DAG validation/state machine/scheduling, mission persistence, resource manager, mutation-capable Builder agents, Git-worktree isolation, Integrator/Test-Agent orchestration, dynamic Agent Factory/replanning, OS/QEMU mission execution.

Historical PR/branch #14 was not stale-merged wholesale; only useful current-compatible read-only model-subagent design was ported.

---

## 4. Permanent Quality contract

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

## 5. Roadmap state

### Issue #32 – UMAF-LC / Native orchestration

Issue #32 remains open. Phase 1–3 foundation is merged via #40.

Target architecture:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> Worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Do not build dozens of hard-coded agent classes. Agent identity remains data-driven:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Logical task parallelism must be separated from model-inference parallelism so many logical tasks can exist without loading many local model contexts simultaneously.

### Issue #30 – llama.cpp / DMC backend

Issue #30 remains open and follows the main orchestration foundation. Ollama stays default. A llama.cpp backend must live below the supervisor, be local/loopback by default, explicit about provider/model selection and process lifecycle, and must not be labeled DMC-enabled until runtime markers/self-tests prove real DMC KV selection/rehydration.

### Repository hygiene

Open issues #22, #23 and #25 are likely superseded by merged work and must be reconciled/closed only after their acceptance criteria are verified. #23 should be checked first after this state refresh because #40 appears to cover its bounded model-backed read-only subagent request.

---

## 6. Exact next implementation step

Current workstream: finish documentation PR #41 on exact-head Quality and merge it into `master`.

After #41 is merged:

1. Verify/close issue #23 if every acceptance item is satisfied by #40.
2. Start a fresh branch from then-current `master` for #32 Phase 4; do not continue on `feat/native-agent-teams`.
3. First Phase-4 slice is **Task DAG + dependency validation only**, with no mutation/worktrees yet.
4. Extend `AgentTask`/adjacent focused types with mission/parent/dependency/task-state semantics while preserving existing single-agent/read-only-child paths.
5. Deterministically reject duplicate IDs, missing dependencies and cycles.
6. Add task states such as proposed/blocked/ready/running/succeeded/failed/cancelled/retryable plus dependency release/failure propagation.
7. Convert Planner `SuggestedTasks` into validated machine-readable DAG proposals; do not parse prose.
8. Add focused tests for duplicate IDs, missing dependencies, cycles, dependency release, failure propagation and multiple independent ready tasks.
9. Do not add Builder mutation, worktrees, asynchronous persistence or broad scheduler concurrency in this first slice.
10. Full Quality on exact head; merge only after 0-behind/mergeability/review checks; immediately refresh STATE/TODO again.

Then proceed to scheduler/resource manager separation of logical queues from model-inference concurrency and mission/task budgets.

`TODO.md` contains the exhaustive remaining roadmap and acceptance criteria.
