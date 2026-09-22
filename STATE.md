<!-- LOCALCODE:STATE:BEGIN -->
# Aktueller Projektstatus

- **Zuletzt aktualisiert:** 2026-09-22T23:50:59+02:00
- **Projekt:** `C:\Users\frede\Projekte\LocalCode`
- **Agentstatus:** bereit
- **Modell:** `qwen2.5-coder:14b`
- **Letzte Aufgabe:** keine laufende Aufgabe
- **Letztes Ergebnis:** noch kein Abschlussbericht
- **Aktualisierungsgrund:** Projekt ausgewählt
- **Git-Branch:** `Tool: git
Pfad: C:\Program Files\Git\cmd\git.exe
Arbeitsordner: C:\Users\frede\Projekte\LocalCode
Argumente: --no-pager branch --show-current
Exitcode: 0
Dauer: 35ms
Status: OK

STDOUT:
master`

## Git-Status

```text
Tool: git
Pfad: C:\Program Files\Git\cmd\git.exe
Arbeitsordner: C:\Users\frede\Projekte\LocalCode
Argumente: --no-pager status --short --branch
Exitcode: 0
Dauer: 40ms
Status: OK

STDOUT:
## master...origin/master
 M CHECKSUMS-SHA256.txt
 M README.md
 M STATE.md
 M TODO.md
 M android/app/src/main/AndroidManifest.xml
 M android/app/src/main/java/com/inetconnector/localcode/MainActivity.java
 M src/agent.go
 M src/android_remote_security_test.go
 M src/cmd/localcode-setup/main.go
 M src/cmd/localcode-setup/setup_test.go
 M src/core_test.go
 M src/coverage_behavior_windows_test.go
 M src/desktop_automation_other.go
 M src/desktop_automation_windows.go
 M src/instruction_context_test.go
 M src/mobile_safe_remote_server.go
 M src/platform_windows.go
 M src/remote_secure_server.go
 M src/remote_server.go
 M src/remote_server_test.go
 M src/server.go
 M src/startup_splash.go
 M src/static/i18n_base.js
 M src/static/index.html
 M src/static/mission_status.js
 M src/static/remote.html
 M src/static/ui_polish.js
 M src/tray_other.go
 M src/tray_test.go
 M src/types.go
?? phone_screen.png
?? phone_screen_after_restart.png
?? phone_screen_approved.png
?? phone_screen_current.png
?? phone_screen_discovered.png
?? phone_screen_paired.png
?? phone_screen_projects.png
?? phone_screen_projects_scrolled.png
?? phone_screen_projects_swiped.png
?? phone_screen_review.png
?? phone_screen_review_active.png
?? phone_screen_swiped.png
?? phone_screen_swiped_projects.png
?? phone_screen_verified_project.png
?? src/agent_multi_project_test.go
?? src/app_window_windows.go
?? src/app_window_windows_test.go
?? src/desktop_type_validation_windows_test.go
?? src/remote_discovery.go
?? src/remote_discovery_test.go
?? src/tray_windows_test.go
```

## Letzte Agentenaktionen

- noch keine Agentenaktion in dieser Sitzung

## Laufzeit- und Sicherheitskonfiguration

- Approval-Modus: `strict`
- Sandbox-Modus: `project`
- Agenten-Netzwerk: `true`
- Setup-Downloads: `true`
- Websuche: `duckduckgo`
- Git-Werkzeuge: `true`
- MCP-Server: filesystem (builtin), git (builtin), powershell (builtin)

## Pflegehinweis

Dieser verwaltete Abschnitt wird bei Projektauswahl, Agentenstart, Werkzeugaktionen und Abschluss automatisch neu geschrieben. Inhalte außerhalb der Marker bleiben erhalten.
<!-- LOCALCODE:STATE:END -->

# LocalCode – canonical current state / kanonischer aktueller Projektstand

**Verified:** 2026-09-22 Europe/Berlin (Android Startup Black Screen Elimination, Persistent Last Project Auto-Restoration via Native SharedPreferences Bridge & `/remote/api/select-project`, Plesk Live Portal & GitHub Downloads Architecture on `https://mesh.inetconnector.com/#downloads`, ComputeMesh Zero-Fee Local GPU & Cluster Collaboration, Antigravity IDE-Style Live Execution Trajectory across Desktop & Mobile Remote, Active Project Name Under LocalCode in identical font size, Android Horizontal Swipe Gesture Tab Switching, Safe Area Insets & Non-Sticky Edge Padding, Obsidian Dark Mode Discovery Wizard, UDP Broadcast Multi-Instance Auto-Discovery, 1-Click Connection, Concurrent Multi-Project Execution, Green Pulsating Active Project Indicators, Android hardware deployed & verified on Samsung Galaxy SM-S931B, Windows amd64 native build + full Go test suite 100% green)
**Repository:** `inetconnector/LocalCode`
**Default branch:** `master`  
**Current authoritative merged master:** `96ad7f7d76e6b2f5b01308be52199cc9d895d9ea` (HEAD equals local origin/master; latest published release checked: `v6.9.3`)
**Active branch:** `master`
**Primary roadmap issue:** #32 `feat: exceed Claw Code native orchestration capabilities`

## Android Startup Black Screen Elimination & Persistent Last Project Auto-Restoration — 2026-09-22

- **User Request**:
  - `"wenn ich die app beende und neu starte ist nur schwarzes bild. ich will dass das richtig startet und zwar mit dem letzten projekt das offen war"`
- **Root Cause & Fixes (`MainActivity.java`, `src/static/remote.html`, `src/remote_server.go`, `src/remote_server_test.go`)**:
  - **Cold/Warm Restart Void Elimination**:
    - Previously, when `MainActivity` resumed or restarted, `discoveryPanel` was hidden immediately while `webView` was loading the remote URL. If the daemon took time to respond or was offline, the user saw a bare black void.
    - Added a sleek **Obsidian Connecting Splash Overlay** (`connectingOverlay`) in `MainActivity.java` with a glowing lightning badge (`⚡`), live target address display, status text (`Verbinde mit Desktop-PC …`), and an instant fallback/cancel button (`Zurück zur Suche`).
    - Added robust error handlers (`onReceivedError`, `onReceivedSslError`) and `WebChromeClient.onProgressChanged` so that failed connections gracefully drop back to the multi-tab discovery wizard instead of hanging.
  - **Persistent Last Project Auto-Restoration**:
    - Added `saveActiveProject(path)` in `remote.html`: updates `state.project`, saves to `localStorage.setItem('localcodeRemoteLastProject')`, notifies the native Android shell via `window.LocalCodeAndroid?.setLastProject(path)` (saved in `SharedPreferences`), and asynchronously synchronizes with the desktop server via `POST /remote/api/select-project`.
    - Added `/remote/api/select-project` authenticated endpoint on `RemoteServer` in `src/remote_server.go`: validates project path containment within `RootProjectDir`, updates `cfg.LastProject` in `config.json`, selects thread, and broadcasts project state updates.
    - On startup and in `refresh()`, `remote.html` automatically reads the saved project from `localStorage` or `SharedPreferences` and immediately restores it.
    - Added automated unit test `TestRemoteSelectProject` in `src/remote_server_test.go` (100% PASS).
  - **Hardware & ADB Verification**:
    - Deployed and verified on Samsung Galaxy SM-S931B: cold restart launches cleanly into the Obsidian connecting overlay and loads directly into the active project workspace.

