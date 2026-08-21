# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-21 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified master before this documentation carrier:** `f767263a30b7ff35256e469938328941e1c5f6cb`  
**Last merged functional PR:** #49 `fix: serialize scheduler cancel and child finalization`  
**Final tested PR #49 head:** `656a36b8269e00031acfa3d3f4924b15acfc868d`  
**Quality run:** `32460575310` – success, including race detector, >=80% coverage gate, Android APK and native Windows builds  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the authoritative self-contained restart point for LocalCode work. `TODO.md` contains unfinished functional work only. Historical implementation detail belongs in Git history and merged PRs rather than being copied indefinitely into these files.

> Documentation carrier note: this refresh is authored on `docs/current-state-cleanup` from master `f767263a...`. Once merged, treat the resulting master as authoritative and do not treat the documentation branch as active development.

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. It combines:

- LocalCode Native plus selectable Aider, Claude Code, OpenCode and Claw Code engines,
- project/task history and project management,
- controlled file/Git/build/test/network/MCP tooling,
- approvals, path boundaries, atomic conflict-safe writes and recovery,
- Desktop UI plus a deliberately narrower Mobile/Android Remote,
- growing Native Agent-Team orchestration for repository-scale work.

Long-term architecture target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

A core local-hardware rule is:

`logical task parallelism != model inference parallelism`

Many tasks may be logically ready while only a bounded number of local model contexts are admitted at once.

## 2. Current merged functional state

### Desktop / Native runtime

Implemented and in active use:

- Windows-native Go application with loopback Desktop HTTP/SSE API.
- LocalCode Native coding-agent tool loop with approvals and deterministic supervisor/reliability guards.
- Selectable LocalCode Native, Aider, Claude Code, OpenCode and Claw Code engines.
- Ollama discovery/bootstrap and local model use.
- Project catalog, persistent tasks/threads, project quarantine/restore and project actions.
- Controlled files, Git, builds/tests, tool discovery, web research and MCP.
- Attachments plus local SVG/raster/image-generation/render/convert boundaries.
- Project/global instructions, compatible rule files, Skills, progressive Skill loading and project slash commands.
- Local durable memories with secret-like-content rejection.
- Context compaction, loop/stagnation guards, completion verification and no-op mutation rejection.
- Durable `run_journal.go` recovery for interrupted runs.

### Android / Mobile Remote

Merged via PR #46 and current on master:

- packaged native Android shell,
- automatic mDNS discovery,
- QR/pair/deep-link handoff,
- private-HTTPS/TLS fingerprint pinning,
- WebView native file picker for attachments,
- Android `RecognizerIntent` speech input through a narrow JS bridge,
- visible file-picker/speech errors,
- pending file chooser callback cleanup on replacement/failure/teardown,
- DE/EN native discovery/status text,
- explicit Mobile UI handlers for pairing, navigation, project/task actions, approvals, attachments, voice, stop, send, engine and project selection,
- send locking and attachment forwarding,
- prompt/attachment clearing only after a successful chat request.

A physical-device smoke test remains useful for OEM-specific picker/speech-provider behavior, but CI builds the native APK and regression-tests the bridge/UI contracts.

### Native Agent Teams

The current executable child roles are intentionally read-only:

- Explorer
- Planner
- Reviewer

Each child receives isolated context, explicit capabilities and hard model/tool/time/estimated-token budgets. Its action schema is limited to project tree/files/search and approval-free LSP plus structured finish. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approvals and recursive child spawning are absent.

Merged orchestration layers:

1. **Structured Agent contracts** – roles, budgets, usage, capabilities and `AgentResult`.
2. **Deterministic Task DAG** – validated stable IDs, dependencies, readiness and terminal propagation.
3. **Scheduler / Resource Manager foundation** – bounded ready queue, resource classes, conservative one-slot default for model inference, capability-aware admission, cancellation and snapshots.
4. **Actual read-only scheduler dispatch** – merged via PR #48 (`02a5908592db88ece914ec6ef0a03b7b216af5e8`). Authorized Explorer/Planner/Reviewer tasks are executed, structured results/usage are collected and dependent DAG tasks are reconciled deterministically. Linear and fan-out/fan-in graphs are covered.
5. **Race-safe scheduler finalization** – merged via PR #49 (`f767263a30b7ff35256e469938328941e1c5f6cb`). Scheduled children receive detached task copies; child finalization and `CancelTask`/`CancelMission` compete at one scheduler lock boundary. Cancellation-first drops late results/usage; completion-first preserves the successful result. Parent-context cancellation is also handled. Deliberate completion-vs-cancel race tests pass under Go's race detector.

