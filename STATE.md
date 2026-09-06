# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-09-06 Europe/Berlin
**Repository:** `inetconnector/LocalCode`
**Default branch:** `master`  
**Current authoritative merged master:** `c790c0a5a676b7e07fcbc4bce18db9e5aa06a382` (Release `v6.9.2`; verified through GitHub API and merged PR #92)
**Last merged functional PR:** #92 `feat(orchestration): add planning mode, interactive plan cards, git path discovery, and modtime project sorting`
**Active branch:** `master`
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

This file is the self-contained restart point. Only merged `master` is authoritative product behavior. `TODO.md` contains unfinished work only.

## Active IDE release workstream — 2026-09-06

User request: build and publish a LocalCode extension for VS Code / Antigravity IDE positioned in the right Secondary Side Bar, support priority prompts during active work, auto-start backend, provide interactive context attachment and clipboard paste, ensure instant German localization, and complete remaining TODO work. Repository Markdown documentation was inspected; embedded demo prompts are examples, not new user instructions. No pre-existing dirty files existed. `aider_edit` is unavailable as an exposed tool; the allowed direct editing fallback was explicitly selected.

Implemented in this branch:

- `extensions/localcode/` is the independent 0.1.0 VSIX package, publisher ID `inetconnector`, VS Code API >=1.85, UI extension host. Registered exclusively in `viewsContainers.secondarySidebar` so that LocalCode docks directly in the right Secondary Side Bar (Auxiliary Bar) beside Google Antigravity Agent. Includes preloaded bilingual JSON translations (`{{initialStrings}}` & `{{initialLanguage}}`) eliminating raw key flashes, Windows Intl locale detection (DE default on German Windows), automatic headless backend auto-start (`LocalCode.exe` with `LOCALCODE_FAST_START=1`), interactive `+` attachment menu, direct clipboard paste button, bounded opt-in editor/selection/diagnostic context, one-time approvals, canonical safe file opening, native HEAD/current Git diff review, and explicit Windows backend launcher.
- Webview CSP denies network and arbitrary scripts; DOM rendering never interpolates model HTML. Bridge actions are allowlisted. Loopback HTTP rejects credentials/redirects/arbitrary hosts and has byte/time limits. Executable/URL settings have application scope; no workspace-injected launch path. Only trusted local workspaces; Remote/virtual workspaces disabled. Unsaved buffers are protected by an explicit save decision before a new run.
- `src/ide_transport.go`: Desktop-only `/api/stop-task` atomically compares `thread_id` and `run_id` under the AppState mutex before cancellation. Old global `/api/stop` remains backward compatible. `/api/ping` advertises `stop-task-v1` and `steering-v1` so older backends receive honest update errors.
- `src/agent_steering.go`, `src/agent.go`, `src/types.go`: ordinary agent-loop steering mailbox. `/api/steer` strictly validates exact active task/run, message ID and bounded text, deduplicates retries, and cancels stale inference. The loop drains before model/action boundaries. New user instructions resolve conflicts without changing tool permissions or original unchanged scope. Pending approvals are rejected on new steering; already-started tools complete safely. Terminal action admission and queue admission serialize; late input is rejected instead of silently lost. Existing step/time budgets stay bounded.
- Limits: 32 KiB/message, 16 queued, 64 IDs/128 KiB cumulative per run. Queue is transient; no crash replay or new recovery authority. Initialization, Mission/recovery and terminal phases reject steering. Applied input enters existing chat history; normal termination warns about undelivered queued input. The extension and Desktop composer both expose follow-up sending during runs. Remote gains no new routes or authority.
- CI adds extension syntax/localization/behavior/real-host/VSIX checks. The existing Quality threshold was 79.5 despite documented 80.0; it is corrected to 80.0. Release pipeline builds and attaches the VSIX alongside Windows/Android artifacts and checksums.
- **Copilot Dark Obsidian Aesthetic Redesign**: Redesigned the primary desktop web interface (`src/static/index.html`, `src/static/ui_polish.js`, `src/static/i18n_base.js`) to match Microsoft Copilot's style:
  - Deep dark obsidian palette (`--bg: #121212`, `--panel: #181818`, `--surface: #202020`, glassmorphism borders `rgba(255,255,255,0.08)`);
  - Rounded capsule pill `+ Neuer Chat` / `+ New chat` button (`border-radius: 9999px`, elevated hover, dark grey background);
  - Centered hero greeting `Hallo, wobei kann ich Ihnen helfen?` (DE) / `Hello, how can I help you today?` (EN) with starter subtitle;
  - Interactive pill suggestion chips (`Architektur analysieren`, `Tests ausführen`, `Git-Review`, `Release bauen`, `Pacman Arcade`) with prompt population and focus on click;
  - Floating capsule composer dock (`border-radius: 26px`, circular attach `+`, circular send `↑`, responsive layout);
  - 100% key-identical bilingual German/English catalog parity across 556 keys.
- **Git Discovery & Graceful Non-Repo Handling**:
  - Expanded `toolCandidatePaths` in `src/tool_registry.go` to find per-user Git installations (`%LOCALAPPDATA%\Programs\Git`, `%USERPROFILE%\AppData\Local\Programs\Git`, `%ProgramFiles(x86)%\Git`, and managed MinGit).
  - Updated `handleGitOverview` in `src/server.go` to distinguish between disabled settings and missing binaries, returning `{is_repo: false, ...}` without raw 400 errors for non-git project folders.
  - Dynamically hide `#gitBtn` in sidebar when `c.git_enabled === false`.
- **Project Catalog ModTime Sorting & Composer Unblocking**:
  - `listProjects` in `src/project_catalog.go` records directory `ModTime` (`UpdatedAt`) and sorts pinned projects first, followed by last modified/accessed projects descending.
  - Interactive project selector `<select id="projectSelect">` in the composer dock. Auto-selects the first available project when starting from an empty state to prevent `#sendBtn` disabling or question response blocking.
- **Autonomous Implementation Planning, Interactive Plan Cards & One-Click Approval Workflow**:
  - Enhanced system prompt in `src/agent.go` to explicitly instruct the agent to generate `implementation_plan.md` before coding complex greenfield projects or architectural refactorings.
  - Added dedicated **Plan Cards** in both Desktop Web UI (`src/static/index.html`) and VS Code / Antigravity IDE Extension (`extensions/localcode/media/app.js`, `style.css`).
  - Added clickable markdown file links (`.file-link`, `.file-chip`) that open files (`implementation_plan.md`, source code) directly in the active IDE editor via `localcode.openFile`.
  - Added one-click **`✓ Plan genehmigen & ausführen (Proceed)`** button to seamlessly approve and execute plans without manual typing.
  - Complete 100% identical German/English localization across all web and extension dictionaries.
  - Added unit tests in `src/planning_mode_test.go` and `src/project_sorting_git_test.go`.

Verification checkpoint: initial full Go race suite passed; full post-implementation race suite passed (localcode 240.346s). Full Windows build passed (`scripts\build.ps1`, isolated test pass + randomized shuffle pass + amd64 GUI & diagnostics binaries). Browser UI smoke passed (`python scripts\ui-e2e-test.py`, `FULL UI E2E OK 43 requests`). VSIX packaged successfully. Focused steering tests prove inference interruption, stale-action rejection, FIFO/idempotence/bounds, concurrent terminal admission and task-bound stop. Node checks and 5 behavior tests passed. Real Extension Host suites passed in installed Antigravity IDE and official VS Code using isolated profiles and fixture HTTP service. Playwright UI visual regression captures in DE and EN confirmed exact Copilot-style layout, capsule pills, hero chips and composer.

Tool discovery: Go at `%LOCALAPPDATA%/Programs/GoToolchains/go1.26.6/go/bin/go.exe`; Node 24.19.0 in Codex dependency runtime; npm 11.6.4 discovered under Visual Studio 18 Community `MSBuild/Microsoft/VisualStudio/NodeJs/node_modules/npm/bin/npm-cli.js` after PATH/cache/LocalCode-tools/VS searches. Python 3.11 includes Playwright; GitHub CLI at `C:/Program Files/GitHub CLI/gh.exe`; installed Antigravity at `%LOCALAPPDATA%/Programs/Antigravity IDE/Antigravity IDE.exe`. Logs with command exit/output evidence are in ignored `logs/ide-*`. Tests never publish personal prompts or use personal backend tasks.

Current release/review: Release v6.9.2 is published on GitHub Releases (merged PR #88 and state sync PR #89). VSIX `localcode-0.1.0.vsix` is packaged, attached to the release with SHA-256 checksums, and installed locally in Antigravity IDE (`Antigravity IDE.exe --install-extension`). Open VSX and VS Marketplace listings remain pending authenticated publisher credentials (documented in `TODO.md`).

Next: Open VSX and Visual Studio Marketplace registry listings remain pending configured publisher credentials. All core tasks, IDE extension, steering, Copilot dark obsidian UI aesthetic, release automation, and local Antigravity installation are complete. No equality of model quality or proprietary feature parity has been demonstrated. `extensions/localcode/state.md` provides extension-specific continuation details.

## 2.1 Merged runtime, Windows platform, Browser & Desktop Automation, & Android Remote (v6.9.1)

- **Persistent Pairing & Explicit Unpairing on Android Remote**: The Android companion app persists the base connection URL and TLS fingerprint without stale pairing codes in query/hash parameters (`MainActivity.java`), enabling persistent automatic reconnection across app/server restarts without re-pairing. The remote web frontend cleanly removes code fragments via `history.replaceState` upon successful pairing and only invalidates tokens on explicit HTTP 401 unauthorized responses. An explicit "Gerät entkoppeln" / "Unpair device" option in the settings modal and drawer navigation allows explicit disconnect/unpair, revoking credentials on both the client and server (`/remote/api/unpair`).
- **Autonomous Browser Automation (Playwright MCP & Headless Chromium)**: First-class controlled browser automation backend (`src/browser_automation.go`) exposing `browser_navigate`, `browser_inspect`, `browser_click`, `browser_type`, `browser_screenshot`, and `browser_extract`. Dispatches to Playwright MCP (`@playwright/mcp@0.0.78`) when enabled, with automatic fallback to headless Chromium/Edge for DOM inspection, structured text/table extraction, and screenshots. Read operations are auto-approved in normal mode; mutations (`browser_click`, `browser_type`) require approval. Integrated into Doctor diagnostic item 7 ("Autonomous Browser Automation").
- **Windows Desktop & UI Automation (Accessible GUI Agent Engine)**: Windows UI Automation engine (`src/desktop_automation_windows.go`, `src/desktop_automation_other.go`) enabling the agent to list visible top-level windows (`desktop_list_windows`), inspect accessibility control trees (`desktop_inspect`), invoke controls (`desktop_click`), enter text (`desktop_type`), and capture window/screen GDI screenshots (`desktop_screenshot`). Strict security guardrails block sensitive system windows (`Task Manager`, `Windows Security`, `LogonUI`, `Credential Prompt`, `UAC`). Integrated into Doctor diagnostic item 8 ("Windows Desktop & UI Automation").
- **Audio / Speech Feedback (TTS) on Android Mobile Remote**: Native Android `TextToSpeech` bridge (`window.LocalCodeAndroid.speak(text)`, `window.LocalCodeAndroid.stopSpeaking()`, `isTtsAvailable()`) in `MainActivity.java` with Web Speech API fallback in `src/static/remote.html`. Includes a settings toggle (`#ttsToggle`) and on-demand read-aloud buttons (🔊) on agent response bubbles.
- **Codex-Style Mobile Remote UI**: Redesigned floating pill composer with glassmorphic backdrop filter, border highlight on focus, sleek circular attachment (+) button, compact inline project selector chip, crisp outline microphone button on the far right, and modern high-contrast circular send and stop buttons.
- **Quick Starter Prompt Cards**: 2x2 grid of modern starter cards on the *Neue Aufgabe* screen (Architektur analysieren 🔍, Tests ausführen 🧪, Git-Status prüfen 🌿, Release-Build erstellen 📦) for fast one-tap task initialization.
- **Colored Syntax-Highlighted Git Diffs**: Line-by-line colored diff rendering (`.diff-box`, `.diff-add`, `.diff-del`, `.diff-hunk`, `.diff-meta`) with copy buttons across approval dialogs and review logs.
- **Lightweight Markdown & Code Blocks**: Markdown formatting supporting bold, italic, inline code, lists, and fenced code blocks with language badge headers and one-tap clipboard copy buttons.
- **Floating Scroll-To-Bottom Button**: Smooth auto-scrolling button (`#scrollToBottomBtn`) dynamically toggled when scrolled up >120px from the bottom.
- **Android Haptic Feedback Bridge**: Native vibration bridge (`window.LocalCodeAndroid.vibrate(ms)`) integrated into `MainActivity.java` and `AndroidManifest.xml` with web fallback (`navigator.vibrate`) for approval triggers, card taps, and task completions.
- **Modern Approval Dialog**: Concise bilingual buttons ("Genehmigen" / "Approve", "Immer erlauben" / "Always allow", "Ablehnen" / "Reject") with clean dark-surface styling, harmonious contrast, and code diff preview.
- **Android Mobile Remote App**: Starts on **Neue Aufgabe / New task** tab. The **Tasks** tab is hidden by default and can be enabled dynamically in the Settings modal (`#showTasksTabToggle`). Navigation swipe and view switching dynamically adapt to active tabs. Approval requests render dynamically as a modern popup over the active view. Transient status noise is cleanly filtered from history upon completion.
- **Voice Input & Fast Dispatch**: Microphone button is positioned on the far right across Desktop and Android Remote composers. Spoken input with speech recognition completion immediately triggers submission without requiring a manual click.
- **Tool Profiles & Scheduler Capabilities**: Expanded `toolProfiles` and candidate search paths to include `cmd`, `wsl`, `bash`, `pwsh`, `winget`, `make`, `gcc`, `clang`, `tar`, `7z`, Windows Task Scheduler (`schtasks`), and Linux/WSL schedulers (`crontab`, `systemctl` timers, `at`). Agent system prompt provides structured scheduling instructions.
- **Model Fallback & Cluster Mesh**: Agent start falls back to `Config.LastModel`, `Config.OllamaDefaultModel`, or `qwen2.5-coder:14b` when no model is explicitly supplied. Local and remote Ollama tags are merged cleanly with automatic failover to local daemon if remote mesh endpoints encounter errors.
- **Native Android Shell**: Persists last accepted Remote URL and TLS fingerprint; falls back to automatic mDNS and bounded parallel LAN discovery (`/remote/api/discovery` / `/remote/api/ping`).
- **Windows Platform & Fast Start**: `START.bat` and `FAST-START.bat` provide instant startup with `LOCALCODE_FAST_START=1`. `src/platform_windows.go` avoids Visual Studio "file not found" errors by opening Explorer when no project file is found.
- **Native Windows Setup Installer & Inno Setup Package**: Standalone Go-based Windows GUI installer and uninstaller (`dist\LocalCode-Setup.exe`, `INSTALL.bat`, `scripts/build-installer.ps1`) that installs to `%LOCALAPPDATA%\Programs\LocalCode`, creates Start Menu and Desktop shortcuts, configures User `PATH`, and registers in Windows Settings *Apps & Features* (`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\LocalCode`) with quiet uninstall support. Includes classic Inno Setup compiler script (`installer/localcode-setup.iss`). The current local worktree fixes native setup PowerShell quoting so `UninstallString` and `QuietUninstallString` are written correctly even when the target path is quoted. It also stages `assets\localcode.ico` into installer payloads, embeds that icon into the native setup EXE resource, and sets Start Menu/Desktop launcher `IconLocation` plus Apps & Features `DisplayIcon` to the installed icon.
- **Windows Firewall Integration**: `src/remote_firewall_windows.go` performs non-elevating read checks, logging status without blocking on UAC. `scripts/install-remote-firewall-rule.ps1` provides an explicit 1-time setup for Administrator installation of LocalCode Remote rules.
- **LocalCode Autonomous Demo Showcase (`LocalCode-Demo/`)**: First-class showcase directory containing a complete, playable 1980s Namco-style Pac-Man Arcade clone built autonomously by LocalCode when driven from the Android Mobile Companion app. Includes full vanilla game files (`index.html`, `style.css`, `audio.js`, `maze.js`, `ghost.js`, `pacman.js`, `game.js`), Windows setup installer (`build-installer.ps1`, `INSTALL.bat`), gameplay screenshots, bilingual documentation (`README.md`, `PROMPTS.md`), and prominent landing links in the repository root README.





## 1. Product objective

LocalCode is a Windows-first, local-first coding-agent/development system centered on local models and controlled tool execution.

Long-term orchestration target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler/Resource Manager -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated workspaces/worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Core hardware rule: `logical task parallelism != model inference parallelism`.

## 2. Current merged runtime and Agent-Team state

Merged runtime includes the Windows-native Go application, loopback Desktop HTTP/SSE API, LocalCode Native agent loop with approvals/reliability guards, selectable Native/Aider/Claude Code/OpenCode/Claw Code engines, Ollama integration, persistent project/task history, controlled file/Git/build/test/tool/web/MCP/attachment/asset operations, context compaction, local memory boundaries and durable run recovery.

Executable Child roles remain read-only **Explorer**, **Planner** and **Reviewer**. Their schemas allow project-tree/file/text/LSP reads and structured finish only. Mutation, shell, Git, network/web, MCP tool calls, installation, memory writes, approvals and recursive spawning remain absent.

Merged orchestration includes structured Agent contracts, deterministic Task DAG, bounded Scheduler/Resource Manager, scheduled read-only dispatch, race-safe finalization/cancellation, governed Mission entry, Mission budgets/accounting, stable `MissionID` separated from execution-scoped `RunID`, Desktop/Mobile observation, diagnostics and reproducible synthetic/opt-in Ollama parallelism benchmarks.

Current Child dispatch is synchronous. Higher configured model-slot capacity alone is not proof of parallel model inference.

## 3. Durable read-only Mission recovery – merged through PR #69

`run_journal.go` remains the **single durable recovery authority**. Read-only Missions use bounded structured metadata in the existing `active-run.json`; there is no second Mission journal.

Recovery layers now merged:

- **#61** restart reconciliation from canonical project/Git identity, exact `HEAD` and hashed worktree evidence. Crash-running work is never inferred successful.
- **#62** bounded completion evidence without copying raw Child/model output into recovery authority.
- **#63** durable lifecycle counters/timestamps; repeated running snapshots cannot double-count attempts.
- **#64** deterministic read-only postcondition verification against fresh project/Git state and canonical completion evidence.
- **#65** deterministic transition planner with three attempts/task and 192 attempts/Mission, verified dependency requirements and fail-closed invalid-state handling.
- **#66** trusted read-only `MissionRecoveryControlSnapshot`; fresh observation, transient verification and bounded journal-stability retry. Snapshot/control data is observation, not a Scheduler lease or execution token.
- **#67** bounded continuation materialization for one explicit current `resume_candidate` or `retry_candidate`, containing only that candidate plus transitively verified dependencies. Trusted role capabilities are regenerated, model identity and historical Usage are revalidated, and crash/offline downtime is excluded from active Mission time.
- **#68** atomic execution-capable continuation admission. Fresh materialization is recomputed while holding the AppState run gate; exact journal fingerprint/file version is checked; a fresh execution RunID is durably reserved before any Scheduler exists; historical task/Mission budgets and accepted Usage remain cumulative; subset execution merges back into the full durable Mission.
- **#69** explicit **Desktop-loopback-only** recovery inspection and continue transport plus bilingual Output-inspector controls. Startup remains passive and Remote/Mobile receives no recovery-control authority.

### 3.1 Attempt, budget and cancellation invariants

A durable reservation is not an attempt. `AttemptReserved` is written before Scheduler admission, while `AttemptCount` advances only after a durable Scheduler `Running` checkpoint proves execution actually started. A crash in the reservation/admission gap does not consume retry budget.

Recovery never silently mints a new budget. Historical scheduler-accepted Usage remains cumulative; task and Mission limits are restored conservatively and can only be narrowed. Offline/crash downtime is not counted as active Mission execution time.

Explicit cancellation uses the existing AppState/Scheduler cancellation owner. Late cancelled Child results are non-authoritative.

### 3.2 Desktop recovery surface merged in #69

`GET /api/mission-recovery` returns a deliberately bounded DTO derived from trusted recovery control state. It exposes only interrupted Run/Mission identity, observation/fingerprint hashes, reconciliation state, plan/candidate availability and per-task durable state/action flags required for an explicit Desktop choice.

It does **not** expose project paths, objectives, capabilities, raw Child/model output, findings, Usage or Mission accounting.

`POST /api/mission-recovery/continue` accepts one explicit task plus inspected stale-state preconditions. The body is size-bounded and strictly decoded. Only a current `resume_candidate` or `retry_candidate` can proceed.

UI-provided hashes are preconditions, never authority. Admission recomputes fresh #67 governance under the AppState run gate, rechecks Mission/action identity and the durable journal fingerprint, restores/caps budgets, and then executes the #68 exact journal CAS. `202 Accepted` is returned only after durable reservation and AppState ownership succeed. Accepted execution then runs asynchronously under the existing Scheduler and remains cancellable through `StopAgent`.

`SnapshotSHA256` records the observed control snapshot but is not reused as an authorization token because that digest includes observation time. Durable staleness is bound by `JournalSHA256`, while current project/Git reconciliation and transition planning are recomputed immediately before admission.

### 3.3 Desktop/Remote trust boundary

`src/static/mission_status.js` renders the bilingual **Interrupted Mission / Unterbrochene Mission** card in the Desktop Output inspector. Only current candidates get explicit **Resume task / Retry task** controls; nothing auto-posts or auto-resumes.

The two recovery routes live only on the Desktop `Server` and inherit its loopback Host, Origin and `Sec-Fetch-Site` checks.

`RemoteServer` has no recovery route. Regression tests require `/remote/api/mission-recovery` and `/remote/api/mission-recovery/continue` to remain `404`. Mobile continues to receive only the narrow active-Mission indicator and existing stop behavior, with no recovery IDs, plan, Scheduler, budget or continuation authority.

## 4. Verification status

Current local installer worktree verification on 2026-09-05:

- `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\build.ps1` passed and rebuilt `dist\LocalCode.exe`, `dist\LocalCode-Debug.exe`, and `CHECKSUMS-SHA256.txt`.
- `go test -count=1 ./cmd/localcode-setup` passed after the setup quoting and installer/launcher icon fixes.
- `go test -count=1 -run "TestWindowsInstallerPackagingUsesLocalCodeIcon|TestWindowsPowerShellScriptEncodings|TestWindowsBatchLaunchersAreASCIIAndCRLF" .` passed after the installer/launcher icon fixes.
- `go fmt ./...`, `go vet ./...`, and `go test -race -count=1 ./...` passed across all packages (`localcode`, `benchharness`, `cmd/localcode-bench*`, `cmd/localcode-setup`).
- `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\build-installer.ps1` passed after the installer/launcher icon fixes and rebuilt `dist\LocalCode-Setup.exe`; it staged `dist\localcode.ico` and generated `src\cmd\localcode-setup\rsrc_windows_amd64.syso`.
- Direct Windows-amd64 GUI, diagnostics, and setup `go build` checks passed; `[System.Drawing.Icon]::ExtractAssociatedIcon` returned a 32x32 icon for the setup build.
- Frontend JavaScript syntax checks passed using Visual Studio's bundled Node.js at `C:\Program Files\Microsoft Visual Studio\18\Community\MSBuild\Microsoft\VisualStudio\NodeJs\node.exe` (`v24.12.0`) for both `src\static\*.js` and inline `<script>` blocks in `src\static\*.html`.
- PowerShell syntax validation passed across all `scripts\*.ps1` files via the PowerShell AST parser.
- `powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\build-android.ps1` passed and produced `C:\Users\frede\AppData\Local\Temp\localcode-android\LocalCode-Remote-debug.apk` with SHA-256 `3750d00df22675ce0af3b79a11e8ea9ca66c749090e701b87dcf48299ca634b4`.
- `dist\LocalCode-Setup.exe --silent` installed successfully to `%LOCALAPPDATA%\Programs\LocalCode`; installed binary hashes match the rebuilt `dist` artifacts; Start Menu/Desktop shortcuts exist; User `PATH` contains the install directory; HKCU uninstall metadata includes `UninstallString` and `QuietUninstallString`.
- Browser UI smoke test `python scripts\ui-e2e-test.py` was repaired (added static asset routing in Playwright mock handler to serve `.js`/`.css`/`.svg` from `src/static/` and updated context menu selector) and passes 100% green (`FULL UI E2E OK 41 requests`).
- Persistent Mission Knowledge (`src/agent_mission_knowledge.go`) implemented with JSON schema v1, project-hash naming, 64-item FIFO eviction, 128 KiB compaction, secret redaction, and atomic writes. Fully covered by `src/agent_mission_knowledge_persist_test.go`.
- Quality statement coverage gate reached >=80.0% across all packages (`localcode`: 80.0%, `localcode/cmd/localcode-bench-aider`: 85.3%, `localcode/cmd/localcode-bench-claw`: 85.2%, `localcode/cmd/localcode-bench-native`: 83.5%, `localcode/cmd/localcode-bench-opencode`: 84.3%).

The older #69 runs that stopped at format/Vet are historical failures only and are not current evidence.

### 4.2 Crash-safe attempt reservation

A durable continuation reservation is admission intent, not proof that execution began.

`MissionTaskLifecycle` therefore has bounded `AttemptReserved` / `AttemptReservedAt` evidence. Reservation sets that marker but does **not** increment `AttemptCount`. The first durable Scheduler `Running` checkpoint consumes the marker, increments `AttemptCount` exactly once, updates `RetryCount`, and records `LastStartedAt`.

This distinction is deliberate: if the process crashes after durable reservation but before Scheduler admission, restart sees a non-running task with the same unconsumed reservation. It can rebuild the fresh recovery materialization and rotate to another execution RunID without consuming another retry. Repeated running snapshots still cannot double-count the attempt.

The fixed maximums remain three actual started attempts per task and 192 actual started attempts per Mission.

### 4.3 Historical task and Mission budgets

Recovery must not silently mint a fresh budget.

For an attempted task, the prior `BudgetSnapshot.Limit` is durable evidence of the normalized total Scheduler budget actually governing prior execution. PR #68 restores a conservative total limit from durable task budget plus that snapshot, then caps it again through current canonical role/configuration defaults so recovery can never widen the trusted runtime envelope.

The graph/Scheduler retains this **total** task limit. Historical accepted Usage stays cumulative. Immediately before Child execution, only the detached execution-task copy is capped to the remaining per-task budget. This prevents the erroneous state `remaining limit + cumulative Usage`, which would shrink the same historical usage twice on the next checkpoint.

Mission budget tracking is similarly seeded with historical accepted Model/Tool/Token usage and a synthetic active-time anchor. Prior durable active Mission time remains charged; crash/offline downtime remains excluded; current continuation active time is added normally.

### 4.4 Cumulative Scheduler accounting

`runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded` copies the historical `UsageByTask` map before admission. Accepted new result usage is added to that seed instead of overwriting it. Untouched historical siblings remain in the map. Negative/corrupt seed usage fails before Scheduler admission.

Late cancelled Child results remain non-authoritative because only Scheduler-finalized `Applied` results are accumulated.

### 4.5 Continuation checkpoint/finalization

PR #68 does **not** call the ordinary `finishMissionRunJournal`, because that routine rebuilds `Mission.Tasks` from the supplied graph and is only correct for a whole fresh Mission.

`finishMissionRecoveryContinuation` instead:

- captures and verifies the exact journal file version before modifying it and commits through the same version-bound atomic write path;
- requires the current execution RunID/MissionID;
- merges Scheduler state/completion evidence only for tasks present in the bounded continuation graph;
- preserves all unrelated durable Mission tasks;
- uses cumulative Scheduler Usage for continued tasks and canonical accepted historical Usage for untouched tasks, including older records whose accepted usage is present only in `BudgetSnapshot.Usage`;
- rebuilds Mission-wide accounting across the full durable Mission;
- terminalizes `succeeded/completed` only when **all durable Mission tasks** are successful;
- terminalizes an explicit context cancellation/deadline as Mission `cancelled`, cancelling any remaining unfinished durable tasks;
- otherwise leaves the Mission nonterminal in `mission-read-only` for a later explicit fresh recovery decision.

After the execution returns, AppState is reset only if its RunID still matches that continuation execution, and cached `Recovery` is refreshed from the single durable journal.

### 4.6 Focused tests and active branch subsystems

Focused active-branch tests in `src/run_journal_mission_admission_test.go`, `src/mission_recovery_transport_test.go`, `src/agent_mission_knowledge_test.go`, `src/agent_worktree_test.go`, `src/agent_integrator_test.go`, `src/computemesh_test.go`, and `src/agent_scheduler_recovery_usage_test.go` cover:

- historical Scheduler Usage seeding, accumulation, defensive copy and fail-closed invalid seed;
- restoration of a previously normalized total task budget when the durable task budget is zero;
- detached remaining-budget capping without mutating the total graph budget;
- stale journal fingerprint rejection without RunID/lifecycle mutation;
- RunID rotation plus explicit parent-run lineage;
- crash after durable reservation but before Scheduler admission, reservation reuse after restart and AttemptCount increment only on the later Running checkpoint;
- preservation of unrelated durable tasks by the continuation finalizer;
- terminal success only when the full durable Mission is successful;
- Mission-wide accounting that includes untouched historical BudgetSnapshot usage;
- terminal Mission cancellation including unrelated unfinished durable tasks;
- rejection of an already active AppState before the executor can run;
- Desktop-only loopback/CSRF-protected recovery transport (`/api/mission/recovery` and `/api/mission/recovery/continuation`) with strict Mobile Remote isolation;
- bounded Mission Memory/Knowledge (`src/agent_mission_knowledge.go` and `/api/mission/knowledge`) for architecture decisions, subsystem contracts, known failures, and test evidence;
- Phase 7 LocalCode-managed Git worktrees (`src/agent_worktree.go`) for isolated mutation-capable Builder agents with directory containment, single-lease concurrency, non-colliding branch allocation, and non-destructive cleanup;
- Phase 8 Integrator, Test Agent, and Independent Reviewer (`src/agent_integrator.go`) with single-authority merge, objective evidence isolation, structured PASS/FAIL/REPAIR decision lifecycle, conflict recovery, and stagnation protection;
- Phase 9 Constrained Agent Factory & Bounded Replanning (`src/agent_factory.go`, `src/agent_mission_replanning.go`, and their focused tests) for type-safe role mapping, inert dynamic role quarantine, deferred tool resolution, and bounded DAG replanning with hard task caps (32 tasks), cycle limits (3 per task), depth caps (5), and symptom-hash stagnation protection;
- Backend-Neutral Local Inference Runner (`src/inference_backend.go`, `src/inference_backend_test.go`) supporting ComputeMesh, Ollama, and llama.cpp/OpenAI-compatible endpoints (`/v1/chat/completions`) without silent provider drift;
- LocalCode Doctor Subsystem (`src/doctor.go`, `src/doctor_test.go`, `GET /api/doctor`) for structured full-system health diagnostics covering ComputeMesh cluster connectivity, GPU/VRAM live specs, local Ollama daemon, Git worktrees, MCP stdio processes, and coding engines;
- ComputeMesh decentralized cluster subsystem (`src/computemesh.go`, `src/computemesh_test.go`, `src/static/computemesh.js`) for zero-config provider self-compute (0% platform fee) with auto-discovery of keys from `.computemesh/provider_config.json`, live workstation node probing, bearer token injection in `OllamaClient`, live hardware/cluster latency and status probing (`https://computemesh.inetconnector.com`), model discovery, REST endpoints (`/api/computemesh/status`, `/api/computemesh/autodetect`, `/api/computemesh/test`), and dual German/English localization;
- E2E Automation & Remote Control Service Harness (`scripts/test-automation-service.ps1`, `scripts/test-android-remote-full.ps1`) validating 10 vital system subsystems: Desktop REST API, Projects, Settings roundtrip, Engines, MCP status, System Doctor / Diagnostics, Remote Pairing, Token Authentication, Android Companion verification via ADB, Thread/Chat lifecycle;
- HTTP 429 Rate-Limit Exponential Backoff in `OllamaClient` (`src/ollama.go`) for resilient inference over ComputeMesh cluster nodes;
- Streamlined & Decluttered UI (`src/static/remote.html`, `src/static/index.html`) with collapsible tool accordions and high-level progress indicators;
- Camera QR Scanner Button on Pairing Screen (`src/static/remote.html`, `MainActivity.java`), top-right Header Gear Settings Menu (`⚙️`) with coding engine selection modal, uncluttered composer dock, and safety confirmation dialog on project switches.

The subsystem results above are historical evidence from the merged installer/runtime workstream. Current IDE-branch verification and merge readiness are recorded in the active workstream section above; they must not be inferred from these older results.

## 5. Safety and correctness invariants

- Canonical project/workspace containment including symlink/junction escape protection where applicable.
- SHA/version preconditions and atomic conflict-aware writes.
- Owned subprocess timeout/cancellation and Windows process-tree termination.
- No default or silently persistent unrestricted capability equivalent.
- Planner-requested or persisted capabilities never become executable authority merely by being present.
- Read-only Child schemas remain mutation-free until a separately reviewed Builder/worktree phase.
- `run_journal.go` remains the single durable Mission recovery authority.
- Current project/Git reconciliation outranks historical verification.
- Crash-running work is never successful by inference.
- Reservation is not an attempt; attempts count only at durable Scheduler `Running`.
- Historical Usage/budget evidence is cumulative and must never be silently reset or widened.
- Offline/crash downtime is not active Mission execution time.
- Recovery plan/snapshot/materialization objects are not reusable execution tokens.
- Desktop recovery remains explicit and loopback-only; Mobile/Remote has no recovery-control authority.
- Stable Mission identity remains separate from execution-scoped RunID.
- Startup remains passive: no automatic Mission resume/retry/replay.
- Statement coverage Quality gate remains >=80.0%; safety/test gates are never weakened to make CI pass.

## 6. Important files

Rules/docs: `AGENTS.md`, `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, `docs/SECURITY.md`, `docs/ORCHESTRATION_BENCHMARKS.md`, `.github/workflows/quality.yml`.

Mission/recovery core: `src/agent_mission.go`, `src/run_journal.go`, `src/run_journal_mission.go`, `src/run_journal_mission_reconcile.go`, `src/run_journal_mission_evidence.go`, `src/run_journal_mission_lifecycle.go`, `src/run_journal_mission_postcondition_verify.go`, `src/run_journal_mission_transition_plan.go`, `src/run_journal_mission_control.go`, `src/run_journal_mission_continuation.go`, `src/run_journal_mission_admission.go`, `src/agent_scheduler.go`, `src/agent_scheduler_dispatch.go`, `src/agent_mission_accounting.go` and focused tests.

Desktop recovery surface: `src/desktop_mission_recovery.go`, `src/desktop_mission_recovery_observer.go`, `src/server.go`, `src/static/mission_status.js`, `src/desktop_mission_recovery_test.go`, `src/desktop_mission_recovery_server_test.go`, `src/desktop_mission_recovery_remote_test.go`.

Mobile boundary: `src/remote_server.go`, `src/remote_mission_status_contract.md`.

Startup/mobile local changes: `START.bat`, `scripts/needs-build.ps1`, `scripts/build.ps1`, `src/main.go`, `src/platform_windows.go`, `src/static/remote.html`, `android/README.md`, `src/mobile_remote_ui_contract_test.go`, `src/windows_packaging_test.go`.

## 7. Exact next development direction

Follow the active IDE release workstream and canonical `TODO.md`. The earlier instruction to merge `release/v6.9.0-installer-automation` was stale: #87 is already merged at the verified base. Phase 7/8/9 implementation slices and tests listed in section 4.6 already exist; they must not be re-created from the old roadmap. This does not establish unrestricted mutation-capable scheduled child execution or model-quality parity.

## 8. Cleanup rule

Only `master` is authoritative after merges. Superseded feature refs and historical workflow runs must never be treated as active development. Delete obsolete branches/runs when the available GitHub integration supports the operation; never claim cleanup that was not actually performed.
