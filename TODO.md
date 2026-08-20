# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Active implementation PR:** #40 `feat: add bounded native agent team roles`  
**Code baseline before this TODO update:** `64dfc0a0758cf55fb67cc3c2512cd0608feb340e`  
**Quality on that baseline:** #397 – success  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  

This file is the authoritative list of unfinished LocalCode work. `STATE.md` describes what is true now; `TODO.md` describes what is still unfinished. They must never contradict each other.

---

## 0. Permanent STATE.md + TODO.md maintenance rule

`STATE.md` and `TODO.md` MUST remain completely current together.

This is a blocking repository invariant:

1. Read both files before material work.
2. Before starting or resuming implementation, verify current `master`, active branch/PR, open issues, review state and the latest required Quality run against GitHub reality.
3. After every material branch/base/head change, PR/merge, CI result, roadmap decision, scope change, completed milestone or safety/architecture change, update both files in the same workstream or immediately afterward.
4. `STATE.md` contains current implemented reality; `TODO.md` contains only unfinished work, dependencies and acceptance gates.
5. Remove or rewrite stale TODO entries when work is completed or superseded. Do not leave an item open merely as historical documentation; Git history and closed issues/PRs are the history.
6. Do not append contradictory snapshots. Replace stale current facts.
7. A feature/fix is not operationally complete until both files reflect the new reality.
8. Before a PR is merged, confirm that any material change to the remaining work is represented in `TODO.md`; immediately after merge, refresh both files for the resulting `master` state.
9. Exact self-commit/merge SHAs cannot be known from inside the commit that records them. Record the verified baseline honestly and update the resulting merge SHA in the next current-state refresh; never invent a SHA.
10. Future agents must enforce this rule together with `AGENTS.md` and the repository Quality/safety contract.

---

## 1. Immediate work – finish PR #40

Priority: **P0 / current**

Current feature scope already implemented on the code baseline and validated by Quality #397:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- model-backed read-only Explorer, Planner and Reviewer roles with separate contexts
- hard model-call, tool-call, elapsed-time and explicitly estimated token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- no child mutation, shell, Git, web/network, MCP, installation, memory, approval request or recursive spawning
- Planner may propose structured follow-up tasks but cannot execute mutation-capable roles
- deterministic read-only fallback on unavailable model, model failure or budget exhaustion
- mandatory edit-reliability preflight stays deterministic and consumes no child-model calls
- UI child traces use `subagent:<role>:<action>`

Remaining before #40 is complete:

- [ ] Re-run/confirm PR metadata on the latest TODO-updated head.
- [ ] Full required Quality must be green on the exact final head after this documentation change; prior #397 is evidence for the code baseline but cannot certify a changed head.
- [ ] Verify branch is 0 commits behind current `master`.
- [ ] Verify mergeability on the exact head.
- [ ] Verify no unresolved review threads and no blocking review submissions.
- [ ] Mark PR #40 ready for review only after all exact-head checks are green.
- [ ] Merge PR #40 with `expected_head_sha`; no force-push/history rewrite.
- [ ] Immediately create/update the post-merge current-state workstream so `STATE.md` and `TODO.md` describe the resulting `master` and no longer describe #40 as open.
- [ ] Decide/record whether issue #23 is fully superseded by #40; close it only after verifying all its acceptance requirements are covered.

---

## 2. Issue #32 – UMAF-LC / Native Agent Teams

Priority: **P0 after #40**

Architecture principle:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

Do not build a rigid hierarchy of dozens of hard-coded agent classes. Keep the orchestration backend-independent and preserve LocalCode's stricter approval, sandbox, atomic-write, recovery and verification boundaries.

### Phase 4 – Task DAG and dependency model

- [ ] Add persistent/runtime task identity fields for mission, parent, dependencies and dependency status without breaking the existing single-agent path.
- [ ] Define explicit task states such as proposed/blocked/ready/running/succeeded/failed/cancelled/retryable.
- [ ] Validate DAGs deterministically: reject duplicate IDs, missing dependencies and cycles.
- [ ] Make Planner output machine-readable task proposals that can be validated into DAG nodes instead of prose parsing.
- [ ] Add structured task inputs/outputs and dependency handoff data.
- [ ] Add deterministic tests for cycle rejection, dependency release, failed dependency propagation and independent parallel-ready tasks.
- [ ] Preserve project-root/sandbox and no-privilege-escalation invariants.

### Phase 5 – Scheduler and resource manager

- [ ] Separate **logical task parallelism** from **model inference parallelism**.
- [ ] Implement bounded queues for model inference, CPU/read/search work, builds/linkers and exclusive integration/test resources.
- [ ] Default local-GPU concurrency conservatively; do not start N model contexts merely because N logical tasks exist.
- [ ] Add mission-level and task-level model/tool/time/estimated-token budgets.
- [ ] Enforce hard stop semantics and structured budget-exhausted results.
- [ ] Surface queued/running/blocked state and remaining budgets in Desktop/Remote without widening Mobile permissions.
- [ ] Add fairness/starvation tests and cancellation propagation.