Still absent from the product path:

- a user-facing Mission Manager that turns a main task/Planner result into a scheduled mission automatically,
- mission-level aggregate budget/persistence/recovery,
- mutation-capable Builder agents,
- Git worktree mutation isolation,
- Integrator/Test-Agent mutation flow.

## 3. Safety and correctness invariants

These remain mandatory:

- Canonical project/workspace path containment including symlink and NTFS-junction escape checks.
- SHA/version preconditions bind approval to the state that was previewed.
- Atomic/conflict-aware mutation paths and checked postconditions.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or persistent `danger-full-access` equivalent.
- Planner `RequestedCapabilities` are planning data only and never self-grant executable `Capabilities`.
- Dynamic role labels are inert until governance maps them to an implemented runtime.
- Mobile permissions stay narrower than Desktop permissions.
- Read-only child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- No unsupervised concurrent mutation of the same workspace.
- `run_journal.go` remains the recovery authority; future mission persistence must integrate with it rather than creating a competing journal.
- Secrets are not intentionally persisted in normal config, memory or recovery metadata.
- Child mutation, once implemented, must be diff-reviewable and verified before mission success.
- Statement coverage Quality gate remains at least 80.0%; safety or test gates may not be weakened merely to make CI pass.

## 4. Important continuation files

Repository rules and documentation:

- `AGENTS.md` – repository-wide rules.
- `STATE.md` – this canonical restart state.
- `TODO.md` – unfinished functional roadmap.
- `README.md` – current DE/EN product guide.
- `docs/ARCHITECTURE.md` – component/runtime boundaries.
- `docs/SECURITY.md` – security model.
- `android/README.md` – native Android Remote details.
- `.github/workflows/quality.yml` – mandatory Windows Quality gate.

Agent/orchestration implementation:

- `src/agent.go` – main Native tool loop and action dispatch.
- `src/agent_supervisor.go` – deterministic supervisor logic.
- `src/edit_reliability.go` – edit/reliability preflight.
- `src/agent_loop_guard.go` – no-progress/repetition controls.
- `src/subagent.go` – deterministic read-only fallback/handoff.
- `src/subagent_model.go` – bounded model-backed Explorer/Planner/Reviewer runtime.
- `src/agent_team_types.go` – Agent/Task/Budget/Usage/Result contracts.
- `src/agent_task_graph.go` – deterministic DAG validation/reconciliation.
- `src/agent_scheduler.go` – queue/resource/cancellation/snapshot logic.
- `src/agent_scheduler_dispatch.go` – actual read-only graph dispatch.
- `src/agent_scheduler_finalize.go` – serialized task preparation/finalization boundary.
- `src/agent_scheduler_dispatch_test.go` – scheduler-dispatch DAG coverage.
- `src/agent_scheduler_dispatch_race_test.go` – cancel/completion race coverage.
- `src/run_journal.go` – durable active-run recovery authority.

UI/remote boundaries:

- `src/server.go` – Desktop API.
- `src/remote_server.go` – paired Mobile Remote API.
- `src/static/index.html`, `src/static/ui_polish.js`, `src/static/i18n.js` – Desktop UI.
- `src/static/remote.html` – Mobile Remote UI.
- `android/app/.../MainActivity.java` – native Android shell/bridge/discovery/TLS handling.

## 5. Exact next development direction

Resume from current master, not from old feature branches.

Priority order:

1. Add a **product-level read-only Mission entry path** that can safely turn explicit Planner/task-graph output into a Scheduler-owned mission; do not silently execute arbitrary Planner suggestions.
2. Add mission-level usage/budget accounting without double-counting existing per-child budgets.
3. Add larger resource-saturation/fairness and mission-entry tests.
4. Expose stable mission/scheduler state in Desktop, then a narrower read-only Mobile view.
5. Add model/resource saturation diagnostics and reproducible logical-parallelism benchmarks.
6. Move into durable Mission metadata/recovery integrated with `run_journal.go`.
7. Only after those contracts are stable, implement Builder/worktree mutation and later Integrator/Test-Agent stages.

## 6. Repository cleanup note

Old merged feature branch refs currently still visible on GitHub include:

- `fix/android-remote-input-reliability`
- `feat/read-only-scheduler-dispatch`
- `feat/read-only-scheduler-dispatch-v2`
- `fix/scheduler-cancel-finalize-race`

They are **not active development** and must never be used as a continuation base. The currently available GitHub connector in this environment does not expose branch-ref deletion or workflow-run deletion, and no authenticated `gh` CLI is installed. Delete those refs/obsolete Actions runs when a GitHub endpoint with delete capability is available; until then, only `master` is authoritative.