## Plesk Live Website Downloads & GitHub Releases Distribution Architecture — 2026-09-22

- **User Request**:
  - `"alles muss immer stets live auf die webseite in plesk zum doenload wenn es da bereits das localcode gibt oder in github der installer und die apk schau wo es zum download aktuell ist und wo es sein soll. es ist ja auch so dass localcode stets mit compumesh zusammenarbeiten soll checke das auch analysiere den plesk server dazu und wie alles zusammenspielt"`
- **Distribution Endpoints & Synchronization (`scripts/publish-downloads.ps1`, `deploy/sync_plesk_portal.sh`)**:
  - **Plesk Production Target**: `https://mesh.inetconnector.com/#downloads` (and `https://apps.inetconnector.com/`), served from `/var/www/vhosts/inetconnector.com/site2` and `/var/www/vhosts/inetconnector.com/apps.inetconnector.com/`.
  - **App Card 5 (LocalCode AI Studio & Remote)** in `portal/index.html` provides:
    - **Windows Setup Installer**: `/downloads/LocalCode-Setup.exe` (SHA-256 verified, native Go installer with embedded icons).
    - **Android Companion APK**: `/downloads/LocalCode-Remote-debug.apk` and `/downloads/LocalCode-Remote.apk`.
    - **VS Code & Antigravity Extension**: `/downloads/localcode-0.1.0.vsix`.
    - **Camera QR-Code**: Direct scannable QR SVG (`/assets/qr-localcode-apk.svg`) for instant mobile APK download.
  - **Automated Staging Pipeline (`scripts/publish-downloads.ps1`)**:
    - Automates compilation of `LocalCode.exe`, `LocalCode-Debug.exe`, `LocalCode-Setup.exe`, `LocalCode-Remote-debug.apk`, and `localcode-0.1.0.vsix`.
    - Stages all release assets to `C:\Users\frede\Projekte\ComputeMesh\portal\downloads\` and verifies SHA-256 checksums.
    - Plesk synchronization script `deploy/sync_plesk_portal.sh` synchronizes the `portal/downloads` directory directly to `/var/www/vhosts/inetconnector.com/site2/downloads/` and `httpdocs/downloads/`.

## ComputeMesh & LocalCode Integrated Collaboration Ecosystem — 2026-09-22

- **Zero-Fee Local Workstation Self-Compute (0% platform fee, zero latency)**:
  - If a local ComputeMesh node is running on `localhost:8080` (utilizing the machine's GPU/VRAM hardware, e.g. NVIDIA RTX 3080 16GB), LocalCode (`src/computemesh.go`) auto-discovers it via `ProbeRunningLocalComputeMeshNodeDetailed`, reads served models (`qwen2.5-coder:14b`, `deepseek-r1:14b`), and executes AI agent coding tasks locally at maximum throughput without incurring platform fees.
- **Provider Key Auto-Discovery & Decentralized Cluster Scale**:
  - `AutoDetectComputeMeshCredentials()` automatically reads active API keys and accounts from `~/.computemesh/provider_config.json` (`cm_tunnel_...`) or environment variables `COMPUTEMESH_API_KEY` / `COMPUTEMESH_URL`.
  - Seamlessly offloads larger reasoning tasks to decentralized ComputeMesh nodes (`https://computemesh.inetconnector.com`) with Bearer token authentication and HTTP 429 exponential backoff in `OllamaClient` (`src/ollama.go`).
- **Unified Multi-Device Control Plane**:
  - LocalCode Desktop daemon executes and coordinates concurrent multi-project tasks, MCP tools, and file operations.
  - LocalCode Android Mobile Remote monitors progress live with full Antigravity-style trajectory streaming.
  - All apps in the InetConnector ecosystem (ComputeMesh, LocalCode, Aegis, Ancesora) share the same underlying decentralized compute infrastructure and Plesk portal.

## Antigravity IDE-Style Live Execution Trajectory (Desktop & Mobile Remote) — 2026-09-22

- **User Request**:
  - `"bei localcode steht nur progres... und keiner weiss was da gerade im moment läuft. das soll alles genau so live sein wie bei antigravity siehe anderes bild"`
- **Trajectory Cards & Visual Hierarchy (`src/agent.go`, `src/static/remote.html`, `src/static/index.html`, `src/static/i18n_base.js`)**:
  - **Thinking & Planning Card**: Glowing breathing card displaying model reasoning, model pill, step counter (`qwen2.5-coder:14b · Schritt 1/60`), and clear status message (`Analysiere Aufgabe & plane nächsten Schritt...`).
  - **File Exploration**: `🔍 Explored <filename>` / `🔍 Explored N files, M searches ›` with collapsible details and syntax highlighting.
  - **Commands**: `⚙️ Ran command: <cmd> ›` with terminal-style box and `$ <cmd>` stdout/stderr output.
  - **File Edits**: `📝 Edited {} <filename>` with colored `+add` (green) and `-del` (red) badges (e.g. `+0 -39`) and syntax-highlighted diffs.
  - **Browser Automation**: `🌐 Exploring browser` with `[Preview]` badge, Goal, URL/selector, and pulsing `Working...` / `✓ Completed` status.
  - **Files With Changes Summary**: Bottom card summarizing all modified files with `[+X -Y] <filename> <filepath>`.
  - **100% Parity Bilingual i18n**: Key-synchronized German and English dictionaries in `i18n_base.js` and `remote.html`.
- **Verification & Testing**:
  - Full Go test suite passed (`go test -count=1 ./...` 100% green).
  - All contract and quality tests passed (`TestTranslationCatalogsHaveIdenticalKeys`, `TestMobileRemoteUIContractMatchesServer`, `TestQualityThreshold`).
  - Windows amd64 native build passed (`scripts/build.ps1`, isolated + randomized shuffle test passes, `dist/LocalCode.exe` and `dist/LocalCode-Debug.exe` built).
  - Live Android verification on Samsung Galaxy SM-S931B via ADB.