### Phase 6 – Persistent missions and recovery

- [ ] Introduce durable mission metadata separate from chat prose.
- [ ] Persist task graph, task status, structured results, attempts, model/tool usage, timestamps and verification state.
- [ ] Integrate with the existing durable run journal instead of creating a competing recovery mechanism.
- [ ] On restart, reconcile project/Git/postconditions before resuming; never blindly replay mutations.
- [ ] Support pause/resume/cancel/retry of individual tasks and whole missions.
- [ ] Record Architecture Decisions, known failures, interfaces/contracts and test results as bounded mission knowledge.
- [ ] Add crash/restart tests for ready, running, failed and partially integrated tasks.

### Phase 7 – Git-worktree mutation agents

- [ ] Add optional isolated worktree workspace type for mutation-capable child agents.
- [ ] Create worktrees only under a LocalCode-managed project-contained/validated area, with path/symlink protections.
- [ ] Give every Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [ ] Builder capabilities must pass through normal LocalCode validation/approval/precondition/backup/process rules; worktrees do not grant extra authority.
- [ ] Track changed files, diff, commits and verification in structured `AgentResult`.
- [ ] Clean up worktrees safely; no destructive global `git clean/reset --hard` shortcuts.
- [ ] Handle cancellation, orphan worktrees and crash recovery deterministically.
- [ ] Add collision, stale-base, symlink/path-escape, cancellation and Windows worktree tests.

### Phase 8 – Integrator, Test Agent and independent Reviewer

- [ ] Implement Integrator as the only component allowed to combine mutation-agent results into the integration target.
- [ ] Require diff inspection and dependency/interface compatibility before integration.
- [ ] Add Test Agent that receives acceptance criteria and artifacts/diff rather than builder self-assessment.
- [ ] Keep Reviewer independent: task + requirements + diff + test evidence, not the Builder's private reasoning narrative.
- [ ] Add structured PASS/FAIL/REPAIR decisions and bounded repair task proposals.
- [ ] Prevent endless repair loops through mission-level stagnation/no-progress accounting.
- [ ] Require suitable verification after the last integrated code/tool/app change.
- [ ] Preserve approval-bound SHA/file preconditions during integration.

### Phase 9 – Dynamic agent spawning and replanning

- [ ] Permit Planner/Mission Manager to spawn validated roles dynamically from data, not Go class proliferation.
- [ ] Support role/objective/capability/budget/workspace/model selection through a constrained Agent Factory.
- [ ] Cap team size, nesting/depth, total model calls, total tool calls and elapsed mission time.
- [ ] Do not permit a child to grant itself capabilities or spawn mutation-capable descendants outside scheduler/governance policy.
- [ ] Add structured replanning after failed dependencies, changed evidence or integration conflicts.
- [ ] Add mission-wide doom-loop/stagnation detection across task cycles.

### Deferred tool discovery / context economy

- [ ] Implement deferred/tool-search capability so large tool registries are not injected into every model context.
- [ ] Keep deterministic minimal core tool schemas and load extended capability definitions only when relevant.
- [ ] Measure prompt size/context savings and task success before claiming improvement.
- [ ] Maintain stable prompt prefixes where useful for local/provider cache efficiency.

### Typed project commands

- [ ] Extend slash/project commands with typed parameters and deterministic validation/expansion.
- [ ] Keep command files as instructions/templates, not implicit shell execution permissions.
- [ ] Preserve project-over-global precedence and current skill/command safety semantics.

### MCP and capability breadth

- [ ] Add broader MCP transports only where authentication, timeout, approval, SSRF, path and secret protections remain enforceable.
- [ ] Keep MCP capability discovery separate from permission granting.
- [ ] Add transport health/reconnect diagnostics and fail-closed behavior.

### Doctor / health diagnostics

- [ ] Add structured Doctor diagnostics for LocalCode Native and every external engine.
- [ ] Report model backend health, toolchains, Git/worktree availability, LSP, MCP, build/test/QEMU tool availability and security-policy blockers.
- [ ] Diagnostics must not auto-install or expose secrets without the existing controlled setup/approval flow.

### Benchmarks for #32

- [ ] Extend the cross-engine benchmark harness with multi-agent/subagent tasks.
- [ ] Add repository-exploration, large-tool-registry, recovery and integration-conflict benchmarks.
- [ ] Compare LocalCode Native against Aider/OpenCode/Claw using the same repository commit, model, quantization, context limit, task and hidden tests where supported.
- [ ] Measure success rate, model calls, tool calls, token/context estimates, wall time, unnecessary diff and recovery behavior.
- [ ] Do not claim parity/superiority without reproducible measured evidence.

---

## 3. OS-scale Mission Controller / LocalCode OS Challenge

Priority: **P1 after stable DAG + scheduler + worktrees + integrator/reviewer/test loop**

