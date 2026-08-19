# LocalCode – canonical project state / kanonische KI-Übergabe

**Stand:** 2026-08-19 Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Current master:** `ddf1606c31d253b5fbbe5a2a67f10f8dae435bee`  
**Active mobile/project branch:** `agent/mobile-safe-project-playstore`  
**Active PR:** #24 `feat: safe mobile project management and Play Store builds`  
**State policy:** This file is the authoritative continuation document for LocalCode itself. Read it completely before changing code. Historical detail belongs in Git history and closed PRs, not in contradictory appended notes.

---

## 0. Mandatory first steps for the next AI

1. Read this entire file.
2. Read `AGENTS.md` and `README.md`.
3. Inspect current `master`, current branch, `git status`, open PRs/issues and the newest required CI run before editing.
4. Never assume a SHA, PR state or CI sentence below is still current if GitHub has advanced; verify it.
5. Never weaken approval, path, secret, remote, atomic-write, race, coverage or verification gates merely to make CI green.
6. Keep all user-facing DE/EN text complete.
7. Do not claim LocalCode beats Aider/OpenCode in a matrix row unless the implementation and benchmark evidence support it.
8. Prefer small, current-master feature PRs. Do not merge stale historical PRs wholesale.
9. Destructive Git operations, secret handling, external login and publishing always require explicit user approval.
10. Before merge require the complete Windows Quality workflow to pass against the current merge state.

---

## 1. Product objective

LocalCode is a Windows-first local coding-agent/development platform. The long-term acceptance criterion is to make every meaningful technical comparison row objectively stronger than Aider and OpenCode while remaining honest about gaps.

Target execution flow:

`User -> Desktop/Android Remote -> supervisor -> model -> repository intelligence -> planner/explorer -> tools/LSP/Git -> approval + precondition -> atomic mutation -> verification -> reviewer/success gate -> result`

Intended engines:

- LocalCode Native
- Aider
- Claude Code
- OpenCode

Primary local-model path is Ollama on Windows. Safety, correctness, recovery and verification are application responsibilities, not prompt-only responsibilities.

Priority comparison rows:

- repository intelligence/context selection
- semantic AST/LSP navigation
- local-model context efficiency
- patch/edit reliability
- stale/concurrent edit protection
- planning/execution/review separation
- subagents/parallel exploration
- verification
- crash recovery
- permissions/security
- process cancellation
- Git workflow
- remote/mobile transparency
- reproducible benchmark success

---

## 2. Current `master` milestones

### PR #16 benchmark harness — MERGED

Merged commit:

`9757520675918f9b9f824595d1ce676be6782704`

Master now contains a reproducible cross-engine benchmark harness:

- package `benchharness`
- CLI `localcode-bench`
- immutable base commit resolution
- fresh detached Git worktree per run
- direct argv execution rather than shell-string interpolation
- separate setup/engine/check timings, exit codes and timeouts
- hidden/required checks
- changed-file/added/deleted/unnecessary-diff metrics
- untracked text-file accounting
- optional adapter metrics for turns, tool calls, tokens, retries, failed patches, compactions and human intervention
- source repository is never the engine working directory
- path/manifest safety validation
- documented fair-run contract for LocalCode/Aider/OpenCode

Benchmark fairness requires same repository commit, task, model, quantization, context limit, hidden tests and environment constraints. Raw manifests/results must be retained; a single self-reported score is not evidence.

### PR #15 crash-safe durable recovery — MERGED

Merged commit:

`ddf1606c31d253b5fbbe5a2a67f10f8dae435bee`

Recovery design:

- persistent active-run journal under LocalCode app data, not the project repository
- redacted operational metadata only: run/thread/project/model/task/phase and selected checkpoint metadata
- no full tool-result transcript, attachment content or command output copied into recovery storage
- secret redaction for Authorization/Bearer, API-key, token, password/passwd and secret forms
- same-directory atomic write layer
- dedicated mutex around journal read/modify/write operations
- detects only valid non-terminal interrupted runs whose project still exists
- startup exposes recoverable run and associated thread where possible
- `Weiter`/`Continue` or the same original task receives a recovery handoff
- handoff explicitly forbids blindly replaying a previous mutation
- filesystem/Git/postconditions must be re-read before deciding what remains
- unrelated new tasks do not inherit old recovery context
- stop/force-stop/terminal completion are journaled

