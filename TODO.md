# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-09-05 Europe/Berlin
**Authoritative merged base for this slice:** master `f9c171b` (Tag: `v6.9.0`)
**Last merged functional PR:** #87 `feat: autonomous browser automation, Windows UI desktop agent, and mobile TTS feedback` (Extras & OCR: #84, Docs: #85, Android OpenAI: #82, State sync: #83, VM Sandbox: #80, Benchmarks: #79, Docs: #78, ADB: #77)
**Active branch:** `release/v6.9.0-installer-automation` with persistent Mission Knowledge, repaired Playwright UI smoke harness, and native installer packaging fixes
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

## P1 – Phase 7: mutation-capable Builder agents in Git worktrees

- [x] Add an optional LocalCode-managed worktree workspace type for mutation-capable children.
- [x] Validate managed worktree paths against path/symlink/junction escapes.
- [x] Give each Builder task its own branch/worktree; never allow unsupervised concurrent mutation of the same workspace.
- [x] Preserve normal LocalCode approvals, SHA/version preconditions, backups, atomic writes and command/network policy inside worktrees.
- [x] Record changed files, diff, commits and verification in structured `AgentResult`.
- [x] Add safe cancellation/cleanup/orphan recovery; no destructive global reset/clean shortcuts.
- [x] Add stale-base, collision, path-escape, cancellation and Windows worktree tests.

## P1 – Phase 8: Integrator, Test Agent and independent Reviewer

- [x] Make Integrator the only component allowed to combine mutation-agent results into an integration target.
- [x] Require diff inspection and dependency/interface compatibility before integration.
- [x] Test Agent receives acceptance criteria plus artifacts/diff, not Builder self-assessment.
- [x] Reviewer receives requirements/diff/test evidence, not Builder private reasoning.
- [x] Add structured PASS/FAIL/REPAIR decisions and bounded repair proposals.
- [x] Add mission-level no-progress/stagnation controls for repair cycles.
- [x] Require suitable verification after the last integrated code/tool/app change.
- [x] Preserve approval-bound SHA/file preconditions during integration.

## P1 – Phase ComputeMesh: Decentralized Cluster Subsystem (Provider Self-Compute)

- [x] Auto-discovery of ComputeMesh provider credentials from `.computemesh/provider_config.json`, env vars, or local workstation probe (`http://localhost:8080`).
- [x] Bearer token auth injection (`Authorization: Bearer <key>`) across all outbound `OllamaClient` requests (`Tags`, `Show`, `PullWithProgress`, `DescribeImages`, `Chat`).
- [x] Gateway and local node probing (`https://computemesh.inetconnector.com/api/tags`), latency measurement, cluster hardware discovery (RTX 3080 GPU + LAN pool: 24.0 GB VRAM & 48.6 TFLOPS), and available model discovery.
- [x] REST endpoints `/api/computemesh/status`, `/api/computemesh/autodetect`, and `/api/computemesh/test`.
- [x] UI settings card, action buttons (*Status prüfen*, *Keys & Node jetzt einlesen*, *Verbindung testen*), and dual German/English localization with 100% key parity.

## P1 – Phase 9: constrained Agent Factory and replanning

- [x] Add a constrained Agent Factory that maps validated role/capability templates to implemented runtimes (`src/agent_factory.go`).
- [x] Keep dynamic Planner role labels inert until mapped by governance (`MapDynamicRoleToGovernedRole` with quarantine fallback).
- [x] Add bounded replanning after blocked/failed tasks without infinite task spawning (`src/agent_mission_replanning.go`, max 32 tasks).
- [x] Add per-mission maximum task/agent/attempt limits (max 3 replan cycles per task, max depth 5).
- [x] Add dependency-aware retry/replan rules and structured reasons (`ReplanRecord` and symptom hashing stagnation protection).
- [x] Add deferred tool/skill discovery for child agents without capability escalation (`ResolveDeferredTools`).

## P2 – broader runtime/platform work

- [x] Optional backend-neutral local inference path such as llama.cpp/DMC without silent model/provider drift (`src/inference_backend.go`).
- [x] Stronger health/doctor diagnostics for local models, tools, engines, MCP, Android Remote and orchestration resources (`src/doctor.go`, `GET /api/doctor`).
- [x] Benchmark LocalCode Native against Aider/OpenCode/Claw on reproducible repository tasks before claiming superiority (`src/benchharness/`, `src/cmd/localcode-bench-*`, `scripts/run-engine-benchmarks.ps1`).
- [x] Evaluate QEMU/VM/OS-building workflows only behind explicit sandbox/resource boundaries and reproducible tests (`src/vm_sandbox.go`, `src/doctor.go`, `src/vm_sandbox_test.go`).

## P1 – autonomous browser and desktop control

- [x] Turn browser automation into a first-class controlled capability: discover/enable the Playwright MCP server from Settings/Doctor, expose page inspection, navigation, click/type, screenshot and extraction actions through explicit agent tools, and keep public web research separate from authenticated/browser-session control.
- [x] Add a Windows desktop-control backend for visible applications using UI Automation or an equivalent inspectable accessibility tree. Required actions: list windows, inspect controls/text, click a named/located button, type text, read current UI state and capture screenshots.
- [x] Gate desktop/app control behind explicit configuration, foreground/allowlist boundaries, approvals, timeouts, cancellation and full action logging. Never allow prompts, memories, skills or webpage text to silently grant unrestricted OS control.
- [x] Add regression tests and an E2E smoke harness for browser navigation and a deterministic Windows test app so requests like "click the button in that program" can be verified reproducibly.
- [x] Extended Mobile Remote with Android native TextToSpeech (TTS) bridge and Web Speech API fallback, on-demand read-aloud response buttons, and dynamic settings synchronization.

## Documentation/cleanup acceptance gates

- [x] Keep `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `STATE.md` and this file consistent with merged reality.
- [x] Do not treat stale merged feature refs or historical non-current Actions runs as active development state.
- [x] Before merging the local native installer registry-quoting and installer/launcher icon fixes, repair or refresh the Browser UI smoke harness and rerun `python scripts\ui-e2e-test.py` green. Completed checks: `go fmt ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `go test -count=1 ./cmd/localcode-setup`, focused Windows packaging icon tests, direct Windows-amd64 GUI/diagnostics/setup builds, `scripts\build-installer.ps1`, native setup icon extraction, JavaScript syntax checks via Visual Studio Node.js `v24.12.0`, PowerShell AST syntax parser across all `scripts/*.ps1`, Android APK build, prior silent install, binary hash comparison, shortcut checks, User `PATH` check, HKCU uninstall metadata check, and `python scripts\ui-e2e-test.py` passing 100% green (`FULL UI E2E OK 41 requests`). Optional Inno compilation remains unrun because `ISCC.exe` is not installed/found.
