# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `f822518e3ca0a7bba171113d9dd30b4cb09524c7`  
**Last merged functional PR:** #54 `fix: terminalize cancelled mission graphs`  
**PR #54 Quality run:** `32473569489` – complete success including tests, race detector, >=80% coverage gate, Android APK and native Windows builds  
**Active work:** PR #55 `test: cover scheduler saturation and fairness at scale`, branch `test/scheduler-saturation-fairness`  
**Source/test head before this STATE refresh:** `1026808be89641b8ae0f2d2acc7d34fb68f33e8c`  
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

## 3. Active PR #55 – saturation/fairness coverage

PR #55 adds tests only; it changes no scheduler policy or authority.

Coverage added:

- Nine simultaneously ready tasks across `model-inference` and `read-cpu` resource classes.
- Two model slots and two read slots are saturated at once.
- An admissible task from a different resource class may bypass older queued work that is blocked only because its own resource class is saturated.
- FIFO ordering must remain stable inside the saturated model-inference class.
- Every admitted lease must release and final resource usage must return to zero.
- A larger scheduled DAG contains one root, twelve fan-out children and one fan-in join (14 total tasks).
- With one model-inference slot the larger DAG must drain deterministically without starvation, collect accepted usage for every task and end with no queued/running resources.

If these assertions expose a scheduler defect, fix the implementation rather than weakening the tests.

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
- `run_journal.go` remains the single durable recovery authority; future Mission persistence must integrate with it.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `.github/workflows/quality.yml`.

Agent/orchestration: `src/agent.go`, `src/subagent_model.go`, `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission.go`, `src/agent_mission_accounting.go`, `src/agent_mission_cancel.go`, `src/run_journal.go`.

Active test file: `src/agent_scheduler_fairness_test.go`.

UI/remote: `src/server.go`, `src/remote_server.go`, `src/static/index.html`, `src/static/remote.html`, `android/app/.../MainActivity.java`.

## 6. Exact next development direction

1. Finish PR #55: require complete Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Surface stable read-only Mission/scheduler state in Desktop: Mission state/reason, per-task state, queue/resource class and budget snapshots.
3. After the Desktop contract is stable, expose a narrower read-only Mobile Mission view without new authority.
4. Add model/resource saturation diagnostics and reproducible logical-parallelism benchmarks.
5. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
6. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
