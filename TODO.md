# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Current functional master:** `5872f7b9d9fe91d8d0c82d6cce29cbbc2cfdbf8f`  
**Active implementation branch:** `feat/native-agent-task-dag`  
**Active draft PR:** #42 `feat: add deterministic native agent task DAG`  
**PR head before this TODO refresh:** `bd7cd85b11bd0193faece982c56bc2cb6e263544`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Issue #23:** completed

This file is the authoritative exhaustive list of unfinished **functional** LocalCode work. `STATE.md` describes current reality and is the self-contained AI bootstrap. Keep both files mutually current; completed implementation details belong in `STATE.md`, Git history and merged PRs rather than remaining as open TODO items here.

---

## 0. Permanent STATE.md + TODO.md maintenance rule

`STATE.md` and `TODO.md` MUST remain completely current together.

1. Read both before material work.
2. Verify current `master`, active implementation branch/PR, open issues, reviews and latest required Quality against GitHub reality before resuming.
3. After every material branch/base/head change, implementation PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, update both files in the same workstream or immediately afterward.
4. `TODO.md` contains only unfinished functional work, dependencies and acceptance gates; history belongs in `STATE.md`, Git and closed issues/PRs.
5. A functional change is not operationally complete until both files reflect the new reality.
6. Never invent self-commit/merge SHAs.

---

## 1. Immediate work – finish PR #42 / #32 Phase 4

Priority: **P0 / current**

Phase-4 implementation is complete on `feat/native-agent-task-dag`: stable task IDs, deterministic DAG/dependency validation, ready/blocked/state-transition semantics, requested-vs-granted capability separation, inert dynamic roles, Planner schema integration, invalid-graph correction inside the bounded child loop, machine-readable `task_graph` output, focused regression tests, DE/EN documentation, and removal of all temporary integration helpers are already present. Focused DAG/Planner tests passed before the permanent integration commit.

Only these merge-gate tasks remain:

- [ ] Run the complete standard Windows Quality workflow on the exact post-STATE/TODO PR head.
- [ ] Require green Go setup/version, `gofmt`, `go vet ./...`, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, `govulncheck`, full-stack loopback integration, complete Go tests, race detector, statement coverage >=80.0%, native Windows builds and `git diff --check`.
- [ ] If Quality fails, fix only the concrete failure; do not weaken safety or the 80% gate.
- [ ] Verify the final tested head is unchanged, PR #42 is 0 behind current `master`, mergeable, has no blocking review submissions and no unresolved review threads.
- [ ] Mark PR #42 ready for review only after the exact-head Quality gate is fully green.
- [ ] Merge PR #42 only with `expected_head_sha`; no force-push/history rewrite.
- [ ] Immediately refresh `STATE.md` + `TODO.md` on resulting `master` using the functional-baseline/self-SHA convention.
- [ ] Start Phase 5 on a fresh branch from the then-current `master`; do not continue new feature work on the merged Phase-4 branch.

Explicitly out of scope for PR #42 and still unfinished: scheduler/resource queues, model-inference concurrency management, persistent missions, Builder mutation, Git worktrees, Integrator/Test-Agent mutation orchestration, Mobile permission expansion and QEMU/OS execution.

---

## 2. Issue #32 – remaining UMAF-LC / Native Agent Teams roadmap

Priority: **P0/P1 after PR #42 merge**

Architecture principle:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Do not create a rigid hierarchy of hard-coded Go agent classes. Orchestration must remain backend-independent and preserve LocalCode approval, sandbox, atomic-write, recovery and verification boundaries. Logical task parallelism must remain separate from actual local-model inference parallelism.

### Phase 5 – Scheduler and resource manager — NEXT FEATURE

- [ ] Separate logical task parallelism from model inference parallelism.
- [ ] Add deterministic bounded logical queues over the Phase-4 `AgentTaskGraph` without enabling mutation-capable children yet.
- [ ] Add resource classes/limits for model inference, CPU/read/search work, builds/linkers and future exclusive integration/test resources.
- [ ] Use conservative local-GPU/model concurrency defaults; logical task count must not imply equal simultaneous model contexts.
- [ ] Add mission/task model-call, tool-call, estimated-token and elapsed-time budget accounting with visible remaining budget.
- [ ] Produce structured budget-exhausted results and hard stops rather than silent continuation.
- [ ] Add cancellation propagation from mission -> queued/running child tasks.
- [ ] Add deterministic queue ordering plus fairness/starvation regression tests.
- [ ] Ensure failed/cancelled dependencies never consume executor resources until legitimately ready/retried.
- [ ] Preserve existing single-agent and read-only child compatibility paths.
- [ ] Surface queued/running/blocked/budget state in Desktop/Remote only after backend contracts are stable; do not widen Mobile permissions.
- [ ] Add focused scheduler/resource tests first, then full exact-head Quality.
- [ ] Keep Builder mutation/worktrees out of this Phase-5 foundation PR.

### Phase 6 – Persistent missions and recovery

- [ ] Introduce durable mission metadata separate from chat prose.
- [ ] Persist graph, task states, structured results, attempts, usage, timestamps and verification state.
- [ ] Integrate with the existing durable run journal instead of creating a competing recovery authority.
- [ ] Reconcile project/Git/postconditions before restart/resume; never blindly replay mutation.
- [ ] Support mission/task pause, resume, cancel and retry.
- [ ] Add bounded mission knowledge for architecture decisions, interfaces/contracts, known failures and test results.
- [ ] Add crash/restart tests for queued, ready, running, failed and partially completed tasks.

