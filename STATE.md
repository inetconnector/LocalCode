# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified master:** `9bae2a2f79b2edf05ab632e373e7a111b33f8b32`
**Last merged functional feature:** PR #44 `feat: add native agent scheduler resource foundation`, merge `9bae2a2f79b2edf05ab632e373e7a111b33f8b32`
**Final tested PR #44 head:** `83ac6df8bbd3cb806030047bfb729eb84fdf9a06`
**GitHub Quality run on that head:** Actions run `32421102087` success on 2026-08-20; full PR gate passed
**Active implementation branch:** none; resume new work from `master`
**Active implementation PR:** none; PR #44 is merged and its remote branch was deleted
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities` – open  
**Current slice:** #32 Phase 5 backend Scheduler / Resource Manager foundation is merged; next increment is real read-only scheduler dispatch integration
**Canonical unfinished-work ledger:** `TODO.md`

`STATE.md` is the authoritative self-contained AI bootstrap. A newly started AI with no chat history, memory or prior context must be able to read this file and safely continue the project immediately. `TODO.md` is the exhaustive list of unfinished functional work and acceptance gates. Replace stale facts rather than appending contradictory snapshots.

---

## 0. Permanent STATE.md + TODO.md invariant

`STATE.md` and `TODO.md` MUST remain completely current and mutually consistent.

1. Before material work read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md`.
2. Verify GitHub reality before resuming: current `master`, active branch/PR, open issues, exact head, CI, reviews and review threads.
3. After every material branch/base/head change, PR/merge, CI result, roadmap/scope decision, completed milestone, known failure or safety/architecture change, update both files in the same workstream or immediately afterward.
4. `STATE.md` describes current implemented/verified reality, architecture, important files, safety/Quality invariants, known problems and exact next steps.
5. `TODO.md` contains only unfinished functional work, dependencies and acceptance criteria. Completed work belongs here in STATE, Git history and merged/closed issues/PRs.
6. Documentation-only carrier PRs may use a self-resolving convention to avoid infinite documentation-merge chains; functional changes do not get that exception.
7. Never invent a future commit, CI or merge SHA.
8. The statement-coverage Quality gate remains **>=80.0%** and may not be weakened to make CI pass.

---

## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution. It is not merely a chat UI: LocalCode acts as supervisor, policy boundary, tool runtime, project manager, recovery layer and UI around LocalCode Native and selectable external coding engines.

Current selectable coding engines:

- LocalCode Native
- Aider
- Claude Code
- OpenCode
- Claw Code

Ollama is the default local inference path. Issue #30 plans an optional backend-neutral llama.cpp/DMC path later, below the LocalCode supervisor. There must be no silent provider/model drift.

Long-term objective: make LocalCode Native objectively stronger than Aider/OpenCode/Claw in repository intelligence, edit reliability, safety, recovery, orchestration, context efficiency, verification and reproducible benchmark success. Do **not** claim parity or superiority without measured, reproducible results.

Target large-scale UMAF-LC architecture:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Agent identity is data-driven rather than a rigid hierarchy of Go classes:

`Agent = Runtime + Role + Mission + Context + Capabilities + Budget + Workspace + Parent`

A core design constraint for local hardware is:

`logical task parallelism != model inference parallelism`

A mission may contain many logically ready tasks while only one or a small bounded number of model contexts are admitted to the local GPU/CPU at once.

---

## 2. Runtime and major existing capabilities

High-level path:

`User/Desktop or Mobile UI -> LocalCode supervisor -> Native/external engine -> controlled tools -> project/Git/build/test/network/MCP boundaries -> verification -> run journal/recovery`

Important already-implemented capabilities include:

