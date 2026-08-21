# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `f2fc84453a35a36aac9a4ed618139bf8a3060211`  
**Last merged functional PR:** #55 `test: cover scheduler saturation and fairness at scale`  
**PR #55 Quality run:** `32474252624` – complete success including tests, race detector, >=80% coverage gate, Android APK and native Windows builds  
**Active work:** PR #56 `feat: surface read-only mission status in Desktop`, branch `feat/desktop-mission-status`  
**Source/test head before this STATE refresh:** `c00f3152c599f84f635e7f4bf926f9e6e94682bb`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule:

`logical task parallelism != model inference parallelism`

Many tasks may be logically ready while actual local model inference remains resource-bounded.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable LocalCode Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, instructions/rules/Skills/slash commands, context compaction, loop guards, local memory boundaries and durable normal-run recovery through `run_journal.go`.

### Android / Mobile Remote

Merged behavior includes native Android packaging, mDNS discovery, QR/pair/deep-link handoff, private HTTPS/TLS fingerprint pinning, native file picker, Android speech input, visible provider/input errors, DE/EN native status text and existing paired Remote controls. Mobile remains deliberately narrower than Desktop and gains no extra tool authority.

### Native Agent Teams / Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers:

1. Structured Agent contracts: roles, capabilities, budgets, usage and `AgentResult`.
2. Deterministic Task DAG with stable IDs/dependencies/readiness/terminal propagation.
3. Scheduler/Resource Manager with bounded queue/resource classes; local model inference defaults to one slot.
4. Actual scheduled read-only child dispatch.
5. Race-safe detached task preparation/finalization: cancellation-first discards late results/usage; completion-first stays successful.
6. Governed explicit read-only Mission entry: validates Mission/task IDs, direct project boundary, DAG, executable roles and requested capability envelope; Planner suggestions stay inert.
7. Mission-level accounting/budgets: accepted `UsageByTask` is counted once; wall-time is separate from summed child work; Mission ceilings only tighten normalized child budgets; terminal reasons are machine-readable.
8. Product-boundary cancellation from #54: stable `MissionID` is separated from fresh execution-scoped `AppState.RunID`; after parent/`StopAgent` cancellation all still-unfinished Mission tasks are terminalized `cancelled`, while already-terminal successful/failed work is preserved.
9. Scheduler saturation/fairness coverage from #55: simultaneous resource-class saturation, cross-class bypass, FIFO inside a saturated class and a 14-task fan-out/fan-in drain are verified without starvation or resource leakage.

## 3. Active PR #56 – Desktop Mission status

PR #56 adds a read-only observation surface to Desktop without adding a Mission start/control API.

Backend/status contract:

- `src/agent_mission_status.go` maintains a bounded in-memory Mission-status registry keyed by the fresh execution-scoped `RunID`; maximum retained entries are 32 and oldest entries are evicted.
- `/api/status` remains the single Desktop status source. A custom `Status.MarshalJSON` adds `mission` only when the status `RunID` has matching Mission telemetry; unrelated runs do not receive Mission data.
- While a read-only Mission executes, a monitor publishes Mission identity/project/model, running state, live scheduler queue/running/resource snapshots, task state/resource class/queue position/budget snapshots and Mission budget usage.
- Final Mission status publishes machine-readable terminal state/reason and the accepted terminal scheduler/accounting snapshot.
- After cancellation, the returned scheduler snapshot is refreshed after graph-wide terminalization so `queued=0`, `running=0` and task terminal states agree across Mission result and Desktop telemetry.
- This registry is **ephemeral observation only**. It is not persistence, cannot resume a Mission and does not compete with `run_journal.go`.

Desktop UI contract:

- `src/static/mission_status.js` renders a Mission card in the existing right-side **Outputs** inspector.
- The card shows Mission state/reason, queued/running counts, Mission budget usage/limits, scheduler resource usage and each task's state/resource class/queue/admission/budget information.
- New visible strings are supplied in synchronized German and English dictionaries.
- The Mission card is read-only and references no chat-send, approval, project-mutation or terminal-command endpoint.
- `src/static/i18n.js` dynamically loads the Mission-status module after the main Desktop state/functions exist.

Focused tests cover JSON scoping, live + terminal cancellation telemetry, bounded registry eviction, the Desktop asset loader/DE+EN contract and absence of mutating endpoint references.

Still absent after #56:

- a narrower read-only Mobile Remote Mission view,
- Mission start/control surface in Desktop or Mobile,
- model/resource saturation diagnostics and reproducible local-concurrency benchmarks,
- durable Mission metadata/recovery integrated with `run_journal.go`,
- mutation-capable Builder/worktree agents and Integrator/Test-Agent mutation flow.

## 4. Safety and correctness invariants

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions bind approval to previewed state.
- Atomic/conflict-aware writes and checked postconditions.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent `danger-full-access` equivalent.
- Planner `RequestedCapabilities` remain inert planning data until trusted governance grants implemented runtime capabilities.
- Dynamic role labels remain inert until mapped to an implemented runtime.
- Mobile permissions remain narrower than Desktop permissions.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- No unsupervised concurrent mutation of the same workspace.
- `run_journal.go` remains the single durable recovery authority; Desktop Mission telemetry is non-durable observation only.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Desktop Mission status grants no new capabilities and exposes no mutation/control endpoint.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `.github/workflows/quality.yml`.

Agent/orchestration: `src/agent.go`, `src/subagent_model.go`, `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission.go`, `src/agent_mission_accounting.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`, `src/run_journal.go`.

Desktop Mission tests/assets: `src/agent_mission_status_test.go`, `src/agent_mission_status_registry_test.go`, `src/agent_mission_status_contract_test.go`, `src/static/mission_status.js`, `src/static/i18n.js`.

UI/remote: `src/server.go`, `src/remote_server.go`, `src/static/index.html`, `src/static/remote.html`, `android/app/.../MainActivity.java`.

## 6. Exact next development direction

1. Finish PR #56: require complete Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Expose a narrower read-only Mobile Remote Mission view without adding Mobile tool/control authority.
3. Add model/resource saturation diagnostics.
4. Add reproducible benchmarks for logical task parallelism versus actual local model concurrency before making performance claims.
5. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
6. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