### Phase 7 – Git-worktree mutation agents

- [ ] Add optional isolated worktree workspace type for mutation-capable child agents.
- [ ] Create worktrees only under LocalCode-managed validated paths with symlink/path protections.
- [ ] Give each Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [ ] Builder actions still pass normal validation, approvals, SHA/preconditions, backups and process rules; worktrees grant no extra authority.
- [ ] Record changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Safe cleanup/cancellation/orphan recovery; no destructive global reset/clean shortcuts.
- [ ] Add stale-base, collision, symlink/path-escape, cancellation and Windows worktree tests.

### Phase 8 – Integrator, Test Agent and independent Reviewer

- [ ] Integrator is the only component allowed to combine mutation-agent results into the integration target.
- [ ] Require diff inspection and interface/dependency compatibility before integration.
- [ ] Test Agent receives acceptance criteria + artifacts/diff rather than Builder self-assessment.
- [ ] Reviewer remains independent: task + requirements + diff + test evidence, not Builder private reasoning.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [ ] Add mission-level stagnation/no-progress controls for repair cycles.
- [ ] Require suitable verification after the last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

### Phase 9 – Dynamic agent spawning and replanning

- [ ] Allow Planner/Mission Manager to request validated dynamic role labels from data rather than Go class proliferation.
- [ ] Constrained Agent Factory: role/objective/requested capabilities/budget/workspace/model/parent; governance grants actual capabilities separately.
- [ ] Cap team size, nesting depth, model calls, tool calls and mission duration.
- [ ] Prevent children self-granting capabilities or spawning mutation descendants outside governance policy.
- [ ] Add structured replanning after failed dependencies, changed evidence or integration conflicts.
- [ ] Add mission-wide stagnation detection across task cycles.

### Deferred tool discovery / context economy

- [ ] Add deferred/tool-search so large registries are not injected into every model context.
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
- [ ] Compare Native/Aider/OpenCode/Claw using the same repo commit, model, quantization, context limit, task and hidden tests where supported.
- [ ] Measure success, model calls, tool calls, token/context estimates, wall time, unnecessary diff and recovery behavior.
- [ ] No parity/superiority claims without reproducible measured evidence.

---

## 3. OS-scale Mission Controller / LocalCode OS Challenge

Priority: **P1 only after stable DAG + scheduler + worktrees + integrator/reviewer/test loop**

- [ ] Controlled discovery of `clang`/`gcc`, `nasm`, `ld`/`lld`, build tools, optional Rust/Cargo, `qemu-system-x86_64`, ISO/boot tools, `gdb`, `objdump`, `readelf`.
- [ ] Never silently install toolchains.
- [ ] QEMU wrapper with timeout, owned-process cancellation and bounded logs/artifacts.
- [ ] Machine-readable serial acceptance markers for boot/memory/scheduler/VFS/userspace stages.
- [ ] Structured QEMU Test Agent results; “build succeeded” is insufficient.
- [ ] Stage benchmark: boot -> kernel entry -> memory -> interrupts/timer -> scheduler -> storage/filesystem -> syscalls/userspace -> keyboard/framebuffer -> FreeDoom launch.
- [ ] Persist every OS task input/context/result/diff/commit/tests/cost/duration for pause/restart/retry.
- [ ] Visual verification only after deterministic serial/build/test criteria.
- [ ] Publish only reproducible results; never market a toy boot stub as Antigravity-equivalent.

---

## 4. Issue #30 – benchmarked llama.cpp / DMC backend

Priority: **P1 after main #32 foundation**

- [ ] Backend-neutral inference interface below Native runtime.
- [ ] Keep Ollama default/current behavior.
- [ ] Optional loopback-only OpenAI-compatible llama.cpp adapter.
- [ ] Explicit backend selection/health; no silent provider/model drift.
- [ ] Managed local llama.cpp process lifecycle, timeout, health and restart.
- [ ] Verify runtime provenance/version before DMC label.
- [ ] Use DMC selection/rehydration only when Windows runtime truly executes it and self-tests prove it.
- [ ] Preserve dense llama.cpp and Ollama fallback.
- [ ] Benchmark same model/task/context across Ollama/dense llama.cpp/true DMC where available.
- [ ] Measure correctness, retained context, first-token latency, total runtime, peak memory/VRAM and long-context recall.
- [ ] No DMC claim without runtime evidence.

---

## 5. Repository hygiene

Priority: **P1; interleave after PR #42**

- [ ] Verify issue #22 against merged #36/session-wide doom-loop guard; close only if all acceptance criteria are satisfied.
- [ ] Verify issue #25 against #26/#33/#38; close only if reversible quarantine + Desktop/Mobile UX acceptance is fully satisfied.
- [ ] Keep #32 open until orchestration/benchmark acceptance is complete.
- [ ] Keep #30 open until backend/runtime/benchmark acceptance is complete.

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

## 7. Permanent Quality and safety gate

Every material implementation PR remains blocked until its exact final head passes the repository's standard Windows Quality workflow and pre-merge checks. Never lower the 80% coverage gate or weaken sandbox, approvals, atomic writes, path/symlink protections, Mobile restrictions, process cancellation, secret handling or no-progress guards to make CI pass.
