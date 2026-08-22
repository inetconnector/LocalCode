# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-22 Europe/Berlin  
**Authoritative merged base for this slice:** master `21a00ee24c729b4c46653041941168c59a69a11f`  
**Active work:** draft PR #66 `feat: add read-only mission recovery control boundary`, branch `feat/mission-recovery-control-boundary`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history, not in this backlog.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] After merge, delete obsolete feature branches and obsolete workflow runs when the available GitHub integration exposes those delete operations. Until then, never treat stale refs as active work.

## P0 – finish PR #66 read-only Mission recovery-control boundary

- [ ] Keep `run_journal.go` as the single durable recovery authority; never use cached startup `AppState.Recovery` as decision authority.
- [ ] Load the exact requested nonterminal read-only Mission by execution `run_id` and bind authority-relevant journal state to a canonical SHA-256 fingerprint.
- [ ] Freshly observe current project/Git state and recompute reconciliation for every control snapshot.
- [ ] Use the merged transition planner to identify only tasks requiring `verify_postconditions`; evaluate those fixed postconditions without model/Child execution.
- [ ] Apply successful postcondition evidence only to a cloned transient Mission and recompute the final transition plan from that clone; do not repair or mutate durable evidence in this slice.
- [ ] Re-read the journal after observation and retry at most three times when its fingerprint changes; fail closed if a stable snapshot cannot be obtained.
- [ ] Expose a trusted `AppState` recovery-control method that refuses snapshots while another agent run is active, including a race where a run becomes active during snapshot construction.
- [ ] Keep the snapshot explicitly non-authoritative for execution: `read_only=true`, `execution_authorized=false`, `scheduler_lease_granted=false`, `persistent_state_modified=false`.
- [ ] Treat the returned snapshot SHA as observation binding only, never as a Scheduler lease, capability grant or dispatch token.
- [ ] Keep startup passive and Mobile unchanged; no automatic Mission resume/retry/replay and no new Remote recovery-control authority.
- [ ] Cover transient verification, malformed historical `verified` evidence, current drift, byte-identical journal before/after, concurrent journal changes, wrong/terminal run IDs, and active-run races.
- [ ] Keep canonical docs consistent; require one fully green exact Quality head plus empty/resolved reviews and review threads before Ready/merge.

## P0/P1 – Phase 6: controlled Mission continuation after verified control snapshot

- [ ] Add a narrow explicit Desktop transport for recovery-control inspection only if/when the product needs it; transport must call the trusted AppState boundary rather than duplicate recovery logic, remain loopback/CSRF protected, and add no Mobile endpoint.
- [ ] Define controlled pause/resume for eligible nonterminal read-only tasks only after a fresh recovery-control snapshot is recomputed immediately at the dispatch boundary.
- [ ] Define controlled retry for eligible failed/retryable tasks; enforce durable per-task/per-Mission attempt limits and preserve existing cancellation semantics.
- [ ] Preserve Scheduler admission/resource limits; a recovery plan, snapshot or prospective attempt reservation is never an execution lease.
- [ ] Preserve historical scheduler-accepted usage and Mission accounting across continuation without double-counting prior attempts.
- [ ] Revalidate journal fingerprint, current reconciliation, verification evidence, dependencies and attempt budgets immediately before resumed/retried dispatch; stale snapshots must fail closed.
- [ ] Add crash/restart tests for partially completed Missions, repeated restarts, drift between snapshot and dispatch, cancel-vs-resume/retry races and budget/accounting continuity.
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
