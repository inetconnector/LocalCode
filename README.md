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

Ein deterministischer Task-DAG, ein begrenzter Scheduler/Resource Manager und ein expliziter read-only Mission-Einstieg sind implementiert. Mehrere logisch bereite Tasks können in der Queue stehen, während lokale Modellinferenz standardmäßig auf einen aktiven Slot begrenzt bleibt. Der Scheduler führt autorisierte Explorer/Planner/Reviewer tatsächlich aus, sammelt strukturierte `AgentResult`- und Usage-Daten und schaltet abhängige Tasks deterministisch frei. Größere Sättigungs-/Fairness-Tests prüfen FIFO innerhalb einer Ressourcenklasse, Cross-Class-Bypass und einen 14-Task-Fan-out/Fan-in-DAG ohne Starvation.

Cancel und Child-Abschluss sind race-sicher serialisiert. Ein Mission-Abbruch terminalisiert noch unfertige Tasks kontrolliert als `cancelled`; verspätete Child-Resultate können den gewonnenen Cancel nicht überschreiben.

Der Desktop kann den **read-only Mission-Status** im rechten Ausgabenbereich anzeigen: Mission-State/-Reason, Queue/Running, Ressourcenklassen, Task-Zustände und Budget-Snapshots. Diese Anzeige ist reine Beobachtung. Sie startet oder verändert keine Mission, vergibt keine Capabilities und ist keine Recovery-Persistenz; `run_journal.go` bleibt die dauerhafte Recovery-Autorität.

Die Mobile Remote bleibt absichtlich schmaler: Wenn eine read-only Mission aktiv ist, zeigt sie lediglich **Mission · Läuft** sowie eine kompakte DE/EN-Hinweiskarte. Dafür werden ausschließlich die bereits vorhandenen authentifizierten Statusfelder `running` und `run_phase` verwendet. Mobile erhält dabei keine Mission-/Task-IDs, Scheduler-, Ressourcen-, Budget- oder Accounting-Daten und keinen neuen Mission-Start-/Steuerpfad.

**Noch nicht implementiert:** eine produktseitige Mission-Start-/Steueroberfläche, dauerhafte Mission-Recovery, mutation-capable Builder-Agenten, parallele Worktree-Schreibagenten und Integrator/Test-Agent-Mutationsfluss.

### Handy-Remote / Android

LocalCode betreibt optional einen getrennten, token-geschützten Remote-Server für das lokale Netzwerk. Über **Hilfe → Remote koppeln** wird ein kurzlebiger Pairing-Code bzw. Pair-/QR-Link erzeugt.

Die native Android-Hülle ist bereits vorhanden. Sie kann LocalCode per mDNS finden, Pair-/QR-/Deep-Link-Daten übernehmen, den TLS-SHA-256-Fingerprint pinnen, die Remote-Web-App öffnen, den nativen Android-Dateipicker verwenden, Spracheingabe starten und Fehler sichtbar anzeigen.

Die Remote-Oberfläche unterstützt Pairing, Projekte/Tasks, Engine-Auswahl, Attachments, Spracheingabe, Senden, Stop, Genehmigungen und Projektaktionen. Die Mission-Anzeige fügt **keinen** neuen Remote-Endpunkt und **keine** neue Autorität hinzu; bestehendes Stop-Verhalten bleibt unverändert. Mobile besitzt keine zusätzlichen Werkzeugrechte; die eigentliche Arbeit läuft auf dem Windows-Rechner.

### Build und Qualität

Die GitHub-Quality-Pipeline prüft unter anderem Go-Version/Setup, `gofmt`, `go vet ./...`, Frontend-JavaScript-Syntax, PowerShell-Syntax, native Android-Remote-APK, Vulnerability Scan, Full-Stack-Loopback-HTTP-Integration, vollständige Go-Tests, Race Detector, Statement Coverage (Gate mindestens 80 %), native Windows-Builds und `git diff --check`.

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

LocalCode can switch between LocalCode Native, Aider, Claude Code, OpenCode and experimental Claw Code. The engine can be selected in the composer and Settings; there is no silent provider or model drift.

### Projects, tasks and tools

LocalCode manages projects and persistent task/thread history. Depending on configuration and approval, the Native agent can read and modify files, use Git, run builds/tests, discover local tooling, use MCP, perform public-web research, process attachments and use local image/asset tools.

Key protections include canonical project/workspace boundaries, approval-gated risky actions, SHA/version preconditions, atomic conflict-aware writes, controlled process cancellation/timeouts, a durable run journal and no automatic authority escalation from prompts/rules/skills/Planner text.

### Native Agent Teams – current state

LocalCode currently has executable **read-only** child roles: Explorer, Planner and Reviewer. Child agents receive isolated context and hard budgets; write actions, shell, Git, web/network, MCP tool calls, installation, memory, approvals and recursive spawning are absent from their action schema.

A deterministic task DAG, bounded Scheduler/Resource Manager and explicit read-only Mission entry are implemented. Local inference defaults to one model slot. Saturation/fairness tests verify FIFO within a resource class, cross-class bypass and a 14-task fan-out/fan-in DAG without starvation.

Cancellation and child completion are serialized safely. Whole-Mission cancellation terminalizes unfinished work as `cancelled`, and late child output cannot overwrite cancellation that won first.

Desktop can display **read-only Mission status** in the Output inspector: Mission state/reason, queued/running counts, resource classes, task states and budget snapshots. This is observation only and is not a Mission-control or recovery surface.

Mobile Remote stays deliberately narrower: while a read-only Mission is active it shows only **Mission · Running** and a compact localized notice, derived from the already-authenticated `running` and `run_phase` status fields. Mobile receives no Mission/task IDs, scheduler/resource/budget/accounting data and no new Mission start/control path.

**Not implemented yet:** a product Mission-start/control surface, durable Mission recovery, mutation-capable Builder agents, parallel worktree mutation agents and Integrator/Test-Agent mutation flow.

### Phone Remote / Android

LocalCode can run a separate token-protected Remote server on the local network. The native Android shell supports mDNS discovery, Pair/QR/deep links, TLS fingerprint pinning, native file picking and Android speech input.

The Remote UI supports pairing, projects/tasks, engine selection, attachments, voice input, send, stop, approvals and project actions. The Mission indicator adds **no** new Remote endpoint or authority; existing stop behavior is unchanged. The actual work remains on the Windows host.

### Build and quality

The GitHub Quality pipeline checks Go setup/version, `gofmt`, `go vet ./...`, frontend JavaScript syntax, PowerShell syntax, native Android Remote APK, vulnerability scan, full-stack loopback HTTP integration, complete Go tests, race detector, statement coverage (minimum 80%), native Windows builds and `git diff --check`.

### Important documents

- `AGENTS.md` – binding repository/work rules
- `STATE.md` – canonical current project state
- `TODO.md` – unfinished functional work only
- `docs/ARCHITECTURE.md` – architecture/runtime boundaries
- `docs/SECURITY.md` – security model
- `android/README.md` – Android Remote details

## License

Apache License 2.0. See `LICENSE`, `NOTICE` and `THIRD_PARTY_NOTICES.md`.
