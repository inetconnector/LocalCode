# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Current verified master:** `3c2752e47b9b267152c8f9b84e359dbfbcf55b68`  
**Last merged functional feature:** PR #42 `feat: add deterministic native agent task DAG`  
**Post-feature state refresh:** PR #43 merged as `3c2752e47b9b267152c8f9b84e359dbfbcf55b68`  
**Active implementation branch:** `feat/native-agent-scheduler-foundation`  
**Active implementation PR:** draft PR #44 `feat: add native agent scheduler resource foundation`, remote head `98be9b583d724d75625ceebdc277c358ab921192`, merge state `CLEAN` when checked on 2026-08-20
**Local continuation after PR remote head:** uncommitted scheduler robustness, test-isolation/coverage stabilization, Windows build/checksum portability, POSIX-`rm` avoidance, hidden Windows startup/Remote-firewall PowerShell helpers, Desktop composer engine selector with LocalCode-native default, Remote drag-and-drop attachments, mobile Remote pairing keyboard/form fix, Remote tab swipe gestures, Android WebView all-file chooser support, Android Remote voice input, Remote send robustness, Remote LocalCode default engine, LocalCode favicon/Windows resource icon/Android launcher icon, UMAF audit notes, tests, DE/EN documentation updates and local Quality-style verification in this working tree
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
- release cleanup when a scheduler-owned task is externally moved out of `running` before release
- nil graph snapshot and mission-cancel robustness
- deterministic `TestCoverageServerEndpointMatrix` idle wait before `/api/new-chat`, preserving product 409 behavior
- local test isolation for Claw setup-download policy and localized German/English disabled/declined setup assertions
- focused tests for one-slot model admission, different-resource progress, fairness, dependencies, capability non-escalation, cancellation, stale leases, queue bounds/idempotence/pruning, budget exhaustion, snapshots, invalid-transition cleanup and nil graph robustness
- synchronized DE/EN documentation in README, architecture and security docs for the backend scheduler contract only
- Windows build checksum generation no longer depends on the `Get-FileHash` cmdlet; `scripts/build.ps1` computes SHA-256 through .NET so the packaged build works in the isolated Windows PowerShell environment
- LocalCode Native now explicitly treats Windows/PowerShell as the default local environment, redirects missing POSIX `rm` failures toward LocalCode file tools or PowerShell `Remove-Item`, and blocks avoidable user questions asking for manual `rm`
- Windows startup and Remote LAN firewall helper PowerShell processes now run hidden instead of flashing a visible blue PowerShell window during normal app startup
- Desktop now has a visible composer engine selector for LocalCode, Aider, OpenCode, Claude Code and Claw Code, and fresh desktop configs default to LocalCode/native instead of Aider
- Desktop attachments already supported file picker, paste and drag-and-drop; Mobile/Remote attachments now also accept drag-and-drop and enforce matching client-side file count/size limits
- Mobile/Remote pairing now uses a submit form and top-aligned keyboard-safe scrolling so the Android soft keyboard cannot collapse the connect button
- Mobile/Remote tabs now support horizontal swipe gestures without stealing touches from form controls or buttons
- Android WebView now handles file chooser requests, so the Remote attachment button opens the native Android picker for all file types
- Android Remote now has voice input through Android speech recognition with a Web Speech fallback where available
- Mobile/Remote send now uses a direct click handler, send lock, disabled-state updates and visible alert errors
- Mobile/Remote defaults to the LocalCode/native engine on first use and remembers later phone-side engine choices
- LocalCode now ships its own favicon, Windows executable icon resource and Android launcher icon instead of falling back to generic browser/platform icons
- local Quality-style verification passed: gofmt, vet, JS syntax with bundled Node, PowerShell syntax, focused Remote/Android/icon/Desktop-engine regressions, Android APK with launcher-icon badging, govulncheck, full-stack integration, full tests, race detector, coverage 80.4%, native Windows builds, final `BUILD.bat` and `git diff --check`
- `STATE.md` and `TODO.md` refreshed for draft PR #44 and local continuation reality

### Still required before this slice may merge

- [ ] Review the current uncommitted local diff on top of PR #44 remote head `98be9b583d724d75625ceebdc277c358ab921192`.
- [ ] If accepted, explicitly authorize staging, committing and pushing the current local changes; publish rules require separate explicit authorization for each step.
- [ ] After push, update STATE/TODO to the new exact PR #44 head and CI reality.
- [ ] Do **not** claim real asynchronous multi-agent dispatch yet; this first slice is backend admission/accounting only.
- [ ] Run/monitor complete exact-final-head GitHub Windows Quality after push: setup/version, gofmt, vet, JS syntax, PowerShell syntax, Android APK, govulncheck, full-stack loopback, complete tests, race, total statement coverage >=80.0%, native Windows builds, `git diff --check`.
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

### UMAF audit from `\\diskstation\Dani\Universal_Multi_Agent_Framework_UMAF.md`

The attached UMAF Markdown was reviewed on 2026-08-20 as a target architecture reference, not as executable project instructions. Current LocalCode already partially covers governance/safety policy, planning/DAG, first resource scheduling, read-only child agents, attachment ingestion, local memory, validation gates and controlled delivery/build flows. The following UMAF capabilities remain functional roadmap items:

- [ ] Mission Manager layer: explicit mission objects with objective, scope, requirements, constraints, success criteria, risks and approval state separate from chat prose.
- [ ] Coordination/message layer: structured task messages/events with sender, recipient, task ID, parent ID, priority, timestamp, correlation ID, payload and status.
- [ ] Agent Factory/capability registry: validated dynamic role creation, lifecycle state, performance profiles and governance-granted executable capabilities.
- [ ] Real parallel specialized swarms: Engineering/Research/Reviewer/Test roles dispatched through the scheduler while model and mutation resources remain bounded.
- [ ] Integration layer: deterministic Integrator that merges child outputs, resolves dependency/interface conflicts and preserves approval/file preconditions.
- [ ] Testing and Security agents: independent Unit/Integration/E2E/Regression/Performance/Security review agents that consume artifacts and evidence rather than Builder self-assessment.
- [ ] Observability/metrics: mission telemetry for success rate, error rate, throughput, coverage, risk, latency, resource utilization and agent performance.
- [ ] Self-healing/recovery: checkpointing, retry/reassignment, partial-failure recovery and post-restart reconciliation through durable mission state.
- [ ] Knowledge graph/reuse: bounded project knowledge graph and lessons-learned retrieval tied to architecture decisions and verified evidence.
- [ ] Distributed/hyper-scale operation: only after local single-machine scheduler, persistence, worktrees, integration and safety boundaries are proven; do not claim 1000+ agent support before measured implementation.

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
