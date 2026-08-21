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

Cancel und Child-Abschluss sind race-sicher serialisiert: Der Child erhält eine abgetrennte Task-Kopie; ein bereits gewonnener Cancel darf nicht durch ein verspätetes Child-Ergebnis überschrieben werden, und ein bereits erfolgreich finalisierter Task wird nicht nachträglich storniert. Ein Mission-Abbruch terminalisiert außerdem noch unfertige Tasks kontrolliert als `cancelled`.

Der Desktop kann den **read-only Mission-Status** im rechten Ausgabenbereich anzeigen: Mission-State/-Reason, Queue/Running, Ressourcenklassen, Task-Zustände und Budget-Snapshots. Diese Anzeige ist reine Beobachtung. Sie startet oder verändert keine Mission, vergibt keine Capabilities und ist keine Recovery-Persistenz; `run_journal.go` bleibt die dauerhafte Recovery-Autorität.

Zusätzlich liefert `/api/status` maschinenlesbare **Orchestrierungsdiagnostik** und zeigt sie im Desktop-Ausgabenbereich an. Unterschieden werden Ollama offline, kein Modell gewählt, gewähltes Modell lokal nicht vorhanden, aktive Mission, Queue-Limit und echte Ressourcensättigung. `at_capacity` bedeutet nur, dass ein Slot vollständig belegt ist; `saturated` wird erst gemeldet, wenn die Ressource voll ist **und** passende Arbeit darauf wartet. Angezeigt werden Queue-Auslastung, logische Ready/Running/Blocked-Zahlen, wartende Modellarbeit sowie Limit/In-Use/Available/Waiting je Ressourcenklasse. Die Diagnostik verändert weder Limits noch Parallelität und ist kein Benchmark.

Für die Orchestrierung gibt es zusätzlich reproduzierbare **Parallelitäts-Benchmarks**. Ein deterministischer synthetischer Benchmark misst vier gleichzeitig logisch bereite read-only Tasks bei Model-Slot-Limits 1/2/4 und berichtet die tatsächlich beobachtete Executor-Überlappung. Der aktuelle synchrone Dispatcher erreicht dabei nicht automatisch echte Parallelität nur weil mehr Model-Slots konfiguriert sind. Ein zweiter, ausdrücklich opt-in ausgeführter Ollama-Benchmark nutzt den produktiven `OllamaClient.Chat`-Pfad und vergleicht ein festes Arbeitsvolumen bei Client-Konkurrenz 1/2/4. Er akzeptiert nur Loopback, verlangt ein bereits installiertes exaktes Modell und startet oder lädt nichts. End-to-End-Request-Überlappung wird dabei nicht als Beweis gleichzeitiger GPU-Inferenz interpretiert. Details: `docs/ORCHESTRATION_BENCHMARKS.md`.

