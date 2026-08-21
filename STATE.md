# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current authoritative merged master:** `a4453534aa513b48d41ae2f754c4c05d6c04358a`  
**Last merged functional PR:** #53 `feat: add mission usage and budget accounting`  
**PR #53 Quality run:** `32471919238` – complete success including tests, race detector, >=80% coverage gate, Android APK and native Windows builds  
**Active work:** PR #54 `fix: terminalize cancelled mission graphs`, branch `fix/mission-cancel-terminal-graph`  
**Source/docs head immediately before this STATE refresh:** `af584cde35232b49c2002ade683a5a728765c09d`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for LocalCode. `TODO.md` contains unfinished work only. Git history and merged PRs remain the detailed implementation record.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. The long-term orchestration target is:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

A core rule is:

`logical task parallelism != model inference parallelism`

Many tasks may be logically ready while local model inference remains explicitly resource-bounded.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active:

- Windows-native Go application with loopback Desktop HTTP/SSE API.
- LocalCode Native coding-agent loop with approvals, deterministic supervisor/reliability guards and completion verification.
- Selectable LocalCode Native, Aider, Claude Code, OpenCode and Claw Code engines.
- Ollama discovery/bootstrap and local model use.
- Project/task history, project management, quarantine/restore and controlled project actions.
- Controlled files, Git, builds/tests, tool discovery, web research, MCP, attachments and asset/image boundaries.
- Project/global instructions, rule files, Skills, slash commands, context compaction, loop/stagnation guards and local memory boundaries.
- `run_journal.go` is the durable recovery authority for normal interrupted runs.

### Android / Mobile Remote

Merged behavior includes native Android packaging, mDNS discovery, QR/pair/deep-link handoff, private HTTPS/TLS fingerprint pinning, native file picker, Android speech input, visible input/provider errors, DE/EN native status text and the existing paired Remote controls. Mobile remains deliberately narrower than Desktop and does not gain tool authority.

### Native Agent Teams / Phase 5

Executable child roles remain intentionally read-only:

- Explorer
- Planner
- Reviewer

Child schemas permit only project-tree reads, file reads, text search, approval-free read-only LSP and structured finish. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive child spawning remain absent.

Merged orchestration layers:

1. Structured Agent contracts: roles, capabilities, per-child budgets, usage and structured `AgentResult`.
2. Deterministic Task DAG with stable IDs, dependencies, readiness and terminal propagation.
3. Scheduler/Resource Manager with bounded queue/resource classes and one model-inference slot by default.
4. Actual scheduler dispatch of authorized Explorer/Planner/Reviewer tasks.
5. Race-safe detached task preparation/finalization: cancellation-first drops late results/usage; completion-first remains successful.
6. Governed read-only Mission entry from PR #52: explicit Mission request validates project boundary, DAG, executable roles and requested capability envelope before fixed read-only capabilities are granted. Planner suggestions remain inert data.
7. Mission usage/budget accounting from PR #53: scheduler-accepted `UsageByTask` is aggregated exactly once; Mission wall-time is separated from summed child-work time; optional Mission ceilings only tighten normalized child budgets; terminal reasons distinguish Mission-budget exhaustion, child-budget exhaustion, cancellation and failure.

## 3. Active PR #54 – cancellation terminal graph fix

Known product-boundary issue found after #53: `StopAgent` cancelled the active scheduled child through the parent context, but already queued siblings and dependency-blocked siblings could remain `ready`/`blocked` in the returned Mission graph even though the Mission itself had stopped.

PR #54 fixes this without expanding scheduler authority:

- `MissionID` remains stable caller-visible product identity.
- `AppState.RunID` is changed to a fresh execution-scoped ID rather than the caller-selected Mission ID, preventing accidental collision with stale run-journal IDs used by shared stop/journal hooks.
- After synchronous scheduled dispatch returns because the parent Mission context was cancelled, every still-unfinished Mission task is terminalized as `cancelled`.
- Already-successful and already-failed terminal tasks are preserved.
- Cancellation-first late Child result/usage remains discarded by the existing scheduler finalization boundary.
- A product-level regression test covers one already-successful task, one running task, one queued ready sibling and one dependency-blocked sibling; it also verifies MissionID/RunID separation.

Current active files:

- `src/agent_mission.go`
- `src/agent_mission_cancel.go`
- `src/agent_mission_cancel_test.go`
- `TODO.md`
- `docs/ARCHITECTURE.md`
- `STATE.md`

No Builder, mutation, HTTP/Remote Mission endpoint, Mobile capability, durable Mission persistence or competing journal is introduced by #54.

## 4. Safety and correctness invariants

Mandatory invariants:

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions bind approval to previewed state.
- Atomic/conflict-aware writes and checked postconditions.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent `danger-full-access` equivalent.
- Planner `RequestedCapabilities` are planning data only and never self-grant executable `Capabilities`.
- Dynamic role labels remain inert until mapped by trusted governance to an implemented runtime.
- Mobile permissions remain narrower than Desktop permissions.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- No unsupervised concurrent mutation of the same workspace.
- `run_journal.go` remains the single durable recovery authority; future Mission persistence must integrate with it.
- Child/Mission usage is never double-counted; cancellation-first late results are non-authoritative.
- Mission budgets may only constrain existing Child budgets, never widen them.
- Stable Mission identity is separate from execution-scoped run/journal identity.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened merely to make CI pass.

## 5. Important continuation files

Rules/docs:

- `AGENTS.md`
- `README.md`
- `STATE.md`
- `TODO.md`
- `docs/ARCHITECTURE.md`
- `docs/SECURITY.md`
- `.github/workflows/quality.yml`

Agent/orchestration:

- `src/agent.go`
- `src/subagent_model.go`
- `src/agent_team_types.go`
- `src/agent_task_graph.go`
- `src/agent_scheduler.go`
- `src/agent_scheduler_dispatch.go`
- `src/agent_scheduler_finalize.go`
- `src/agent_mission.go`
- `src/agent_mission_accounting.go`
- `src/agent_mission_cancel.go`
- `src/run_journal.go`

UI/remote:

- `src/server.go`
- `src/remote_server.go`
- `src/static/index.html`
- `src/static/remote.html`
- `android/app/.../MainActivity.java`

## 6. Exact next development direction

1. Finish PR #54: require full Quality success for the exact head, inspect reviews/threads, mark Ready and merge automatically.
2. Add larger Mission/DAG resource-saturation and fairness coverage beyond existing linear/fan-out/fan-in and basic class-bypass tests.
3. Expose stable read-only Mission/scheduler state in Desktop: Mission terminal reason, per-task state/resource class and budget snapshots.
4. After the Desktop contract is stable, add a narrower read-only Mobile Mission view with no new authority.
5. Add model/resource saturation diagnostics and reproducible logical-parallelism benchmarks.
6. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
7. Only then implement mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 7. Cleanup rule

Only `master` is an authoritative continuation base after merges. Superseded PR carriers are closed rather than reused. Obsolete merged branch refs and obsolete Actions runs should be physically deleted when the GitHub integration exposes delete operations; the current connector still does not expose branch-ref or workflow-run deletion, so stale refs must never be treated as active development.
