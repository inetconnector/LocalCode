# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified functional master before this documentation-only state update:** `743cf5b9b70d73f4b66a7d59761c7a489204dd7b`  
**Last verified functional merge:** PR #33 `feat: add safe mobile project quarantine controls`  
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

### Quality coverage strengthening – PR #29 merged

The >=80% statement coverage gate remains intact. Real compatibility/formatting tests were added rather than weakening CI.

### Release-note consolidation – PR #27 merged

Version-specific release-note files were consolidated into canonical `RELEASE-NOTES.md`.

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

PR #28 passed the complete Windows Quality contract before merge.

### Mobile quarantine controls – PR #33 merged

Merge commit:

`743cf5b9b70d73f4b66a7d59761c7a489204dd7b`

Current Mobile Remote now exposes the already-merged reversible quarantine backend through a deliberately narrow authenticated API:

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
- regression tests prove unauthenticated list/action requests return Unauthorized and authenticated requests work
- regression tests also cover list/restore, exact purge confirmation, running-agent block, invalid IDs and forbidden actions

PR #33 was synchronized onto the post-Claw/post-STATE master without force push, was 0 commits behind master, had no unresolved review threads, and passed the complete Windows Quality workflow on exact head `49821752857da147bb7fdd4122f733e6bd0d5564` before merge.

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

## 4. Immediate next reliability feature – session-wide doom-loop/no-op guard

There is no active implementation PR for this feature at this snapshot. Start from the latest master after this STATE update.

Do NOT merge stale historical PR #7 or its branch wholesale. Port only the useful feature delta onto current code.

Historical source branch actually available in GitHub:

`agent/session-doom-loop-guard`

Required behavior:

- reject `write_file` when requested bytes are already identical
- reject `replace_text` when replacement produces no actual change
- preserve current path locks, SHA/version preconditions, atomic writes and backups for real edits
- structured action fingerprint includes tool/edit payload and arguments but ignores human explanation text
- structured outcome/progress fingerprint
- block a third unchanged action after two matching failures
- block a third unchanged read/tool action after two identical no-progress outcomes
- detect short repeated A/B and A/B/C cycles; period-4 detection is acceptable if tested and not over-broad
- changed tool output counts as new evidence and clears stale history
- successful real project mutation resets stagnation history
- successful project verification resets stagnation history
- `finish` and `ask_user` are excluded
- preserve the existing immediate identical-action block as a complementary first layer
- user-visible loop warnings/hints must remain correct in DE and EN; do not port German-only UI strings blindly

Useful historical tests cover payload-sensitive fingerprints, repeated failures across intervening actions, same outcomes, changed outcomes, mutation reset, 2/3/4-step cycles, control actions and no-op edits.

Suggested PR theme:

`feat: block stagnant agent tool loops`

---

## 5. Open issue #31 – finish project UX across Desktop and Mobile

Backend quarantine semantics and the narrow Mobile API are now merged. Remaining work is primarily coherent visual UX and consistency.

Required end state:

- Desktop and Mobile clearly distinguish `New project` from `New folder`
- `New project` uses the already-merged `create_project` semantics
- empty-folder deletion remains simple and safe
- non-empty deletion wording clearly says the project moves to recoverable LocalCode quarantine
- visible Quarantine/Trash UI on Desktop and Mobile
- `Restore` surfaced simply
- permanent purge visually separated and requires exact `PURGE <project>` confirmation
- DE/EN wording matches backend semantics
- restore occupied-destination and symlink/path boundaries remain enforced
- project/chat references stay consistent across quarantine/restore
- Mobile UI must call the PR #33 narrow quarantine API; do not add direct filesystem operations

---

## 6. Open issue #32 – make LocalCode Native match/exceed Claw orchestration

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

## 7. Open issue #30 – benchmarked llama.cpp / DMC backend

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

## 8. Additional competitive work

After the immediate loop guard and project UX:

- prompt/context cache stability and deterministic prefix ordering
- context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs
- Git diff/undo/commit UX polish while preserving stronger preconditions
- provider breadth kept below the supervisor/safety layer
- structural/fuzzy patch-drift recovery without bypassing approved SHA semantics
- Desktop/Android transparency for plan, phase, tools, approvals, verification and recovery
- benchmark tasks specifically exercising subagents, large repositories/tool registries and crash recovery

---

## 9. Immediate execution order

Unless a new blocker changes priority:

1. merge this post-#33 STATE refresh
2. implement the session-wide doom-loop/no-op guard on fresh master
3. refresh `STATE.md` immediately after that merge
4. finish project Trash/Quarantine UX (#31)
5. refresh `STATE.md`
6. build Native orchestration/subagents (#32)
7. refresh `STATE.md`
8. add/benchmark inference backend and DMC path (#30)

Every material step ends with a fully current `STATE.md`. A future agent must never have to infer the present state from contradictory historical sections.