Journal performance/security optimization now on master:

- durable events are limited to recovery-relevant checkpoints
- redundant `user`, `agent_step`, `tool_result`, generic `status` and assistant transcript-like events do not cause atomic journal rewrites
- task is already stored separately
- action/phase checkpoints carry recovery progress
- less free text is persisted and fewer synchronous disk writes occur

The final #15 Quality run passed format, vet, Android, vulnerability scan, full-stack, normal tests, race detector, coverage, Windows builds and diff check before merge.

---

## 3. Core safety architecture already present

### File mutation invariants

LocalCode mutations are transactions, not model success strings.

Preserve:

- SHA-256 file versions
- approval-bound file preconditions
- per-path locking
- same-directory temporary writes
- revalidation immediately before commit
- atomic replace/move primitives including Windows-native replacement behavior
- conflict instead of silent overwrite if a file changed externally
- postconditions with existence/type/size/hash where applicable
- backup/Git recovery paths
- process-tree cancellation for owned external processes

Never regress existing-file edits to an unchecked `os.WriteFile` after approval.

### Path and deletion boundary

- canonicalize paths
- enforce project/root containment
- project-folder destructive actions are limited to direct children of configured project root
- reject nested/outside targets
- never follow symlink targets during delete preview
- a non-empty project must never disappear after one ambiguous click

### Approvals

- mobile is intentionally narrower than Desktop
- phone must never create globally persistent approval rules
- phone may approve once or for the current project where supported
- publishing, external login, release-track changes and secret-bearing operations need a separate explicit approval

### Secrets

Never log, persist or display:

- API keys
- bearer tokens
- passwords
- keystore passwords
- upload keys/private keys
- service-account credentials

Paired-device tokens are stored hashed. Release helpers must not invent or rotate an existing Play upload key.

---

## 4. Repository intelligence / semantics

Current architecture combines multiple evidence sources:

- import-aware repository graph
- task/path/identifier relevance
- definitions/references and relation weighting
- PageRank-like propagation
- focused snippets
- build/test pairing and verification-command inference
- Go compiler AST for Go
- multi-language semantic parsing where available
- lexical/import-aware fallback when a stronger provider is unavailable
- deterministic Windows-safe fallback must remain available

Verify exact language-provider coverage in current source before making a public comparison claim.

Important files are under:

- `src/code_intelligence_*.go`
- `src/repo_intelligence.go`

### LSP

Current LocalCode supports read-only semantic navigation with language-server candidates for major languages when installed. Operations include definition, references, hover, document/workspace symbols, implementation and call hierarchy. A persistent pool/reuse path has been integrated; verify exact health/restart behavior before claiming superiority over OpenCode.

### Parallel exploration

Independent read-only repository probes can run with bounded parallelism and stable deterministic output order. This does not grant mutation/shell/network/MCP permissions to the parallel path.

A true model-backed child-agent architecture remains an open gap; see Section 11.

---

## 5. Permanent Quality contract

Workflow:

`.github/workflows/quality.yml`

Required gates include:

- Go setup/version
- `gofmt`
- `go vet ./...`
- frontend JavaScript syntax, including inline scripts
- PowerShell syntax parsing when the PR #24 workflow change is present
- native Android Remote APK build
- `govulncheck`
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- statement coverage >= 80.0%
- native Windows GUI/diagnostic builds
- `git diff --check`

Never lower the 80% threshold to rescue a PR. Add meaningful tests or remove unnecessary untested complexity.

Known CI timing issue:

- `TestAgentRejectsEmptyWriteFileAndRetries` has a polling wait with a 4-second deadline.
- One #24 Windows run exceeded it by roughly 0.4 s while the immediate rerun passed normal tests.
- This is an old generic agent test, not a Mobile feature assertion.
- If this recurs, harden the test timing on current master; do not change Mobile product behavior to satisfy it.
- Because the wait helper polls and returns immediately on completion, a larger CI-safe maximum need not slow normal passing tests.

---

