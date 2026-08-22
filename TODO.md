# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-21 Europe/Berlin  
**Authoritative merged base for this slice:** master `f54e69bdfee270314aa32f53f3a9c7d5cbca96c9`  
**Active work:** PR #64 `feat: verify recovered mission postconditions`, branch `feat/mission-recovery-postcondition-verifier`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history, not in this backlog.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] After merge, delete obsolete feature branches and obsolete workflow runs when the available GitHub integration exposes those delete operations. Until then, never treat stale refs as active work.

## P0 – finish current postcondition-verifier slice

- [ ] Finish PR #64 on one exact head: keep canonical docs synchronized; require complete Quality success; inspect reviews/threads; mark Ready and merge.
- [ ] Re-observe current project/Git state before verification; only `matched` reconciliation may pass.
- [ ] Verify only durable non-running successful tasks with valid immutable completion evidence.
- [ ] Bind verification evidence to Mission/task identity, completion-result digest, fixed checks and current hashed/HEAD Git observation; do not persist raw paths or Child/model output.
- [ ] Persist refreshed reconciliation together with the verification outcome and reject a write if Mission recovery state changed during the observation window.
- [ ] Never let historical `verified` override current project/Git drift; current mismatch must remain blocked.
- [ ] Preserve the safety boundary: no automatic Mission resume/retry/replay and no new Scheduler/capability authority in #64.

## P0/P1 – Phase 6: safe Mission continuation after verified reconciliation

- [ ] Define a deterministic recovery transition planner/state machine for queued, ready, blocked, running-at-crash, failed, cancelled, succeeded-but-unverified, verification-failed and verified work.
- [ ] Define dependency-aware continuation eligibility: a task may become resumable/retryable only when its own reconciliation/verification state and all dependency postconditions permit it.
- [ ] Add explicit per-task/per-Mission attempt limits using durable `AttemptCount`/`RetryCount` without changing historical accepted usage.
- [ ] Support Mission/task pause and controlled resume only after the transition planner authorizes a non-mutating continuation plan.
- [ ] Add controlled retry preserving existing cancel semantics, Scheduler admission/resource limits and no double-counting of prior usage.
- [ ] Preserve Mission/task resource and budget accounting across restart/resume and prevent double-counting accepted usage.
- [ ] Add crash/restart tests for queued, ready, running, failed, verification-failed, verified and partially completed Mission work, including Git drift between process death and restart.
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
