# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-22 Europe/Berlin  
**Authoritative merged base for this slice:** master `a4fad2494cd4b6b509647a8c514807e09f5736aa`  
**Last merged functional PR:** #69 `feat: add explicit desktop mission recovery controls`  
**Active work:** draft PR #70 `docs: finalize desktop mission recovery state`, branch `docs/finalize-desktop-mission-recovery`  
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

## P0 – finish PR #70 documentation synchronization

- [ ] Obtain one complete exact-head Quality success for the final documentation head: format, Vet, frontend syntax, PowerShell, Android, vulnerability scan, full-stack loopback integration, Go tests, Race Detector, >=80.0% coverage, native Windows builds and clean Git diff.
- [ ] Inspect PR #70 reviews and inline review threads; resolve any blocking feedback without changing the documented recovery/security boundary incorrectly.
- [ ] Mark PR #70 Ready only after the exact head is fully green, re-check the head SHA, then squash-merge with `expected_head_sha`.
- [ ] Verify the resulting authoritative `master` SHA and update canonical state before starting the next functional slice.

## P0/P1 – bounded Mission Memory / Knowledge

- [ ] Define a versioned, bounded Mission Memory schema for architecture decisions, subsystem contracts, known failures and test/verification evidence.
- [ ] Define explicit retention limits: maximum entries, maximum bytes, per-field limits and deterministic eviction/compaction behavior.
- [ ] Define privacy/redaction rules before persistence: no secrets, credentials, raw Child/model transcripts, arbitrary tool output or unrestricted file content.
- [ ] Keep Mission Memory **separate from execution authority**. Memory may inform context/planning but cannot grant capabilities, satisfy postconditions, authorize recovery, mint Scheduler leases or bypass current project/Git reconciliation.
- [ ] Decide the durable storage relationship without creating a second active-Mission recovery authority. If Mission Memory is persisted alongside a Mission, `run_journal.go` remains the only active recovery authority and recovery decisions must continue to rely on canonical recovery evidence rather than narrative memory.
- [ ] Add validation, corruption/fail-closed behavior, schema-version handling and backward-compatible absence semantics.
- [ ] Add deterministic tests for size caps, redaction, eviction, malformed entries and proof that memory cannot change admission/capability decisions.
- [ ] Document the final Memory/Recovery boundary in README, Architecture, Security, STATE and TODO.

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