- Windows native Go application with embedded browser UI and loopback Desktop HTTP/SSE server.
- Separate narrow Mobile Remote server with pairing/token boundaries; Mobile has fewer permissions than Desktop.
- Project catalog, task/chat history, project creation, empty folders, reversible project quarantine/trash, restore and exact-confirmation permanent purge.
- Ollama discovery, model bootstrap and bounded local-model calls.
- External Aider, Claude Code, OpenCode and Claw Code engine integration under LocalCode supervision.
- Repository intelligence/reference graph and task-ranked context.
- LSP navigation.
- Git tools, build/deployment discovery and controlled commands.
- Web research with public-IP/SSRF protections.
- MCP builtin/stdio/HTTP support with explicit configuration, process control, timeouts and auth handling.
- Project/global instructions, Cursor-compatible rules, Skills and progressive skill/resource loading.
- Project slash commands as deterministic text templates; they do not grant shell/tool permission.
- Local persistent memories with secret-like-content rejection.
- Context compaction and token-economy guards.
- Attachments, SVG/raster asset creation, local image-generation boundary, image conversion and local browser rendering.
- Approval rules, SHA-bound edit preconditions, backups, atomic conflict-safe writes and postconditions.
- Task/tool hooks running inside the normal command safety boundary.
- Durable active-run journal and interrupted-run recovery.
- Session-wide repeated-action/result and short-cycle stagnation guard plus explicit no-op mutation rejection.
- Full Windows Quality workflow with race detector and >=80% statement coverage gate.

---

## 3. Permanent safety and correctness baseline

These invariants are mandatory for all future orchestration work:

- Project-root containment and canonical path/symlink/NTFS-junction escape protection.
- SHA-256/version preconditions bind approval to the previewed file state.
- Per-path locking and same-directory staging/atomic replacement where applicable.
- External modification produces a conflict instead of silent overwrite.
- Checked mutation postconditions and backup/Git recovery paths.
- Owned external processes have timeout/cancellation and Windows process-tree termination.
- No default or persistent `danger-full-access` equivalent; no silent privilege escalation.
- `RequestedCapabilities` are request/planning data only. They never become executable `Capabilities` automatically.
- Dynamic role labels emitted by a Planner are inert until governance maps them to an executable runtime.
- No concurrent unsupervised mutation of the same workspace.
- Future worktrees grant isolation, not extra authority; normal approvals/preconditions remain mandatory.
- Durable `run_journal.go` remains the recovery authority; future mission persistence must integrate with it, not compete with it.
- Mobile permissions remain narrower than Desktop.
- Secrets are not intentionally persisted/logged/displayed.
- Child mutation must eventually be diff-reviewable and verified before mission success.
- Phase 5 first slice remains **child-mutation free**.

---

## 4. Important continuation files / code map

Repository rules and docs:

- `AGENTS.md` – repository-wide coding, security, localization and STATE/TODO rules.
- `STATE.md` – this full AI bootstrap.
- `TODO.md` – exhaustive unfinished functional roadmap.
- `README.md` – DE/EN user-facing product and feature contract.
- `docs/ARCHITECTURE.md` – runtime/component boundaries.
- `docs/SECURITY.md` – application security model and permission boundaries.
- `.github/workflows/quality.yml` – mandatory Windows Quality gate; triggers on `master`, PRs to `master`, or manual dispatch.

Native runtime/orchestration:

- `src/agent.go` – main Native action schema, main agent loop, approvals and tool dispatch.
- `src/agent_supervisor.go` – deterministic intent/supervisor logic.
- `src/edit_reliability.go` – deterministic edit-reliability preflight.
- `src/agent_loop_guard.go` – session-wide no-progress/repeated-action guard.
- `src/subagent.go` – deterministic read-only repository handoff/fallback.
- `src/agent_team_types.go` – AgentRole, AgentTask, AgentBudget, AgentUsage, AgentResult, capabilities and structured child contracts.
- `src/subagent_model.go` – bounded model-backed read-only Explorer/Planner/Reviewer runtime.
- `src/agent_task_graph.go` – Phase-4 deterministic DAG validation, readiness/dependency reconciliation and state transitions.
- `src/agent_scheduler.go` – **active Phase-5 branch:** backend-only bounded scheduler/resource-manager foundation.
- `src/agent_scheduler_test.go` and `src/agent_scheduler_additional_test.go` – **active Phase-5 branch:** focused scheduler/resource/budget/cancellation tests.
- `src/run_journal.go` – durable active-run/recovery state; future mission persistence must extend this authority.
- `src/types.go` – `Config`, `AppState`, global main-agent runtime state.
- `src/config.go` – default configuration; first scheduler slice intentionally does not yet add persisted scheduler concurrency settings.