## Active Project Name Display Under "LocalCode" (Mobile Remote UI) — 2026-09-22

- **User Request**:
  - `"unter LocalCode soll man den Projektnamen in dem man sich befindet sehen (selbe grösse)"`
- **Implementation & Visual Design (`src/static/remote.html`)**:
  - **Hero & Empty State Headers**: Updated `renderNew()` and `renderEvents()` to extract active project basename (`projectName(state.project)`) and render `.hero-project` directly beneath `<h1>LocalCode</h1>`.
  - **Identical Font Size & Weight**: Styled `.new-hero .hero-project` with `font-size: 22px; font-weight: 700; color: #38bdf8; margin: 0 0 6px; letter-spacing: -0.02em; word-break: break-all;` to match the exact size and bold weight of `<h1>LocalCode</h1>` (`font-size: 22px; font-weight: 700;`).
  - **Live Active Task Indicator**: If the active project is currently running an agent task (`state.runningProjects.includes(state.project)`), an inline glowing green pulsating dot (`.project-pulse-dot`) is rendered alongside the project name.
  - **Hardware Verification**: Captured and verified live on Samsung Galaxy SM-S931B (`phone_screen_verified_project.png`).

## Horizontal Swipe Gesture Tab Navigation (Android & Remote UI) — 2026-09-22

- **User Request**:
  - `"wischen nach links oder rechts soll immer zwischen den tabs webseln merke dir das bei android programmen für immer"`
- **Architecture & Permanent Rule**:
  - **Android Native Tab Swiping (`MainActivity.java`)**:
    - Integrated `GestureDetector` with `SimpleOnGestureListener.onFling` in `MainActivity.java` and attached `dispatchTouchEvent(ev)`.
    - Horizontal swipe left (`diffX < 0`) switches to the next wizard tab (`📡 Auto-Suche` ➔ `📷 QR-Scan` ➔ `⌨️ Manuell`).
    - Horizontal swipe right (`diffX > 0`) switches to the previous wizard tab (`⌨️ Manuell` ➔ `📷 QR-Scan` ➔ `📡 Auto-Suche`).
    - Tactile vibration feedback on swipe transition (`triggerHapticTick()`).
  - **Remote Mobile WebView Tab Swiping (`src/static/remote.html`)**:
    - Global touch gesture listener in `bindSwipe()` detecting horizontal velocity and displacement while ignoring typing inputs (`<input>`, `<textarea>`, `<select>`, `<pre>`, `.diff-view`).
    - Smoothly transitions between navigation tabs (`Neue Aufgabe` ➔ `Projekte` ➔ `Papierkorb` ➔ `Review`) with active tab auto-scrolling (`scrollIntoView`) and haptic pulse.
    - Verified via live ADB swipe gestures on Samsung Galaxy SM-S931B (`phone_screen_projects_swiped.png`, `phone_screen_verified_project.png`).

## Safe Area Insets, WindowInsets & Obsidian Dark Mode Discovery Wizard — 2026-09-22

- **User Request**:
  - `"mach da mal ein screenshot das klebt ja total oben am rand was soll das merke dir. wenn du nocchmal ui machst soll es nie so am rand kleben weder sollen die android standardbuttons verdeckt werden noch oben die taskleiste. schau dass du die localcode app neu startest so dass se auch gefunden wird von localcode auf dem handy"`
- **Android Window Insets & Edge-to-Edge Safe Padding (`MainActivity.java`)**:
  - **Window Insets Listener**: Configured explicit `setOnApplyWindowInsetsListener` on the root container with `root.setFitsSystemWindows(false)` to reliably compute top status bar/notch insets and bottom system navigation bar insets across API 30+ (`WindowInsets.Type.systemBars()`) and legacy Android APIs.
  - **Safe Top & Bottom Clearance**: Enforced minimum safe margins (`Math.max(top, dp(24))` top, `Math.max(bottom, dp(24))` bottom) plus generous internal content padding (`dp(22), dp(20), dp(22), dp(44)`). The header branding and cards never touch the top status bar / camera notch, and the bottom action buttons / chat input never collide with or get obscured by the Android 3-button navigation bar (`|||`, `○`, `<`) or gesture handle.
  - **Non-Intrusive LAN & UDP Discovery**: `startLanProbeDiscovery()` and `startUdpBroadcastDiscovery()` populate `discoveredInstances` without auto-jumping, displaying discovered PC instances with hostname, version tag, active projects, glowing pulse indicators, and the prominent `"⚡ 1-Klick Verbinden"` button.
- **Server Lifecycle & Verification**:
  - LocalCode daemon running on PC with HTTP port `32145`, Remote HTTPS port `32146`, and UDP broadcast listener on port `32147`.
  - Android APK recompiled and re-installed on Samsung Galaxy SM-S931B via ADB.
  - Live hardware screenshots captured demonstrating discovery and paired remote workflow.

- **User Request**:
  - `"kann das auch automatisch koppeln er soll mit udp broadcast suchen localcode soll sich melden und es wird dann angezeigt in einem schönen button dann verbinden mit einem klick. es können auch mehrere instanzen gefunden werden und die sollen dann angezeigt werden. ausserdem soll auch an mehreren projekten gleichzeitig gearbeitet werden so wie in codex. dann leuchtet hinter dem projektname ein grüner pulsierender punkt falls er da gerade arbeitet mach das alles vollkommen funktionalbel fehlerfrei und mit allem was dazugehört ausführlichst da rein"`
