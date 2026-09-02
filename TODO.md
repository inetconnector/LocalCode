# LocalCode – canonical TODO / kanonische Aufgabenliste

**Verified:** 2026-09-02 Europe/Berlin  
**Authoritative merged base for this slice:** master `0a83117` (Tag: `v6.5.0`)  
**Last merged functional PR:** #77 `feat: complete ADB mastery, auto launcher detection, reverse tunneling, and REST API` (Docs: #76, Fixes: #75, Pipeline: #74)  
**Active branch:** `master`  
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

## Documentation/cleanup acceptance gates

- [ ] Keep `README.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `STATE.md` and this file consistent with merged reality.
- [ ] Do not treat stale merged feature refs or historical non-current Actions runs as active development state.