Project/UI/remote boundaries:

- `src/server.go` – Desktop loopback API.
- `src/remote_server.go` and mobile-safe API files – paired Mobile Remote boundary.
- `src/project_catalog.go`, `src/project_quarantine*.go` – project management/quarantine.
- `src/static/ui_polish.js` – Desktop UI behavior.
- `src/static/remote.html` – narrow Mobile Remote UI.

Prefer focused new files for orchestration rather than growing `agent.go` into a monolith. Preserve the existing single-main-agent and bounded read-only child paths while new orchestration layers are added above them.

---

## 5. Merged Native Agent Teams foundation – PR #40

PR #40 merged as `97bdd80e8d068bcc6622ba8296b43ea7c8ea1bc8`; final feature head `9c3b25b1b070d80c075e9b697a9fffe86f0d3184` passed Quality #406.

Merged capabilities:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- model-backed read-only Explorer, Planner and Reviewer roles
- separate curated child-model contexts
- hard per-child model-call, tool-call, elapsed-time and estimated-token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- mutation, shell, Git, network/web, MCP, install, memory, approval requests and recursive spawning absent from child schema
- Planner may emit structured follow-up proposals but cannot execute mutation-capable roles
- Reviewer independent of Builder reasoning
- deterministic read-only fallback when model unavailable/fails or budget ends
- deterministic mandatory edit-reliability preflight
- visible `subagent:<role>:<action>` traces

Issue #23 was closed after this capability was verified; historical PR #14 was not stale-merged wholesale.

---

## 6. Merged Phase 4 – deterministic Native Agent Task DAG, PR #42

PR #42 merged as `c576f27cf75b642987aa56c7227840a133d00e07`. Final tested feature head `9bbd616d054c767030a6f6e7f0c89b8da005c545`; Quality #433 passed with **80.2%** total statement coverage.

Merged capability:

- stable explicit machine-readable `AgentTaskProposal.ID`
- `AgentTaskGraph{MissionID, Tasks}`
- deterministic validation for invalid/duplicate IDs, missing/self/duplicate/cyclic dependencies and inconsistent mission/state/parent metadata
- graph-specific `proposed`, `ready`, `succeeded`, `cancelled`, `retryable` states while retaining legacy standalone child states
- deterministic dependency reconciliation and ready/blocked state
- successful dependency release and failed/cancelled dependency blocking
- controlled state transitions and retry semantics
- Planner schema requires stable task IDs; dependencies reference IDs rather than prose roles
- invalid Planner graphs are rejected and corrected inside the existing bounded child-model loop
- machine-readable `task_graph` included in formatted Planner results
- Planner dynamic role labels remain inert plan data
- Planner `RequestedCapabilities` remain separate from actually granted `Capabilities`; DAG creation grants no authority
- focused regression tests and synchronized DE/EN architecture/security documentation

Not implemented by #42: scheduler/resource queues, model-concurrency management, mission persistence, Builder mutation, worktrees, Integrator mutation, Mobile permission expansion or QEMU/OS mission execution.

---

## 7. Merged Phase-5 first slice – Scheduler / Resource Manager foundation

Branch `feat/native-agent-scheduler-foundation` was created from master `3c2752e47b9b267152c8f9b84e359dbfbcf55b68`, completed as PR #44 and merged to `master` as `9bae2a2f79b2edf05ab632e373e7a111b33f8b32` on 2026-08-20. The remote branch was deleted after merge.

### Implemented by PR #44

`src/agent_scheduler.go` adds backend-only contracts; it does **not** dispatch mutation-capable children.

Resource model:

- `AgentResourceClass`: `model-inference`, `read-cpu`, `build`, `exclusive-integration`.
- `AgentResourceLimits` uses bounded defaults: max queued 256, model inference **1**, read/CPU 4, build 1, exclusive integration 1.
- Logical ready queue and actual resource admission are separate.
- Queue order is deterministic from DAG/task order.
- A saturated resource class can be skipped so a later task using a free different class may run; older tasks of the same resource class retain precedence.
- Queue insertion is bounded and atomic: if adding the current ready set exceeds the configured queue limit, no partial append occurs.
- Queued entries are pruned if the DAG state is no longer `ready`.

Governance/security admission:

- Planner-proposed `RequestedCapabilities` do not grant executable authority.
- Admission requires the executable role to be one of the current Native read-only roles and requires the role's actual granted capabilities in `AgentTask.Capabilities`.
- Dynamic labels such as `kernel-memory-specialist` remain queued/visible planning data but are not admitted to an executable runtime.
- Default resource class for executable current child tasks is model inference; explicit backend resource overrides can classify cheap read/CPU work without changing permissions.

Runtime accounting:

- Resource admission creates an exact scheduler lease with child `context.Context` under a mission context.
- Admission transitions DAG task `ready -> running` only after role/capability/resource checks succeed.
- Release validates the exact lease, performs the normal DAG state transition and releases the resource slot.
- Per-task cancellation removes queued entries or cancels a running child context and releases the resource.
- Mission cancellation cancels all scheduler-owned active contexts, clears queued/active resource ownership and transitions queued/running scheduler tasks to `cancelled` where allowed.
- `AgentSchedulerSnapshot` exposes mission ID, queued/running counts, per-resource limits/in-use/available state and per-task queue/resource/running state.
- Generic `AgentBudgetSnapshot` computes limit, usage, remaining model/tool/token/time budget plus structured exhausted reason. It can represent a task or mission budget without changing permissions.
- `agentBudgetHardStop` returns `AgentResultBudgetExhausted` rather than silently continuing after an exhausted budget.
- Release now cleans up the scheduler-owned lease if the graph task was externally moved out of `running` before release; invalid transitions are still reported as errors.
- Nil graph snapshots and mission cancellation are safe and expose resource state without panicking.
- Local test isolation now hides real user Claw installations from download-policy tests and accepts localized German/English setup-decline/disabled wording where the behavior is the same.
- Windows build checksum generation no longer depends on the `Get-FileHash` cmdlet; `scripts/build.ps1` uses .NET SHA-256 hashing so isolated Windows PowerShell builds can still produce `CHECKSUMS-SHA256.txt`.
- Native agent guidance now states that the local default environment is Windows/PowerShell. Missing POSIX `rm` attempts are redirected toward `delete_file`/`copy_path`/`move_path` or explicit PowerShell `Remove-Item`, and avoidable manual-`rm` questions are blocked.
- Windows startup and Remote LAN firewall helper PowerShell processes now run through hidden-window paths: `START.bat` passes `-WindowStyle Hidden`, the outer firewall runner uses `hideCommandWindow`, and the elevated firewall rule process requests `-WindowStyle Hidden`.
- The Desktop composer now exposes a visible `engineSelect` next to the model selector for LocalCode, Aider, OpenCode, Claude Code and Claw Code. Changing it immediately persists `editing_engine`, keeps Settings synchronized and disables the selector while a run is active. Fresh desktop defaults now use `native`/LocalCode instead of Aider.
- Desktop attachments already supported file picker, paste and drag-and-drop; `src/static/remote.html` now supports Remote drag-and-drop attachments with matching client-side file count/size limits before the shared backend attachment validation.
- The Remote pairing screen now uses a real form submit path and a top-aligned scrollable keyboard-safe layout so Android soft-keyboard resizing does not collapse the `Verbinden`/`Connect` button.
- Remote tabs now support horizontal touch swipe gestures in addition to explicit tab buttons, while form controls and buttons keep their normal touch behavior.
- The Android shell now implements WebView `onShowFileChooser`, so the Remote attachment button opens Android's native picker for all file types and returns selected URIs to the browser file input.
- The Android shell now exposes a narrow `LocalCodeAndroid.startVoiceInput()` bridge that starts Android speech recognition and returns recognized text to the Remote prompt; browser/WebView pages without the native bridge can fall back to Web Speech when available.
- Remote send handling now uses a direct click handler, send-in-progress lock, disabled-state updates for empty/running/sending states, and visible alert errors instead of silent rejected promises.
- Remote on first use defaults to the `native`/LocalCode engine through phone-local preference storage; later manual engine choices are remembered on the phone.
- LocalCode now includes a shared SVG favicon, a generated multi-size Windows ICO embedded through `src/rsrc_windows_amd64.syso`, and an Android vector launcher icon referenced from the manifest.

