# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-21 Europe/Berlin  
**Authoritative base before this documentation carrier:** master `f767263a30b7ff35256e469938328941e1c5f6cb`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history, not in this backlog.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] After merge, delete obsolete feature branches and obsolete workflow runs when the available GitHub integration exposes those delete operations. Until then, never treat stale refs as active work.

## P0 – Finish Phase 5: product-level read-only orchestration

- [ ] Add an explicit product-level **Mission entry path** above the existing Planner/DAG/Scheduler stack.
  - Must not silently execute arbitrary Planner suggestions.
  - Governance must explicitly validate the graph, executable roles and granted capabilities.
  - Read-only Explorer/Planner/Reviewer only in this phase.
  - Mission start must have a stable ID and observable state.
- [ ] Connect actual child usage to mission-level aggregate accounting without double-counting per-child usage/budgets.
- [ ] Define mission-level budget exhaustion behavior and expose a machine-readable reason.
- [ ] Add larger DAG resource-saturation/fairness tests beyond the existing linear and fan-out/fan-in dispatch tests.
- [ ] Add mission-entry cancellation tests covering queued, admitted, completing and already-terminal tasks at the product boundary.
- [ ] Surface stable mission/scheduler state in Desktop: queued/running/blocked/cancelled/succeeded, resource class and budget state.
- [ ] Add a narrower read-only Mobile Remote mission view without adding Mobile tool authority.
- [ ] Add model-backend/resource saturation diagnostics.
- [ ] Add reproducible benchmarks for logical task parallelism vs actual local model concurrency before making performance claims.

## P0/P1 – Phase 6: durable Missions and recovery

- [ ] Introduce durable Mission metadata separate from chat prose: objective, project/scope, constraints, success criteria, graph and current state.
- [ ] Integrate Mission persistence into `run_journal.go`; do not create a competing recovery authority.
- [ ] Persist DAG/task state, attempts, structured results, model/tool/resource usage, timestamps and verification state.
- [ ] Reconcile project/Git/postconditions on restart before resuming; never blindly replay a mutation.
- [ ] Support mission/task pause, resume, cancel and controlled retry.
- [ ] Preserve mission/task resource and budget accounting across recovery.
- [ ] Add bounded Mission Memory/Knowledge for architecture decisions, subsystem contracts, known failures and test evidence.
- [ ] Add crash/restart tests for queued, ready, running, failed and partially completed work.

## P1 – Phase 7: mutation-capable Builder agents in Git worktrees

- [ ] Add an optional LocalCode-managed worktree workspace type for mutation-capable children.
- [ ] Validate managed worktree paths against path/symlink/junction escapes.
- [ ] Give each Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [ ] Preserve normal LocalCode approvals, SHA/version preconditions, backups, atomic writes and command/network policy inside worktrees.
- [ ] Record changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Add safe cancellation/cleanup/orphan recovery; no destructive global reset/clean shortcuts.
- [ ] Add stale-base, collision, path-escape, cancellation and Windows worktree tests.

## P1 – Phase 8: Integrator, Test Agent and independent Reviewer

- [ ] Make Integrator the only component allowed to combine mutation-agent results into an integration target.
- [ ] Require diff inspection and dependency/interface compatibility before integration.
- [ ] Test Agent receives acceptance criteria plus artifacts/diff, not Builder self-assessment.
- [ ] Reviewer receives requirements/diff/test evidence, not Builder private reasoning.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [ ] Add mission-level no-progress/stagnation controls for repair cycles.
- [ ] Require suitable verification after the last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

## P1 – Phase 9: constrained Agent Factory and replanning

- [ ] Add a constrained Agent Factory that maps validated role/capability templates to implemented runtimes.
- [ ] Keep dynamic Planner role labels inert until mapped by governance.
- [ ] Add bounded replanning after blocked/failed tasks without infinite task spawning.
- [ ] Add per-mission maximum task/agent/attempt limits.
- [ ] Add dependency-aware retry/replan rules and structured reasons.
- [ ] Add deferred tool/skill discovery for child agents without capability escalation.
- [ ] Add typed slash/mission commands where useful without turning text templates into executable authority.

## P2 – broader runtime/platform work

- [ ] Optional backend-neutral local inference path such as llama.cpp/DMC without silent model/provider drift.
- [ ] Stronger health/doctor diagnostics for local models, tools, engines, MCP, Android Remote and orchestration resources.
- [ ] Evaluate QEMU/VM/OS-building workflows only behind explicit sandbox/resource boundaries and reproducible tests.
- [ ] Benchmark LocalCode Native against Aider/OpenCode/Claw on reproducible repository tasks before claiming superiority.

## Documentation/cleanup acceptance gates

- [ ] Keep `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `STATE.md` and this file consistent with merged reality.
- [ ] Remove stale statements that native Android shell or mDNS/QR discovery are future work.
- [ ] Delete stale merged feature branch refs and obsolete GitHub Actions runs once delete-capable GitHub access is available.