Read-only Missions besitzen außerdem **dauerhafte Recovery-Metadaten und Restart-Reconciliation** im bestehenden `active-run.json`. Beim Missionsstart wird eine begrenzte Baseline aus kanonischer Projektidentität, Git-Repository-Identität, `HEAD` und einem SHA-256-Fingerprint des Porcelain-Worktree-Status gespeichert. Rohe `git status`-Pfade werden nicht persistiert. Nach einem Prozessabbruch vergleicht LocalCode beim nächsten Start die aktuelle Projekt-/Git-Sicht mit dieser Baseline und klassifiziert den Zustand maschinenlesbar als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence`. Diese Reconciliation ist ausschließlich Beobachtung: Ein beim Absturz laufender Task gilt niemals als erfolgreich; selbst durable `succeeded`-Tasks benötigen vor einer späteren Wiederaufnahme noch Postcondition-Verifikation. Automatisches Resume, Retry oder Replay ist weiterhin nicht implementiert.

Die Mobile Remote zeigt bei einer aktiven read-only Mission lediglich **Mission · Läuft** und eine kompakte DE/EN-Hinweiskarte. Dafür werden nur die bereits vorhandenen authentifizierten Statusfelder `running` und `run_phase` verwendet. Es gibt keinen neuen Remote-Mission-Endpunkt, keine Mission-/Task-IDs, keine Scheduler-/Ressourcen-/Budget-/Accounting-Daten und keinen neuen Mission-Start-/Steuerpfad.

**Noch nicht implementiert:** eine produktseitige Mission-Start-/Steueroberfläche, kontrolliertes Mission-Pause/Resume/Retry nach Reconciliation und Postcondition-Verifikation, mutation-capable Builder-Agenten, parallele Worktree-Schreibagenten und Integrator/Test-Agent-Mutationsfluss.

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

Die Remote-Oberfläche unterstützt Pairing, Projekte/Tasks, Engine-Auswahl, Attachments, Entfernen von Attachments, Spracheingabe, Senden, Stop, Genehmigungen und Projektaktionen. Attachments werden gemeinsam mit Projekt, Thread und Modell an den Windows-Rechner übertragen; doppelte Send-Aufrufe werden gesperrt. Prompt und Attachments werden erst nach erfolgreichem Chat-Request geleert. Die Mission-Anzeige fügt keinen neuen Remote-Endpunkt und keine neue Authority hinzu; bestehendes Stop-Verhalten bleibt unverändert. Mobile besitzt keine zusätzlichen Werkzeugrechte; die eigentliche Arbeit läuft auf dem Windows-Rechner.

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
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproduzierbare Orchestrierungs-/Ollama-Benchmarks und Interpretationsgrenzen
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

A deterministic task DAG, bounded Scheduler/Resource Manager and explicit read-only Mission entry are implemented. Multiple logically ready tasks can be queued while local model inference defaults to one active slot. The scheduler actually dispatches authorized Explorer/Planner/Reviewer tasks, collects structured `AgentResult` and usage data, and deterministically unlocks dependent tasks. Larger saturation/fairness tests verify FIFO inside a resource class, cross-class bypass and a 14-task fan-out/fan-in DAG without starvation.

Cancellation and child completion are serialized safely: the child receives a detached task copy; a cancellation that wins first cannot be overwritten by a late child result, and a successfully finalized task cannot be cancelled afterwards. Whole-Mission cancellation also terminalizes unfinished tasks as `cancelled`.

Desktop can display **read-only Mission status** in the Output inspector: Mission state/reason, queued/running counts, resource classes, task states and budget snapshots. This is observation only. It cannot start or mutate a Mission, grant capabilities or act as recovery persistence; `run_journal.go` remains the durable recovery authority.

`/api/status` also exposes machine-readable **orchestration diagnostics**, rendered in the Desktop Output inspector. It distinguishes Ollama offline, no selected model, selected model not installed locally, active Mission, queue-limit pressure and actual resource saturation. `at_capacity` only means every slot is occupied; `saturated` requires a full resource **and** work waiting for it. The diagnostics show queue utilization, logical ready/running/blocked counts, waiting model work and limit/in-use/available/waiting per resource class. Diagnostics do not change limits/concurrency and are not benchmark evidence.

LocalCode now also has reproducible **orchestration parallelism benchmarks**. A deterministic synthetic benchmark measures four simultaneously logically-ready read-only tasks with model-slot limits 1/2/4 and reports the executor overlap actually observed. The current synchronous dispatcher therefore does not become genuinely parallel merely because more model slots are configured. A second explicitly opt-in Ollama benchmark uses the production `OllamaClient.Chat` path and compares a fixed workload at client concurrency 1/2/4. It accepts loopback only, requires an exact already-installed model, and never starts or downloads anything. End-to-end request overlap is not interpreted as proof of simultaneous GPU inference. See `docs/ORCHESTRATION_BENCHMARKS.md`.

Read-only Missions also have **durable recovery metadata and restart reconciliation** inside the existing `active-run.json`. Mission start captures a bounded baseline from canonical project identity, Git repository identity, `HEAD`, and a SHA-256 fingerprint of porcelain worktree status. Raw `git status` paths are never persisted. After a process interruption, the next startup compares the current project/Git observation with the baseline and classifies it as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Reconciliation is observation only: a task that was running at the crash is never treated as successful, and even a durable `succeeded` task still requires postcondition verification before any future continuation. Automatic resume, retry, and replay remain unimplemented.

Mobile Remote shows only **Mission · Running** and a compact localized notice while a read-only Mission is active. It derives this from the already-authenticated `running` and `run_phase` status fields. No new Remote Mission endpoint, Mission/task IDs, scheduler/resource/budget/accounting data or Mission start/control path is added.

**Not implemented yet:** a product Mission-start/control surface, controlled Mission pause/resume/retry after reconciliation and postcondition verification, mutation-capable Builder agents, parallel worktree mutation agents and Integrator/Test-Agent mutation flow.

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

The Remote UI supports pairing, projects/tasks, engine selection, attachments, attachment removal, voice input, send, stop, approvals and project actions. Attachments are sent together with project, thread and model to the Windows host; duplicate submissions are locked. Prompt and attachments are cleared only after a successful chat request. The Mission indicator adds no new Remote endpoint or authority and leaves existing stop behavior unchanged. Mobile gets no extra tool authority; the actual work remains on the Windows machine.

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
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproducible orchestration/Ollama benchmarks and interpretation boundaries
- `android/README.md` – Android Remote details

## License

Apache License 2.0. See `LICENSE`, `NOTICE` and `THIRD_PARTY_NOTICES.md`.