Focused tests added:

- one-model-slot admission and release/re-admission
- independent read/CPU resource progress while model slot is saturated
- same-resource fairness/no starvation of the older queued model task
- dependency readiness and release
- requested-vs-granted capability separation and dynamic-role non-execution
- mission cancellation of queued/running tasks and child contexts
- task cancellation for queued and active leases
- stale-lease rejection and release on failure
- bounded queue atomicity
- duplicate QueueReady idempotence and non-ready pruning
- parent-context cancellation
- all budget exhaustion dimensions and zero-clamped remaining values
- active resource/task snapshots
- invalid resource-class rejection
- release cleanup after an invalid graph transition caused by an externally terminal task
- nil graph mission-cancel and snapshot robustness
- localized setup-policy and install-decline assertions for the local wave3/bootstrap tests
- Windows/POSIX `rm` prompt and recovery redirection
- Desktop and Remote composer attachment drag-and-drop affordances

Synchronized DE/EN documentation now describes the backend scheduler contract, Remote/Android polish, engine defaults, app icons and Windows-helper behavior in `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/CODING-ENGINES.md`, `android/README.md`, `STATE.md` and `TODO.md` without claiming real asynchronous child-agent dispatch.

Verification for PR #44 final head `83ac6df8bbd3cb806030047bfb729eb84fdf9a06`:

