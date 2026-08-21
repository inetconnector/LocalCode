# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `c140dfbf9b02d9e3533382c877732a6fd3403064`  
**Last merged functional PR:** #59 `test: benchmark orchestration parallelism`  
**PR #59 Quality run:** `32514351391` – complete success including frontend syntax, Android APK, full-stack integration, Go tests, race detector, >=80% coverage gate, native Windows builds and diff check  
**Active work:** PR #60 `feat: persist durable Mission metadata in run journal`, branch `feat/durable-mission-metadata`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only; Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active: Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, project/task history, controlled files/Git/builds/tests/tool discovery/web/MCP/attachments/assets, context compaction, local memory boundaries and durable normal-run recovery through `run_journal.go`.

Desktop Mission telemetry remains bounded and ephemeral. `/api/status` attaches the richer `mission` payload only when it matches the execution-scoped `RunID`; the Outputs inspector renders Mission state/reason, queue/running, resources, tasks and budgets without adding Mission control.

Orchestration diagnostics from #58 distinguish backend/model availability, active Mission state, queue pressure and true resource saturation. `at_capacity` means a resource is full; `saturated` additionally requires matching waiting work. Diagnostics are read-only and never alter Scheduler policy or concurrency.

### Android / Mobile Remote

Mobile Remote exposes only the narrow active read-only Mission indicator derived from authenticated `running` and `run_phase`. It receives no Desktop Mission/task IDs, scheduler/resource/budget/accounting detail or new Mission-control authority. Existing Remote stop behavior is unchanged.

### Native Agent Teams / completed Phase 5

Executable child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Child schemas permit project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration layers now include structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, actual scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution `RunID`, scheduler fairness/saturation coverage, Desktop/Mobile observation, orchestration diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

PR #59 established an explicit evidence boundary: configured model slots, logical task readiness, executor overlap and Ollama client overlap are different measurements. Current scheduled Child dispatch remains synchronous; higher model-slot limits alone do not create or prove parallel Child model execution. Benchmark output never automatically changes Scheduler limits.

## 3. Active PR #60 – durable Mission metadata in the existing run journal

PR #60 begins Phase 6 without creating a second recovery authority.

Durable Mission metadata:

- `RunRecoveryState` gains an optional structured Mission checkpoint while `active-run.json` remains the single durable active-run journal.
- A validated read-only Mission persists stable Mission identity, objective, direct project/scope, bounded constraints, bounded success criteria, Mission budget and bounded DAG/task metadata separately from normal chat prose.
- Task checkpoint data includes task ID/parent/dependencies, executable role, state/reason, requested/granted capabilities, model, task budget, resource class/queue position/running flag and budget snapshot.
- Scheduler-owned checkpoints update the same Mission record during dispatch rather than writing a competing file.
- Final Mission state/reason, Mission accounting and scheduler-accepted per-task usage are stored in the same record.
- Raw Child/model result text, findings and tool transcripts are deliberately not copied into durable Mission metadata.
- Secret-like free text is sanitized through the existing run-journal redaction path and bounded before persistence.

Recovery boundary:

- An interrupted Mission is detectable at startup as structured Mission state.
- The existing normal chat `Weiter`/`Continue` handoff explicitly refuses Mission journal entries, so a Mission cannot accidentally be replayed as a normal prompt.
- This slice does **not** auto-resume, retry or replay Mission work. Restart reconciliation of project/Git/postconditions is the next required layer.
- `run_journal.go` remains the sole durable recovery authority; Desktop Mission telemetry and orchestration diagnostics remain observation-only.

Focused tests cover durable Mission shape/redaction/bounds, scheduler checkpointing, terminal accounting/usage persistence and the no-normal-continue rule.

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
- `run_journal.go` is the single durable recovery authority; no second Mission journal may be introduced.
- Durable Mission metadata is bounded operational state, not a second transcript or authority source.
- Interrupted Missions must be reconciled against current project/Git/postconditions before any future resume; no blind replay.
- Child/Mission usage is not double-counted and cancelled late results are non-authoritative.
- Mission budgets only constrain Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Diagnostics and benchmark output must not automatically alter Scheduler limits, admission or model concurrency.
- Real Ollama benchmark traffic is opt-in and loopback-only and must never trigger model download/start/install behavior.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are not weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery: `src/agent_mission.go`, `src/run_journal_mission.go`, `src/run_journal_mission_test.go`, `src/run_journal.go`, `src/run_journal_test.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go`.

Agent/orchestration: `src/agent_team_types.go`, `src/agent_task_graph.go`, `src/agent_scheduler.go`, `src/agent_scheduler_finalize.go`, `src/agent_mission_cancel.go`, `src/agent_mission_status.go`.

Benchmarks/diagnostics/UI: `src/agent_orchestration_parallelism_benchmark_test.go`, `src/agent_orchestration_diagnostics_test.go`, `src/static/mission_status.js`, `docs/ORCHESTRATION_BENCHMARKS.md`.

Mobile contract: `src/static/remote.html`, `src/remote_mission_status_test.go`, `src/remote_mission_status_contract.md`.

## 6. Exact next development direction

1. Finish PR #60: require complete Quality success for the exact documentation+code head, inspect reviews/threads, mark Ready and merge automatically.
2. Add restart reconciliation for interrupted Missions: verify project identity and current observable project/Git/task postconditions before deciding what can resume.
3. Persist remaining recovery semantics such as attempts/verification and add bounded pause/resume/controlled retry while preserving resource/budget accounting.
4. Add crash/restart cases for queued, ready, running, failed and partially completed Mission work.
5. Only after durable Mission recovery is sound, implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is authoritative after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still lacks branch-ref/workflow-run deletion, so stale refs must never be treated as active development.
