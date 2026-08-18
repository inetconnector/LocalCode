# LocalCode – canonical project state / kanonische KI-Übergabe

**Stand:** 2026-08-19 00:xx Europe/Berlin  
**Repository:** `inetconnector/LocalCode`  
**Default branch:** `master`  
**Master at start of the current mobile/project work:** `16c429bd911fa520b504c11f052f887cd6043569`  
**Active feature branch:** `agent/mobile-safe-project-playstore`  
**Active PR:** #24 `feat: safe mobile project management and Play Store builds` (Draft until complete Quality is green)  
**State policy:** This file is the authoritative current handoff. Historical detail remains in Git history. Do not reconstruct project status from old chat logs if this file and current Git state are available.

---

## 0. What the next AI must do first

1. Read this entire file before changing code.
2. Check `git status`, current branch, current `master`, open PRs and required CI.
3. Read `AGENTS.md`, `README.md` and any more specific project instructions for files being changed.
4. Never assume an old PR branch is current. Compare it to `master` first.
5. Never weaken Quality, approval, path, secret, atomic-write or remote-security gates merely to make a PR green.
6. Do not claim LocalCode is better than Aider/OpenCode in a matrix row unless the implementation exists and the comparison is defensible. The long-term goal is to beat both honestly, not by changing scoring rules.
7. For code changes, keep DE/EN user-facing text consistent.
8. Before merge, require the complete Windows Quality workflow to pass.

---

## 1. Product goal

LocalCode is a Windows-first local coding-agent/development platform. It should provide a native agent that is at least as capable as Aider/OpenCode while being more deterministic, transparent, recoverable and safe for local repositories.

Target flow:

`User -> LocalCode UI/Android Remote -> agent supervisor -> local/provider model -> repository intelligence -> tools/files/Git/shell/LSP/MCP -> approval/precondition -> atomic mutation -> verification -> reviewer/success gate -> result`

Supported editing engines are intended to include:

- LocalCode Native
- Aider
- Claude Code
- OpenCode

Primary local-model path is Ollama on Windows. The system must remain useful when the model is imperfect: safety, file integrity, verification and recovery are application responsibilities, not prompt-only responsibilities.

### Long-term competitive acceptance criterion

Every meaningful technical comparison row should eventually be objectively LocalCode-leading, especially:

- repository intelligence/context selection
- AST/semantic navigation
- LSP/session reuse
- patch/edit reliability
- stale/concurrent edit protection
- approvals/security
- planning/execution/review separation
- subagents/parallel exploration
- verification
- recovery
- local-model efficiency
- Git workflow
- tool/MCP breadth
- UI/remote transparency
- reproducible coding benchmarks

Do not fake this criterion. If Aider/OpenCode is better in a row, record the gap and implement it.

---

## 2. Current architecture and capabilities already on `master`

### Native agent / supervisor

- Structured agent action schema with validation/normalization.
- Intent classification and supervisor-forced reliability actions.
- Completion guards block empty/placeholder/incomplete implementations and missing post-edit verification where required.
- Bounded repair of invalid model actions.
- Large mutation payloads are compacted in model history; exact current file content can be re-read.
- Tool outputs remain visible to the user while model context is bounded.
- Native path supports project/file tools, shell/tool discovery, Git, MCP, web tools, project automation, assets and image operations.

### File mutation reliability

LocalCode treats mutations as transactions rather than trusting a model success string.

Current invariants include:

- SHA-256 file versions.
- approval-bound file preconditions.
- per-path edit locking.
- same-directory temporary writes.
- revalidation immediately before commit.
- atomic replace primitives, including Windows-native replacement behavior.
- conflict errors instead of silently overwriting an externally changed file.
- postconditions reporting existence/type/size/SHA where applicable.
- backups/recovery paths for managed edits and Aider integration.

Never regress these to plain `os.WriteFile` after an approval for an existing file.

### Repository intelligence

Current stack combines multiple evidence sources rather than one regex repo map:

- import-aware file graph.
- task/identifier/path relevance.
- definitions/references and relation weighting.
- PageRank-like propagation/ranking.
- task navigation and focused snippets.
- build/test pairing and verification-command inference.
- Go compiler AST for Go.
- Tree-sitter under CGO for JavaScript/JSX/MJS/CJS, TypeScript, TSX, Python, Rust, C and C/C++ headers/sources.
- lexical/import-aware fallback where the stronger provider is unavailable.
- deterministic no-CGO fallback is retained so Windows deployment is not made dependent on one parser runtime.

Important source files:

- `src/code_intelligence_*.go`
- `src/repo_intelligence.go`

### LSP

LocalCode has native read-only LSP navigation and a persistent pool. Supported server candidates include Go, JS/TS, Python, Rust, C/C++, C#, Java and Kotlin when their language servers are available.

Operations include:

- definition
- references
- hover
- document/workspace symbols
- implementation
- call hierarchy preparation
- incoming/outgoing calls

Persistent LSP sessions are keyed/reused with health/invalidation behavior rather than starting one process per every query.

### Parallel exploration

Independent repository reads can fan out with bounded parallelism while results are merged in stable input order. This path is read-only; it does not grant mutation/shell/network/MCP rights merely because work is parallelized.

A true model-backed child-agent implementation was validated in old PR #14 but the old branch became too stale to merge safely. See Issue #23 below.

### Verification / CI

The permanent Quality workflow is `.github/workflows/quality.yml`.

Required stages currently include:

- Go version/setup
- gofmt
- `go vet ./...`
- frontend JavaScript syntax, including inline scripts
- native Android Remote APK build
- `govulncheck`
- full-stack loopback HTTP integration
- complete Go tests
- race detector
- statement coverage gate >= 80.0%
- native Windows builds
- `git diff --check`

Do not lower the 80% gate to make a feature pass. Add useful tests or reduce unnecessary untested complexity.

The obsolete temporary coverage diagnostic workflow was removed in PR #21 and must not be restored.

---

## 3. Security invariants

These are product requirements, not optional style preferences.

### Repository/file boundary

- Resolve paths canonically and require them to remain inside the allowed project/root boundary.
- Destructive project-folder operations are limited to direct children of the configured project root.
- Nested/outside paths must be rejected.
- Symlinks must not be used to escape a root or to make a preview recursively inspect an external target.
- A non-empty project folder must never be recursively deleted on a single ambiguous click.

### Approvals

- Mutating/destructive/external actions use approval rules.
- File mutation approval should remain bound to the file version shown/approved.
- Mobile Remote is intentionally narrower than Desktop.
- Mobile Remote must not create a globally persistent approval rule. Mobile may approve once or persist for the current project only.
- Publishing, external login, release-track changes and secret-bearing operations require explicit separate approval.

### Secrets

- Do not log or persist raw API keys, bearer tokens, passwords, keystore passwords, upload keys or other credentials.
- Run-journal redaction must cover authorization/bearer, API-key variants, password/secret variants and generic `token=...` forms.
- Mobile paired-device tokens are stored hashed, not in plaintext configuration.
- Play Store automation must not manufacture/replace/rotate a keystore/upload key as a convenience shortcut.

### Process control

- Timeouts/cancel should terminate the owned process tree, not merely the immediate parent process.
- External processes must not survive a cancelled run accidentally.

### Remote network

Existing design:

- Desktop API remains loopback-only.
- LAN Remote uses a separate server.
- Non-loopback production Remote requires TLS.
- Pairing is short-lived, numeric and attempt-limited.
- Long-lived paired-device tokens are random and stored as SHA-256 hashes.
- Mutation requests are protected against cross-origin/fetch-site abuse.
- CSP/no-store/nosniff/X-Frame/referrer/permissions headers are set by Remote.
- SSE uses a separately issued short-lived stream ticket rather than ordinary API query-token authentication.

Current PR #24 further tightens Android/phone behavior; see Section 6.

No engineer or AI should describe any non-trivial networked software as mathematically “100% safe”. The project objective is defense in depth with explicit, testable invariants and least privilege.

---

## 4. Project management – current behavior

Before PR #24, `master` already supported safe folder-level actions in `src/project_catalog.go`:

- `create_folder`
- `rename_folder`
- `delete_empty`
- `delete_recursive`
- alias rename
- pin/unpin
- hide/remove/restore

Existing tests in `src/project_folder_actions_test.go` already checked unsafe Windows names, reference migration on rename, empty-only deletion, exact-name recursive confirmation, and rejection of nested/outside paths.

### PR #24 additions

`ProjectDeletePreview` now records:

- project path/name
- empty/non-empty
- file count
- directory count
- symlink count
- total bytes of regular files
- whether confirmation is required
- exact required confirmation text (project folder basename)

`inspectProjectDelete(root, path)`:

- first enforces the direct-project-folder boundary.
- rejects missing/non-directory projects.
- returns immediately for a truly empty folder.
- uses `filepath.WalkDir` for a non-empty preview.
- counts symlink entries without following their targets.

New action `create_project`:

- only allowed at the configured project root.
- validates a Windows-safe direct folder name.
- refuses existing targets.
- creates the directory.
- when project docs are enabled, initializes `README.md`, `AGENTS.md` and managed `STATE.md` state.
- rolls back the newly created directory if initial handoff-document generation fails.

Deletion semantics in PR #24:

- `delete_empty` calls the server-side preview and succeeds only if actually empty.
- `delete_recursive` refuses an empty folder (caller must use the safer empty mode).
- non-empty recursive deletion requires case-insensitive exact project-folder-name confirmation.
- project/chat/config references are cleaned and affected chat history is archived rather than discarded.

Do not weaken this to a generic path + recursive flag API.

---

## 5. Project handoff documents

`src/state_doc.go` owns LocalCode-managed project handoff behavior.

When project docs are enabled:

- `README.md` is created if missing.
- `AGENTS.md` is created if missing.
- `STATE.md` is managed through LocalCode state markers.

Generated `AGENTS.md` tells coding agents to read README/STATE, inspect Git/tests first, preserve conventions, verify work, avoid secrets and request approval for destructive/external/publishing work.

For newly created projects, PR #24 immediately writes an initial managed STATE entry so the project is not an undocumented empty folder.

Repository-level `STATE.md` (this file) is deliberately different: it is the canonical continuation document for LocalCode itself.

---

## 6. Android / Phone Remote

### Existing `master` behavior

Files:

- `src/remote_server.go`
- `src/remote_secure_server.go`
- `src/remote_stream_ticket.go`
- `src/remote_device_lifecycle.go`
- `src/static/remote.html`
- `android/app/src/main/java/com/inetconnector/localcode/remote/MainActivity.java`

Existing Remote supports:

- pairing
- project list
- thread list/select/new
- chat submission
- attachments/camera through file input
- snapshots
- live SSE events
- stop
- approvals
- status/model/engine display

Native Android shell:

- discovers `_localcode._tcp.` via Android NSD/mDNS.
- consumes TLS fingerprint advertised by LocalCode.
- understands `localcode://pair` deep links carrying target URL + fingerprint.
- loads the Remote web UI in a WebView.

### PR #24 mobile project API

New files:

- `src/remote_project_api.go`
- `src/mobile_safe_remote_server.go`

The actually started Remote server is switched in `src/main.go` from `startProductionRemoteServer` to `startMobileSafeProductionRemoteServer`.

The hardened server registers authenticated:

- `POST /remote/api/project-action`
- `GET /remote/api/project-delete-preview?path=...`

Remote project-action allowlist is intentionally only:

- `create_project`
- `delete_empty`
- `delete_recursive`

Desktop-only broader management actions are not implicitly exposed to the phone.

The mobile-safe HTTP wrapper additionally rejects `decision=global` for `/remote/api/approve` before it can reach the normal approval handler.

### PR #24 Android hardening

`MainActivity.java` now:

- installs `WebChromeClient` so the Remote UI can use explicit prompt/confirm dialogs.
- still disables WebView file access and mixed content.
- allows navigation only when candidate URL has exactly the same HTTPS host + effective port as the currently paired Remote origin.
- no longer accepts every IPv6 address merely because it contains `:`.
- accepts only addresses classified by `InetAddress` as loopback, link-local or site-local.
- rejects userinfo-bearing URLs.
- rejects discovered services without a private address, TLS marker and fingerprint.
- retains exact SHA-256 certificate fingerprint comparison for the paired/discovered self-signed LocalCode certificate.

