# LocalCode 6.9.0

[Deutsch](#deutsch) · [English](#english) · [Releases & Downloads](https://github.com/inetconnector/LocalCode/releases)

LocalCode is a Windows-first, local-first coding-agent application centered on Ollama and controlled tool execution. It combines project/task management, a native coding-agent runtime, selectable external coding engines, autonomous browser and Windows desktop GUI automation, Git/build tooling, MCP, web research, attachments, approvals, durable recovery, a Desktop UI and a mobile Android Remote with TTS audio feedback. LocalCode is an independent project and is not OpenAI Codex.

[![GitHub Releases](https://img.shields.io/github/v/release/inetconnector/LocalCode?include_prereleases&label=Latest%20Release)](https://github.com/inetconnector/LocalCode/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

---

## Deutsch

### Installation & Download

Die aktuellen vorkompilierten Binärdateien und Installer stehen unter **[GitHub Releases](https://github.com/inetconnector/LocalCode/releases)** zum Download bereit:

- **Windows Setup-Installer:** `LocalCode-Setup.exe` (in jedem Release-Asset oder über `INSTALL.bat`)
- **Windows Portables Paket:** `LocalCode.exe` & `LocalCode-Debug.exe`
- **Android Mobile Remote App:** `LocalCode-Remote-debug.apk`

#### Option 1: Windows Setup-Installer (Empfohlen)

1. Lade `LocalCode-Setup.exe` von den [GitHub Releases](https://github.com/inetconnector/LocalCode/releases) herunter (oder führe `INSTALL.bat` im Repository aus).
2. Starte den Installer. LocalCode wird standardmäßig in `%LOCALAPPDATA%\Programs\LocalCode` installiert (keine Administratorrechte erforderlich).
3. Der Installer erstellt automatisch Verknüpfungen im **Startmenü** und auf dem **Desktop**, trägt `localcode` in den Benutzer-`PATH` ein und registriert sich sauber in den Windows-Einstellungen (*Installierte Apps / Apps & Features*).
4. **Befehlszeilen-Optionen:**
   ```powershell
   # Stille / unbeaufsichtigte Installation und anschließender Start:
   .\LocalCode-Setup.exe --silent --launch

   # Benutzerdefiniertes Installationsverzeichnis:
   .\LocalCode-Setup.exe --dir "C:\Tools\LocalCode"

   # Deinstallation:
   .\LocalCode-Setup.exe --uninstall
   ```

#### Option 2: Portabler Schnellstart

1. Repository klonen oder Release-ZIP entpacken.
2. `START.bat` oder `FAST-START.bat` starten. Der Start nutzt einen schnellen Pfad und öffnet die Desktop-UI im Browser.
3. Falls Binärdateien fehlen, baut `BUILD.bat` die Anwendung automatisch lokal.

#### Option 3: Android Mobile Remote App

1. `dist\LocalCode-Remote-debug.apk` aus dem Release auf das Smartphone laden oder über ADB installieren:
   ```powershell
   adb install -r dist\LocalCode-Remote-debug.apk
   ```
2. App auf dem Smartphone öffnen und den auf dem PC unter *Hilfe → Remote koppeln* angezeigten QR-Code scannen.
3. Bietet vollständige Steuerung, Audio-TTS-Sprachausgabe (`TextToSpeech`), Aufgabenstarter und Genehmigungs-Popups im privaten WLAN.

### Schnellstart & Erste Schritte

1. Repository bzw. ZIP vollständig in einen Ordner legen oder den Installer nutzen.
2. `START.bat` starten. Der normale Start verwendet einen schnellen, geloggten Pfad und öffnet die UI ohne blockierende Runtime-Dialoge oder automatische Firewall-UAC-Abfrage.
3. Falls der native Windows-Build fehlt oder veraltet ist, delegiert `START.bat` einmalig an `BUILD.bat`; danach nutzt die Frischeprüfung einen Build-State-/Git-Fingerprint.
4. Ollama, Modelle und Coding-Agent-Engines werden im laufenden Produkt über Status/Doctor und bei erster Nutzung geprüft; vollständige Diagnose bleibt über `DIAGNOSE.bat` verfügbar.
5. Für dauerhaften Handy-Remote-Zugriff im privaten LAN kann einmalig `scripts\install-remote-firewall-rule.ps1` als Administrator ausgeführt werden. `START.bat` löst diese Rechteabfrage nicht automatisch aus.
6. Frische Installationen verwenden standardmäßig `qwen2.5-coder:14b` und **LocalCode Native**.

Startprotokolle liegen in `logs/start.log`; Runtime-Details liegen im LocalCode-Log unter `%LOCALAPPDATA%\LocalCode\localcode.log`. Große Modelldownloads können beim ersten tatsächlichen Setup oder bei erster Nutzung dauern.


### Sprache

- `Automatisch (Windows)` folgt der Windows-Anzeigesprache: Deutsch auf deutschem Windows, sonst Englisch.
- Deutsch und Englisch können manuell gewählt werden.
- Sichtbare Produkttexte und zentrale Dokumentation werden DE/EN synchron gehalten.

### Coding-Agent-Engines

LocalCode kann zwischen **LocalCode Native**, **Aider**, **Claude Code**, **OpenCode** und dem experimentellen **Claw Code** umschalten. Die Engine kann in der Eingabeleiste und in den Einstellungen gewählt werden. Es gibt keine stille Provider- oder Modellumschaltung.

### Projekte, Aufgaben und Werkzeuge

LocalCode verwaltet Projekte und persistente Aufgaben/Threads. Der Native-Agent kann – abhängig von Konfiguration und Genehmigung – Dateien lesen und ändern, Git verwenden, Builds/Tests starten, lokale Werkzeuge erkennen, MCP anbinden, öffentliche Webrecherche durchführen, Attachments verarbeiten und lokale Bild-/Asset-Werkzeuge nutzen.

Wichtige Schutzmechanismen:

- kanonische Projekt-/Workspace-Pfadgrenzen einschließlich Symlink-/NTFS-Junction-Prüfung,
- genehmigungsgebundene Datei-, Befehls-, Netzwerk- und Installationsaktionen,
- SHA-/Versions-Preconditions und atomare konfliktbewusste Dateiänderungen,
- kontrollierter Prozessabbruch und Timeouts,
- dauerhafter Run-Journal für unterbrochene Läufe,
- keine automatische Rechteeskalation durch Prompt-, Regel-, Skill-, Memory- oder Planner-Text.

### Native Agent Teams – aktueller Stand

Aktuell ausführbare Child-Rollen sind ausschließlich **Explorer**, **Planner** und **Reviewer**. Sie besitzen getrennten Kontext, harte Modell-/Tool-/Zeit-/Tokenbudgets und nur read-only Aktionen: Projektbaum, Dateien, Textsuche, genehmigungsfreies LSP und strukturiertes `finish`. Schreiben, Shell, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Genehmigungen und rekursives Spawning fehlen bewusst aus ihrem Schema.

Ein deterministischer Task-DAG, ein begrenzter Scheduler/Resource Manager und ein expliziter read-only Mission-Einstieg sind implementiert. Der Scheduler führt autorisierte Explorer/Planner/Reviewer aus, sammelt strukturierte `AgentResult`-/Usage-Daten und schaltet Dependencies deterministisch frei. Mehrere Tasks können logisch bereit sein, während lokale Modellinferenz standardmäßig nur einen aktiven Slot besitzt. Der aktuelle Dispatcher ist synchron; höhere konfigurierte Model-Slot-Limits allein beweisen keine echte parallele Modellinferenz.

Cancel und Child-Abschluss sind race-sicher serialisiert. Cancellation-first verwirft verspätete Child-Ergebnisse; Completion-first bleibt terminal erfolgreich. Ein vollständiger Mission-Abbruch terminalisiert noch unfertige Tasks kontrolliert als `cancelled`.

### Mission-Status, Diagnostik und Benchmarks

Der Desktop zeigt read-only Mission-Status im Output-Inspector: Mission-State/-Reason, Queue/Running, Ressourcenklassen, Task-Zustände und Budget-Snapshots. `/api/status` liefert zusätzlich maschinenlesbare Orchestrierungsdiagnostik für Backend, Queue und Ressourcensättigung. Diese Beobachtung verändert keine Scheduler-Limits oder Capabilities.

Reproduzierbare Parallelitäts-Benchmarks trennen logische Bereitschaft, konfigurierte Slots und tatsächlich beobachtete Executor-/Ollama-Überlappung. Der reale Ollama-Benchmark ist opt-in, Loopback-only und startet oder lädt keine Modelle. Details: `docs/ORCHESTRATION_BENCHMARKS.md`.

### Durable Mission-Recovery und explizite Desktop-Fortsetzung

Read-only Missions besitzen dauerhafte, begrenzte Recovery-Metadaten im bestehenden `active-run.json`. `run_journal.go` bleibt die **einzige** dauerhafte Recovery-Autorität; es gibt kein zweites Mission-Journal.

Beim Missionsstart wird eine begrenzte Projekt-/Git-Baseline aus kanonischer Projektidentität, Git-Repository-Identität, exaktem `HEAD` und einem SHA-256-Fingerprint des Porcelain-Worktree-Status gespeichert. Rohe `git status`-Pfade werden nicht persistiert. Nach einem Prozessabbruch wird der Zustand frisch als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence` klassifiziert. Crash-running Arbeit ist niemals automatisch erfolgreich. Durable erfolgreiche Tasks benötigen passende Postcondition-Verifikation; Dependencies und Attempt-Grenzen werden vor einer Fortsetzung erneut geprüft.

LocalCode besitzt inzwischen eine **explizite Desktop-only Recovery-Steuerung**:

- `GET /api/mission-recovery` liefert ausschließlich einen begrenzten Inspektions-Datensatz mit Run/Mission-ID, Hash-Preconditions, Reconciliation und Task-Transitionen.
- `POST /api/mission-recovery/continue` akzeptiert genau einen aktuellen `resume_candidate` oder `retry_candidate` plus die zuvor inspizierten Stale-State-Preconditions.
- Die UI-Hashes sind keine Ausführungsberechtigung. Direkt vor Admission werden Projekt/Git, Transition-Plan, Capabilities, Modellidentität, Budgets und der aktuelle Journal-Fingerprint erneut vertrauenswürdig geprüft.
- `202 Accepted` wird erst nach erfolgreicher dauerhafter Reservation und AppState-Ownership zurückgegeben. Danach läuft die Fortsetzung unter dem bestehenden Scheduler und bleibt über `StopAgent` abbrechbar.
- Eine Reservation ist noch kein Attempt. `AttemptCount` steigt erst, wenn ein dauerhafter Scheduler-`Running`-Checkpoint tatsächlichen Start beweist.
- Historische Usage/Budgets bleiben kumulativ; Recovery darf keine frischen Limits minten. Crash-/Offline-Downtime zählt nicht als aktive Mission-Laufzeit.
- Startup bleibt passiv: **kein automatisches Resume, Retry oder Replay**.

Die Desktop-Karte **Unterbrochene Mission** zeigt nur aktuelle Resume-/Retry-Kandidaten und startet nie automatisch.

### Handy-Remote / Android

LocalCode betreibt optional einen getrennten, token-geschützten Remote-Server für das lokale Netzwerk. Über **Hilfe → Remote koppeln** wird ein kurzlebiger Pairing-Code bzw. Pair-/QR-Link erzeugt. Die Android-Hülle unterstützt gespeicherte Wiederverbindung, mDNS plus direkten LAN-Fallback-Scan, TLS-Fingerprint-Pinning, WebView, nativen Dateipicker und Android-Spracheingabe.

Mobile bleibt absichtlich schmaler als Desktop. Die Remote zeigt bei einer aktiven read-only Mission nur **Mission · Läuft** auf Basis der bereits authentifizierten Felder `running` und `run_phase`. Es gibt **keine** Remote-Recovery-Route, keine Mission-/Task-Recovery-IDs, keine Scheduler-/Budget-/Accounting-Daten und keine Mobile Resume-/Retry-Autorität. Das bestehende Remote-Stop-Verhalten bleibt unverändert.

### Noch nicht implementiert

- persistente, begrenzte **Mission Memory/Knowledge** mit eigener Privacy-/Retention-Grenze,
- mutation-capable Builder-Agenten in isolierten Git-Worktrees,
- Integrator-/Test-Agent-Mutationsfluss,
- eine automatische Recovery-Fortsetzung beim Start – diese bleibt bewusst verboten.

### Build und Qualität

Die GitHub-Quality-Pipeline prüft unter anderem:

- Go-Version / Setup,
- `gofmt`,
- `go vet ./...`,
- Frontend-JavaScript-Syntax,
- PowerShell-Syntax,
- native Android-Remote-APK,
- Vulnerability Scan,
- Full-Stack-Loopback-HTTP-Integration,
- vollständige Go-Tests,
- Race Detector,
- Statement Coverage (Gate mindestens 80 %),
- native Windows-Builds,
- `git diff --check`.

### Wichtige Dokumente

- `AGENTS.md` – verbindliche Repository-/Arbeitsregeln
- `STATE.md` – kanonischer aktueller Projektstand
- `TODO.md` – ausschließlich offene funktionale Arbeit
- `docs/ARCHITECTURE.md` – Architektur und Laufzeitgrenzen
- `docs/SECURITY.md` – Sicherheitsmodell
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproduzierbare Orchestrierungs-/Ollama-Benchmarks
- `android/README.md` – Android-Remote-Details

---

## English

### Installation & Downloads

Pre-built binaries and installers are available directly on **[GitHub Releases](https://github.com/inetconnector/LocalCode/releases)**:

- **Windows Setup Installer:** `LocalCode-Setup.exe` (available in release assets or via `INSTALL.bat`)
- **Windows Portable Executables:** `LocalCode.exe` & `LocalCode-Debug.exe`
- **Android Mobile Remote App:** `LocalCode-Remote-debug.apk`

#### Option 1: Windows Setup Installer (Recommended)

1. Download `LocalCode-Setup.exe` from [GitHub Releases](https://github.com/inetconnector/LocalCode/releases) (or execute `INSTALL.bat` in the repository).
2. Run the installer. LocalCode is installed to `%LOCALAPPDATA%\Programs\LocalCode` by default (no Administrator privileges required).
3. The installer automatically creates shortcuts in the **Start Menu** and on the **Desktop**, adds `localcode` to the User `PATH`, and registers cleanly in Windows Settings (*Installed Apps / Apps & Features*).
4. **Command Line Options:**
   ```powershell
   # Silent installation and immediate launch:
   .\LocalCode-Setup.exe --silent --launch

   # Custom installation directory:
   .\LocalCode-Setup.exe --dir "C:\Tools\LocalCode"

   # Uninstallation:
   .\LocalCode-Setup.exe --uninstall
   ```

#### Option 2: Portable Fast Startup

1. Clone the repository or extract the release ZIP.
2. Launch `START.bat` or `FAST-START.bat`. The launcher opens the Desktop UI in your browser.
3. If binaries are missing, `BUILD.bat` automatically builds the application locally.

#### Option 3: Android Mobile Remote App

1. Transfer `dist\LocalCode-Remote-debug.apk` from the release to your Android device or install via ADB:
   ```powershell
   adb install -r dist\LocalCode-Remote-debug.apk
   ```
2. Open the app on your phone and scan the QR code shown on the desktop under *Help → Pair Remote*.
3. Enjoy mobile task control, Text-to-Speech audio feedback, quick prompt cards, and review approval popups on your private LAN.

### Quick Start & First Steps

1. Place the repository or extracted ZIP in a new directory, or use the setup installer.
2. Run `START.bat`. The normal launcher uses a fast, logged path and opens the UI without blocking runtime dialogs or automatic firewall UAC prompts.
3. If the native Windows build is missing or stale, `START.bat` delegates once to `BUILD.bat`; after that the freshness check uses a build-state/Git fingerprint.
4. Ollama, models and coding-agent engines are checked inside the running product through Status/Doctor and on first use; full diagnostics remain available through `DIAGNOSE.bat`.
5. For durable phone Remote access on the private LAN, run `scripts\install-remote-firewall-rule.ps1` once as Administrator. `START.bat` does not trigger that elevation prompt automatically.
6. Fresh installs default to `qwen2.5-coder:14b` and **LocalCode Native**.

Launcher logs are written to `logs/start.log`; runtime details are written to `%LOCALAPPDATA%\LocalCode\localcode.log`. Large model downloads can take time during the first actual setup or first use.


### Language

- `Automatic (Windows)` follows the Windows display language: German on German Windows, English otherwise.
- German and English can be selected manually.
- Visible product text and central documentation are maintained in synchronized DE/EN form.

### Coding-agent engines

LocalCode can switch among **LocalCode Native**, **Aider**, **Claude Code**, **OpenCode**, and experimental **Claw Code**. The engine is selectable in the composer and Settings. There is no silent provider or model drift.

### Projects, tasks and tools

LocalCode manages projects and persistent task/thread history. Depending on configuration and approval, the Native agent can read and modify files, use Git, run builds/tests, discover local tooling, use MCP, perform public-web research, process attachments and use local image/asset tools.

Key protections include canonical workspace containment including symlink/NTFS-junction checks, approval-gated file/command/network/install actions, SHA/version preconditions, atomic conflict-aware writes, controlled cancellation/timeouts, durable run recovery, and no automatic authority escalation from prompts, rules, skills, memories or Planner text.

### Native Agent Teams – current state

The only executable child roles are **Explorer**, **Planner**, and **Reviewer**. They receive isolated context, hard model/tool/time/token budgets and a read-only action schema: project tree, files, text search, approval-free LSP and structured `finish`. File mutation, shell, Git mutation, web/network, MCP tool calls, installation, memory writes, approval requests and recursive spawning are deliberately absent.

A deterministic task DAG, bounded Scheduler/Resource Manager and explicit read-only Mission entry are implemented. The Scheduler executes authorized children, collects structured `AgentResult`/usage data, and unlocks dependencies deterministically. Multiple tasks may be logically ready while local model inference defaults to one active slot. The current dispatcher is synchronous; a higher configured model-slot limit alone does not prove real parallel inference.

Cancellation and child completion are serialized safely. Cancellation-first discards late child results; completion-first remains terminal success. Whole-Mission cancellation terminalizes unfinished tasks as `cancelled`.

### Mission status, diagnostics and benchmarks

Desktop renders read-only Mission status in the Output inspector. `/api/status` also exposes machine-readable orchestration diagnostics for backend, queue and saturation state. Observation does not change Scheduler limits or capabilities.

Reproducible parallelism benchmarks separate logical readiness, configured capacity and actually observed executor/Ollama overlap. The real Ollama benchmark is opt-in, loopback-only and never starts or downloads a model. See `docs/ORCHESTRATION_BENCHMARKS.md`.

### Durable Mission recovery and explicit Desktop continuation

Read-only Missions persist bounded recovery metadata in the existing `active-run.json`. `run_journal.go` remains the **only durable recovery authority**; there is no second Mission journal.

Mission start captures bounded project/Git evidence from canonical project identity, repository identity, exact `HEAD` and a SHA-256 fingerprint of porcelain worktree state. Raw `git status` paths are not persisted. After interruption, current state is freshly classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is never inferred successful. Durable success requires appropriate postcondition verification, and dependencies/attempt limits are re-evaluated before continuation.

LocalCode now has **explicit Desktop-only Mission recovery control**:

- `GET /api/mission-recovery` exposes only a bounded inspection DTO containing Run/Mission identity, hash preconditions, reconciliation and task transitions.
- `POST /api/mission-recovery/continue` accepts exactly one current `resume_candidate` or `retry_candidate` plus inspected stale-state preconditions.
- UI hashes are not execution authority. Immediately before admission, project/Git state, transition plan, capabilities, model identity, budgets and the current journal fingerprint are recomputed and revalidated by trusted runtime code.
- `202 Accepted` is returned only after durable reservation and AppState ownership succeed. Execution then runs under the existing Scheduler and remains cancellable through `StopAgent`.
- Reservation is not an attempt. `AttemptCount` increases only after a durable Scheduler `Running` checkpoint proves execution actually started.
- Historical usage/budgets remain cumulative; recovery cannot mint fresh limits. Crash/offline downtime is excluded from active Mission execution time.
- Startup remains passive: **no automatic resume, retry, or replay**.

The Desktop **Interrupted Mission** card only offers explicit controls for current Resume/Retry candidates and never auto-posts.

### Phone Remote / Android

LocalCode can run a separate token-protected Remote server on the local network. **Help → Pair Remote** creates a short-lived pairing code or pair/QR link. The Android shell supports saved reconnection, mDNS plus a direct LAN fallback scan, TLS fingerprint pinning, WebView, native file picking and Android speech recognition.

Mobile deliberately remains narrower than Desktop. While a read-only Mission is active, Remote shows only **Mission · Running** from the already-authenticated `running` and `run_phase` fields. There is **no** Remote recovery endpoint, no Mission/task recovery identifiers, no Scheduler/budget/accounting payload and no Mobile Resume/Retry authority. Existing Remote stop behavior is unchanged.

### Not implemented yet

- persistent bounded **Mission Memory/Knowledge** with explicit privacy/retention limits,
- mutation-capable Builder agents in isolated Git worktrees,
- Integrator/Test-Agent mutation flow,
- automatic recovery continuation on startup – this remains deliberately forbidden.

### Build and quality

The GitHub Quality pipeline checks Go setup/version, `gofmt`, `go vet ./...`, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, vulnerability scan, full-stack loopback HTTP integration, complete Go tests, Race Detector, >=80% statement coverage, native Windows builds and `git diff --check`.

### Important documents

- `AGENTS.md` – binding repository/work rules
- `STATE.md` – canonical current project state
- `TODO.md` – unfinished functional work only
- `docs/ARCHITECTURE.md` – architecture/runtime boundaries
- `docs/SECURITY.md` – security model
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproducible orchestration/Ollama benchmarks
- `android/README.md` – Android Remote details

## License

Apache License 2.0. See `LICENSE`, `NOTICE` and `THIRD_PARTY_NOTICES.md`.