## 6. PR #24 – mobile-safe projects and Play Store build

Branch:

`agent/mobile-safe-project-playstore`

Base after recovery merge:

`master` at `ddf1606c31d253b5fbbe5a2a67f10f8dae435bee`

PR remains Draft until a fresh complete Quality run against this base is green.

### Project delete preview

PR #24 adds `ProjectDeletePreview` and server-side `inspectProjectDelete`:

- path/name
- empty flag
- file count
- directory count
- symlink count
- total bytes of regular files
- whether confirmation is required
- exact project-basename confirmation string

Preview rules:

- target must be a direct child project directory
- missing/non-directory target is rejected
- empty folder returns immediately
- non-empty preview walks with `filepath.WalkDir`
- symlink entries are counted but their targets are not followed

### Project actions

The branch supports:

- `create_folder`
- `create_project`
- `delete_empty`
- `delete_recursive`
- existing rename/pin/hide/restore catalog actions on Desktop

Deletion mode separation:

- `delete_empty` succeeds only for an actually empty project
- `delete_recursive` refuses empty projects
- non-empty recursive delete requires exact case-insensitive project-basename confirmation
- project/config references are cleaned and affected chat threads are archived

### IMPORTANT KNOWN SEMANTIC GAP — must be fixed next

Current branch implementation of `create_project` still scaffolds README/AGENTS/STATE only when old config flag `CreateProjectDocs` is enabled.

Desired contract is different:

- `create_folder` = intentionally bare folder
- `create_project` = always a real project scaffold with README.md + AGENTS.md + STATE.md, independent of the old automatic-doc preference

Do not claim this desired contract is already implemented. Fix it in the immediately following consolidated project-safety/quarantine PR unless #24 is deliberately amended and revalidated first.

Desktop project creation should also use `create_project` when the user chooses “new project”, so Desktop and phone semantics match.

---

## 7. Phone/Android Remote security in PR #24

Relevant files:

- `src/remote_server.go`
- `src/remote_secure_server.go`
- `src/remote_project_api.go`
- `src/mobile_safe_remote_server.go`
- `src/static/remote.html`
- `android/app/src/main/java/com/inetconnector/localcode/remote/MainActivity.java`

Existing remote capabilities include pairing, projects, threads, chat, attachments/camera, live events, stop and approvals.

PR #24 adds authenticated:

- `POST /remote/api/project-action`
- `GET /remote/api/project-delete-preview?path=...`

Mobile project-action allowlist is deliberately only:

- `create_project`
- `delete_empty`
- `delete_recursive`

Do not expose rename, arbitrary filesystem operations, shell or broad admin APIs merely for convenience.

Mobile approval wrapper rejects global persistence before the ordinary approval handler receives the request.

### Android WebView invariants

`MainActivity.java` is hardened to:

- HTTPS only
- navigate only within exactly the paired remote host + effective HTTPS port
- mixed content disabled
- file access disabled
- reject userinfo-bearing URLs
- no blanket “any IPv6 is private” rule
- discovered hosts must be loopback/link-local/site-local private addresses
- discovered service requires TLS marker and fingerprint
- self-signed LocalCode TLS proceeds only when SHA-256 certificate fingerprint matches the expected pin

Manual connection is stricter:

- only numeric private IP or localhost; no arbitrary DNS host
- requires an explicit valid 64-hex SHA-256 fingerprint
- retains that fingerprint rather than clearing pinning

QR/mDNS is the preferred easy path because URL and fingerprint arrive together.

`src/android_remote_security_test.go` isolates the manual click handler and verifies that it requires and retains the pin. Do not revert to a broad source-string check that mistakes the initial empty field declaration for a pin-clear operation.

No networked software should be described as mathematically “100% safe”. The design goal is least privilege, defense in depth and testable boundaries.

---

## 8. Remote project UX in PR #24

`src/static/remote.html` adds a Projects area with:

- New project
- Delete project
- Play Store build

Delete flow:

1. ask server for delete preview
2. if empty, use explicit final confirmation then `delete_empty`
3. if non-empty, show content counts
4. require typing exact project name
5. send exact value to `delete_recursive`
6. server validates again

