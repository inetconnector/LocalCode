# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 08:56 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Verified functional master before this documentation-only state update:** `92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`  
**Last verified functional merge:** PR #28 `feat: add Claw Code as managed engine`  
**Open implementation PR:** #33 `feat: add safe mobile project quarantine controls`  

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

LocalCode is the UI/supervisor shell even when an external coding engine performs a delegated operation.

Long-term objective: make LocalCode Native objectively stronger than Aider/OpenCode/Claw across repository intelligence, edit reliability, safety, recovery, orchestration, context efficiency, verification and measurable benchmark success. Do not claim superiority without implementation plus reproducible evidence.

---

## 2. Current master – major merged capabilities

### Conflict-safe and approval-bound mutation

Merged foundations include:

- SHA-256 file-version preconditions
- per-path locking
- same-directory staging
- atomic replacement/move behavior, including Windows-native replacement paths
- conflict instead of silent overwrite after external change
- approval bound to the exact file version used for preview
- checked delete/write/replace postconditions
- backup/Git recovery paths
- process-tree cancellation for owned external processes

Never regress approved edits to unchecked overwrite behavior.

### Repository intelligence and semantic navigation

Current master includes:

- import-aware repository graph
- Go compiler AST path
- Tree-sitter-backed JavaScript/JSX, TypeScript/TSX, Python, Rust, C and C++ where CGO/provider support is available
- deterministic lexical/import fallback
- weighted typed graph relations such as imports, references, calls, inherits, implements and test-of
- task-ranked source context and graph relevance
- bounded parallel independent read-only exploration
- native read-only LSP navigation
- persistent/recovering LSP session pool with project/server isolation

LSP remains a navigation/diagnostic channel; server-originated mutation does not grant write authority.

### Durable agent recovery

Current master has crash-safe active-run recovery:

- persistent redacted run journal under LocalCode app data
- atomic/mutex-protected journal updates
- no second full transcript or secret-bearing command-output archive
- interrupted-run detection
- recovery handoff that explicitly forbids blind replay
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

### Reversible project quarantine – PR #26 merged

This supersedes older STATE text that described reversible deletion as future work.

Current master now implements:

- `create_folder` intentionally bare
- `create_project` always creates `README.md`, `AGENTS.md` and `STATE.md`, independent of the old automatic-doc preference
- confirmed non-empty delete moves the project into LocalCode-managed same-volume quarantine instead of immediate permanent `RemoveAll`
- minimal atomic quarantine metadata
- validated list/restore/permanent-purge primitives
- restore refuses an occupied original destination
- permanent purge requires exact `PURGE <project>` confirmation
- quarantine root and symlink escape targets are rejected
- symlink targets are not followed by preview/quarantine/purge
- no risky cross-volume copy-then-delete fallback
- chat/project references are handled deterministically and affected history is archived/preserved as designed

### Quality coverage strengthening – PR #29 merged

The >=80% statement coverage gate remains intact. Real compatibility/formatting tests were added rather than weakening CI.

### Release-note consolidation – PR #27 merged

Version-specific release-note files were consolidated into canonical `RELEASE-NOTES.md`.

### Claw Code managed engine – PR #28 merged

Merge commit:

`92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`

Claw is now an optional fifth coding engine and is not the default.

Implemented contract:

- central coding-engine router/status/setup integration
- settings/UI engine selection
- edit/repository-map/lint/test routed through LocalCode reliability, backup/undo, timeout and cancellation paths
- managed Windows/MSVC Rust build preparation
- exact pinned upstream Claw source revision verification
- fail-closed existing-binary version verification using structured `claw version --output-format json` / `git_sha`
- process-scoped `OLLAMA_HOST`
- ambient OpenAI/Anthropic/xAI/DashScope credential/provider variables stripped from Claw subprocesses
- `read-only` for analysis and `workspace-write` for mutation; no default/permanent `danger-full-access`
- Windows Authenticode/MSVC preparation and safe temporary batch invocation
- safe authenticated mobile engine selection, blocked while an agent is running
- explicit Claw benchmark adapter using an isolated benchmark worktree
- no Claw Studio installation/launch; LocalCode remains the shell

PR #28 final Quality run passed format, vet, JavaScript/PowerShell syntax, Android APK, vulnerability scan, full-stack loopback, complete Go tests, race detector, coverage >=80%, native Windows builds and diff check before merge.

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

## 4. Current open PR – #33 mobile quarantine controls

PR #33: `feat: add safe mobile project quarantine controls`

Current verified state before this STATE update:

- state: open
- draft: yes
- head branch: `feat/project-quarantine-ux`
- head SHA: `e86bef82b9f50048e201f397cb287defe29c7792`
- recorded PR base SHA is stale: `159a454a6344993f942cb22216a548cf119eb48e`
- current functional master is newer: `92ac486f1abe0a42eca4d4b3d8a997f31ba4b42c`
- GitHub currently reports the PR as not mergeable in its stale state

Intended PR #33 scope:

- authenticated read-only quarantine listing on Mobile Remote
- restore by opaque quarantine ID
- permanent purge with exact server-side `PURGE <project>` confirmation
- restore/purge blocked while an agent is running
- invalid IDs/actions fail closed
- no user-controlled filesystem path accepted by quarantine endpoints
- mobile-safe routes only; no install/login/global-approval/admin expansion
- regression tests for auth, restore/list, exact purge confirmation, running-agent block, invalid IDs and forbidden actions

Immediate required action:

1. synchronize/port #33 onto current `master` after the current STATE update lands
2. resolve overlap with mobile files changed by Claw without dropping either feature
3. inspect current patch and review threads
4. run full current Windows Quality pipeline
5. merge only if the exact current head is green, mergeable and 0 commits behind master

---

## 5. Open issue #31 – finish project UX across Desktop and Mobile

Backend quarantine semantics are already merged; remaining work is primarily coherent user-facing UX and consistency.

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

PR #33 provides the narrow mobile API contract but does not by itself finish the visual UX.

---

## 6. Next reliability feature – session-wide doom-loop/no-op guard

This remains the highest-priority native reliability item after #33/state cleanup.

Do NOT merge stale historical loop-guard branches wholesale. Port the useful behavior onto current master.

Required behavior:

- reject `write_file` when requested bytes are already identical
- reject `replace_text` when replacement produces no actual change
- structured action fingerprint includes tool/edit payload but ignores human explanation text
- structured outcome/progress fingerprint
- block repeated identical failed actions
- block repeated same-result no-progress read/tool loops
- detect short A/B and A/B/C cycles
- changed tool output counts as new evidence
- successful real project mutation resets mutation-stagnation history
- successful verification resets stagnation history
- `finish` and `ask_user` excluded from loop-action blocking
- preserve existing immediate repeated-action guard where complementary

Useful historical branch for feature-delta porting: `reliability/session-doom-loop-guard`; it is stale and must not be merged wholesale.

Suggested commit/PR theme:

`feat: block stagnant agent tool loops`

---

## 7. Open issue #32 – make LocalCode Native match/exceed Claw orchestration

Running Claw externally is now implemented. Native still needs the useful architectural capabilities internally.

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

Historical closed PR #14 contains a useful read-only model-subagent design; port feature delta only, never merge the stale branch wholesale.

---

## 8. Open issue #30 – benchmarked llama.cpp / DMC backend

Goal: introduce an inference-backend abstraction below the Native agent loop while keeping Ollama default and behavior stable.

Planned path:

- Ollama backend remains default
- optional loopback-only OpenAI-compatible llama.cpp backend
- backend-specific health/model discovery/process lifecycle
- no silent provider fallback/drift
- explicit UI status/selection without exposing secrets
- exact runtime provenance before any DMC label
- Windows must not be called DMC-enabled unless the DMC KV selection/rehydration path is actually active and self-tested
- benchmark Ollama vs dense llama.cpp vs real DMC-enabled runtime only where available
- measure correctness, retained context, latency, runtime, memory/VRAM and long-context recall

Do not confuse DMC with RAG or replace LocalCode semantic repository intelligence/context compaction with it.

---

## 9. Additional competitive work after the above

- prompt/context cache stability and deterministic prefix ordering
- context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs
- Git diff/undo/commit UX polish while preserving stronger preconditions
- provider breadth kept below the supervisor/safety layer
- structural/fuzzy patch-drift recovery without bypassing approved SHA semantics
- Desktop/Android transparency for plan, phase, tools, approvals, verification and recovery
- benchmark tasks specifically exercising subagents, large repositories/tool registries and crash recovery

---

## 10. Immediate execution order

Unless a new blocker changes priority:

1. land this current `STATE.md` repair and enforce the permanent maintenance rule
2. synchronize/fix/test/merge PR #33
3. immediately refresh `STATE.md` again to record the #33 merge and remove stale PR facts
4. implement the session-wide doom-loop/no-op guard on fresh master
5. update `STATE.md` again after that merge
6. finish project Trash/Quarantine UX (#31)
7. build Native orchestration/subagents (#32)
8. add/benchmark inference backend and DMC path (#30)

Every step ends with a fully current `STATE.md`. A future agent must never have to infer the present state from contradictory historical sections.