Do not begin with a 100-agent demonstration before the orchestration primitives are reliable.

- [ ] Add toolchain discovery contracts for `clang`/`gcc`, `nasm`, `ld`/`lld`, `cmake`/`make`/`ninja`, optional Rust/Cargo, `qemu-system-x86_64`, `xorriso`/boot tooling, `gdb`, `objdump` and `readelf`.
- [ ] Do not silently install toolchains; use existing discovery/setup/approval infrastructure.
- [ ] Add QEMU execution wrapper with timeout, owned-process cancellation and bounded artifact/log capture.
- [ ] Add machine-readable serial acceptance markers, e.g. boot/memory/scheduler/VFS/userspace stages.
- [ ] Add structured QEMU Test Agent result rather than accepting “build succeeded” as OS success.
- [ ] Define a staged x86-64 OS benchmark: boot → kernel entry → memory → interrupts/timer → scheduler → storage/filesystem → syscalls/userspace → keyboard/framebuffer → FreeDoom launch.
- [ ] Persist every OS task's input/context/result/diff/commit/tests/cost/duration so the mission can pause/restart/retry.
- [ ] Add screenshots/visual verification only after deterministic serial/build/test criteria exist.
- [ ] Publish benchmark methodology/results only when reproducible; do not market a toy boot stub as an Antigravity-equivalent OS result.

---

## 4. Issue #30 – benchmarked llama.cpp / DMC backend

Priority: **P1 after the main #32 orchestration foundation**

- [ ] Introduce a backend-neutral inference interface below the Native agent runtime.
- [ ] Keep Ollama as the default path and preserve existing behavior/tests.
- [ ] Add optional loopback-only OpenAI-compatible llama.cpp adapter.
- [ ] Add explicit backend selection and health/status without exposing secrets or silently drifting providers/models.
- [ ] Add managed process-tree lifecycle, timeout, health and restart handling for local llama.cpp.
- [ ] Verify exact runtime provenance/version before any DMC label.
- [ ] Port/consume DMC selection/rehydration only when the Windows runtime actually executes it and self-tests prove it.
- [ ] Preserve dense llama.cpp and Ollama fallback paths.
- [ ] Benchmark same model/task/context under Ollama vs dense llama.cpp vs true DMC-enabled runtime where available.
- [ ] Measure correctness, retained context, first-token latency, total runtime, peak memory/VRAM and long-context recall.
- [ ] No DMC marketing claim without runtime evidence.

---

## 5. Repository hygiene / stale issue reconciliation

Priority: **P1, interleave with post-merge documentation**

- [ ] Verify issue #22 against merged PR #36/session-wide doom-loop guard; close #22 if all requested feature deltas are already satisfied.
- [ ] Verify issue #25 against PRs #26/#33/#38; close #25 if reversible quarantine + Desktop/Mobile UX acceptance is fully satisfied.
- [ ] Verify issue #23 after PR #40; close it if bounded model-backed read-only subagent requirements are fully superseded/satisfied.
- [ ] Keep issue #32 open until the remaining orchestration capabilities and benchmark acceptance are actually complete.
- [ ] Keep issue #30 open until backend/runtime/benchmark acceptance is actually complete.
- [ ] Ensure closed/superseded work is removed from active TODO sections rather than left as stale backlog.

---

## 6. Competitive/reliability work after core orchestration

Priority: **P2**

- [ ] Prompt/context cache stability and deterministic prefix ordering.
- [ ] Context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs.
- [ ] Git diff/undo/commit UX improvements without weakening SHA/precondition protection.
- [ ] Provider breadth only below the LocalCode supervisor/safety layer.
- [ ] Structural/fuzzy patch-drift recovery that never bypasses approved SHA semantics.
- [ ] Desktop/Android transparency for mission plan, task state, agents, budgets, tools, approvals, verification, recovery and integration.
- [ ] Benchmark tasks for large repositories, large tool registries, subagents and crash recovery.

---

## 7. Quality and safety gate for every material implementation PR

Every relevant PR remains blocked until the exact final head passes the repository's full required Quality workflow, including at least:

- [ ] Go version/setup
- [ ] `gofmt`
- [ ] `go vet ./...`
- [ ] frontend JavaScript syntax
- [ ] PowerShell syntax
- [ ] native Android Remote APK build
- [ ] `govulncheck`
- [ ] full-stack loopback HTTP integration
- [ ] complete Go tests
- [ ] race detector
- [ ] statement coverage >=80.0%
- [ ] native Windows builds including GUI path
- [ ] `git diff --check`
- [ ] exact-head, mergeability, behind/master and review-thread checks before merge
- [ ] `STATE.md` + `TODO.md` refreshed after the material result/merge

Never lower the 80% coverage gate or weaken sandbox, approvals, atomic writes, path/symlink protections, Mobile restrictions, process cancellation, secret handling or no-progress guards to make CI pass.
