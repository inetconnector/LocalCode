# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified functional master before this documentation-only state update:** `b12e4d4664200f9eae0f114690cb925e0b5598e1`  
**Last verified functional merge:** PR #36 `feat: block stagnant agent tool loops`  
**Open implementation PR:** none at this snapshot  

This file is the authoritative continuation document for LocalCode. Historical snapshots belong in Git history and closed PRs, not as contradictory appended sections here.

---

## 0. Permanent STATE.md maintenance rule

`STATE.md` MUST remain completely current.

This is a blocking repository invariant, not an optional documentation task:

1. Read `STATE.md`, `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` and `docs/SECURITY.md` before material changes.
2. Before starting work, verify current `master`, open PRs/issues, current branch/head, review threads and latest required Quality run against GitHub reality.
3. After every merged feature/fix, branch/base change, material CI result, roadmap decision or change to an important safety/architecture invariant, update `STATE.md` in the same workstream or immediately afterward.
4. Replace stale current facts. Do not append a second historical snapshot that contradicts older text.
5. A completed code change is not fully done while `STATE.md` still describes the previous repository reality.
6. Exact self-commit/merge SHAs cannot be known from inside the commit that records them. When necessary, record the last verified functional baseline and then update the resulting merge SHA in the next current-state update. Never invent a SHA.
7. `AGENTS.md` independently requires keeping `STATE.md` fully current; future agents must enforce both rules.

---

## 1. Product objective

LocalCode is a Windows-first local coding-agent/development platform centered on local models and controlled tool execution. Primary local-model path remains Ollama. Safety, correctness, recovery and verification are application responsibilities, not prompt-only responsibilities.

Current selectable coding engines:

- LocalCode Native
- Aider
- Claude Code
- OpenCode
- Claw Code

LocalCode remains the UI/supervisor shell even when an external engine performs a delegated operation.

Long-term objective: make LocalCode Native objectively stronger than Aider/OpenCode/Claw across repository intelligence, edit reliability, safety, recovery, orchestration, context efficiency, verification and measurable benchmark success. Do not claim superiority without implementation plus reproducible evidence.

---

## 2. Current master – major merged capabilities

### Conflict-safe and approval-bound mutation

Current master preserves:

- SHA-256 file-version preconditions
- per-path locking
- same-directory staging
- atomic replacement/move behavior, including Windows-native replacement paths
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
- Tree-sitter-backed JavaScript/JSX, TypeScript/TSX, Python, Rust, C and C++ where provider support is available
- deterministic lexical/import fallback
- weighted typed graph relations
- task-ranked source context and graph relevance
- bounded parallel independent read-only exploration
- native read-only LSP navigation
- persistent/recovering LSP session pool with project/server isolation

LSP is navigation/diagnostic authority, not an implicit mutation permission.

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

Current master includes `benchharness` plus `localcode-bench`:

- immutable base commit resolution
- fresh detached worktree per run
- argv execution without shell-string interpolation
- setup/engine/check timings, exit codes and timeouts
- hidden/required checks
- changed/unnecessary diff metrics
- optional adapter metrics
- source repository is not used as the engine worktree

Fair comparisons require the same repository commit, task, model, quantization, context limit, hidden tests and environment constraints.

### Mobile-safe project management and Play Store build path – PR #24 merged

Master includes:

- authenticated narrow mobile project actions
- project delete preview with server-generated counts
- Android Remote origin/TLS pinning hardening
- no arbitrary mobile shell/filesystem/admin surface
- phone cannot create globally persistent approval rules
- Play Store build action starts a normal LocalCode agent task rather than a publishing backdoor
- `scripts/build-playstore.ps1` uses project Gradle wrapper, verifies release artifacts and hashes them
- no automatic keystore generation/rotation, secret disclosure, Play Console upload or publication
- Quality includes PowerShell syntax checking

### Reversible project quarantine backend – PR #26 merged

Current master implements:

- `create_folder` intentionally bare
- `create_project` always creates `README.md`, `AGENTS.md` and `STATE.md`, independent of the old automatic-doc preference
- confirmed non-empty delete moves the project into LocalCode-managed same-volume quarantine rather than immediate permanent `RemoveAll`
- minimal atomic quarantine metadata
- validated list/restore/permanent-purge primitives
- restore refuses an occupied original destination
- permanent purge requires exact `PURGE <project>` confirmation
- quarantine-root and symlink escape targets are rejected
- symlink targets are not followed by preview/quarantine/purge
- no risky cross-volume copy-then-delete fallback
- project/chat references are handled deterministically

### Release-note and coverage maintenance – PR #27/#29 merged