- **UDP Broadcast Auto-Discovery & Multi-Instance Discovery Cards (`src/remote_discovery.go`, `src/remote_secure_server.go`, `android/.../MainActivity.java`)**:
  - **UDP Discovery Service (`src/remote_discovery.go`)**: Runs a persistent UDP broadcast listener on port `32147` (`DefaultUDPDiscoveryPort`). Responds to broadcast probes (`{"cmd":"discover"}`) with structured JSON `UDPDiscoveryPayload`:
    - Application identity (`"LocalCode Remote"`), version (`6.9.3`), hostname, TLS certificate SHA-256 fingerprint, server port (`32146`), reachable HTTPS/HTTP URLs, active projects, and list of currently running projects.
  - **Android Multi-Instance Broadcast Discovery & Dark Mode Wizard (`MainActivity.java`, `AndroidManifest.xml`)**:
    - `startUdpBroadcastDiscovery()` sends UDP broadcast datagrams to `255.255.255.255:32147` and all network interface broadcast addresses (`InterfaceAddress.getBroadcast()`).
    - Collects responses in `ConcurrentHashMap<String, DiscoveredInstance> discoveredInstances` and automatically updates `renderDiscoveredInstances()`.
    - **Premium Obsidian Dark Mode Wizard (`#0A0D14`)**:
      - Edge-to-edge dark theme (`Theme.DeviceDefault.NoActionBar`) with custom branding header (`⚡ LocalCode Remote`).
      - Glowing status badge with radar discovery indicator.
      - **3-Tab Wizard Controller**:
        - `📡 Auto-Suche` / `📡 Auto-Find`: Dynamic obsidian cards for discovered instances + 1-Click Connect + Empty-state guide with restart button.
        - `📷 QR-Scan`: Clear 3-step PC setup instructions + high-contrast purple camera scan button.
        - `⌨️ Manuell` / `⌨️ Manual`: Dark input fields for URL and TLS SHA-256 fingerprint + cyan connect button.
      - Discovered instances are displayed as obsidian cards with rounded corners (`#141A28`), host icon, version tag, URLs, and active running project indicators.
      - Added high-contrast `"⚡ 1-Klick Verbinden"` / `"⚡ 1-Click Connect"` action button for instant one-touch connection without manual URL/PIN entry.
- **Concurrent Multi-Project Execution (Codex-Style) (`src/agent.go`, `src/types.go`, `src/server.go`, `src/remote_server.go`)**:
  - Upgraded execution architecture to support multiple independent agent runs simultaneously across different projects/threads.
  - Replaced single global run locks with thread- and project-level tracking:
    - Added `ActiveAgentRun` model, `ActiveRuns map[string]*ActiveAgentRun`, `RunningProjects map[string]int` on `AppState`.
    - Added `RegisterActiveRun`, `UnregisterActiveRun`, `IsProjectRunning`, `GetRunningProjects`, `IsThreadRunning`, and `GetActiveRunForThread`.
    - Thread-level isolation in `StartAgentForThread`, `StopAgentForThread`, `ForceStopAgent`, and `finishAgentRun`.
    - Server API `/api/status`, `/api/snapshot`, `/remote/api/status`, and `/remote/api/snapshot` expose `running_projects` map and active run metadata.
- **Green Pulsating Active Project Indicator (`src/static/index.html`, `src/static/remote.html`)**:
  - Added `@keyframes projectPulseDot` and `.project-pulse-dot` CSS:
    - Radiant green dot (`#22c55e`) pulsing smoothly with scaling (0.85x to 1.2x) and glowing halo shadow (`rgba(34, 197, 94, 0.8)`).
  - **Desktop UI Integration (`src/static/index.html`)**:
    - Sidebar Project Tree (`#projectTree`): Renders `<span class="project-pulse-dot" title="..."></span>` immediately beside the project name for every active project.
    - Header Subtitle (`#headerSub`): Displays pulsating green indicator when current project is running.
    - Composer Project Selector (`#projectSelect`): Prepends `🟢 ` indicator before running project options.
  - **Mobile / Remote UI Integration (`src/static/remote.html`)**:
    - Project Drawer (`#drawerProjectsList`): Shows green pulse dot next to active project names.
    - Mobile Task List & Project Tools: Badges running projects with glowing indicator and localized "Arbeitet..." / "Working..." status.
- **Bilingual Maintenance & Test Verification**:
  - Preserved 100% key-identical DE/EN catalog parity in `src/static/i18n_base.js` (`TestTranslationCatalogsHaveIdenticalKeys` passed).
  - Added unit & integration tests in `src/remote_discovery_test.go` and `src/agent_multi_project_test.go`.
  - Full Go test suite passed (`go test -count=1 ./...` 100% green across all packages).
  - Rebuilt Windows binaries (`dist/LocalCode.exe`, `dist/LocalCode-Debug.exe`) with `scripts/build.ps1`.
  - Rebuilt Android APK (`dist/LocalCode-Remote-debug.apk`) with `scripts/build-android.ps1`, installed and verified on Samsung Galaxy SM-S931B via ADB.

## Animated Breathing Progress & "Arbeite..." Activity Indicator — 2026-09-21

- **User Request**:
  - `"progress.. arbeite soll animiert sein solange gearbeitet wird. soll heller und dunkler werden"`
- **Implementation (`src/static/index.html`, `src/static/remote.html`)**:
  - **Breathing Brightness & Glow Keyframe Animations (`workBreathing`, `workIconGlow`, `workDotPulse`)**:
    - Smooth 1.8s cyclical ease-in-out breathing animation oscillating from lower opacity (`opacity: 0.42; filter: brightness(0.75)`) to radiant brightness (`opacity: 1; filter: brightness(1.3); text-shadow: 0 0 10px rgba(255,255,255,0.5), 0 0 18px rgba(56,189,248,0.35)`).
    - Dot and icon pulsing: scale oscillation (0.88x to 1.15x) with glowing accent box shadows.
  - **Desktop UI & Deduplication (`src/static/index.html`)**:
    - Applied `.workBreathing` and glowing icon animations to `#runPhase` ("Arbeite...") and `.run-dot` in the top header run control.
    - Updated `renderChat()` to dynamically mark the active in-progress activity row with `.active-running` while `state.running` is true, animating `.activity-title`, `.activity-copy`, and `.activity-icon`.
    - **Activity Row Deduplication**: Filtered redundant `status` startup events (`Arbeite...` / `Bereit`) and collapsed consecutive identical activity rows (`lastAct === key`) so only a single animated `Arbeite... · <model>` row renders without duplicate entries.
  - **Mobile / Android UI (`src/static/remote.html`)**:
    - Integrated `@keyframes workBreathing` and `@keyframes workDotPulse` in mobile stylesheet.
    - Updated `eventHTML()` and `renderEvents()` to filter redundant `status` events, collapse duplicate consecutive steps, and tag active running tool-steps and progress events with `.active-running` / `.breathing-text` while execution is live.
- **Verification & Deployment**:
  - `go test -count=1 ./...` passed across all packages.
  - Rebuilt Android APK (`dist/LocalCode-Remote-debug.apk`) and re-installed via ADB to connected Samsung Galaxy SM-S931B.
  - Rebuilt Windows desktop binary (`dist/LocalCode.exe`) with `scripts/build.ps1`.

## Automatic Remote Pairing & 1-Click Desktop Push Approval — 2026-09-21

- **User Request**:
  - `"kann das koppeln auch automatisch funktionieren wenn nur eine instanz zu finden ist"`
