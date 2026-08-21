# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `c49c9fac642a6031f7680c0868687a451e425f55`  
**Last merged functional PR:** #57 `feat: show active read-only Mission in Mobile Remote`  
**PR #57 Quality run:** `32481345080` – complete success including frontend syntax, Android APK, full-stack integration, Go tests, race detector, >=80% coverage gate, native Windows builds and diff check  
**Active work:** PR #58 `feat: add orchestration saturation diagnostics`, branch `feat/orchestration-saturation-diagnostics`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, context compaction, local memory boundaries and durable normal-run recovery through `run_journal.go`.

Desktop Mission telemetry from #56 is bounded and ephemeral. `/api/status` attaches the richer `mission` payload only when it matches the execution-scoped `RunID`; the Outputs inspector renders Mission state/reason, queue/running, resources, tasks and budgets without adding Mission control or persistence.

### Android / Mobile Remote

PR #57 is merged. Mobile Remote shows only a narrow active read-only Mission indicator derived from the already-authenticated `running` and `run_phase` status fields. It does not receive the Desktop `mission` payload, Mission/task IDs, scheduler/resource/budget/accounting details or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers:

1. Structured Agent contracts: roles, capabilities, budgets, usage and `AgentResult`.
2. Deterministic Task DAG with stable IDs/dependencies/readiness/terminal propagation.
3. Scheduler/Resource Manager with bounded queue/resource classes; local model inference defaults to one slot.
4. Actual scheduled read-only child dispatch.
5. Race-safe detached task preparation/finalization.
6. Governed explicit read-only Mission entry with validated project/DAG/role/capability boundaries.
7. Mission-level accounting/budgets with machine-readable terminal reasons.
8. Product-boundary cancellation with stable `MissionID` separated from execution-scoped `RunID` and graph-wide terminal cancellation.
9. Scheduler saturation/fairness coverage including cross-class bypass, FIFO within a saturated class and a 14-task fan-out/fan-in drain without starvation.
10. Desktop Mission telemetry from #56, bounded in-memory and scoped to the matching execution `RunID`.
11. Narrow Mobile Mission observation from #57 without a new Remote endpoint or authority.

## 3. Active PR #58 – orchestration saturation diagnostics

PR #58 adds observation only and does not change Scheduler policy or concurrency.

Machine-readable `/api/status` diagnostics:

- Always adds an `orchestration` object to Desktop status JSON.
- Separately classifies `ready`, `active`, `saturated`, `backend_unavailable` and `model_unavailable`.
- Machine-readable reasons distinguish `idle`, `mission_running`, `ollama_offline`, `no_model_selected`, `selected_model_missing`, `queue_limit_reached` and `resource_waiting`.
- Backend diagnostics report Ollama online state, selected model, whether that exact model is present in the returned local model list, installed-model count and backend error text.
- Queue diagnostics report queued count, actual scheduler queue limit, available slots, fill percentage and whether the queue limit is reached.
- Logical task diagnostics report ready/running/blocked task counts and how many tasks are waiting specifically for model inference.
- Per-resource diagnostics report class, limit, in-use, available, waiting, `at_capacity` and `saturated`.
- **At capacity is deliberately not the same as saturated:** saturation requires a full resource **and** waiting work for that resource.
- Actual normalized Mission resource limits are retained in the ephemeral Mission status so diagnostics do not assume default limits while a Mission is active.

Desktop UI:

- The existing `src/static/mission_status.js` module renders a read-only Orchestration card in the Outputs inspector.
- It shows backend/model state, queue utilization, logical task counts and each resource class with in-use/limit/waiting plus capacity/saturation state.
- DE/EN strings are synchronized.
- The diagnostics UI references no mutating chat/approval/project/terminal endpoint.

Focused tests cover backend-failure distinctions, at-capacity-vs-saturation semantics, waiting model-inference pressure, queue-limit saturation, idle readiness, JSON inclusion and the read-only UI contract.

## 4. Safety and correctness invariants

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
- `run_journal.go` remains the single durable recovery authority; Mission telemetry and orchestration diagnostics are non-durable observation only.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Diagnostics must not alter Scheduler limits, admission or model concurrency.
- No performance/superiority claim may be made from diagnostics alone; reproducible benchmarks are still required.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `.github/workflows/quality.yml`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission.go`, `src/agent_mission_accounting.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`, `src/run_journal.go`.

Diagnostics/tests/UI: `src/agent_orchestration_diagnostics_test.go`, `src/agent_mission_status_contract_test.go`, `src/static/mission_status.js`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 6. Exact next development direction

1. Finish PR #58: require complete Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Add reproducible benchmarks for logical task parallelism versus actual local model concurrency before making performance claims.
3. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
4. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
