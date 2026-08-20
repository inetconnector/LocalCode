# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Current verified functional master before this documentation-only refresh:** `c576f27cf75b642987aa56c7227840a133d00e07`  
**Last merged feature:** PR #42 `feat: add deterministic native agent task DAG`  
**Final tested PR #42 head:** `9bbd616d054c767030a6f6e7f0c89b8da005c545`  
**Quality #433:** success; total statement coverage **80.2%**  
**Documentation refresh carrier:** PR #43 `docs: refresh canonical state after task DAG merge`, branch `docs/state-after-task-dag`  
**Carrier baseline before this metadata refresh:** `47123d2a16e779ff3c4e13d97d0d4118d24d5707`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the authoritative exhaustive list of unfinished **functional** LocalCode work. `STATE.md` describes current verified reality and is the self-contained AI bootstrap. Completed Phase-4 implementation is no longer backlog; the next functional work is Phase 5.

**Self-resolving carrier rule:** when this content is read from `master`, PR #43 is already merged by definition. Do not create another documentation PR merely to record #43's own merge SHA/status; verify GitHub reality before resuming.

---

## 0. Permanent STATE.md + TODO.md maintenance rule

`STATE.md` and `TODO.md` MUST remain completely current together.

1. Read both before material work.
2. Verify current `master`, active implementation branch/PR, open issues, reviews and latest required Quality against GitHub reality before resuming.
3. After every material functional branch/base/head change, implementation PR/merge, CI result, roadmap/scope decision, completed milestone or safety/architecture change, update both files in the same workstream or immediately afterward.
4. Documentation-only carrier PRs may use the self-resolving convention documented in `STATE.md`; do not create an infinite chain merely to record the preceding docs-only merge.
5. `TODO.md` contains only unfinished functional work, dependencies and acceptance gates; completed work belongs in `STATE.md`, Git and closed PRs/issues.
6. Never invent self-commit/merge SHAs.
7. Never weaken the 80% Quality gate or safety boundaries to complete a task.

---

## 1. Immediate functional work – #32 Phase 5 Scheduler / Resource Manager

Priority: **P0 / NEXT FEATURE**

Phase 4 Task DAG is merged via PR #42. Start Phase 5 only from the current docs-refreshed `master` on a fresh branch.

### First Phase-5 slice – backend scheduler/resource foundation

- [ ] Read current orchestration/safety files before code changes: `src/agent_task_graph.go`, `src/agent_team_types.go`, `src/subagent_model.go`, `src/types.go`, cancellation/process helpers and current config/budget logic.
- [ ] Add focused scheduler/resource-manager types in new files rather than expanding `agent.go`.
- [ ] Separate **logical task readiness/queueing** from **actual resource admission/execution**.
- [ ] Allow multiple DAG nodes to be logically ready without implying simultaneous local-model contexts.
- [ ] Add deterministic bounded ready queues over `AgentTaskGraph`.
- [ ] Add explicit resource classes at minimum for model inference and cheap CPU/read/search work; model future build/linker and exclusive integration/test resources without activating mutation orchestration yet.
- [ ] Use a conservative local model-inference default of **one active slot** unless a later explicit configuration changes it.
- [ ] Resource requests must never grant capabilities; `RequestedCapabilities` remains inert planning data and granted `Capabilities` remain governance-controlled.
- [ ] Blocked/failed/cancelled tasks must consume no executor resource until legitimately ready/retried.
- [ ] Add deterministic task admission/release semantics and resource accounting.
- [ ] Add mission/task model-call, tool-call, estimated-token and elapsed-time budget snapshots with visible remaining values.
- [ ] Produce structured hard-stop / `budget_exhausted` outcomes when a budget is depleted; never silently continue.
- [ ] Add cancellation propagation from mission/scheduler to queued and running child tasks using existing context/process cancellation semantics where applicable.
- [ ] Add deterministic ordering and fairness/starvation protection; avoid an unbounded queue or starvation by one resource class.
- [ ] Preserve current single-agent and bounded read-only Explorer/Planner/Reviewer execution as compatibility paths.
- [ ] Do **not** add Builder mutation, Git worktrees, Integrator mutation, persistent mission storage or Mobile permission expansion in this first scheduler slice.
- [ ] Add focused tests for multiple logically ready tasks, one-slot model admission, independent cheap-resource admission, release/re-admission, blocked tasks, cancellation, budget exhaustion, deterministic ordering and fairness.
- [ ] Update DE/EN README/architecture/security docs for the implemented backend contract only; do not claim asynchronous multi-agent mutation support.
- [ ] Update `STATE.md` + `TODO.md` to exact branch/PR reality.
- [ ] Full exact-head Windows Quality and pre-merge head/behind/mergeability/review checks.
- [ ] Merge with `expected_head_sha`, then immediately refresh STATE/TODO before the next increment.

### Later Phase-5 increments

- [ ] Integrate the resource manager with real read-only child-agent dispatch above the existing bounded child runtime.
- [ ] Surface queued/running/blocked/resource/budget state in Desktop UI after backend contracts stabilize.
- [ ] Surface a narrower read-only view in Mobile Remote without widening Mobile permissions.
- [ ] Add resource health/diagnostics for model backend availability and queue saturation.
- [ ] Extend scheduler tests for larger DAGs and cancellation races.
- [ ] Benchmark logical parallelism vs actual model concurrency before making performance claims.

---