- **Architecture & Workflow**:
  1. **Direct Auto-Pairing Option (`RemoteAutoPair bool` / `remote_auto_pair`)**:
     - Configurable toggle in Desktop Settings -> Connections -> Remote (`#setRemoteAutoPair`).
     - When enabled, discovery of a single local instance directly issues a token without manual PIN or approval prompt.
  2. **1-Click Push-Approval on Desktop (Safe Default)**:
     - When direct auto-pairing is disabled (`remote_auto_pair: false`), the mobile client sends an auto-pairing request `POST /remote/api/pair` (`{auto: true, device_name: "..."}`).
     - Server creates a pending pairing with a 60-second TTL and broadcasts a real-time `remote_pairing_request` event (`AppState.Broadcast`).
     - A desktop modal popup appears: *"📱 Neues Gerät möchte sich verbinden: <DeviceName> (<ClientIP>) [Zulassen] [Ablehnen]"*.
     - The mobile client polls `GET /remote/api/pair-status?request_id=...` (with 25s server-side channel wait on `pending.DoneCh`).
     - Clicking "Zulassen" on desktop calls `POST /api/remote/pairing/decision` (`{request_id: "...", approve: true}`), generating and assigning the authentication token to the device and returning it immediately to the mobile client.
  3. **Android Native Hardware Bridge**:
     - Added `getDeviceName()` to `@JavascriptInterface AndroidBridge` returning `Build.MANUFACTURER + " " + Build.MODEL` (e.g. `samsung SM-S931B`). Bumped bridge version to `2.2`.
  4. **Fallbacks**:
     - Standard 8-digit numeric PIN entry and QR code scanning remain 100% operational as manual fallbacks.
- **Modified & Added Components**:
  - `src/types.go`: Added `RemoteAutoPair bool` to `Config`, `PendingRemotePairing` struct, and `PendingRemotePairings map[string]*PendingRemotePairing` on `AppState`.
  - `src/remote_server.go`: Added `GET /remote/api/pair-status`, extended `handlePair`, added `DirectPairRemoteDevice`, `CreatePendingRemotePairing`, `GetPendingRemotePairing`, `DecidePendingRemotePairing`, and desktop handlers `handleRemotePairingDecision` and `handleRemotePairingPending`.
  - `src/server.go`: Registered desktop API routes `/api/remote/pairing/decision` and `/api/remote/pairing/pending`.
  - `src/static/i18n_base.js`: Updated bilingual dictionary with 17 new keys across German and English. Parity verified via `TestTranslationCatalogsHaveIdenticalKeys`.
  - `src/static/index.html`: Added `#setRemoteAutoPair` toggle, `showRemotePairingRequestModal(req)`, and wired real-time event listener + pending checks.
  - `src/static/remote.html`: Added auto-pairing flow on startup (`tryAutoPair`), approval wait spinner/modal, and updated pairing status feedback.
  - `android/app/src/main/java/com/inetconnector/localcode/MainActivity.java`: Added `getDeviceName()` to `AndroidBridge`.
  - `src/remote_server_test.go`: Added `TestRemoteAutoPairDirect`, `TestRemoteAutoPairPendingApproval`, and `TestRemoteAutoPairPendingRejection`.
- **Verification & Hardware Testing**:
  - `go test -count=1 ./...` passed across all 8 packages (localcode, benchharness, cmd/localcode-bench, cmd/localcode-setup, etc.).
  - `go vet ./...` and `go fmt ./...` clean.
  - Built updated Android APK (`scripts/build-android.ps1`), deployed to Samsung Galaxy S25 / SM-S931B (`192.168.1.73:39053`) via ADB.
  - Real-device hardware verification: Samsung phone launched, broadcast auto-pair request with device name `"samsung SM-S931B"`, received approval decision from desktop, and authenticated immediately to `/remote/api/status`.
  - Built Windows GUI and Debug executables (`scripts/build.ps1`). Executable: `dist/LocalCode.exe`.

## Prompt Enter Submission & Single-Line "Arbeite..." Activity Stream — 2026-09-20

- **User Requests**:
  1. `"auf arbeite... ändern. und dann auch immer schreiben was gerade gemacht wird in derselben zeile."`
  2. `"im eingabefenster soll enter die eingabe abschicken"`
- **Prompt Keyboard Submission (`src/static/index.html`, `src/static/remote.html`)**:
  - Bound Enter keydown in the prompt composer (`#prompt`) across Desktop UI (`src/static/index.html`) and Mobile/Remote UI (`src/static/remote.html`) supporting standard Enter, NumpadEnter, and keycode 13 while preserving Shift+Enter for multiline input.
  - Enhanced project selection fallback in `send()`: resolves `$('#projectSelect')?.value || state.projects[0]?.path` immediately when starting a new session before project selection is finalized.
- **Activity & Step Display Unification (`src/agent.go`, `src/static/index.html`, `src/static/i18n_base.js`, `src/core_test.go`)**:
  - Replaced `"Modellschritt %d von %d"` and `"Agent arbeitet"` / `"Agent setzt Aufgabe fort"` with localized `"Arbeite..."` (de) / `"Working..."` (en) in `src/agent.go` (lines 622, 737, 826) and UI clock/phase indicator.
  - Implemented `defaultActionDescription(action, cfg)` in `src/agent.go` to provide human-readable inline action strings when models execute tools without an explicit message (e.g. `Datei lesen: <path>`, `Quellcode bearbeiten: <path>`, `Befehl ausführen: <cmd>`, `Git: <args>`).
  - Restructured `.activity-row`, `.activity-copy`, `.activity-title`, and `.activity-sub` in `src/static/index.html` as a single horizontal inline flex layout (`display: flex; align-items: center; gap: 4px; flex-wrap: wrap; font-size: 12px; line-height: 1.4`). Activity messages and details render side-by-side in the same line (`<span class="activity-title">${escapeHTML(msg)}</span><span class="activity-sub"> · ${escapeHTML(detail)}</span>`).
  - Maintained exact German and English dictionary parity in `src/static/i18n_base.js` with `"Arbeite...": "Arbeite..."` (de) and `"Arbeite...": "Working..."` (en).
- **Local Installation & Live Runtime**:
  - Rebuilt Windows setup package (`LocalCode-Setup.exe`).
  - Installed updated build 6.9.3 to `%LOCALAPPDATA%\Programs\LocalCode`. Executable SHA-256 matches `0B0CF0231A4A71281CBC816FA4A643FC36A4EC6D1D8F300A2F42FAA4853E2285`.
  - Prior binary backed up to `%LOCALAPPDATA%\LocalCode\install-backups\20260920\LocalCode-pre-enter.exe`.
  - Started LocalCode process with `/tray`; verified live response from `http://127.0.0.1:32145/api/ping` (`version: 6.9.3`, capabilities: `stop-task-v1`, `steering-v1`).