- `go version` – local Go `go1.26.6 windows/amd64` (GitHub Quality uses Go `1.25.13`).
- `gofmt -l` over all Go files – clean.
- `go vet ./...` in `src` – passed.
- Frontend JavaScript syntax – passed with bundled Codex Node `C:\Users\frede\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe`; system `node` was not on PATH.
- PowerShell syntax for `scripts/*.ps1` – passed, 6 files.
- Focused Remote firewall regression `go test -count=1 -run "TestRemoteFirewall|TestFullStackLoopbackDesktopAndRemoteHTTP|TestRemotePairing" .` – passed.
- Focused desktop engine selector/default regression `go test -count=1 -run "Test.*Engine|Test.*Aider|TestRuntimeBootstrap|TestRequiredModels|TestDefault.*Config|TestSettingsEndpoint|TestComposerSupportsEnterAndGeneralAttachments" .` – passed.
- Focused regression `go test -count=1 -run "TestComposerSupportsEnterAndGeneralAttachments|TestRemoteComposerSupportsDragDropAttachments|TestValidateGeneralAttachments|TestExtractTextAndZipAttachments|TestWindowsPromptAndRecoveryAvoidPOSIXRemoveTool|TestBlockedAvoidanceQuestion|TestWindowsPowerShellScriptEncodings|TestBuildBatchUsesPowerShellDriver" .` – passed.
- Frontend JavaScript syntax for `src/static/i18n_base.js`, embedded `src/static/index.html` scripts and embedded `src/static/remote.html` scripts – passed with bundled Codex Node.
- Native Android Remote APK – passed via `./scripts/build-android.ps1`; final installed debug APK SHA-256 `f55f9142e248485dd480f6b2812d598753be0038b3a3f0faa3fc307e9f952833`. `aapt dump badging` confirms `application-icon-160:'res/drawable/ic_launcher.xml'`.
- `govulncheck@v1.1.4 ./...` – passed, no vulnerabilities found.
- Full-stack loopback HTTP integration `go test ./... -run '^TestFullStackLoopbackDesktopAndRemoteHTTP$' -count=1 -timeout=5m` – passed.
- `go test -count=1 ./...` in `src` – passed.
- `go test ./... -race -count=1 -timeout=25m` in `src` – passed.
- Coverage `go test ./... -count=1 -covermode=atomic` plus `go tool cover` – passed, total statement coverage **80.4%**.
- Native Windows GUI and diagnostics builds – passed to `C:\Users\frede\AppData\Local\Temp\localcode-build-check`.
- Final `cmd /c BUILD.bat` after Remote drag-and-drop, Android pairing/send/voice/file-picker/default-engine fixes, app icons, Windows/POSIX portability changes, hidden startup/Remote-firewall PowerShell helpers and the Desktop composer engine selector – passed all six stages on 2026-08-20. Generated checksums: `LocalCode.exe` SHA-256 `DBDC00DB66988C66E4AE6E439BEF05BFE44C65899F835E7A3566AC83F38F2DD5`; `LocalCode-Debug.exe` SHA-256 `ECA211FAED46C42EA3A598B2757142CD3ABA371689F5392EDD364D9F7D891550`.
- `dist\LocalCode.exe` was started after the final build; observed process PID `23016` from `C:\Users\frede\Projekte\LocalCode\dist\LocalCode.exe`. Desktop status returned version `6.4.4`, `editing_engine:"native"` and selected model `qwen2.5-coder:14b`; the served Desktop HTML contains `engineSelect`, `value="claw"` and `changeEditingEngine(e.target.value)`.
- `git diff --check` – passed; Git only warned that `CHECKSUMS-SHA256.txt`, `android/app/src/main/AndroidManifest.xml`, `android/app/src/main/java/com/inetconnector/localcode/remote/MainActivity.java` and `scripts/build-android.ps1` line endings will normalize when Git touches those files.
- GitHub Actions run `32421102087` for PR #44 head `83ac6df8bbd3cb806030047bfb729eb84fdf9a06` – success on 2026-08-20. The job passed setup, gofmt, vet, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, vulnerability scan, full-stack loopback HTTP integration, complete tests, race detector, coverage, native Windows builds and `git diff --check`.

The previously known `TestCoverageServerEndpointMatrix` timing flake has a merged stabilization: after `/api/chat`, the test now waits for the agent to become idle and fails explicitly if it remains running, rather than continuing to `/api/new-chat` and receiving the correct product 409 response.

`\\diskstation\Dani\Universal_Multi_Agent_Framework_UMAF.md` was reviewed on 2026-08-20 as a target architecture reference, not as executable instructions. LocalCode currently covers only parts of that UMAF target: governance/safety policy, planning/DAG, first scheduler/resource admission, read-only child-agent analysis, attachment ingestion, memory, validation gates and controlled build/delivery flows. Missing UMAF layers are now planned in `TODO.md`: explicit mission objects, structured task messaging, constrained Agent Factory/capability registry, real bounded parallel swarms, Integrator/Test/Security agents, observability/metrics, self-healing recovery, knowledge graph/reuse and any distributed/hyper-scale operation.

### Explicitly out of scope for this first Phase-5 slice

- real scheduler-driven child-agent dispatch integration (planned as the next Phase-5 increment after the backend contract is stable)
- mutation-capable Builder agents
- Git worktrees
- Integrator/Test-Agent mutation orchestration
- durable mission persistence
- Mobile permission widening
- QEMU/OS execution

---

## 8. Known Quality flake discovered during PR #43

PR #43 was documentation-only. Its first Quality #437 attempt passed setup, format, vet, JS, PowerShell, Android, vulnerability scan, full-stack integration, complete tests and race detector, but the coverage run failed inside `TestCoverageServerEndpointMatrix`.

Observed failure:

- test expected HTTP 200 but received 409 `Agent läuft gerade`
- location: `src/coverage_expansion_test.go`, around the chat/new-chat lifecycle
- coverage run then reported 79.4% only because the test process failed