## 2. Phase 6 – Persistent missions and recovery

Priority: **P0/P1 after stable scheduler foundation**

- [ ] Introduce durable mission metadata separate from chat prose.
- [ ] Persist DAG, queued/ready/running/terminal task states, structured results, attempts, model/tool/resource usage, timestamps and verification state.
- [ ] Integrate with existing `run_journal.go`; do not create a competing recovery authority.
- [ ] On restart reconcile project/Git/postconditions before resuming; never blindly replay mutation.
- [ ] Support mission/task pause, resume, cancel and retry.
- [ ] Persist bounded mission knowledge for architecture decisions, interfaces/contracts, known failures and test results.
- [ ] Add crash/restart tests for queued, ready, running, failed and partially completed tasks.
- [ ] Preserve resource accounting and cancellation semantics across recovery.

---

## 3. Phase 7 – Git-worktree mutation agents

Priority: **P1 after scheduler + persistence**

- [ ] Add optional isolated worktree workspace type for mutation-capable child agents.
- [ ] Create worktrees only under LocalCode-managed validated paths with symlink/path protections.
- [ ] Give each Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [ ] Builder actions still pass normal validation, approvals, SHA/preconditions, backups and process rules; worktrees grant no extra authority.
- [ ] Record changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Safe cleanup/cancellation/orphan recovery; no destructive global reset/clean shortcuts.
- [ ] Add stale-base, collision, symlink/path-escape, cancellation and Windows worktree tests.

---

## 4. Phase 8 – Integrator, Test Agent and independent Reviewer

- [ ] Integrator is the only component allowed to combine mutation-agent results into the integration target.
- [ ] Require diff inspection and interface/dependency compatibility before integration.
- [ ] Test Agent receives acceptance criteria + artifacts/diff rather than Builder self-assessment.
- [ ] Reviewer remains independent: task + requirements + diff + test evidence, not Builder private reasoning.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [ ] Add mission-level stagnation/no-progress controls for repair cycles.
- [ ] Require suitable verification after the last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

---

## 5. Phase 9 – Dynamic Agent Factory and replanning

- [ ] Allow Planner/Mission Manager to request validated dynamic role labels from data rather than Go class proliferation.
- [ ] Constrained Agent Factory: role/objective/requested capabilities/budget/workspace/model/parent; governance grants actual capabilities separately.
- [ ] Cap team size, nesting depth, model calls, tool calls, resource consumption and mission duration.
- [ ] Prevent children self-granting capabilities or spawning mutation descendants outside governance policy.
- [ ] Add structured replanning after failed dependencies, changed evidence or integration conflicts.
- [ ] Add mission-wide stagnation detection across task cycles.

---

## 6. Additional #32 orchestration work

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
- [ ] Report model backend health, scheduler/resource saturation, Git/worktree availability, LSP, MCP, build/test/QEMU tools and security-policy blockers.
- [ ] Do not auto-install or expose secrets outside current controlled setup/approval flows.

### Benchmarks for #32

- [ ] Extend cross-engine harness with subagent/multi-agent tasks.
- [ ] Add repository-exploration, scheduler/resource-concurrency, large-tool-registry, recovery and integration-conflict benchmarks.
- [ ] Compare Native/Aider/OpenCode/Claw using the same repo commit, model, quantization, context limit, task and hidden tests where supported.
- [ ] Measure success, model calls, tool calls, token/context estimates, queue time, wall time, unnecessary diff and recovery behavior.
- [ ] No parity/superiority claims without reproducible measured evidence.

---

## 7. OS-scale Mission Controller / LocalCode OS Challenge

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

## 8. Issue #30 – benchmarked llama.cpp / DMC backend

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

## 9. Repository hygiene

Priority: **P1; interleave after current orchestration slice**

- [ ] Verify issue #22 against merged #36/session-wide doom-loop guard; close only if all acceptance criteria are satisfied.
- [ ] Verify issue #25 against #26/#33/#38; close only if reversible quarantine + Desktop/Mobile UX acceptance is fully satisfied.
- [ ] Keep #32 open until orchestration/benchmark acceptance is complete.
- [ ] Keep #30 open until backend/runtime/benchmark acceptance is complete.

---

## 10. Competitive/reliability work after core orchestration

Priority: **P2**

- [ ] Prompt/context cache stability and deterministic prefix ordering.
- [ ] Context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs.
- [ ] Git diff/undo/commit UX without weakening SHA/precondition protection.
- [ ] Provider breadth only below LocalCode supervisor/safety layer.
- [ ] Structural/fuzzy patch-drift recovery without bypassing approved SHA semantics.
- [ ] Desktop/Android transparency for mission plan, task state, agents, budgets, resources, tools, approvals, verification, recovery and integration.
- [ ] Benchmark large repos, large registries, subagents and crash recovery.

---

## 11. Permanent Quality and safety gate

Every material implementation PR remains blocked until its exact final head passes the standard Windows Quality workflow: Go setup/version, gofmt, vet, frontend JS syntax, PowerShell syntax, Android Remote APK, govulncheck, full-stack integration, full tests, race, statement coverage >=80.0%, Windows builds and `git diff --check`, followed by exact-head/0-behind/mergeability/review-thread checks. Never lower the 80% gate or weaken sandbox, approvals, atomic writes, path/symlink protections, Mobile restrictions, process cancellation, secret handling, budget hard stops or no-progress guards to make CI pass.