## Windows Maximized App Windows & Cross-Platform Stubs — 2026-09-20

- **Maximized App Window Launch Policy (`src/app_window_windows.go`, `src/app_window_windows_test.go`, `src/platform_windows.go`, `src/startup_splash.go`)**:
  - Implemented single Win32 enumeration callback and `maximizeLocalCodeAppWindows` in `src/app_window_windows.go` with scoping (`isLocalCodeAppWindow`) matching title `"LocalCode"`, class `"Chrome_WidgetWin_1"`, and clean browser image path.
  - Unified Chromium `--start-maximized` launch policy across bootstrap splash and main UI.
  - Unit tests verified in `src/app_window_windows_test.go`.
- **Cross-Platform Build Tag & Stub Hardening (`src/desktop_automation_other.go`, `src/tray_other.go`, `src/tray_windows_test.go`, `src/cmd/localcode-setup/main.go`, `src/cmd/localcode-setup/setup_test.go`, `src/core_test.go`)**:
  - Added `isBlockedDesktopWindow` and `escapePowerShellString` stubs in `src/desktop_automation_other.go` for non-Windows platforms.
  - Added `openBrowserMaximizedHook` and `exitAppHook` to `src/tray_other.go` with graceful `os.Exit(0)` handling matching Windows semantics.
  - Moved Windows-only tray tests (`TestNotifyIconDataStructSize`, `TestLoadAppIcon`, `TestTrayWndProcMock`, `TestTrayManagerRunLifecycle`) into `src/tray_windows_test.go` with `//go:build windows`, leaving `src/tray_test.go` completely portable.
  - Added `writeWindowsCmdFixture` helper to `src/core_test.go` for cross-platform ADB/SDK test compatibility.
  - Added `//go:build windows` to `src/cmd/localcode-setup/main.go` and `src/cmd/localcode-setup/setup_test.go`.
- **Full Verification Suite Passed**:
  - `go vet ./...` passed 100% clean across `GOOS=windows`, `GOOS=linux`, and `GOOS=darwin`.
  - `go test -count=1 ./...` passed across all 8 packages (localcode 119.232s, benchharness 2.775s, cmd/localcode-bench 2.235s, cmd/localcode-setup 9.358s).
  - Browser UI E2E smoke test passed (`python scripts/ui-e2e-test.py`, `FULL UI E2E OK 43 requests`).
  - Full native build passed (`scripts/build.ps1`, isolated + shuffled test runs + Windows GUI + Debug executables).
  - Executable SHA-256 digests:
    - `LocalCode.exe`: `49509F0F26ADE13793C4543C0D9614CDFE384B161BD66DCDF5BDBF635127C838`
    - `LocalCode-Debug.exe`: `B50DBE52038444F647561B9A2DCF000B1FD6815D746625117317D1E39AB6DC37`

## Desktop validation and race compiler fix / Desktop-Validierung und Race-Compiler — 2026-09-20

- Fixed the actual missing-target validation in `src/desktop_automation_windows.go`: trim both window/control names and reject empty or whitespace-only targets before PowerShell/UI Automation. Removed the empty-control wildcard; existing approval, cancellation and timeout paths remain intact. Validation errors use the existing DE/EN locale helper. The existing failing test was not weakened.
- Added Windows regression test `src/desktop_type_validation_windows_test.go`: both languages, empty/whitespace title or control, cancelled context proving validation occurs before command execution without touching real desktop windows. Focused tests passed with race detection.
- Installed official w64devkit 2.10.0 / GCC 16.2.0 at `%LOCALAPPDATA%\Programs\w64devkit-2.10.0\w64devkit`; appended its `bin` directory to user PATH. Archive SHA-256 verified against GitHub asset digest: `18D0A4C71A166F8401AB6305781BEC5882B40B5E06BA9807C61CB5F3B3C6325E`. GCC version and `libsynchronization.a` resolution verified. Existing terminals need a fresh PATH/new session; test subprocesses explicitly received the compiler path and `CGO_ENABLED=1`.
- Passed full `go test -race -count=1 ./...` across all eight packages (localcode 245.307s), ordinary suite (203.733s), go fmt, go vet, browser UI smoke (44 requests), static JavaScript syntax and extension DE/EN catalog parity. Logs: `logs/fix-20260920-race.log`, `logs/fix-20260920-build.log`, `logs/fix-20260920-ui.log`.
- Standard build completed successfully, including the shuffled suite (localcode 182.267s), Windows amd64 GUI and diagnostics binaries. Installed corrected 6.9.3 using native setup (exit 0); installed/build EXE SHA-256 matches `8EA076418D8F62869A01EEE241E2771A7EEC68FABF8626DCA93AECA09DDFF93F`. Restarted installed app with `/tray` and verified `/api/ping` reports 6.9.3. CHECKSUMS-SHA256.txt and dist/build-state.json now describe this successful build. Android and IDE packages required no change.
- Both reported blockers are resolved and removed from TODO.md. Preserve all earlier uncommitted UI/test/doc changes. Direct patches were the explicitly selected fallback because no callable aider_edit was available. No PR, commit, CI run or public release was created for this fix. Remaining work: the authenticated registry listings in TODO.md.

## Earlier local installation / Frühere lokale Installation — 2026-09-20

- Installed current working-tree Windows build 6.9.3 including the existing September 19 UI changes at `%LOCALAPPDATA%\Programs\LocalCode`. Setup exit 0. Built/installed EXE SHA-256: `0661CBB6C2155915925363E22B97F7D5434210E600C5CECF7978B62162558916`. Started with `/tray`; `/api/ping` returned 6.9.3, `stop-task-v1`, and `steering-v1`.
- Bisherige Windows-EXE gesichert unter `%LOCALAPPDATA%\LocalCode\install-backups\20260920\LocalCode.exe`; keine App-Daten gelöscht.
- Samsung SM-S931B updated with `adb install -r` using the v6.9.3 release APK; activity launch and process verified. Android manifest still reports versionName **1.0**, versionCode **1**. Installed APK hash verified on device against release: `0BA9CE4D0C53F54FA32CED4496E3A4D40FA8DBAC73751EE453A23E73B6E6E03F`. App data preserved.
- Antigravity IDE: CLI successfully installed and listed `inetconnector.localcode@0.1.0`. Existing IDE windows may need reload. No new release or registry publication occurred.
- Passed: go fmt, go vet, static JavaScript syntax, extension syntax/DE-EN catalog parity, browser UI smoke (`FULL UI E2E OK 43 requests`), native installer build, both Windows amd64 executable builds. Logs: `logs/install-20260920-*.log`.
- Ordinary full Go suite FAILED after 194.002s: `TestCoverageBoostDesktopAutomation`, `coverage_boost_pairing_test.go:333`, expected error for empty desktop_type control name. Standard build stopped before its shuffled tests/build stages. Explicit vet and Windows builds then passed separately; do not describe the full standard build/test suite as green.
- Race suite attempted with CGO enabled but compilation FAILED: gcc not in PATH. Discovery checked PATH, C:\msys64, .tools, LocalAppData Programs, Chocolatey, LLVM, Scoop, WinGet and common MinGW/tool directories; no gcc/clang found there. No compiler installed during this workstream.
- Preserved pre-existing edits in STATE.md, instruction_context_test.go, static/index.html, static/mission_status.js and static/ui_polish.js. No source changes for installation. README/TODO/checksums record this state. `dist/build-state.json` still describes the earlier successful standard build; use the verified hashes for these newer binaries.
- The test/compiler failures above describe the earlier installation attempt; the subsequent fix and fresh results are recorded in the preceding section. Remaining work is tracked in TODO.md.