The browser prompt is not the security boundary. Server-side path validation + mode separation + exact confirmation are mandatory.

Future improvement immediately after #24: non-empty deletion should become reversible quarantine rather than immediate `os.RemoveAll`; see Section 10.

---

## 9. Play Store build path in PR #24

### Phone action

The phone Play-Store button starts a normal LocalCode agent task for the selected project. It is not a shell backdoor.

The fixed task tells LocalCode to inspect:

- Gradle/SDK
- applicationId/package
- versionCode/versionName
- manifest
- min/target SDK
- existing signing configuration
- tests/lint

It requests release AAB/APK generation where supported and requires artifact path/size/hash reporting.

It explicitly forbids automatic:

- upload-key/keystore replacement
- secret disclosure
- Play Console upload
- release-track changes
- publishing

Those are separate approved operations.

### `scripts/build-playstore.ps1`

Deterministic helper rules:

- use project `gradlew.bat`, not a random global Gradle
- enumerate available tasks
- run available `test` and `lint` unless skipped
- require `bundleRelease`
- run `bundleRelease`
- run `assembleRelease` when available
- find release `.aab`/`.apk` below `build/outputs`
- exclude obvious debug/unaligned/unsigned final candidates
- require at least one AAB
- output path, bytes, SHA-256 and machine-readable JSON
- require AAB signature verification through `jarsigner -verify`
- verify APK with `apksigner` when available
- never generate/replace/rotate a keystore
- never print signing secrets
- never upload or publish

`build-android.ps1` is the debug/CI builder for the LocalCode Remote APK using an ephemeral debug key. It is not the production Play Store signing path.

---

## 10. Immediate next project-safety PR after #24

A branch name was reserved earlier:

`agent/reversible-project-quarantine`

It was originally created from an older master and must be recreated or safely synchronized to the newest master after #24 merge before coding.

Implement one coherent contract:

1. `create_folder` remains bare.
2. `create_project` always creates README.md + AGENTS.md + STATE.md independent of old `CreateProjectDocs` preference.
3. Desktop “new project” uses `create_project`.
4. Phone uses the same server-side project semantics.
5. Confirmed non-empty delete does **not** permanently `RemoveAll` immediately.
6. Move non-empty project into a LocalCode-managed quarantine on the same volume where possible.
7. Store minimal metadata: unique ID, original path/name, quarantine time, preview counts/bytes.
8. Provide restore; fail safely if original destination is occupied.
9. Permanent purge is a separate destructive action with its own exact confirmation.
10. Quarantine/restore/purge must not follow symlink targets.
11. Do not silently fall back to risky copy-then-delete across volumes.
12. Preserve/restore or explicitly archive relevant chat/config references in a deterministic way.
13. Add race/path/collision/occupied-restore/purge-confirmation/interruption tests.

After this feature exists, Mobile may expose list/restore quarantine entries; purge should remain tightly gated.

---

## 11. Remaining competitive work

### A. Session doom-loop/no-op guard

Do not merge old stale PR #7 wholesale. Port only useful behavior to current master:

- reject no-op `write_file` when requested content is already identical
- reject no-op `replace_text` when replacement leaves file unchanged
- structured action fingerprint includes edit payload/tool args but ignores human explanation text
- outcome fingerprint
- block third identical failed action
- block repeated same-result no-progress reads/tools
- detect short A/B and A/B/C style cycles
- changed tool output counts as new evidence
- successful real project mutation resets stagnation history
- successful verification resets stagnation history
- `finish` and `ask_user` are excluded

Suggested commit:

`feat: block stagnant agent tool loops`

### B. True model-backed subagents / roles

Old PR #14 contained a validated design but is stale. Port feature delta only:

- isolated child context
- bounded maximum child steps
- Explorer capability only: list/read/search/LSP/finish
- no write/delete/move
- no shell/Git/MCP/network/install
- no approval bypass
- no recursive child spawning
- deterministic repository-intelligence fallback
- visible `subagent:*` trace

Then evolve toward:

- Explorer
- Planner
- Executor
- Reviewer

No concurrent mutations. Reviewer should receive task + plan + diff + verification evidence rather than the full chat transcript.

### C. Prompt/context efficiency