- version-specific release-note files were consolidated into canonical `RELEASE-NOTES.md`
- real compatibility/formatting tests strengthened Quality coverage
- the statement coverage gate remains >=80.0%; it was not weakened

### Claw Code managed engine – PR #28 merged

Merge commit:

`92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`

Claw is an optional fifth coding engine and is not the default.

Implemented contract includes:

- central coding-engine router/status/setup integration
- settings/UI engine selection
- edit/repository-map/lint/test through LocalCode reliability, backup/undo, timeout and cancellation paths
- managed Windows/MSVC Rust build preparation
- exact pinned upstream revision and structured executable-version verification
- process-scoped `OLLAMA_HOST`
- ambient OpenAI/Anthropic/xAI/DashScope credential/provider variables stripped from Claw subprocesses
- `read-only` for analysis and `workspace-write` for mutation; no default/permanent `danger-full-access`
- Windows Authenticode/MSVC preparation and safe temporary batch invocation
- safe authenticated mobile engine selection, blocked while an agent is running
- explicit isolated Claw benchmark adapter
- no Claw Studio installation/launch; LocalCode remains the shell

### Mobile quarantine controls – PR #33 merged

Merge commit:

`743cf5b9b70d73f4b66a7d59761c7a489204dd7b`

Current Mobile Remote exposes the reversible quarantine backend through a deliberately narrow authenticated API:

- authenticated `GET /remote/api/project-quarantine`
- authenticated `POST /remote/api/project-quarantine-action`
- list returns server-derived quarantine entries
- restore accepts only an opaque validated quarantine ID
- purge accepts only an opaque validated ID plus exact server-checked `PURGE <project>` confirmation
- restore/purge reject requests while an agent is marked running
- invalid IDs and unsupported actions fail closed
- no user-controlled filesystem path is accepted by these endpoints
- routes exist only on the mobile-safe Remote server
- existing authenticated `/remote/api/editing-engine` route remains intact
- tests prove unauthenticated list/action requests are rejected and authenticated requests work

### Session-wide stagnant-loop/no-op guard – PR #36 merged

Merge commit:

`b12e4d4664200f9eae0f114690cb925e0b5598e1`

Final tested PR head:

`f89b0f55cfb6a0cbc57c2978374023b630d8d3b4`

LocalCode Native now has a session-scoped deterministic no-progress guard in addition to the older immediate-identical-action block:

- structured action fingerprint covers the complete action payload/arguments while deliberately ignoring only human explanation text
- normalized result fingerprint tracks whether repeated diagnostics return the same evidence
- a third unchanged structured action is blocked after two matching failures
- a third unchanged read/tool action is blocked after two identical no-progress outcomes
- repeated A/B, A/B/C and short period-4 action/result cycles are detected and blocked
- changed output from the same diagnostic is treated as new evidence and clears stale history
- successful real project mutation resets stagnation history
- successful project verification resets stagnation history
- `finish` and `ask_user` are excluded from session-loop blocking
- immediate-identical-action feedback and session-loop feedback are available in DE and EN

Native edit no-op handling is now also explicit:

- `write_file` rejects an existing file whose bytes already equal the requested content
- `replace_text` rejects a replacement that leaves content unchanged
- approval/version-bound mutation paths enforce the same rule
- known no-ops are rejected while capturing approval preconditions, before showing a pointless approval prompt
- defensive no-op checks remain directly before backup/atomic write
- no-op rejection occurs without creating a backup or fake mutation
- real writes still retain path locks, SHA/version preconditions, backups and atomic conflict-safe replacement

PR #36 was ported as a small current-master feature delta rather than merging stale historical PR #7. The final PR diff contained only six functional files; temporary branch-local patch/format workflows were removed before merge. Quality #379 passed the full required Windows pipeline on the exact final head: format, vet, JS/PowerShell syntax, Android APK, vulnerability scan, full-stack loopback, complete Go tests, race detector, coverage >=80%, native Windows builds and diff check. The branch was 0 commits behind master, mergeable and had no unresolved review threads before merge.

---

## 3. Permanent safety and quality contract

Do not weaken these invariants merely to make a feature or CI pass:

- project/root path containment and symlink/path-escape protection
- no silent overwrite after stale approval/precondition
- atomic conflict-safe writes
- no unsupervised concurrent mutation to the same workspace
- mobile permissions narrower than desktop
- no silent provider/model drift
- no default or persistent `danger-full-access` equivalent
- secrets are not logged/persisted/displayed
- external processes have timeout/cancellation/process-tree handling
- destructive operations use explicit narrow confirmation and reversible paths where designed
- all user-visible DE/EN strings remain synchronized
- deterministic anti-loop/no-op guards may not bypass approval, precondition or verification semantics

