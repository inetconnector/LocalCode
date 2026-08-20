# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-08-20 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current verified functional master before this documentation-only state update:** `f84a38f25a09231fb973022867e9177e8de04974`  
**Last verified functional merge:** PR #38 `feat: finish safe project UX across desktop and mobile`  
**Open implementation PR:** none at this snapshot  
**Immediate implementation priority:** issue #32 `feat: exceed Claw Code native orchestration capabilities`  

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
- `create_project` always creates `README.md`, `AGENTS.md` and `STATE.md`
- confirmed non-empty delete moves the project into LocalCode-managed same-volume quarantine rather than immediate permanent deletion
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

### Session-wide stagnant-loop/no-op guard – PR #36 merged

Merge commit:

`b12e4d4664200f9eae0f114690cb925e0b5598e1`

LocalCode Native has a session-scoped deterministic no-progress guard in addition to the immediate-identical-action block:

- structured action fingerprint covers complete action payload/arguments while ignoring only human explanation text
- normalized result fingerprint tracks whether repeated diagnostics return the same evidence
- repeated unchanged structured actions/read-tool outcomes and short cycles are detected and blocked
- changed output is treated as new evidence
- successful real mutation or project verification resets stagnation history
- `finish` and `ask_user` are excluded from session-loop blocking
- feedback exists in DE and EN

Native edit no-op handling is explicit:

- `write_file` rejects existing identical bytes
- `replace_text` rejects unchanged output
- approval/version-bound mutation paths enforce the same rule
- no-op rejection occurs before pointless approval and before backup/atomic write
- real writes retain path locks, SHA/version preconditions, backups and atomic conflict-safe replacement

### Safe project UX across Desktop and Mobile – PR #38 merged

Merge commit:

`f84a38f25a09231fb973022867e9177e8de04974`

Final tested PR head:

`cb2f18cae0259106350e2aa69fd0f0a9a00fb55b`

Issue #31 is closed as completed.

Current project UX now matches the reversible backend semantics:

- Desktop and Mobile clearly distinguish **New project** from **New folder**
- `create_project` always creates `README.md`, `AGENTS.md` and `STATE.md`
- `create_folder` remains intentionally empty; Desktop no longer auto-selects it in a way that scaffolds project docs
- empty folders can be deleted after a simple confirmation
- non-empty deletion uses a server-generated preview with file/directory/byte counts
- non-empty project-name confirmation is exact and case-sensitive
- non-empty deletion wording states that the project moves to LocalCode Trash/Quarantine and can be restored; it is not described as permanent deletion
- Desktop and Mobile both expose a visible Trash/Quarantine view with **Restore**
- permanent purge is visually separate and requires exact `PURGE <project>` confirmation
- restore reactivates the preserved project threads that were archived during quarantine
- Restore continues to refuse occupied original destinations
- quarantine/path/symlink protections remain inherited from the backend
- Mobile Remote remains narrower than Desktop; the only added project action is the already-safe `create_folder`, with no arbitrary filesystem/shell/admin API
- DE/EN UI semantics were updated together

Quality #384 passed on the exact final head and completed the full Windows pipeline:

- format
- vet
- frontend JavaScript syntax
- PowerShell syntax
- native Android Remote APK
- vulnerability scan
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- statement coverage >=80.0%
- native Windows builds including GUI path
- final Git diff check

Before merge the branch was 0 commits behind `master`, mergeable, and had no review submissions or unresolved review threads.

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

## 4. Immediate next feature – issue #32 Native orchestration/subagents

Issue #32 is open. There is no open implementation PR at this snapshot. Start from current `master` after this STATE refresh.

Goal: port useful architectural ideas identified from Claw Code into LocalCode Native while keeping LocalCode's stricter safety/reliability model.

Target capabilities:

1. real model-backed subagents with explicit Explorer/Planner/Reviewer roles and separate curated contexts
2. optional Git-worktree isolation for mutation-capable child agents; read-only exploration stays cheaper
3. deferred/tool-search capability for large tool registries
4. structured project slash commands with typed parameters and deterministic expansion
5. explicit per-run token/tool/time budgets with visible remaining budget and hard stops
6. broader MCP transports only if LocalCode auth/timeout/approval/SSRF/path protections remain intact
7. structured machine-readable child-agent results rather than prose parsing
8. health/doctor diagnostics for external engines and Native capabilities
9. benchmark tasks that exercise subagents, repository exploration, large tool registries and recovery

Safety requirements remain stronger than orchestration convenience:

- approval bound to file SHA/preconditions
- atomic conflict-safe writes
- no concurrent unsupervised same-workspace mutation
- durable crash journal remains authoritative
- child mutation must be diff-reviewable and verified before success
- no default/silent `danger-full-access` equivalent
- Mobile remains narrower than Desktop

Historical closed PR #14 may contain useful read-only model-subagent design ideas; port feature delta only, never merge a stale branch wholesale.

Implementation should be split into current-master reviewable increments rather than one large orchestration rewrite.

---

## 5. Open issue #30 – benchmarked llama.cpp / DMC backend

Issue #30 remains open.

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

## 6. Additional competitive work

After the main Native orchestration work:

- prompt/context cache stability and deterministic prefix ordering
- context/token economy benchmarks against Aider/OpenCode/Claw with identical inputs
- Git diff/undo/commit UX polish while preserving stronger preconditions
- provider breadth kept below the supervisor/safety layer
- structural/fuzzy patch-drift recovery without bypassing approved SHA semantics
- Desktop/Android transparency for plan, phase, tools, approvals, verification and recovery
- benchmark tasks specifically exercising subagents, large repositories/tool registries and crash recovery

---

## 7. Immediate execution order

Unless a new verified blocker changes priority:

1. merge this post-#38 STATE refresh
2. implement issue #32 in small current-master increments, beginning with the safest useful Native orchestration/subagent foundation
3. refresh `STATE.md` after every material #32 merge
4. extend benchmark coverage for the newly implemented Native orchestration capability before claiming parity/superiority
5. implement and benchmark issue #30 inference-backend / llama.cpp / DMC path
6. refresh `STATE.md` after each material merge

Every material step ends with a fully current `STATE.md`. A future agent must never have to infer present repository reality from stale or contradictory historical sections.