- stable deterministic prompt-prefix ordering
- hash/cache stable context segments
- reduce timestamps/random ordering in cacheable prefix
- reuse curated repository context
- use provider-native caching only where actually supported
- do not make unsupported Ollama KV-cache claims

### D. Context economy / Aider comparison

Use the merged benchmark harness to compare correctness per context/token, not only raw repo-map richness. Same task/model/quantization/ctx, hidden tests and repo commit.

### E. Git workflow polish

Common diff/undo/commit flows should become at least as easy as Aider while retaining stronger preconditions and recovery.

### F. Provider breadth

Keep provider adapters separate from safety/supervisor logic so Native is not permanently tied to Ollama.

### G. Patch drift recovery

Improve fuzzy/structural edit recovery when exact old text drifts, but never bypass approved SHA/precondition semantics.

### H. UI transparency

Desktop and Android should surface plan, phase, tool execution, approvals, verification and recoverability at OpenCode-level or better.

---

## 12. Current PR #24 validation state

Important history:

- Earlier #24 run passed all early gates but one generic agent mock test exceeded its 4-second polling deadline by ~0.4 s.
- Immediate rerun passed the complete normal test suite and entered Race/Coverage.
- That run was against the pre-recovery merge state.
- PR base is now current master `ddf1606c31d253b5fbbe5a2a67f10f8dae435bee`.
- This `STATE.md` commit intentionally creates a real branch synchronize event so Quality must run again against the current master combination.

**Next AI/action:** inspect the newest #24 Quality run created after this commit. Do not merge based only on the older rerun.

Before merge require:

- [ ] gofmt
- [ ] vet
- [ ] JS syntax
- [ ] PowerShell syntax
- [ ] Android Remote APK build
- [ ] govulncheck
- [ ] full-stack loopback integration
- [ ] complete Go tests
- [ ] race detector
- [ ] coverage >=80.0%
- [ ] native Windows builds
- [ ] diff check
- [ ] project create/delete regression tests
- [ ] Android pinning/private-origin tests
- [ ] mobile route authentication
- [ ] mobile global-approval block
- [ ] Play Store helper still never uploads/publishes/rotates keys
- [ ] PR mergeable with current master

Only after every box is green:

1. mark PR #24 ready for review
2. re-read PR head SHA
3. merge with `expected_head_sha`
4. record merge SHA in this file on the next master-based feature branch
5. recreate/sync quarantine branch from resulting master

---

## 13. Branch and workflow hygiene

- Delete merged feature branches when safe tooling/access permits.
- Do not reuse stale closed branches as implementation bases.
- Do not force-move refs to simulate deletion.
- Do not restore obsolete temporary coverage workflows.
- Keep Quality and real release workflows unless a replacement is demonstrably stronger.
- Before deleting an Actions workflow, verify it is unused/obsolete rather than guessing from filename.

The repository has historical branches from atomic edit, approvals, AST/LSP, parallel reads and security work. Recheck current branch/PR listings before cleanup because status changes as merges occur.

---

## 14. Definition of done for the user’s current direction

The user’s request is broader than “add a phone button”. The intended state is:

1. Projects are easy to create from Desktop and phone.
2. A true `create_project` always has useful handoff docs.
3. Empty folders can be removed safely.
4. Non-empty projects require content-informed exact confirmation and then become reversible before permanent destruction.
5. Phone Remote has least privilege rather than a broad shell/admin surface.
6. Android connection is private-origin restricted and certificate-pinned.
7. A Play Store release build can be initiated from the phone without silently changing signing identity or publishing.
8. AAB/APK artifacts are verified and hashed.
9. Interrupted agent work is recoverable without blind mutation replay.
10. LocalCode/Aider/OpenCode can be benchmarked fairly from identical inputs.
11. Doom loops/no-op edits are blocked deterministically.
12. Model-backed Explorer/Planner/Reviewer roles are isolated and bounded.
13. `STATE.md` is always sufficient for a fresh AI to resume from current repository reality.
14. The comparison matrix is updated only from implemented and measured evidence until LocalCode objectively leads every row.

When updating this file later, **replace stale current facts**. Do not append contradictory historical snapshots.