## Active UI Redesign, Obsidian Palette & Combobox Overflow Fix workstream — 2026-09-19

User request: "das soll nicht so scheisse aussehen da sind comboboxen die rauslaufen schau ins andere Bild die schriftgrösse ist zu gross und es muss edler und bersichtlicher aussehen aber schon so bleiben wie es eigentlich ist"

Implemented in this workstream:

- **Combobox & Composer Toolbar Refinement (`src/static/index.html`, `src/static/ui_polish.js`)**:
  - Eliminated horizontal layout overflow in the bottom composer dock by applying flexible width constraints (`max-width`, `min-width`, `flex: 0 1 auto`, `text-overflow: ellipsis`, `overflow: hidden`, `white-space: nowrap`) across `#approvalQuick`, `#projectSelect`, `#engineSelect`, and `#modelSelect`.
  - Replaced native bulky OS select chevrons with a sleek custom SVG down-arrow background (`appearance: none; -webkit-appearance: none;`).
  - Added streamlined button dimensions for `#attachBtn`, `#voiceBtn` (28x28px circular pills), and `#sendBtn` (30x30px high-contrast round action button).
  - Ensured `.composer` is bounded with `border-radius: 16px`, deep obsidian background (`#171a1f`), subtle border (`rgba(255,255,255,0.09)`), and elegant focus/shadow states that never bleed into sidebars or adjacent panels.
- **High-Density & Noble Typography Scaling (`src/static/index.html`, `src/static/ui_polish.js`, `src/static/mission_status.js`)**:
  - Scaled base UI font from 14px down to 13px (`font: 13px/1.45 var(--ui-font)`) matching modern AI development interfaces (Codex, Antigravity IDE, Cursor).
  - Scaled hero greeting title from 30px down to 22px (`font-size: 22px; font-weight: 600; letter-spacing: -0.015em;`), hero subtitle to 12.5px, and suggestion chips to compact pills (12px, 6px 14px padding).
  - Compacted header height from 56px to 44px with breadcrumb styling (13px title, 11px subtitle).
  - Compacted sidebar tree rows, nav rows, search box (28px), and right inspector tab bar (42px) with 10.5-11.5px metadata typography.
  - Aligned `.mission-status-card` and `.orchestration-diagnostics-card` styling in `mission_status.js` to deep obsidian dark (`#16191e` with `rgba(255,255,255,0.07)` border and compact grid metadata).
- **Test Isolation & Verification (`src/instruction_context_test.go`)**:
  - Added hermetic `USERPROFILE` and `HOME` environment isolation in `TestProjectInstructionContextLoadsGlobalProjectRulesAndSkills` and `TestProjectInstructionContextFallsBackWhenNoDocsExist` to prevent host IDE builtin skills from polluting isolated test runs.
  - Verified 100% green pass on the full Go test suite (`go test -count=1 ./...`, localcode 120.464s).
  - Verified 100% green pass on the Playwright UI E2E suite (`python scripts/ui-e2e-test.py`, 43 requests).

## Active Progressive Skill Discovery, Negative Knowledge & Quality Gate workstream — 2026-09-17

User request: Thoroughly analyze all documents in `C:\Users\frede\Desktop\pipeline` and implement high-leverage architectural refinements into LocalCode.

Implemented in this workstream:

- **Progressive Skill Routing & Context Budget Management (`src/instruction_context.go`, `src/instruction_context_test.go`)**:
  - Expanded `availableSkillRoots` to discover skills from `.agents/skills`, `.localcode/skills`, `.gemini/config/skills`, and Antigravity IDE builtin paths alongside existing `.codex` and `.cursor` roots.
  - Enhanced frontmatter parsing in `localSkillSummaries` for `domains`, `side_effect_level`, `activation`, and `negative_triggers` / `negative_activation`.
  - Added negative trigger matching (`negativeActivationMatchesTask`): suppresses heavy or conflicting skills when matching negative triggers (e.g., `quick_fix`, `no_ui`, `dry_run`).
  - Implemented Context Budget Guard: limits auto-embedded full-text skills to the 2 highest-priority matching skills to preserve context window and VRAM on local models, leaving remaining skills available in the compact index for on-demand `skill_read`.
- **Negative Rules & Procedural Knowledge Store (`src/agent_mission_knowledge.go`, `src/agent_mission_knowledge_test.go`)**:
  - Added `MissionKnowledgeCategoryNegativeRule` (`"negative_rule"`) for persistent "Do Not" project constraints and `MissionKnowledgeCategoryProcedural` (`"procedural_workflow"`) for standard development workflows.
  - Prominent prompt injection: `[CRITICAL CONSTRAINT]` items rendered at the very top of prompt context under `## ⚠️ Critical Constraints & Negative Rules`.
  - Full persistence support under `%LOCALAPPDATA%\LocalCode\knowledge\<hash>.json` with 128 KiB cap, secret redaction, and atomic writes.
- **Tool Side-Effect Taxonomy & Adversarial Self-Review Protocol (`src/agent.go`)**:
  - Formalized Tool Side-Effect hierarchy (`ReadOnly`, `SafeWrite`, `Mutating`, `SystemExecution`) adhering to the *Principle of Least Privilege*.
  - Added mandatory 4-step Pre-Completion Quality Gate (Adversarial Self-Review before `finish`):
    1. Acceptance Verification (explicit user criteria)
    2. Constraint & Negative Rule Check (`[CRITICAL CONSTRAINT]`)
    3. Clean Diff, No Placeholders, and Verified Automated Tests
    4. Truthful Reporting (report strictly verified actions).