Required Windows Quality workflow includes at least:

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

## 4. Immediate next feature – issue #31 project UX across Desktop and Mobile

There is no open implementation PR at this snapshot. Start from current master after this STATE update.

Backend quarantine semantics and the narrow Mobile API are already merged. The remaining gap is coherent, non-technical user-facing project creation/deletion/recovery UX.

Required end state from issue #31:

- Desktop and Mobile clearly distinguish `New project` from `New folder`
- `New project` uses the already-merged `create_project` backend and therefore always creates `README.md`, `AGENTS.md` and `STATE.md`
- `New folder` remains intentionally empty
- empty-folder deletion remains simple and safe
- non-empty delete preview displays server-derived file/directory/byte information
- non-empty deletion wording explicitly says the project moves to recoverable LocalCode quarantine; do not call the quarantine action permanent deletion
- visible Quarantine/Trash view on Desktop and Mobile
- `Restore` is simple and visible
- permanent purge is visually separate and requires exact `PURGE <project>` confirmation
- Desktop and Mobile wording remains semantically correct in DE and EN
- restore refuses occupied destinations
- no symlink/path escape
- project/chat references remain consistent across quarantine/restore
- Mobile UI must use the already-merged PR #33 narrow API; do not add arbitrary filesystem/shell/admin endpoints

Implementation should be split into a current-master feature PR small enough to review. Full Windows Quality is required on the exact final head.

---

## 5. Open issue #32 – make LocalCode Native match/exceed Claw orchestration

Running Claw externally is implemented. Native still needs the useful architectural capabilities internally.

Target capabilities:

1. real model-backed subagents with explicit Explorer/Planner/Reviewer roles and separate curated contexts
2. optional Git-worktree isolation for mutation-capable child agents; read-only exploration stays cheaper
3. deferred/tool-search capability for large tool registries
4. structured project slash commands with typed parameters and deterministic expansion
5. explicit per-run token/tool/time budgets with visible remaining budget and hard stops
6. broader MCP transports only if LocalCode auth/timeout/approval/SSRF/path protections remain intact
7. structured machine-readable child-agent results rather than prose parsing
8. health/doctor diagnostics for external engines and Native capabilities

Safety requirements remain stronger than orchestration convenience:

- approval bound to file SHA/preconditions
- atomic conflict-safe writes
- no concurrent unsupervised same-workspace mutation
- durable crash journal remains authoritative
- child mutation must be diff-reviewable and verified before success

Historical closed PR #14 may contain useful read-only model-subagent design ideas; port feature delta only, never merge a stale branch wholesale.

---

## 6. Open issue #30 – benchmarked llama.cpp / DMC backend

Goal: introduce an inference-backend abstraction below the Native agent loop while keeping Ollama default and behavior stable.

Planned path:

- Ollama remains default
- optional loopback-only OpenAI-compatible llama.cpp backend
- backend-specific health/model discovery/process lifecycle
- no silent provider fallback/drift
- explicit UI status/selection without exposing secrets
- exact runtime provenance before any DMC label
- Windows must not be called DMC-enabled unless DMC KV selection/rehydration is actually active and self-tested
- benchmark Ollama vs dense llama.cpp vs real DMC-enabled runtime only where available
- measure correctness, retained context, latency, runtime, memory/VRAM and long-context recall

Do not confuse DMC with RAG or replace LocalCode semantic repository intelligence/context compaction with it.

---

## 7. Additional competitive work

After issue #31 and the main Native orchestration work:

- prompt/context cache stability and deterministic prefix ordering
- context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs
- Git diff/undo/commit UX polish while preserving stronger preconditions
- provider breadth kept below the supervisor/safety layer
- structural/fuzzy patch-drift recovery without bypassing approved SHA semantics
- Desktop/Android transparency for plan, phase, tools, approvals, verification and recovery
- benchmark tasks specifically exercising subagents, large repositories/tool registries and crash recovery

---

## 8. Immediate execution order

Unless a new verified blocker changes priority:

1. merge this post-#36 STATE refresh
2. implement/finish project Trash/Quarantine UX (#31) on fresh master
3. refresh `STATE.md` immediately after that merge
4. build Native orchestration/subagents (#32) in current-master reviewable increments
5. refresh `STATE.md` after each material merge
6. add/benchmark inference backend and DMC path (#30)
7. refresh `STATE.md`

Every material step ends with a fully current `STATE.md`. A future agent must never have to infer present repository reality from stale or contradictory historical sections.
