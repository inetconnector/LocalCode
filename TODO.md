# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Current functional master:** `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`  
**Last merged feature:** PR #40 `feat: add bounded native agent team roles`  
**Final tested PR #40 head:** `9c3b25b1b070d80c075e9b697a9fffe86f0d3184`  
**Quality on final PR #40 head:** #406 – success  
**Current documentation PR:** #41 `docs: refresh bootstrap state and TODO after native agent teams`  
**Documentation content baseline before PR-metadata sync:** `143bd1abfba147434d42820f7f39b1fb0b58d81a`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the authoritative exhaustive list of unfinished LocalCode work. `STATE.md` describes current reality and is the self-contained AI bootstrap. `TODO.md` contains only work that is still unfinished, its dependencies and acceptance gates. They must never contradict each other.

---

## 0. Permanent STATE.md + TODO.md maintenance rule

`STATE.md` and `TODO.md` MUST remain completely current together.

`STATE.md` is the mandatory self-contained AI bootstrap: a newly started AI without chat history, memory or prior context must be able to understand the complete project state and resume implementation immediately. `TODO.md` is the exhaustive unfinished-work ledger; `STATE.md` must summarize enough of this roadmap and the exact next action to make continuation possible on its own.

Blocking rules:

1. Read both files before material work.
2. Verify `master`, active branch/PR, open issues, reviews and latest required Quality against GitHub reality before resuming.
3. After every material branch/base/head change, PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, update both files in the same workstream or immediately afterward.
4. Remove or rewrite completed/superseded TODO items; history belongs in Git and closed issues/PRs.
5. Replace stale facts instead of appending contradictory snapshots.
6. A feature/fix is not operationally complete until both files reflect the new reality.
7. Before merge, ensure changed remaining work is represented here; immediately after merge refresh both files for resulting `master`.
8. Never invent self-commit/merge SHAs.

---

## 1. Immediate work after PR #40

Priority: **P0 / current**

PR #40 is complete and merged. Its former merge checklist is no longer TODO work.

Current documentation workstream:

- [x] Create post-#40 branch `docs/state-todo-after-native-agent-teams` from merge `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`.
- [x] Rewrite `STATE.md` as a self-contained restart/bootstrap document with architecture, key files, safety/Quality baseline, #40 outcome and exact next steps.
- [x] Rewrite `TODO.md` so completed #40 tasks are removed and Phase 4 DAG work is next.
- [x] Open documentation PR #41.
- [x] Record PR #41 in `STATE.md` and `TODO.md` using the self-referential baseline convention.
- [ ] Full required Quality must pass on the exact final PR #41 head.
- [ ] Verify #41 branch is 0 behind current `master`.
- [ ] Verify #41 mergeability on the exact final head.
- [ ] Verify no unresolved review threads or blocking review submissions.
- [ ] Mark #41 ready only after all exact-head checks are green.
- [ ] Merge #41 with `expected_head_sha`.
- [ ] Verify resulting `master`; if the merged documents still contain an open-PR fact that became stale because of the merge itself, perform the minimal current-state refresh required by the self-SHA convention before starting feature work.
- [ ] Verify issue #23 acceptance against merged PR #40 and close #23 as completed if fully satisfied.
- [ ] Start #32 Phase 4 on a fresh branch from the then-current `master`.

---

## 2. Issue #32 – UMAF-LC / Native Agent Teams

Priority: **P0**

