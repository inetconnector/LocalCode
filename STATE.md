# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `5f6dac0b1f29ab7bb8ba45656fd58a512685f095`  
**Last merged functional PR:** #58 `feat: add orchestration saturation diagnostics`  
**PR #58 Quality run:** `32488109852` – complete success including frontend syntax, Android APK, full-stack integration, Go tests, race detector, >=80% coverage gate, native Windows builds and diff check  
**Active work:** PR #59 `test: benchmark orchestration parallelism`, branch `bench/orchestration-parallelism`  
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

PR #58 is merged. `/api/status` also contains machine-readable orchestration diagnostics that distinguish backend/model availability, active Mission state, queue pressure and actual resource saturation. `at_capacity` means a resource is full; `saturated` requires that the resource is full and matching work is waiting. Diagnostics include queue utilization, logical ready/running/blocked counts, waiting model work and per-resource limit/in-use/available/waiting data. The Desktop UI renders these facts read-only and does not alter Scheduler policy or concurrency.

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
12. Observation-only backend/queue/resource saturation diagnostics from #58.

## 3. Active PR #59 – orchestration parallelism benchmarks

PR #59 adds benchmark evidence only; it does not change Scheduler limits, admission policy, child capabilities or model concurrency.

Deterministic synthetic benchmark:

- `BenchmarkScheduledReadOnlyDispatcherParallelism` creates four independent governed read-only Explorer tasks that are logically ready together.
- It repeats the same workload with model-inference resource limits 1, 2 and 4.
- It reports logical ready tasks, configured model slots, peak executor calls actually in flight, tasks/op and executor-to-ready ratio.
- The executor contains a short deterministic delay so overlap is observable if dispatch becomes concurrent later.
- Current dispatch remains synchronous in `runScheduledReadOnlyAgentGraphWithExecutor`; therefore a higher model-slot limit alone is not evidence of concurrent child-model execution.

Explicitly opt-in real Ollama benchmark:

- `TestOllamaConcurrencyBenchmarkOptIn` is fixed-work rather than auto-calibrated because local LLM requests are expensive.
- It uses the production `OllamaClient.Chat` path with the exact same already-installed model for client concurrency 1, 2 and 4.
- It is disabled unless `LOCALCODE_BENCH_OLLAMA=1` and requires `LOCALCODE_BENCH_MODEL`.
- It accepts loopback Ollama endpoints only, never calls `EnsureRunning`, never pulls/installs a model and never changes LocalCode/Scheduler configuration.
- It emits machine-readable `ORCHESTRATION_BENCH` JSON with wall time, mean/p95 latency, requests/second, client-overlap factor and speedup relative to sequential execution.
- End-to-end client overlap is not claimed to prove simultaneous GPU kernels or simultaneous token generation inside Ollama.

`docs/ORCHESTRATION_BENCHMARKS.md` documents commands, bounds and interpretation rules.

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
- Benchmark output must not automatically alter Scheduler policy; capacity changes require a separate reviewed change with VRAM/memory, fairness, cancellation and stability evidence.
- Real Ollama benchmark traffic is opt-in and loopback-only and must never trigger model download/start/install behavior.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission.go`, `src/agent_mission_accounting.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`, `src/run_journal.go`.

Diagnostics/tests/UI: `src/agent_orchestration_diagnostics_test.go`, `src/agent_mission_status_contract_test.go`, `src/static/mission_status.js`.

Benchmarks: `src/agent_orchestration_parallelism_benchmark_test.go`, `docs/ORCHESTRATION_BENCHMARKS.md`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 6. Exact next development direction

1. Finish PR #59: require complete Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Move to durable Mission metadata/recovery integrated with `run_journal.go`; do not create a competing journal.
3. Add restart reconciliation and bounded pause/resume/retry semantics on top of that durable Mission state.
4. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
