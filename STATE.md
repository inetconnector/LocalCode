# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current functional master:** `4a45134a8b7defa9c3b86e4779e9a774e22a9ee1`  
**Last merged current-state update:** PR #39, merge `4a45134a8b7defa9c3b86e4779e9a774e22a9ee1`  
**Active implementation PR:** #40 `feat: add bounded native agent team roles`  
**Active branch:** `feat/native-agent-teams`  
**Last fully Quality-verified code baseline for #40:** `64dfc0a0758cf55fb67cc3c2512cd0608feb340e`  
**Quality on that code baseline:** #397 – success  
**Documentation head immediately before this STATE update:** `2bfaff04d44676d712d6ee7635f10d7b4db63076`  
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`  
**Canonical unfinished-work list:** `TODO.md`  

`STATE.md` is the authoritative description of what is true now. `TODO.md` is the authoritative list of what is still unfinished. Git history and closed PRs/issues are the history; neither file should accumulate contradictory snapshots.

---

## 0. Permanent STATE.md + TODO.md maintenance rule

`STATE.md` and `TODO.md` MUST remain completely current and mutually consistent.

This is a blocking repository invariant:

1. Read `AGENTS.md`, `STATE.md`, `TODO.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md` before material work.
2. Before starting or resuming implementation, verify current `master`, active branch/PR, open issues, current head, review state and the latest required Quality run against GitHub reality.
3. After every material branch/base/head change, PR/merge, CI result, roadmap decision, scope change, completed milestone or safety/architecture change, refresh both files in the same workstream or immediately afterward.
4. `STATE.md` describes currently implemented/verified reality. `TODO.md` contains only unfinished work, dependencies and acceptance gates.
5. Replace stale facts; do not append a second contradictory snapshot.
6. A code change is not operationally complete while either file still describes the previous repository reality.
7. Before merge, confirm that material changes to remaining work are represented in `TODO.md`; immediately after merge, update both files for the resulting `master` state.
8. Exact self-commit/merge SHAs cannot be known from inside the commit recording them. Record the verified baseline honestly and update the resulting SHA in the next current-state refresh; never invent a SHA.
9. `AGENTS.md` independently requires this dual-file maintenance rule; future agents must enforce it.

---

## 1. Product objective

LocalCode is a Windows-first local coding-agent/development platform centered on local models and controlled tool execution. The primary local-model path remains Ollama. Safety, correctness, recovery and verification are application responsibilities, not prompt-only responsibilities.

Current selectable coding engines on `master`:

- LocalCode Native
- Aider
- Claude Code
- OpenCode
- Claw Code

LocalCode remains the UI/supervisor shell even when an external engine performs delegated work.

Long-term objective: make LocalCode Native objectively stronger than Aider/OpenCode/Claw across repository intelligence, edit reliability, safety, recovery, orchestration, context efficiency, verification and reproducible benchmark success. Do not claim parity or superiority without implementation plus measured evidence.

---

## 2. Current `master` – major merged capabilities

### Conflict-safe and approval-bound mutation

Current master preserves:

- SHA-256 file-version preconditions
- per-path locking
- same-directory staging
- atomic replacement/move behavior including Windows-native paths
- conflict instead of silent overwrite after external change
- approval bound to the exact file version used for preview
- checked mutation postconditions
- backup/Git recovery paths
- process-tree cancellation for owned external processes

Never regress approved edits to unchecked overwrite behavior.

### Repository intelligence and semantic navigation

Current master includes:

- import-aware repository graph
- Go compiler AST path
- Tree-sitter-backed JavaScript/JSX, TypeScript/TSX, Python, Rust, C and C++ where supported
- deterministic lexical/import fallback
- weighted typed graph relations
- task-ranked source context and graph relevance
- bounded parallel independent read-only exploration
- native read-only LSP navigation
- persistent/recovering LSP session pool with project/server isolation

LSP is navigation/diagnostic authority, not implicit mutation permission.

### Durable agent recovery

Current master has crash-safe active-run recovery:

- persistent redacted run journal under LocalCode app data
- atomic/mutex-protected journal updates
- no second full transcript or secret-bearing command-output archive
- interrupted-run detection
- recovery handoff that forbids blind mutation replay
- project/Git/postconditions must be re-read before continuing
- unrelated new tasks do not inherit stale interrupted context

### Cross-engine benchmark harness

Current master includes `benchharness` plus `localcode-bench` with:

- immutable base commit resolution
- fresh detached worktree per run
- argv execution without shell-string interpolation
- setup/engine/check timings, exit codes and timeouts
- hidden/required checks
- changed/unnecessary diff metrics
- optional adapter metrics
- source repository never used as the engine worktree

Fair comparisons require the same repository commit, task, model, quantization, context limit, hidden tests and environment constraints.

### Mobile-safe project management / Play Store path

Current master includes authenticated narrow mobile project actions, server-side project-delete preview, Android Remote origin/TLS pinning hardening, no arbitrary mobile shell/filesystem/admin surface, no globally persistent approval creation from the phone, and a controlled Play Store build path that does not generate/rotate keystores or publish automatically.

### Reversible project quarantine and full project UX

The merged project lifecycle now provides:

- `create_folder` intentionally bare
- `create_project` creating `README.md`, `AGENTS.md` and `STATE.md`
- same-volume managed quarantine instead of immediate recursive destruction for confirmed non-empty deletion
- atomic quarantine metadata
- validated list/restore/permanent-purge primitives
- occupied-target refusal on restore
- exact `PURGE <project>` permanent purge confirmation
- quarantine-root and symlink/path-escape protections
- no cross-volume copy-then-delete fallback
- deterministic project/chat handling
- Desktop and Mobile distinction between New project and New folder
- server-generated non-empty delete preview counts
- exact case-sensitive project-name confirmation
- visible Trash/Quarantine view with Restore on Desktop and Mobile
- preserved project threads reactivated on restore
- Mobile API remaining narrower than Desktop

PR #38 merged as `f84a38f25a09231fb973022867e9177e8de04974`; issue #31 is completed.

### Claw Code managed engine

PR #28 merged as `92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`.

Claw remains optional and non-default. LocalCode enforces managed/pinned executable verification, process-scoped `OLLAMA_HOST`, ambient cloud credential stripping, read-only/workspace-write sandbox choices, no default/permanent `danger-full-access`, Mobile-safe engine selection and isolated benchmark integration. LocalCode remains the shell; Claw Studio is not installed/launched.

### Mobile quarantine controls

PR #33 merged as `743cf5b9b70d73f4b66a7d59761c7a489204dd7b`.

Mobile Remote exposes only authenticated narrow quarantine list/restore/purge actions using opaque IDs. Restore/purge are blocked while an agent runs; unsupported actions/invalid IDs fail closed; no arbitrary filesystem path is accepted.

### Session-wide stagnant-loop/no-op guard

PR #36 merged as `b12e4d4664200f9eae0f114690cb925e0b5598e1`.

Current Native protections include structured action fingerprints, repeated unchanged action/result and short-cycle detection, changed output as new evidence, reset on real mutation/verification, and explicit no-op rejection for `write_file`/`replace_text` before pointless approval/backup/write. Real writes retain locks, SHA/version preconditions, backups and atomic conflict-safe replacement.

---

## 3. Active work – PR #40 / first UMAF-LC Native Agent Teams foundation

PR #40 is open and remains draft until the exact final head is fully revalidated after the documentation updates in this workstream.

The last fully tested code baseline is:

`64dfc0a0758cf55fb67cc3c2512cd0608feb340e`

Quality #397 completed successfully on that exact code baseline, including format, vet, frontend syntax, PowerShell syntax, Android Remote APK, vulnerability scan, full-stack loopback integration, complete Go tests, race detector, statement coverage >=80%, native Windows builds and final diff check.

Implemented on #40:

- reusable `AgentTask`, `AgentBudget`, capability and structured `AgentResult` contracts
- real model-backed read-only Explorer, Planner and Reviewer roles
- separate child model context
- hard model-call, tool-call, elapsed-time and explicitly estimated token budgets
- child action schema limited to `list_files`, `read_file`, `search_text`, approval-free `lsp`, `finish`
- no child mutation, shell, Git, network/web, MCP, installation, memory, approval request or recursive spawning
- Planner can return structured task proposals but cannot execute Builder/mutation roles
- Reviewer is a separate role designed for explicit task/evidence rather than builder self-justification
- deterministic read-only fallback if no model is available, the child fails or budget is exhausted
- mandatory edit-reliability preflight stays deterministic and consumes no child-model calls
- child steps surfaced as `subagent:<role>:<action>` events
- README, architecture and security documentation updated in German/English

Not implemented by #40 and intentionally deferred:

- Task DAG scheduling
- mission persistence
- mutation-capable Builder agents
- Git-worktree isolation for child mutation
- Integrator/Test-Agent orchestration
- dynamic large agent teams/replanning
- OS/QEMU mission execution

The detailed unfinished work and acceptance order are canonical in `TODO.md`.

Because `TODO.md`, `AGENTS.md` and this `STATE.md` were changed after code baseline `64dfc0a0…`, Quality #397 does not certify the new documentation head. Full required Quality must pass again on the exact final PR head before #40 may be marked ready/merged.

---

## 4. Permanent safety and quality contract

Do not weaken these invariants merely to make orchestration or CI pass:

- project/root path containment and symlink/path-escape protection
- no silent overwrite after stale approval/precondition
- atomic conflict-safe writes
- no unsupervised concurrent mutation to the same workspace
- Mobile permissions narrower than Desktop
- no silent provider/model drift
- no default or persistent `danger-full-access` equivalent
- secrets are not logged/persisted/displayed
- external processes have timeout/cancellation/process-tree handling
- destructive operations use explicit narrow confirmation and reversible paths where designed
- all user-visible DE/EN strings remain synchronized
- deterministic anti-loop/no-op guards may not bypass approval, precondition or verification semantics
- child-agent roles/capabilities are data constrained and cannot self-escalate

Required Windows Quality includes at least:

- Go version/setup
- gofmt
- `go vet ./...`
- frontend JavaScript syntax
- PowerShell syntax
- native Android Remote APK build
- `govulncheck`
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- statement coverage >=80.0%
- native Windows builds including GUI path
- `git diff --check`

Never lower the 80% threshold to rescue a PR.

---

## 5. Roadmap state

### Issue #32 – Native orchestration / UMAF-LC

Issue #32 remains open. PR #40 implements the first safe runtime foundation only.

After #40, the intended order is:

1. Task DAG + dependency validation
2. scheduler/resource manager separating logical parallelism from model-inference parallelism
3. durable missions + recovery integration
4. Git-worktree mutation agents
5. Integrator + Test Agent + independent Reviewer loop
6. dynamic Agent Factory / replanning / mission-level stagnation controls
7. deferred tool discovery, typed commands, broader safe MCP transports and Doctor diagnostics
8. reproducible multi-agent benchmark expansion
9. OS-scale QEMU challenge only after the underlying primitives are stable

Detailed tasks and acceptance criteria are maintained only in `TODO.md` to avoid duplicate drifting backlogs.

### Issue #30 – benchmarked llama.cpp / DMC backend

Issue #30 remains open and follows the main orchestration foundation. Ollama stays default. Any llama.cpp backend must remain backend-neutral below the supervisor, loopback/local by default, explicit about provider/model selection, lifecycle/timeout managed, and must not be called DMC-enabled until runtime markers/self-tests prove real DMC KV selection/rehydration.

### Repository issue hygiene

Open issues #22, #23 and #25 require reconciliation against already merged/superseding work. They remain TODO items until verified and closed appropriately; they must not silently remain stale forever.

---

## 6. Immediate execution order

Unless a new verified blocker changes priority:

1. run the full required Quality workflow on the exact final #40 documentation-inclusive head
2. verify #40 head, 0-behind/master, mergeability, reviews/threads
3. mark #40 ready and merge only with `expected_head_sha`
4. immediately refresh `STATE.md` and `TODO.md` for the resulting `master`
5. reconcile/close stale issues where acceptance is demonstrably satisfied
6. continue #32 with Task DAG + scheduler in a fresh small current-master PR
7. update `STATE.md` and `TODO.md` after every material step/merge
8. extend measured orchestration benchmarks before any parity/superiority claim
9. implement/benchmark issue #30 after the main #32 foundation

Every material step ends with both `STATE.md` and `TODO.md` current. A future agent must never have to infer repository reality from stale or contradictory documents.
