# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Current verified master:** `3c2752e47b9b267152c8f9b84e359dbfbcf55b68`  
**Last merged functional feature:** PR #42 `feat: add deterministic native agent task DAG`  
**Post-feature state refresh:** PR #43 merged as `3c2752e47b9b267152c8f9b84e359dbfbcf55b68`  
**Active implementation branch:** `feat/native-agent-scheduler-foundation`  
**Implementation/test baseline before current STATE/TODO refresh:** `ff50afc033786baf3d3e727b604f840a7a9a6e14`  
**Active implementation PR:** none; previous draft-PR creation attempt was blocked by the connector safety gate and search confirmed no PR exists  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the authoritative exhaustive list of unfinished **functional** work. `STATE.md` describes current implemented reality and is the self-contained AI bootstrap. Remove completed items rather than preserving historical checkboxes.

---

## 0. Permanent STATE.md + TODO.md rule

- [ ] Keep `STATE.md` and `TODO.md` fully current and mutually consistent after every material branch/head/PR/CI/merge/roadmap/scope/safety change.
- [ ] Before material implementation, read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md` and the relevant code.
- [ ] Verify current GitHub master, active branch/PR, issues, exact head, CI, reviews and review threads before resuming.
- [ ] Keep TODO limited to unfinished functional work/dependencies/acceptance gates; history belongs in STATE/Git/closed PRs/issues.
- [ ] Never invent future SHAs and never lower the >=80.0% coverage gate or weaken safety to finish a task.

These are permanent operating rules, not milestones to delete.

---

## 1. P0 – finish #32 Phase 5 first slice: backend Scheduler / Resource Manager

### Already implemented on `feat/native-agent-scheduler-foundation`

Completed branch work, therefore not open backlog:

- focused `src/agent_scheduler.go` instead of enlarging `agent.go`
- logical DAG readiness separated from actual resource admission
- deterministic bounded ready queue
- resource classes: model inference, cheap read/CPU, future build, future exclusive integration
- conservative default **1 active model-inference slot**
- bounded defaults and queue cap
- deterministic admission/release accounting and exact leases
- free resource classes may progress while another class is saturated; same-class older tasks keep precedence
- blocked/failed/cancelled/non-ready DAG tasks are not admitted
- `RequestedCapabilities` never self-grant executable authority
- dynamic Planner role labels remain non-executable planning data
- scheduler admission requires current executable Native role plus actually granted capabilities
- per-task and mission-context cancellation propagation for scheduler-owned work
- machine-readable resource/task scheduler snapshots
- generic model/tool/token/time budget snapshots with remaining values and exhaustion reason
- structured `AgentResultBudgetExhausted` hard-stop helper
- focused tests for one-slot model admission, different-resource progress, fairness, dependencies, capability non-escalation, cancellation, stale leases, queue bounds/idempotence/pruning, budget exhaustion and snapshots
- `STATE.md` and `TODO.md` refreshed for the active branch and known PR-publication block

### Still required before this slice may merge

- [ ] Review `src/agent_scheduler.go` for nil/error-state robustness and make only justified fixes.
- [ ] Perform a local isolated syntax/type sanity check where possible; authoritative compile/test remains Windows Quality.
- [ ] Ensure new scheduler production code is sufficiently covered to keep repository total >=80.0%; add tests rather than reducing the gate.
- [ ] Add/verify explicit test for no resource leak if cancellation/release encounters an invalid graph transition.
- [ ] Decide whether generic `AgentBudgetSnapshot` is sufficient for both mission/task contracts in this backend-only slice; if not, add a minimal explicit mission snapshot without persistence or UI coupling.
- [ ] Stabilize the known `TestCoverageServerEndpointMatrix` timing flake if an exact safe edit path is available. Product behavior must remain unchanged.
- [ ] Add synchronized DE/EN documentation to README / `docs/ARCHITECTURE.md` / `docs/SECURITY.md` for the implemented scheduler contract only.
- [ ] Do **not** claim real asynchronous multi-agent dispatch yet; this first slice is backend admission/accounting only.
- [ ] Verify no existing PR for branch, then create exactly one draft PR to `master` when connector publication is permitted. Previous attempt failed and created nothing; do not blindly duplicate.
- [ ] After PR creation, update STATE/TODO to exact PR number/head/CI reality.
- [ ] Run focused scheduler/DAG tests and fix concrete failures.
- [ ] Run complete exact-final-head Windows Quality: setup/version, gofmt, vet, JS syntax, PowerShell syntax, Android APK, govulncheck, full-stack loopback, complete tests, race, total statement coverage >=80.0%, native Windows builds, `git diff --check`.
- [ ] If the known coverage test flakes, inspect exact failure. A retry is acceptable only when failure is demonstrably the same timing flake; do not normalize repeated unexplained retries.
- [ ] Verify exact head unchanged, 0 behind current `master`, mergeable, no blocking reviews and no unresolved review threads.
- [ ] Mark ready and merge only with `expected_head_sha`; no force push/history rewrite.
- [ ] Immediately refresh STATE/TODO on resulting `master` before any next feature increment.

Explicitly out of scope for this first Phase-5 slice:

- mutation-capable Builder agents
- Git worktrees
- Integrator/Test-Agent mutation orchestration
- durable mission storage/recovery extension
- Mobile permission expansion
- QEMU/OS execution

---

## 2. P0 – Phase 5 later increments after backend foundation merge

- [ ] Integrate the scheduler/resource manager with actual read-only Explorer/Planner/Reviewer dispatch above the existing bounded child runtime.
- [ ] Ensure multiple logically ready tasks can be represented while local model execution remains bounded independently.
- [ ] Connect actual child usage to scheduler/task/mission budget accounting without double-counting the existing child budgets.
- [ ] Add scheduler-owned result collection and deterministic graph transition after child completion/fallback/budget exhaustion.
- [ ] Add cancellation-race tests for queued, admitted, completing and already-terminal tasks.
- [ ] Add larger-DAG fairness/resource-saturation tests.
- [ ] Surface queued/running/blocked/resource/budget state in Desktop after backend contracts stabilize.
- [ ] Add a narrower read-only Mobile Remote view without widening Mobile permissions.
- [ ] Add model-backend/resource saturation diagnostics.
- [ ] Benchmark logical parallelism vs actual model concurrency before any performance claim.

---

## 3. P0/P1 – Phase 6 durable missions and recovery

- [ ] Introduce durable mission metadata separate from chat prose.
- [ ] Persist DAG, queued/ready/running/terminal states, structured results, attempts, model/tool/resource usage, timestamps and verification state.
- [ ] Integrate with `run_journal.go`; do not create a competing recovery authority.
- [ ] Reconcile project/Git/postconditions on restart before resuming; never blindly replay mutation.
- [ ] Support mission/task pause, resume, cancel and retry.
- [ ] Preserve resource/budget accounting across recovery.
- [ ] Add bounded Mission Memory/Knowledge for architecture decisions, subsystem contracts, known failures and test evidence.
- [ ] Add crash/restart tests for queued, ready, running, failed and partially completed work.

---

## 4. P1 – Phase 7 Git-worktree mutation agents

- [ ] Add optional LocalCode-managed worktree workspace type for mutation-capable children.
- [ ] Validate all managed worktree paths against symlink/junction/path escape.
- [ ] Give each Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [ ] Keep normal LocalCode validation, approvals, SHA preconditions, backup and process rules inside worktrees.
- [ ] Record changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Add safe cancellation/cleanup/orphan recovery; no destructive global reset/clean shortcuts.
- [ ] Add stale-base, collision, path-escape, cancellation and Windows worktree tests.

---

## 5. P1 – Phase 8 Integrator, Test Agent and independent Reviewer

- [ ] Make Integrator the only component allowed to combine mutation-agent results into the integration target.
- [ ] Require diff inspection and dependency/interface compatibility before integration.
- [ ] Test Agent receives acceptance criteria + artifacts/diff, not Builder self-assessment.
- [ ] Reviewer receives task/requirements/diff/test evidence, not Builder private reasoning.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [ ] Add mission-level no-progress/stagnation controls for repair cycles.
- [ ] Require suitable verification after the last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

---

## 6. P1 – Phase 9 constrained dynamic Agent Factory and replanning

- [ ] Allow validated dynamic role labels as data without creating hard-coded Go agent classes.
- [ ] Agent Factory input: role, objective, requested capabilities, budget, workspace, model, parent; governance grants executable capabilities separately.
- [ ] Cap team size, nesting depth, model calls, tool calls, resources and mission duration.
- [ ] Prevent child self-granting or unauthorized mutation descendants.
- [ ] Add structured replanning for failed dependencies, changed evidence and integration conflicts.
- [ ] Add mission-wide stagnation detection across task cycles.

---

## 7. Additional #32 orchestration work

### Deferred tool discovery / context economy

- [ ] Add deferred/tool-search so large registries are not injected into every model context.
- [ ] Keep a deterministic minimal core schema and load extended capability definitions only when relevant.
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
- [ ] Do not auto-install or expose secrets outside existing controlled setup/approval flows.

### Benchmarks

- [ ] Extend cross-engine harness with subagent/multi-agent tasks.
- [ ] Add repository-exploration, scheduler/resource-concurrency, large-tool-registry, recovery and integration-conflict benchmarks.
- [ ] Compare Native/Aider/OpenCode/Claw using same repo commit, model, quantization, context limit, task and hidden tests where supported.
- [ ] Measure success, model calls, tool calls, token/context estimates, queue time, wall time, unnecessary diff and recovery behavior.
- [ ] No parity/superiority claims without reproducible evidence.

---

## 8. P1 – OS-scale Mission Controller / LocalCode OS Challenge

Only after stable DAG + scheduler + persistence + worktrees + Integrator/Reviewer/Test loop:

- [ ] Controlled discovery of `clang`/`gcc`, `nasm`, `ld`/`lld`, build tools, optional Rust/Cargo, `qemu-system-x86_64`, ISO/boot tools, `gdb`, `objdump`, `readelf`.
- [ ] Never silently install toolchains.
- [ ] QEMU wrapper with timeout, owned-process cancellation and bounded logs/artifacts.
- [ ] Machine-readable serial acceptance markers for boot/memory/scheduler/VFS/userspace stages.
- [ ] Structured QEMU Test Agent results; “build succeeded” alone is insufficient.
- [ ] Stage benchmark: boot -> kernel entry -> memory -> interrupts/timer -> scheduler -> storage/filesystem -> syscalls/userspace -> keyboard/framebuffer -> FreeDoom launch.
- [ ] Persist task input/context/result/diff/commit/tests/cost/duration for pause/restart/retry.
- [ ] Use visual verification only after deterministic serial/build/test criteria.
- [ ] Publish only reproducible results; never market a toy boot stub as Antigravity-equivalent.

---

## 9. P1 – Issue #30 benchmarked llama.cpp / DMC backend

- [ ] Add backend-neutral inference interface below Native runtime.
- [ ] Keep Ollama default/current behavior.
- [ ] Optional loopback-only OpenAI-compatible llama.cpp adapter.
- [ ] Explicit backend/model selection and health; no silent provider drift.
- [ ] Managed local llama.cpp lifecycle, timeout, health and restart.
- [ ] Verify runtime provenance/version before any DMC label.
- [ ] Use DMC selection/rehydration only when Windows runtime truly executes it and self-tests prove it.
- [ ] Preserve dense llama.cpp and Ollama fallback.
- [ ] Benchmark same model/task/context across Ollama/dense llama.cpp/true DMC where available.
- [ ] Measure correctness, retained context, first-token latency, total runtime, peak memory/VRAM and long-context recall.
- [ ] No DMC claim without runtime evidence.

---

## 10. Repository hygiene / known reliability issue

- [ ] Stabilize `TestCoverageServerEndpointMatrix`: current test waits only three seconds for the started agent to stop and can then receive correct HTTP 409 `Agent läuft gerade` during coverage. Preserve product semantics; fix test synchronization, not the endpoint or coverage gate.
- [ ] Verify issue #22 against merged #36/session-wide doom-loop guard and close only if all acceptance criteria are satisfied.
- [ ] Verify issue #25 against #26/#33/#38 and close only if reversible quarantine + Desktop/Mobile UX acceptance is fully satisfied.
- [ ] Keep #32 open until orchestration/benchmark acceptance is complete.
- [ ] Keep #30 open until backend/runtime/benchmark acceptance is complete.

---

## 11. P2 competitive/reliability work after core orchestration

- [ ] Prompt/context cache stability and deterministic prefix ordering.
- [ ] Context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs.
- [ ] Git diff/undo/commit UX without weakening SHA/precondition protection.
- [ ] Provider breadth only below LocalCode supervisor/safety layer.
- [ ] Structural/fuzzy patch-drift recovery without bypassing approved SHA semantics.
- [ ] Desktop/Android transparency for mission plan, task state, agents, budgets, resources, tools, approvals, verification, recovery and integration.
- [ ] Benchmark large repos, large registries, subagents and crash recovery.

---

## 12. Permanent merge gate for every material implementation PR

- [ ] Exact final head passes Go setup/version.
- [ ] `gofmt` clean.
- [ ] `go vet ./...` green.
- [ ] Frontend JavaScript syntax green.
- [ ] PowerShell syntax green.
- [ ] Native Android Remote APK green.
- [ ] `govulncheck` green.
- [ ] Full-stack loopback HTTP integration green.
- [ ] Complete Go tests green.
- [ ] Race detector green.
- [ ] Total statement coverage >=80.0%.
- [ ] Native Windows builds including GUI path green.
- [ ] `git diff --check` green.
- [ ] Exact head unchanged, branch 0 behind master, mergeable, no blocking reviews, no unresolved review threads.
- [ ] Merge only with `expected_head_sha`.
- [ ] Immediately refresh STATE/TODO before starting the next functional increment.