A source-level regression test `src/android_remote_security_test.go` pins these invariants.

### PR #24 Remote UI

`src/static/remote.html` now has a Projects tab with:

- New project
- Delete project
- Play Store build

New project calls the authenticated server action and uses `create_project`, so scaffolding happens server-side.

Delete project:

1. gets server-side preview.
2. if empty, asks for a simple final confirmation and calls `delete_empty`.
3. if non-empty, shows content counts and requires typing the exact project name.
4. sends that value to `delete_recursive`, where it is checked again server-side.

The client confirmation is not the security boundary. The server-side check is mandatory.

---

## 7. Play Store / Android release-build path

### User goal

From the phone, a user should be able to select an Android project and trigger a complete local release-build task without having to type a long build instruction.

### PR #24 implementation

The Remote UI button **Play-Store-Build**:

- requires a selected project.
- asks the user to confirm starting the release task.
- creates a fresh chat for that project so release context does not accidentally inherit another project/thread.
- sends a fixed DE/EN LocalCode task instructing the native/selected coding engine to inspect:
  - project structure
  - Gradle/SDK
  - applicationId/package
  - versionCode/versionName
  - manifest
  - min/target SDK
  - existing signing configuration
- requires existing tests/lint where available.
- requests `bundleRelease` (.aab) and `assembleRelease` (.apk) where the project supports them.
- requires artifact path/size/SHA-256 reporting.
- explicitly forbids automatic keystore/upload-key replacement, secret disclosure, Play Store upload, release-track change or publishing.

This deliberately runs through the normal LocalCode agent, approval, process-control and verification path. The phone button is not a shell backdoor.

### Deterministic helper

New script: `scripts/build-playstore.ps1`

Behavior:

- uses an existing project `gradlew.bat`; no global Gradle installation.
- enumerates Gradle tasks.
- runs available `test` and `lint` unless skipped.
- requires `bundleRelease` to exist for a Play Store build.
- runs `bundleRelease` and, if present, `assembleRelease`.
- locates release `.aab`/`.apk` below `build/outputs`.
- ignores obvious debug/unaligned/unsigned artifacts when reporting final candidates.
- requires at least one release AAB.
- prints exact path, byte size and SHA-256 plus machine-readable JSON.
- never creates/replaces a keystore, reads passwords for display, uploads to Google Play or publishes.

`build-android.ps1` remains the CI/debug builder for the LocalCode Remote APK and uses an ephemeral debug key. It is not the Play Store signing path.

---

## 8. Open reliability/competitive work outside PR #24

### PR #15 – crash-safe durable agent recovery

Branch: `agent/durable-run-recovery`  
Head currently documented: `628ba4544cd528e7fa78000b69318da33d001cdb`

Feature:

- persistent run journal in app data.
- redacted operational metadata rather than a second full transcript.
- atomic journal writes.
- serialized journal state updates.
- detects interrupted non-terminal runs.
- continuation creates a recovery handoff requiring reconciliation of actual Git/filesystem/postconditions.
- never blindly replays a previous mutation after crash.
- generic `token=...` redaction gap was fixed.

Quality run #240 completed successfully.

Current merge blocker is repository-rule status freshness: GitHub reports required check `test` as expected after `master` moved, even though the old head Quality run is green. Do not bypass the rule. Update/merge current `master` into the branch (or otherwise produce a fresh PR head/merge state) and rerun Quality, then merge if green.

### PR #16 – reproducible cross-engine benchmark harness

Branch: `agent/engine-benchmark-harness`  
Head currently documented: `df1d384d17df8534544f95d14cdd100a8a1e4de8`

Feature:

- `benchharness` package + `localcode-bench` CLI.
- immutable base commit resolution.
- fresh detached Git worktree per benchmark run.
- direct argv execution rather than shell-string interpolation.
- separate setup/engine/check timing and status.
- hidden/required checks.
- changed-file/line/unnecessary-diff metrics.
- adapter metrics for turns/tool calls/tokens/retries/compactions/human intervention.
- source repo remains untouched.
- process-tree timeout cleanup.
- manifest/path validation.

