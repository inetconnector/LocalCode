# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-22 Europe/Berlin  
**Authoritative merged base for this slice:** master `bcd17f6975ce63dd28b23dbdeea34d42e1d53ad4`  
**Active work:** draft PR #67 `feat: materialize safe mission recovery continuation`, branch `feat/mission-recovery-dispatch-gate`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history, not in this backlog.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] After merge, delete obsolete feature branches and obsolete workflow runs when the available GitHub integration exposes those delete operations. Until then, never treat stale refs as active work.

## P0 – finish PR #67 bounded Mission continuation materialization

- [ ] Materialize only an explicit current `resume_candidate` or `retry_candidate`; never select work implicitly.
- [ ] Reuse the #66 fresh project/Git reconciliation and transient verification path against the same stable durable journal fingerprint.
- [ ] Include only the selected candidate plus its transitively `reuse_verified` dependency closure; unrelated ready work must never leak into the continuation DAG.
- [ ] Regenerate executable read-only capabilities from the canonical Explorer/Planner/Reviewer role envelope; never trust persisted granted capability data as execution authority.
- [ ] Revalidate persisted requested capabilities and model identity; fail closed on capability escalation, unsupported roles or silent model drift.
- [ ] Preserve scheduler-adjacent historical usage from BudgetSnapshot evidence and reject negative/conflicting accounting facts.
- [ ] If a task has a recorded execution attempt, require BudgetSnapshot usage evidence; missing historical accounting must not be interpreted as zero usage.
- [ ] Reject materialization when the recovered Mission budget is already exhausted by historical accepted usage or elapsed wall time.
- [ ] Keep the materialization explicitly non-authoritative: no Child/model execution, no Scheduler admission/lease, no journal write, `execution_authorized=false`.
- [ ] Re-read the durable journal after observation; retry boundedly on fingerprint changes and fail closed when stability cannot be proven.
- [ ] Reject already-active runs and a run that becomes active during materialization; do not claim this closes the future dispatch-time TOCTOU window.
- [ ] Cover resume/retry, dependency closure, unrelated-work exclusion, capability regeneration/escalation rejection, model/usage corruption, exhausted budget, missing attempted-task usage evidence, no-write behavior and active-run races.
- [ ] Keep canonical docs synchronized; require one fully green exact Quality head plus empty/resolved reviews and review threads before Ready/merge.

## P0 – next Phase 6 slice: atomic continuation admission and execution

- [ ] Recompute the #67 materialization immediately at the actual continuation boundary; a previously returned materialization or snapshot must never authorize execution.
- [ ] Couple final journal/reconciliation/active-run validation to Scheduler admission so stale-state TOCTOU cannot occur between authorization and dispatch.
- [ ] Persist the new attempt at the scheduler-authoritative transition without losing prior `AttemptCount`/`RetryCount` evidence; enforce three attempts/task and 192 attempts/Mission.
- [ ] Carry historical scheduler-accepted Usage/Accounting into the continued Mission exactly once; late/cancelled results must remain non-authoritative and prior work must never be double-counted.
- [ ] Preserve Mission and child budget limits across continuation; continuation may constrain remaining budgets but must never widen the original limits.
- [ ] Preserve existing Scheduler resource limits, detached-task execution, lease ownership and cancellation-vs-finalization race semantics.
- [ ] Define the new execution-scoped `RunID`/journal transition while preserving stable `MissionID` and explicit linkage to the interrupted run evidence.
- [ ] Add crash/restart tests for repeated restarts, drift between materialization/admission, cancellation during continuation, attempt-limit exhaustion and accounting continuity.
- [ ] Keep startup passive; no automatic Mission resume/retry/replay.

## P0/P1 – later Phase 6 recovery product surface

- [ ] Add a narrow explicit Desktop recovery inspection/control transport only after the atomic execution boundary is sound; it must call trusted AppState governance, inherit loopback/CSRF protections and add no Mobile endpoint.
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
