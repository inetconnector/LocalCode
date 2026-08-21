# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `25e507041b9e6044aceebf63a40425f9360e48e3`  
**Last merged functional PR:** #56 `feat: surface read-only mission status in Desktop`  
**PR #56 Quality run:** `32478663724` – complete success including frontend syntax, full-stack integration, Go tests, race detector, >=80% coverage gate, Android APK and native Windows builds  
**Active work:** PR #57 `feat: show active read-only Mission in Mobile Remote`, branch `feat/mobile-mission-status`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, context compaction, local memory boundaries and durable normal-run recovery through `run_journal.go`.

PR #56 added bounded, ephemeral Desktop Mission telemetry to the existing `/api/status` source. The Desktop Outputs inspector can observe Mission state/reason, queue/running counts, resources, task state/resource/queue/admission data and Mission/task budget snapshots. This surface is observation only: it adds no Mission start/control endpoint and does not replace `run_journal.go`.

### Android / Mobile Remote

Merged behavior includes native Android packaging, mDNS discovery, QR/pair/deep-link handoff, private HTTPS/TLS fingerprint pinning, native file picker, Android speech input, DE/EN text and paired Remote controls. Mobile remains deliberately narrower than Desktop and gains no extra tool authority.

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

## 3. Active PR #57 – narrow Mobile Mission observation

PR #57 deliberately does **not** expose the richer Desktop Mission payload to Mobile.

Current slice:

- Reuses the already-authenticated `/remote/api/status` fields `running` and `run_phase`; no new Remote endpoint is introduced.
- When `running && run_phase == "mission-read-only"`, the Remote header identifies the active run as a Mission and the Tasks view shows a compact DE/EN read-only Mission card.
- Mobile receives no Mission/task identifier, scheduler/task detail, resource/budget/accounting payload or Mission start action from this slice.
- Existing Remote stop behavior is unchanged; no new control action or tool/capability authority is added.
- Contract tests require the Remote status to remain without a `mission` payload and reject Mission endpoint/control markers or Desktop-only accounting details in the Remote asset.
- `src/remote_mission_status_contract.md` records this source-level authority boundary.

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
- `run_journal.go` remains the single durable recovery authority; Mission telemetry is non-durable observation only.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `.github/workflows/quality.yml`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission.go`, `src/agent_mission_accounting.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`, `src/run_journal.go`.

Mission UI/tests: `src/static/mission_status.js`, `src/agent_mission_status*_test.go`, `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 6. Exact next development direction

1. Finish PR #57: require complete Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Add model/resource saturation diagnostics.
3. Add reproducible benchmarks for logical task parallelism versus actual local model concurrency before making performance claims.
4. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
5. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