Merged foundation (#40): generic AgentTask/role/capability/budget/result contracts plus model-backed read-only Explorer/Planner/Reviewer. Everything below remains unfinished.

Architecture principle:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Do not create a rigid hierarchy of many hard-coded Go agent classes. Orchestration must remain backend-independent and preserve LocalCode approval, sandbox, atomic-write, recovery and verification boundaries.

### Phase 4 – Task DAG and dependency model — NEXT FEATURE SLICE

- [ ] Add mission ID, parent task ID, dependency IDs and explicit task-state semantics adjacent to merged `AgentTask` without breaking existing single-agent/read-only-child paths.
- [ ] Define task states: proposed, blocked, ready, running, succeeded, failed, cancelled, retryable.
- [ ] Implement deterministic DAG validation.
- [ ] Reject duplicate task IDs.
- [ ] Reject missing dependency IDs.
- [ ] Reject cycles deterministically/fail closed.
- [ ] Convert Planner `SuggestedTasks` into validated machine-readable DAG proposals; no prose parsing.
- [ ] Add only the structured task input/output/dependency handoff data needed for this slice.
- [ ] Release dependents deterministically when prerequisites succeed.
- [ ] Propagate failed-dependency/blocked state deterministically.
- [ ] Represent multiple independent ready tasks as logical readiness without broad asynchronous execution yet.
- [ ] Add tests for duplicate IDs, missing dependencies, cycles, dependency release, failure propagation and independent ready tasks.
- [ ] Preserve project-root/sandbox/no-privilege-escalation invariants.
- [ ] Keep first DAG slice free of Builder mutation, Git worktrees and mission persistence.
- [ ] Full exact-head Quality + review/behind/mergeability gates before merge.
- [ ] Immediately refresh `STATE.md` + `TODO.md` after merge.

### Phase 5 – Scheduler and resource manager

- [ ] Separate logical task parallelism from model inference parallelism.
- [ ] Add bounded queues/resource classes for model inference, CPU/read/search work, builds/linkers and exclusive integration/test resources.
- [ ] Use conservative local-GPU/model concurrency defaults; logical task count must not imply equal simultaneous model contexts.
- [ ] Extend mission/task model/tool/time/estimated-token budgets.
- [ ] Enforce hard-stop semantics with structured budget-exhausted results.
- [ ] Add cancellation propagation.
- [ ] Add fairness/starvation tests.
- [ ] Surface queued/running/blocked state and remaining budgets in Desktop/Remote without widening Mobile permissions.

### Phase 6 – Persistent missions and recovery

- [ ] Introduce durable mission metadata separate from chat prose.
- [ ] Persist graph, task states, structured results, attempts, model/tool usage, timestamps and verification state.
- [ ] Integrate with existing durable run journal instead of creating a competing recovery authority.
- [ ] On restart reconcile project/Git/postconditions before resuming; never blindly replay mutation.
- [ ] Support mission/task pause, resume, cancel and retry.
- [ ] Add bounded mission knowledge for architecture decisions, interfaces/contracts, known failures and test results.
- [ ] Add crash/restart tests for ready, running, failed and partially integrated tasks.

### Phase 7 – Git-worktree mutation agents

- [ ] Add optional isolated worktree workspace type for mutation-capable child agents.
- [ ] Create worktrees only under LocalCode-managed validated paths with symlink/path protections.
- [ ] Give each Builder task its own branch/worktree.
- [ ] Never allow unsupervised concurrent mutation of the same workspace.
- [ ] Builder actions still pass normal validation, approvals, SHA/preconditions, backups and process rules; worktrees grant no extra authority.
- [ ] Record changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Clean worktrees safely; no destructive global reset/clean shortcuts.
- [ ] Handle cancellation, orphan worktrees and crash recovery deterministically.
- [ ] Add stale-base, collision, symlink/path-escape, cancellation and Windows worktree tests.

### Phase 8 – Integrator, Test Agent and independent Reviewer

- [ ] Integrator is the only component allowed to combine mutation-agent results into the integration target.
- [ ] Require diff inspection and interface/dependency compatibility before integration.
- [ ] Test Agent receives acceptance criteria + artifacts/diff rather than builder self-assessment.
- [ ] Reviewer remains independent: task + requirements + diff + test evidence, not Builder private reasoning.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [ ] Add mission-level stagnation/no-progress controls for repair cycles.
- [ ] Require suitable verification after last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

### Phase 9 – Dynamic agent spawning and replanning

- [ ] Allow Planner/Mission Manager to request validated dynamic roles from data, not Go class proliferation.
- [ ] Constrained Agent Factory: role/objective/capabilities/budget/workspace/model/parent.
- [ ] Cap team size, nesting depth, model calls, tool calls and mission duration.
- [ ] Prevent children self-granting capabilities or spawning mutation descendants outside governance policy.
- [ ] Add structured replanning after failed dependencies, changed evidence or integration conflicts.
- [ ] Add mission-wide stagnation detection across task cycles.

### Deferred tool discovery / context economy

- [ ] Add deferred/tool-search so large tool registries are not injected into every model context.
- [ ] Keep deterministic minimal core schemas; load extended capability definitions only when relevant.
- [ ] Measure context savings and task success before claiming benefit.
- [ ] Preserve stable prompt prefixes where useful for caching.

### Typed project commands

- [ ] Extend slash/project commands with typed parameters and deterministic validation/expansion.
- [ ] Keep commands as templates/instructions, never implicit shell permission.
- [ ] Preserve project-over-global precedence and current skill/command safety semantics.

### MCP breadth

- [ ] Add broader MCP transports only where auth, timeout, approval, SSRF, path and secret protections remain enforceable.
- [ ] Separate capability discovery from permission granting.
- [ ] Add health/reconnect diagnostics and fail-closed transport behavior.

### Doctor / health diagnostics

- [ ] Add structured Doctor diagnostics for Native and each external engine.
- [ ] Report model backend health, Git/worktree availability, LSP, MCP, build/test/QEMU tools and security-policy blockers.
- [ ] Do not auto-install or expose secrets outside current controlled setup/approval flows.

### Benchmarks for #32

- [ ] Extend cross-engine harness with subagent/multi-agent tasks.
- [ ] Add repository-exploration, large-tool-registry, recovery and integration-conflict benchmarks.
- [ ] Compare Native/Aider/OpenCode/Claw using same repo commit, model, quantization, context limit, task and hidden tests where supported.
- [ ] Measure success, model calls, tool calls, token/context estimates, wall time, unnecessary diff and recovery behavior.
- [ ] No parity/superiority claims without reproducible measured evidence.

---

## 3. OS-scale Mission Controller / LocalCode OS Challenge

Priority: **P1 only after stable DAG + scheduler + worktrees + integrator/reviewer/test loop**

- [ ] Discover `clang`/`gcc`, `nasm`, `ld`/`lld`, `cmake`/`make`/`ninja`, optional Rust/Cargo, `qemu-system-x86_64`, ISO/boot tooling, `gdb`, `objdump`, `readelf` through controlled tool discovery.
- [ ] Do not silently install toolchains.
- [ ] Add QEMU wrapper with timeout, owned-process cancellation and bounded logs/artifacts.
- [ ] Add machine-readable serial acceptance markers for boot/memory/scheduler/VFS/userspace stages.
- [ ] Add structured QEMU Test Agent results; “build succeeded” is insufficient.
- [ ] Stage benchmark: boot -> kernel entry -> memory -> interrupts/timer -> scheduler -> storage/filesystem -> syscalls/userspace -> keyboard/framebuffer -> FreeDoom launch.
- [ ] Persist each OS task input/context/result/diff/commit/tests/cost/duration for pause/restart/retry.
- [ ] Add visual verification only after deterministic serial/build/test criteria.
- [ ] Publish only reproducible results; never market a toy boot stub as Antigravity-equivalent.

---

## 4. Issue #30 – benchmarked llama.cpp / DMC backend

Priority: **P1 after main #32 foundation**

- [ ] Backend-neutral inference interface below Native runtime.
- [ ] Keep Ollama default/current behavior.
- [ ] Add optional loopback-only OpenAI-compatible llama.cpp adapter.
- [ ] Explicit backend selection/health; no secrets or silent provider/model drift.
- [ ] Managed process-tree lifecycle, timeout, health and restart for local llama.cpp.
- [ ] Verify runtime provenance/version before DMC label.
- [ ] Use DMC selection/rehydration only when Windows runtime truly executes it and self-tests prove it.
- [ ] Preserve dense llama.cpp and Ollama fallback.
- [ ] Benchmark same model/task/context across Ollama/dense llama.cpp/true DMC where available.
- [ ] Measure correctness, retained context, first-token latency, total runtime, peak memory/VRAM, long-context recall.
- [ ] No DMC claim without runtime evidence.

---

## 5. Repository hygiene / stale issue reconciliation

Priority: **P1; #23 immediately after PR #41**

- [ ] Verify issue #23 against merged #40; close if all bounded model-backed read-only subagent requirements are satisfied.
- [ ] Verify issue #22 against merged #36/session-wide doom-loop guard; close if fully satisfied.
- [ ] Verify issue #25 against #26/#33/#38; close if reversible quarantine + Desktop/Mobile UX acceptance is fully satisfied.
- [ ] Keep #32 open until orchestration/benchmark acceptance complete.
- [ ] Keep #30 open until backend/runtime/benchmark acceptance complete.
- [ ] Remove closed/superseded work from active TODO sections.

---

## 6. Competitive/reliability work after core orchestration

Priority: **P2**

- [ ] Prompt/context cache stability and deterministic prefix ordering.
- [ ] Context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs.
- [ ] Git diff/undo/commit UX without weakening SHA/precondition protection.
- [ ] Provider breadth only below LocalCode supervisor/safety layer.
- [ ] Structural/fuzzy patch-drift recovery without bypassing approved SHA semantics.
- [ ] Desktop/Android transparency for mission plan, task state, agents, budgets, tools, approvals, verification, recovery and integration.
- [ ] Benchmark large repos, large registries, subagents and crash recovery.

---

## 7. Quality and safety gate for every material implementation PR

Every relevant PR remains blocked until exact final head passes:

- [ ] Go setup/version
- [ ] `gofmt`
- [ ] `go vet ./...`
- [ ] frontend JavaScript syntax
- [ ] PowerShell syntax
- [ ] native Android Remote APK
- [ ] `govulncheck`
- [ ] full-stack loopback HTTP integration
- [ ] complete Go tests
- [ ] race detector
- [ ] statement coverage >=80.0%
- [ ] native Windows builds including GUI path
- [ ] `git diff --check`
- [ ] exact-head, mergeability, behind/master and review-thread checks before merge
- [ ] `STATE.md` + `TODO.md` refresh after material result/merge

Never lower the 80% gate or weaken sandbox, approvals, atomic writes, path/symlink protections, Mobile restrictions, process cancellation, secret handling or no-progress guards to make CI pass.
