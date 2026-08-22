# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-22 Europe/Berlin  
**Authoritative merged base for this slice:** master `74f0a50ca8d62696340677479e5d3e8e44fd99bb`  
**Active work:** PR #65 `feat: plan safe mission recovery transitions`, branch `feat/mission-recovery-transition-planner`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history, not in this backlog.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] After merge, delete obsolete feature branches and obsolete workflow runs when the available GitHub integration exposes those delete operations. Until then, never treat stale refs as active work.

## P0 – finish current recovery-transition-planner slice

- [ ] Finish PR #65 on one exact head: keep canonical docs synchronized; require complete Quality success; inspect reviews/threads; mark Ready and merge.
- [ ] Classify recovery only; do not execute Mission/task work, grant capabilities, admit Scheduler leases, call models or invoke tools from the planner.
- [ ] Keep crash-running work `interrupted_review_required`; never convert interruption directly into resume/retry.
- [ ] Require every dependency of reusable/resumable/retryable work to be a currently matched, verified durable success.
- [ ] Enforce fixed planning limits of 3 attempts/task and 192 attempts/Mission from durable lifecycle evidence; failed/retryable legacy work without lifecycle evidence must remain non-retryable.
- [ ] Fail closed on malformed durable task graphs or lifecycle counters; invalid recovery structures may produce only `invalid_recovery_state`.

## P0/P1 – Phase 6: controlled Mission continuation after verified planning

- [ ] Add an explicit read-only Mission recovery-control boundary that recomputes current reconciliation, required postcondition verification and the transition plan immediately before any control decision.
- [ ] Keep startup passive: no automatic Mission resume/retry/replay.
- [ ] Define controlled pause/resume for eligible nonterminal read-only tasks only after the fresh transition plan authorizes them.
- [ ] Define controlled retry for eligible failed/retryable tasks; enforce durable per-task/per-Mission attempt limits and preserve existing cancellation semantics.
- [ ] Preserve Scheduler admission/resource limits; a recovery plan/reservation is never an execution lease.
- [ ] Preserve historical scheduler-accepted usage and Mission accounting across continuation without double-counting prior attempts.
- [ ] Revalidate dependencies immediately before resumed/retried dispatch; stale `verified` or stale plan data must not override current drift.
- [ ] Add crash/restart tests for partially completed Missions, repeated restarts, verification drift between plan and dispatch, cancel-vs-resume/retry races and budget/accounting continuity.
- [ ] Add bounded Mission Memory/Knowledge for architecture decisions, subsystem contracts, known failures and test evidence.

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
- [ ] Delete stale merged feature branch refs and obsolete GitHub Actions runs once delete-capable GitHub access is available.