- **Verification & Testing**:
  - All unit and integration tests passed (`go test -race -count=1 ./...`).
  - Full build suite passed (`scripts/build.ps1`, isolated + shuffled passes, amd64 binaries).
  - Browser UI smoke test passed (`python scripts/ui-e2e-test.py`, `FULL UI E2E OK 43 requests`).
  - JavaScript syntax checks passed with Node.js.

## Active System Tray (`/tray`), Startup Flicker Fix & Windows Setups workstream — 2026-09-15

User request: Start LocalCode with `/tray` in the Windows system tray with application icon, bilingual context menu (*Öffnen* / *Beenden*), and double-click to open UI maximized; eliminate top-left white rectangular flash on Windows startup; rebuild all binaries (`LocalCode-Android.apk`, `LocalCode-Remote-debug.apk`, `LocalCode.exe`, `LocalCode-Setup.exe`, `localcode-0.1.0.vsix`), commit and push, and make binaries live on GitHub Releases.

Implemented in this workstream:

- **Windows System Tray (`/tray`) Architecture (`src/tray_windows.go`, `src/tray_other.go`, `src/tray_test.go`)**:
  - Implemented pure Win32 system tray manager (`TrayManager`) using `syscall.NewLazyDLL` ("user32.dll", "shell32.dll", "kernel32.dll") and `syscall.NewCallback`.
  - Created message-only host window (`LocalCodeTrayWindowClass` with `HWND_MESSAGE`) with `Shell_NotifyIconW` integration (`NIM_ADD`, `NIM_MODIFY`, `NIM_DELETE`).
  - Embedded / staged icon extraction via `ExtractIconExW`, `LoadImageW` (`localcode.ico`), and standard application icon fallback.
  - Double-click on tray icon (`WM_LBUTTONDBLCLK`) triggers `openBrowserMaximized(url)` with Chromium `--start-maximized` or system default browser.
  - Right-click on tray icon (`WM_RBUTTONUP` / `WM_CONTEXTMENU`) renders native popup menu with **Öffnen** (bold/default) and **Beenden** with 100% bilingual DE/EN localization.
  - "Beenden" cleanly removes tray icon (`NIM_DELETE`), destroys window, and gracefully stops the application.
- **Top-Left White Rectangular Startup Flash Elimination (`src/tray_windows.go`, `src/platform_windows.go`)**:
  - Replaced top-level desktop window creation with `HWND_MESSAGE` (`(HWND)-3`), creating a pure message-only window that Desktop Window Manager (DWM) does not visually rasterize.
  - Added `--force-dark-mode` and `--enable-features=WebContentsForceDark` in Chromium app launcher (`openChromiumApp`) to prevent white canvas repaint flashes prior to CSS initialization.
- **CLI Flag Integration & Background Startup (`src/main.go`)**:
  - Recognizes `/tray`, `-tray`, and `--tray` (case-insensitive).
  - When started with `/tray`, bypasses opening an initial browser window and runs silently in the Windows system tray.
  - When already running, invoking with `/tray` avoids duplicate browser window spawning.
- **Installer & Setup Shortcuts (`src/cmd/localcode-setup/main.go`, `installer/localcode-setup.iss`, `src/windows_packaging_test.go`)**:
  - Added dedicated *"LocalCode (System Tray)"* Start Menu shortcut with the `/tray` argument in Go native installer and Inno Setup script.
  - Updated `README.md` documentation in both German and English.

## Active ComputeMesh zero-fee local integration & Go toolchain workstream — 2026-09-14

User request: Verify if localcode is up to date with git remote, fix Go toolchain error in notification, thoroughly inspect and implement optimal integration with ComputeMesh to auto-discover and utilize the free local mesh node (0% platform fee, zero latency, local self-compute), update all installers, ensure local machine installation is current, and verify all tests, tags, and commits.

Implemented in this workstream:

- **ComputeMesh Zero-Fee Local Workstation Integration & Direct Node Prioritization**:
  - Implemented `ProbeRunningLocalComputeMeshNodeDetailed(ctx, candidates...)` in `src/computemesh.go` querying `http://127.0.0.1:8080/api/status` and `http://localhost:8080/api/status`.
  - Automatically parses live hardware telemetry (`NVIDIA GeForce RTX 3080 Laptop GPU, 16384 MiB`), Node ID, Provider Account, and Active Models directly from the running local workstation daemon.
  - Automatically prioritizes `DirectLocal = true` for 0% platform fee, zero latency, and private GPU self-compute without remote routing.
  - Enhanced gateway failover logic: if remote cluster gateway is unreachable or responds with error, seamlessly falls back to the active local mesh node and Ollama.
  - Updated `Doctor` (Diagnostic item 1 in `src/doctor.go`) to distinguish between direct local node (0% fee) and remote cluster gateway.
  - Updated `src/static/computemesh.js` UI status card and tooltips with 100% key-identical bilingual DE/EN dictionaries.
  - Hermetic Ollama client isolation in `src/ollama.go` preventing background daemons from polluting isolated mock test servers.
- **Go Toolchain & Environment Optimization**:
  - Configured `.vscode/settings.json` specifying `go.goroot`, `go.gopath`, and `go.alternateTools.go` targeting `C:\Users\frede\AppData\Local\Programs\GoToolchains\go1.26.6\go\bin\go.exe`.
  - Cleaned test temp pollution from Windows User PATH registry.
  - Hardened `localcode-setup` (`src/cmd/localcode-setup/main.go` and `setup_test.go`) with execution timeouts (30s), `-ExecutionPolicy Bypass`, and `t.Cleanup` registry cleanup.
- **Code Graph Relations & Polyglot Symbol Extraction**:
  - Updated `repoIntelSymbolPatterns` in `src/repo_intelligence.go` to match TypeScript/JavaScript `interface`, `type`, and `enum` definitions alongside classes.
- **Installer & Local Deployment Verification**:
  - Hardened `scripts/build-installer.ps1` with automated toolchain discovery and graceful fallback for `.syso` resource compilation.
  - Full `scripts/build.ps1` suite passed (formatting, isolated tests, vet, shuffle tests, GUI/debug binary compilation).
  - Built `dist/LocalCode-Setup.exe` and performed silent machine update via `dist/LocalCode-Setup.exe --silent` to `%LOCALAPPDATA%\Programs\LocalCode`.
  - Verified system diagnostics with `LocalCode-Debug.exe --diagnose`, JS syntax with `node --check`, and browser UI E2E suite with `python scripts/ui-e2e-test.py` (`FULL UI E2E OK 43 requests`).

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