Quality run #242 completed successfully after coverage was raised with real manifest-contract tests rather than lowering the gate.

It has the same likely status-freshness/base-update issue as PR #15. Refresh against current master, rerun Quality, then merge if green.

### Issue #22 – port session doom-loop guard

Old PR #7 was intentionally closed because it was too stale and carried unwanted old workflow changes.

Port only the useful behavior to current master:

- reject no-op `write_file`/`replace_text` as no observable change.
- session-wide structured action/result fingerprinting.
- block repeated identical failed/no-progress actions.
- detect short A/B or A/B/C style stagnation cycles.
- reset stagnation on actual project mutation or successful verification/new evidence.
- keep finish/ask_user out of the loop guard.

Do not merge the old PR wholesale.

### Issue #23 – port bounded read-only model subagents

Old PR #14 was fully Quality-validated but became materially behind current master.

Port only the feature delta:

- separate model-backed child exploration context.
- hard maximum child-step budget (validated design used 8).
- child capabilities only list/read/search/LSP/finish.
- no write/delete/move, shell, Git, MCP, web/network, installation, approval bypass or recursive child spawning.
- deterministic repository-intelligence fallback.
- visible `subagent:*` UI trace.
- deterministic mandatory reliability preflight remains model-independent.

Do not reintroduce stale agent code from the closed PR branch.

---

## 9. Remaining matrix gaps after the already integrated AST/LSP/reliability work

Priority order after PR #24/#15/#16 is merged:

1. **Model subagents / role separation** – port #23, then evolve toward Explorer/Planner/Reviewer with isolated curated context and no concurrent mutations.
2. **Session doom-loop/no-op guard** – port #22.
3. **Prompt/context caching efficiency** – stable prompt prefix ordering, deterministic context segment hashes, reduced churn, provider-native caching where supported; do not make unsupported Ollama cache claims.
4. **Context economy** – benchmark against Aider repo-map/token efficiency and beat it with AST/import/LSP evidence per token.
5. **Git workflow polish** – make common diff/undo/commit flows at least as convenient as Aider without weakening safety.
6. **Provider breadth** – LocalCode native should not require Ollama forever; keep provider adapters isolated from agent safety logic.
7. **Fuzzy/structural patch recovery** – improve edits when exact old text drifts, but never bypass SHA approval/precondition semantics.
8. **UI polish/transparency** – Desktop and Android should expose plan, current phase, tool execution, verification and recoverability at OpenCode-level or better.
9. **Benchmark evidence** – once #16 is merged, run the same repos/tasks/model/quantization/context limits against LocalCode Native, Aider and OpenCode. Publish raw manifests/results, not only a score.
10. **Release engineering** – production Play Store signing remains user-controlled. A future release workflow can consume configured secrets securely, but should never auto-generate a replacement upload key for an existing Play app.

---

## 10. Current PR #24 file-level change map

At the time this canonical state was written, PR #24 changes/adds at least:

- `src/project_catalog.go`
  - ProjectDeletePreview
  - inspectProjectDelete
  - create_project
  - stricter delete mode separation
- `src/project_folder_actions_test.go`
  - project scaffold tests
  - preview counts
  - symlink non-follow test (skip if Windows runner lacks symlink privilege)
  - empty recursive-delete rejection
- `src/remote_project_api.go`
  - authenticated narrow project management endpoints
- `src/mobile_safe_remote_server.go`
  - route registration
  - mobile global-approval blocking wrapper
  - secure HTTP/TLS startup using the mobile-safe handler
- `src/main.go`
  - production startup switched to `startMobileSafeProductionRemoteServer`
- `src/static/remote.html`
  - Projects tab
  - create/delete flows
  - Play Store quick action
- `android/app/src/main/java/com/inetconnector/localcode/remote/MainActivity.java`
  - same-origin navigation
  - private-address classification
  - WebChromeClient dialogs
- `src/android_remote_security_test.go`
  - source invariants for Android hardening
- `src/mobile_project_remote_test.go`
  - auth/allowlist/delete-confirmation/global-approval tests
