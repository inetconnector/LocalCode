# LocalCode 6.4.4

[Deutsch](#deutsch) · [English](#english)

LocalCode is a Windows-first, local-first coding-agent application centered on Ollama and controlled tool execution. It combines project/task management, a native coding-agent runtime, selectable external coding engines, Git/build tooling, MCP, web research, attachments, approvals, recovery, a Desktop UI and a narrow Android/phone Remote. LocalCode is an independent project and is not OpenAI Codex.

---

## Deutsch

### Schnellstart

1. Repository bzw. ZIP vollständig in einen neuen Ordner legen.
2. `START.bat` oder `BUILD-AND-RUN.bat` starten.
3. LocalCode prüft die Windows-/Go-Laufzeit, Ollama, das konfigurierte Modell und die ausgewählte Coding-Agent-Engine.
4. Fehlende unterstützte Komponenten können benutzerlokal installiert und anschließend verifiziert werden.
5. Frische Installationen verwenden standardmäßig `qwen2.5-coder:14b` und die Engine **LocalCode Native**.

Große Modelldownloads können beim ersten Start dauern. Diagnoseausgabe liegt im LocalCode-Log; `DIAGNOSE.bat` führt zusätzliche Prüfungen aus.

### Sprache

- `Automatisch (Windows)` folgt der Windows-Anzeigesprache: Deutsch auf deutschem Windows, sonst Englisch.
- Deutsch und Englisch können manuell gewählt werden.
- Sichtbare Produkttexte und zentrale Dokumentation werden DE/EN synchron gehalten.

### Coding-Agent-Engines

LocalCode kann zwischen folgenden Engines umschalten:

- **LocalCode Native** – eigene Werkzeugschleife mit Genehmigungen, Sandbox-/Pfadgrenzen, Recovery und Agent-Team-Bausteinen.
- **Aider** – unterstützt lokale Ollama-Modelle.
- **Claude Code** – benötigt eine geeignete Anmeldung/Provider-Konfiguration.
- **OpenCode** – unterstützt lokale Ollama-Modelle und optionale Provider-Anmeldung.
- **Claw Code** – experimentelle externe Engine hinter der LocalCode-Genehmigungsgrenze.

Die Engine kann in der Eingabeleiste und in den Einstellungen gewählt werden. Es gibt keine stille Provider- oder Modellumschaltung.

### Projekte, Aufgaben und Werkzeuge

LocalCode verwaltet Projekte und persistente Aufgaben/Threads. Der native Agent kann – abhängig von Konfiguration und Genehmigung – Dateien lesen und ändern, Git verwenden, Builds/Tests starten, lokale Werkzeuge erkennen, MCP anbinden, öffentliche Webrecherche durchführen, Dateien/Bilder anhängen und lokale Bild-/Asset-Werkzeuge nutzen.

Wichtige Schutzmechanismen:

- kanonische Projekt-/Workspace-Pfadgrenzen einschließlich Symlink-/NTFS-Junction-Prüfung,
- genehmigungsgebundene Datei-, Befehls-, Netzwerk- und Installationsaktionen,
- SHA-/Versions-Preconditions und atomare konfliktarme Dateiänderungen,
- kontrollierter Prozessabbruch und Timeouts,
- dauerhafter Run-Journal für unterbrochene Läufe,
- keine automatische Rechteeskalation durch Prompt-, Regel-, Skill- oder Planner-Text.

### Native Agent Teams – aktueller Stand

LocalCode besitzt aktuell ausführbare **read-only** Child-Rollen:

- Explorer
- Planner
- Reviewer

Child-Agenten haben getrennten Kontext, harte Modell-/Tool-/Zeit-/Tokenbudgets und können nur Projektbaum, Dateien, Textsuche und genehmigungsfreies LSP lesen. Schreiben, Shell, Git, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory, Genehmigungen und rekursives Spawning gehören nicht zu ihrem Action-Schema.

Ein deterministischer Task-DAG und ein begrenzter Scheduler/Resource Manager sind implementiert. Mehrere logisch bereite Tasks können in der Queue stehen, während lokale Modellinferenz standardmäßig auf einen aktiven Slot begrenzt bleibt. Der Scheduler führt autorisierte Explorer/Planner/Reviewer tatsächlich aus, sammelt strukturierte `AgentResult`- und Usage-Daten und schaltet abhängige Tasks deterministisch frei.

Cancel und Child-Abschluss sind race-sicher serialisiert: Der Child erhält eine abgetrennte Task-Kopie; ein bereits gewonnener Cancel darf nicht durch ein verspätetes Child-Ergebnis überschrieben werden, und ein bereits erfolgreich finalisierter Task wird nicht nachträglich storniert.

**Noch nicht implementiert:** mutation-capable Builder-Agenten, parallele Worktree-Schreibagenten, Integrator/Test-Agent-Mutationsfluss und eine vollwertige produktseitige Mission-Oberfläche.

### Handy-Remote / Android

LocalCode betreibt optional einen getrennten, token-geschützten Remote-Server für das lokale Netzwerk. Über **Hilfe → Remote koppeln** wird ein kurzlebiger Pairing-Code bzw. Pair-/QR-Link erzeugt.

Die native Android-Hülle ist bereits vorhanden. Sie kann:

- LocalCode per mDNS im lokalen Netzwerk finden,
- Pair-/QR-/Deep-Link-Daten übernehmen,
- den TLS-SHA-256-Fingerprint pinnen,
- die Remote-Web-App in einer WebView öffnen,
- über die Büroklammer den nativen Android-Dateipicker nutzen,
- Spracheingabe über Android `RecognizerIntent` starten,
- erkannten Text nur in das Promptfeld einfügen,
- Fehler von Dateipicker/Speech sichtbar in der Remote-Ansicht anzeigen.

Die Remote-Oberfläche unterstützt Pairing, Projekte/Tasks, Engine-Auswahl, Attachments, Entfernen von Attachments, Spracheingabe, Senden, Stop, Genehmigungen und Projektaktionen. Attachments werden gemeinsam mit Projekt, Thread und Modell an den Windows-Rechner übertragen; doppelte Send-Aufrufe werden gesperrt. Prompt und Attachments werden erst nach erfolgreichem Chat-Request geleert. Mobile besitzt keine zusätzlichen Werkzeugrechte; die eigentliche Arbeit läuft auf dem Windows-Rechner.

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
- `android/README.md` – Android-Remote-Details

---

## English

### Quick start

1. Place the repository or extracted ZIP in a new directory.
2. Run `START.bat` or `BUILD-AND-RUN.bat`.
3. LocalCode verifies the Windows/Go runtime, Ollama, the configured model and the selected coding-agent engine.
4. Missing supported components can be installed for the current user and verified afterwards.
5. Fresh installs default to `qwen2.5-coder:14b` and **LocalCode Native**.

Large model downloads can take time on first startup. Diagnostics are written to the LocalCode log; `DIAGNOSE.bat` performs additional checks.

### Language

- `Automatic (Windows)` follows the Windows display language: German on German Windows, English otherwise.
- German and English can be selected manually.
- Visible product text and central documentation are maintained in synchronized DE/EN form.

### Coding-agent engines

LocalCode can switch between:

- **LocalCode Native** – the built-in tool loop with approvals, sandbox/path boundaries, recovery and Agent-Team building blocks.
- **Aider** – supports local Ollama models.
- **Claude Code** – requires a suitable sign-in/provider configuration.
- **OpenCode** – supports local Ollama models and optional provider authentication.
- **Claw Code** – experimental external engine behind LocalCode's approval boundary.

The engine can be selected in the composer and in Settings. There is no silent provider or model drift.

### Projects, tasks and tools

LocalCode manages projects and persistent task/thread history. Depending on configuration and approval, the Native agent can read and modify files, use Git, run builds/tests, discover local tooling, use MCP, perform public-web research, process attachments and use local image/asset tools.

Key protections include:

- canonical project/workspace boundaries including symlink/NTFS-junction checks,
- approval-gated file, command, network and installation actions,
- SHA/version preconditions and atomic conflict-aware file changes,
- controlled process cancellation and timeouts,
- a durable run journal for interrupted runs,
- no automatic authority escalation from prompts, rules, skills or Planner text.

### Native Agent Teams – current state

LocalCode currently has executable **read-only** child roles:

- Explorer
- Planner
- Reviewer

Child agents receive isolated context, hard model/tool/time/token budgets and may only read the project tree, files, text search and approval-free LSP. Write actions, shell, Git, web/network, MCP tool calls, installation, memory, approvals and recursive spawning are absent from their action schema.

A deterministic task DAG and bounded Scheduler/Resource Manager are implemented. Multiple logically ready tasks can be queued while local model inference defaults to one active slot. The scheduler now actually dispatches authorized Explorer/Planner/Reviewer tasks, collects structured `AgentResult` and usage data, and deterministically unlocks dependent tasks.

Cancellation and child completion are serialized safely: the child receives a detached task copy; a cancellation that wins first cannot be overwritten by a late child result, and a successfully finalized task cannot be cancelled afterwards.

**Not implemented yet:** mutation-capable Builder agents, parallel worktree mutation agents, Integrator/Test-Agent mutation flow, and a full product-level Mission UI.

### Phone Remote / Android

LocalCode can run a separate token-protected Remote server on the local network. **Help → Pair Remote** creates a short-lived pairing code or pair/QR link.

The native Android shell already exists. It can:

- discover LocalCode over mDNS,
- consume pair/QR/deep-link data,
- pin the TLS SHA-256 fingerprint,
- load the Remote web app in a WebView,
- open Android's native file picker from the paperclip control,
- start Android `RecognizerIntent` speech input,
- append recognized text to the prompt without auto-sending,
- surface file-picker/speech failures visibly inside Remote.

The Remote UI supports pairing, projects/tasks, engine selection, attachments, attachment removal, voice input, send, stop, approvals and project actions. Attachments are sent together with project, thread and model to the Windows host; duplicate submissions are locked. Prompt and attachments are cleared only after a successful chat request. Mobile gets no extra tool authority; the actual work remains on the Windows machine.

### Build and quality

The GitHub Quality pipeline checks, among other things:

- Go setup/version,
- `gofmt`,
- `go vet ./...`,
- frontend JavaScript syntax,
- PowerShell syntax,
- native Android Remote APK,
- vulnerability scan,
- full-stack loopback HTTP integration,
- complete Go tests,
- race detector,
- statement coverage (minimum gate 80%),
- native Windows builds,
- `git diff --check`.

### Important documents

- `AGENTS.md` – binding repository/work rules
- `STATE.md` – canonical current project state
- `TODO.md` – unfinished functional work only
- `docs/ARCHITECTURE.md` – architecture/runtime boundaries
- `docs/SECURITY.md` – security model
- `android/README.md` – Android Remote details

## License

Apache License 2.0. See `LICENSE`, `NOTICE` and `THIRD_PARTY_NOTICES.md`.
