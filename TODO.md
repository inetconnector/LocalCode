# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-22 Europe/Berlin  
**Authoritative merged base for this slice:** master `fd5c7e38788b0485f0548f01de89fe25f320ec7f`  
**Active work:** draft PR #69 `feat: add explicit desktop mission recovery controls`, branch `feat/desktop-mission-recovery-control`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file contains unfinished functional work only. Completed PR history belongs in `STATE.md` and Git history.

## Permanent rules

- [ ] Keep `STATE.md` and `TODO.md` current after every material merge/roadmap/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
- [ ] Verify current master, active PR/head, CI, reviews and review threads before merging.
- [ ] Merge only a green exact head; do not weaken the >=80.0% statement-coverage gate or safety rules.
- [ ] Never introduce a second durable Mission recovery authority; `run_journal.go` remains canonical.
- [ ] Keep startup passive; recovery must never automatically resume/retry/replay work.
- [ ] Keep Mobile/Remote authority narrower than Desktop.

## P0 – finish PR #69 Desktop recovery control

- [ ] Obtain one complete exact-head Quality success after the final source/test/documentation commit: format, Vet, frontend syntax, PowerShell, Android, vulnerability scan, full-stack loopback integration, Go tests, race detector, >=80.0% coverage, native Windows builds and clean Git diff.
- [ ] Inspect PR #69 reviews and inline review threads; resolve blocking feedback without weakening recovery, stale-state, accounting, concurrency or Desktop/Remote boundaries.
- [ ] Confirm the Desktop inspection DTO remains bounded: no project paths, objectives, capabilities, raw Child/model output, Usage or accounting.
- [ ] Confirm POST admission remains explicit by Run/Mission/task/action, strictly decoded and bound to the inspected journal fingerprint while fresh project/Git recovery governance is recomputed at admission.
- [ ] Confirm `202 Accepted` is returned only after durable #68 reservation plus AppState ownership, and accepted execution remains cancellable through existing `StopAgent` semantics.
- [ ] Confirm both recovery routes inherit Desktop loopback Host/Origin/`Sec-Fetch-Site` protections and no Remote/Mobile recovery route exists.
- [ ] Confirm the bilingual Desktop card never auto-posts; only explicit Resume/Retry candidate buttons may start continuation.
- [ ] Mark PR #69 Ready only after the exact head is fully green, re-check that the head did not move, then squash-merge with `expected_head_sha`.
- [ ] Verify the resulting authoritative `master` SHA after merge.

## P0/P1 – remaining Phase 6 recovery/memory work

- [ ] Add bounded Mission Memory/Knowledge for architecture decisions, subsystem contracts, known failures and test evidence without creating a second recovery authority.
- [ ] Define retention/size/privacy limits for Mission Memory before persistence is introduced.
- [ ] Keep recovery inspection/control Desktop-only unless a separately reviewed security design explicitly changes the Remote trust boundary.

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
- [ ] Do not treat stale merged feature refs or historical non-current Actions runs as active development state.