- `scripts/build-playstore.ps1`
  - deterministic local Gradle release helper
- `STATE.md`
  - this canonical handoff

Initial branch commits include:

- `17688e48...` safe project creation/delete previews
- `a3a5193f...` project scaffold/delete-preview tests
- `766b7ecb...` remote project API
- `875a9680...` Play Store build helper
- `5f3e0144...` Android Remote hardening
- `632f299f...` Android security regression test
- `0d3e816b...` mobile project/Play Store UI
- `41b75f77...` hardened remote server startup
- `e3737613...` main startup switch
- `1e6f5ba1...` mobile global-approval block
- `47a0c11b...` Windows-safe remote project test serialization

PR #24 Quality run #243 was queued/running when this state section was authored. The next AI must inspect the newest run rather than trusting this sentence.

---

## 11. PR #24 merge checklist

Do not mark ready or merge until all are true:

- [ ] gofmt green
- [ ] go vet green
- [ ] remote.html inline JavaScript syntax green
- [ ] Android Remote APK compiles on Windows runner
- [ ] govulncheck green
- [ ] full-stack loopback integration green
- [ ] all Go tests green
- [ ] race detector green
- [ ] statement coverage >=80.0%
- [ ] native Windows builds green
- [ ] git diff check green
- [ ] project create/delete tests pass on Windows
- [ ] Android security source test passes
- [ ] no new route bypasses Remote token auth
- [ ] mobile global approval remains blocked
- [ ] Play Store button does not upload/publish/rotate signing keys
- [ ] PR still merges cleanly with current master

If `master` advances because #15/#16 are merged, compare/reconcile PR #24 and rerun Quality on the new merge state before merging.

---

## 12. Branch/repository hygiene

Repository cleanup policy:

- Merged feature branches should be deleted when tooling/access allows it.
- Closed stale branches should not remain as the basis for new changes.
- Current connector used by the AI does not expose a safe delete-ref operation; do not simulate branch deletion by force-moving refs.
- Do not restore `.github/workflows/one-time-coverage-analysis.yml`; it was intentionally obsolete and deleted.
- Keep `.github/workflows/quality.yml` and release workflow(s) unless a replacement is demonstrably better.

Known branches that were previously classified as historical/merged/stale include old approval, atomic-edit, code-intelligence, LSP, Tree-sitter, parallel-read and security-hardening branches. Use current PR/branch listings before deleting anything manually.

---

## 13. Coding/release conventions

- Windows is the primary target; WSL support is useful but must not make Windows startup dependent on WSL.
- Go code must be gofmt-clean.
- User-facing changes must keep DE/EN complete.
- Use explicit, bounded timeouts and cancellation.
- Prefer deterministic safety checks to prompt-only instructions.
- No force-push/history rewrite unless the user explicitly requests it and risk is understood.
- Do not commit generated release binaries, keystores, tokens or passwords.
- Commit messages should be concise conventional descriptions, e.g. `feat: ...`, `fix: ...`, `test: ...`, `security: ...`, `docs: ...`.

---

## 14. Definition of done for the user’s current direction

The user’s current direction is not merely “add a phone button”. The intended end state is:

1. New projects can be created cleanly from Desktop or phone and immediately contain useful AI handoff docs.
2. Empty project folders can be removed safely.
3. Non-empty project folders require explicit, content-informed confirmation and an exact server-side confirmation value.
4. Phone Remote has least-privilege project management, not a broad administrative shell.
5. Android connectivity is pinned to the paired private LocalCode endpoint.
6. Release builds can be initiated simply from phone while all dangerous signing/publishing operations remain separately controlled.
7. STATE.md always tells a fresh AI exactly what is implemented, what is in flight, what is blocked, and what to do next.
8. Crash recovery and reproducible cross-engine benchmarks are merged after fresh rule-compliant CI.
9. Model subagents and doom-loop protection are ported from their validated old designs to current master.
10. The comparison matrix is repeatedly updated from measured implementation reality until LocalCode leads every row.

When this file is updated after future work, replace stale “current” facts instead of endlessly appending contradictory release notes.
