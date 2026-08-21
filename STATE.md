# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Authoritative merged base for this slice:** `1e6cc45e17677263df112b07ac77e7e9126c9efc`  
**Last merged functional PR:** #52 `feat: add governed read-only mission entry`  
**PR #52 Quality run:** `32470934218` – success across the complete Quality workflow  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point for current LocalCode work. `TODO.md` contains unfinished work only; PR-by-PR implementation history belongs in Git history rather than being duplicated here.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. The long-term orchestration target is:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

A core local-hardware rule remains:

`logical task parallelism != model inference parallelism`

Many tasks may be logically ready while local model inference stays bounded by explicit resource limits.

## 2. Current merged functional state

### Desktop / Native runtime

Merged and active:

- Windows-native Go application with loopback Desktop HTTP/SSE API.
- LocalCode Native coding-agent tool loop with approvals, deterministic supervisor/reliability guards and completion verification.
- Selectable LocalCode Native, Aider, Claude Code, OpenCode and Claw Code engines.
- Ollama discovery/bootstrap and local model use.
- Project/task history, project management, quarantine/restore and controlled project actions.
- Controlled files, Git, builds/tests, tool discovery, web research, MCP, attachments and asset/image boundaries.
- Project/global instructions, compatible rule files, Skills and project slash commands.
- Context compaction, loop/stagnation guards, local memory boundaries and durable `run_journal.go` recovery for normal interrupted runs.

### Android / Mobile Remote

Current merged behavior includes:

- packaged native Android shell,
- mDNS discovery plus QR/pair/deep-link handoff,
- private HTTPS/TLS fingerprint pinning,
- WebView native file picker and Android speech input through a narrow JS bridge,
- visible input/provider errors and callback cleanup,
- DE/EN native status text,
- paired Remote controls for project/task navigation, attachments, voice, stop, send and existing approvals.

Mobile remains deliberately narrower than Desktop. A physical-device smoke remains useful for OEM-specific picker/speech behavior; CI builds the native APK and checks the bridge/UI contracts.

### Native Agent Teams / Phase 5

Executable child roles remain intentionally read-only:

- Explorer
- Planner
- Reviewer

Each child receives isolated context, fixed role capabilities and hard per-child model/tool/time/estimated-token budgets. Mutation, shell, Git, network/web, MCP tool calls, installation, memory, approvals and recursive child spawning remain outside the child action schema.

Merged orchestration layers now are:

1. Structured Agent contracts: roles, budgets, usage, capabilities and `AgentResult`.
2. Deterministic Task DAG: validated IDs, dependencies, readiness and terminal propagation.
3. Scheduler / Resource Manager: bounded queues/resources, one model-inference slot by default, capability-aware admission, cancellation and snapshots.
4. Actual read-only scheduler dispatch: authorized Explorer/Planner/Reviewer tasks execute through the Native child runtime and return structured result/usage.
5. Race-safe scheduler finalization: detached task copies and one scheduler lock boundary serialize cancellation versus child completion; cancellation-first drops late results/usage.
6. Governed product-level read-only Mission entry, merged in #52: an explicit mission request validates mission/task IDs, direct project boundary, DAG, executable roles and requested capability envelope before fixed role capabilities are granted. Planner suggestions remain inert data and never self-execute.
7. Mission start reuses the existing global `AppState` `Running`/`Cancel`/`RunID` authority so a Mission does not create a competing active-run state. `StopAgent` cancels an active Mission; `ForceStopAgent` cannot be undone by a late Mission completion.

The current #53 slice adds mission-level aggregate usage and optional additional budget ceilings. Final accounting is recomputed only from scheduler-accepted `UsageByTask`; cancelled late child output is not authoritative. Mission limits only tighten normalized child budgets and never increase child resources or capabilities.

Still absent:

- Desktop HTTP/UI Mission start/status surface,
- narrower Mobile read-only Mission view,
- larger product-level resource-saturation/fairness coverage,
- durable Mission persistence/recovery integrated with `run_journal.go`,
- mutation-capable Builder/worktree isolation,
- Integrator/Test-Agent mutation flow.

## 3. Safety and correctness invariants

Mandatory invariants:

- Canonical project/workspace containment, including symlink/junction escape protection where applicable.
- SHA/version preconditions bind approval to previewed state.
- Atomic/conflict-aware writes and checked postconditions.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or persistent `danger-full-access` equivalent.
- Planner `RequestedCapabilities` are planning data only; they never self-grant executable `Capabilities`.
- Dynamic role labels remain inert until governance maps them to an implemented runtime.
- Mobile permissions remain narrower than Desktop permissions.
- Read-only child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- No unsupervised concurrent mutation of the same workspace.
- `run_journal.go` remains the durable recovery authority; future Mission persistence must integrate with it rather than create another journal.
- Child/Mission usage must not be double-counted; cancelled late results are not accepted as completed work.
- Mission budgets may only constrain existing child budgets, never widen them.
- Statement coverage remains >=80.0%; safety/test gates are not weakened to make CI pass.

## 4. Important continuation files

Rules/docs:

- `AGENTS.md`
- `STATE.md`
- `TODO.md`
- `README.md`
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
- `src/agent_mission_accounting.go` (current #53 slice until merged)
- `src/run_journal.go`

UI/remote:

- `src/server.go`
- `src/remote_server.go`
- `src/static/index.html`
- `src/static/remote.html`
- `android/app/.../MainActivity.java`

## 5. Exact next development direction

After #53 is green and merged:

1. Add larger Mission/DAG resource-saturation and fairness tests.
2. Complete product-boundary cancellation coverage for queued/admitted/completing/already-terminal Mission tasks.
3. Expose stable read-only Mission/scheduler state in Desktop, including terminal reason, per-task state/resource class and budget snapshots.
4. Only after the Desktop contract is stable, expose a narrower read-only Mobile Mission view without new authority.
5. Add model/resource saturation diagnostics and reproducible logical-parallelism benchmarks.
6. Move to durable Mission metadata/recovery integrated with `run_journal.go`.
7. Only then begin mutation-capable Builder/worktree and later Integrator/Test-Agent stages.

## 6. Cleanup rule

Only `master` is an authoritative continuation base. Superseded PR carriers are closed rather than reused. Obsolete merged feature refs and obsolete Actions runs should be physically deleted when the available GitHub integration exposes delete operations; the current connector does not expose branch-ref or workflow-run deletion, so stale refs must never be treated as active development in the meantime.