Root cause visible in the test: after starting `/api/chat`, the test waits at most 3 seconds for `state.Running` to become false, but it does not assert that the agent really stopped before continuing to `/api/new-chat`. On a slow coverage run the agent may still be active and the endpoint correctly returns 409.

The exact same unchanged PR #43 head was rerun once. The second complete Quality #437 attempt passed **all** gates, including coverage, Windows builds and diff check. That proves the docs diff did not cause the failure and strongly identifies a timing/test-isolation flake. A deterministic stabilization of this test remains desirable; do not lower coverage or loosen product behavior to hide it.

---

## 9. Quality contract

Every material implementation PR must pass the standard Windows Quality workflow on its exact final head:

- Go setup/version
- gofmt
- `go vet ./...`
- frontend JavaScript syntax
- PowerShell syntax
- native Android Remote APK
- `govulncheck`
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- total statement coverage >=80.0%
- native Windows builds including GUI path
- `git diff --check`

Before merge additionally verify exact head unchanged, 0 behind current `master`, mergeable, no blocking reviews and no unresolved review threads. Merge only with `expected_head_sha`. Never weaken safety or the coverage threshold to make a run pass.

---

## 10. Remaining roadmap

### Issue #32 – UMAF-LC / Native orchestration

Phase 1–3: merged via #40.  
Phase 4 Task DAG: merged via #42.  
Phase 5 Scheduler/Resource Manager: **active branch described above**.

After a stable Phase-5 backend foundation:

1. integrate scheduler/resource admission with real read-only Explorer/Planner/Reviewer dispatch
2. surface read-only queue/resource/budget state in Desktop and a narrower Mobile view without permission expansion
3. add resource health/diagnostics and larger-DAG/cancellation-race tests
4. Phase 6 durable missions/recovery integrated with `run_journal.go`
5. Phase 7 optional isolated Git-worktree mutation agents under normal approvals/preconditions
6. Phase 8 Integrator + Test Agent + independent Reviewer loop
7. Phase 9 constrained dynamic Agent Factory, replanning and mission-level stagnation controls
8. deferred tool discovery/context economy, typed project commands, broader safe MCP transports and Doctor diagnostics
9. reproducible multi-agent benchmarks against Native/Aider/OpenCode/Claw using same model/repo/task where supported
10. OS/QEMU challenge only after scheduler, persistence, worktrees and independent integration/review/test loops are stable

### OS-scale LocalCode challenge

Eventually the benchmark may build a bootable x86-64 OS with memory, interrupts, scheduler, drivers, filesystem, userspace and FreeDoom under QEMU. Success must be machine-verifiable through build artifacts, bounded QEMU execution and structured serial/test markers. Do not market a toy boot stub as Antigravity-equivalent.

### Issue #30 – llama.cpp / DMC backend

Still open and intentionally separate from orchestration. Ollama remains default. Future backend abstraction must be explicit and loopback/local by default. Do not claim DMC execution until runtime provenance/self-tests prove real DMC behavior. Benchmark Ollama vs dense llama.cpp vs true DMC where available.

### Repository hygiene

- Issue #23: completed.
- Issue #22: still needs reconciliation against merged session-wide doom-loop guard #36 before closing.
- Issue #25: still needs reconciliation against project quarantine/Trash work #26/#33/#38 before closing.
- Keep #32 open until orchestration and benchmark acceptance are complete.
- Keep #30 open until backend/runtime/benchmark acceptance is complete.

---

## 11. Exact next action

From `master` at `9bae2a2f79b2edf05ab632e373e7a111b33f8b32`:

1. Delete only already-merged stale local/remote branches after confirming they are contained in `master`.
2. Start the next #32 Phase-5 increment on a fresh branch from `master`.
3. Integrate the scheduler/resource manager with actual read-only Explorer/Planner/Reviewer dispatch above the existing bounded child runtime.
4. Keep child mutation, worktrees, Integrator mutation and durable mission persistence out of scope until the read-only dispatch path is measured and verified.

`TODO.md` is authoritative for every unfinished functional item.
