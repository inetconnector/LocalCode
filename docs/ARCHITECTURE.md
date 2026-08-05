# Architecture / Architektur

## Deutsch

LocalCode besteht aus einer einzelnen Go-Anwendung mit eingebettetem Web-Frontend.

- `main.go`: Start, Ollama-Erkennung, Versionswechsel und lokaler HTTP-Server.
- `server.go`: ausschließlich lokale API auf `127.0.0.1`, SSE, Projekte, Chats, aufgabenspezifische Snapshots, Einstellungen und Genehmigungen.
- `project_catalog.go` und `history.go`: Projekt-Aliase, Anheften/Entfernen/Wiederherstellen sowie persistente Aufgabenaktionen.
- `agent.go` und `agent_supervisor.go`: Agentenschleife, deterministische Aufgabensteuerung, Fortsetzungen und Abbruch.
- `context_compaction.go`: kontrollierte Kontextkomprimierung.
- `tool_registry.go`, `tool_install.go`, `project_automation.go`: Werkzeugerkennung, Installation, Build und Deployment.
- `static/index.html` und `static/i18n.js`: Desktop-Oberfläche und Sprachkataloge.

Zustände werden unter einem Mutex verwaltet. Chatereignisse werden im Speicher sofort veröffentlicht und über einen einzelnen, zusammenfassenden Hintergrundschreiber atomar gespeichert. Dadurch blockieren Dateischreibvorgänge weder Agent noch Oberfläche.

Offene Genehmigungen gehören zum Backendzustand. Die Oberfläche liest sie aus Snapshots und Ereignissen und zeigt sie zusätzlich in einer festen Leiste unten mittig an. Die Projektwurzel kann über eine API manuell gesetzt oder durch einen Windows-Ordnerdialog gewählt werden.

Jedes UI-Ereignis kann eine Aufgaben-ID tragen. Ein Fenster fordert seinen Snapshot gezielt für diese Aufgabe an und filtert Live-Ereignisse entsprechend. Chat-Anfragen senden Projekt- und Aufgaben-ID explizit, damit mehrere geöffnete Aufgabenfenster nicht versehentlich denselben Auswahlzustand verwenden. Der Agent selbst bleibt global auf einen gleichzeitig aktiven Lauf begrenzt.

## English

LocalCode is a single Go application with an embedded web frontend.

- `main.go`: startup, Ollama discovery, version handover, and local HTTP server.
- `server.go`: loopback-only API, SSE, projects, chats, task-targeted snapshots, settings, and approvals.
- `project_catalog.go` and `history.go`: project aliases, pin/remove/restore behavior, and persistent task actions.
- `agent.go` and `agent_supervisor.go`: agent loop, deterministic supervision, continuations, and cancellation.
- `context_compaction.go`: controlled context compaction.
- `tool_registry.go`, `tool_install.go`, `project_automation.go`: tool discovery, installation, builds, and deployment.
- `static/index.html` and `static/i18n.js`: desktop UI and language catalogs.

State is protected by a mutex. Chat events are published immediately in memory and persisted by one coalescing background writer using atomic file replacement. Disk writes therefore do not block the agent or the UI.

Pending approvals are backend state. The client restores them from snapshots and events and duplicates them in a fixed bottom-center decision bar. The project root can be entered manually through the API or selected with the Windows folder dialog.

Each UI event can carry a task ID. A window requests a snapshot for its explicit task and filters live events accordingly. Chat requests include the explicit project and task ID so multiple task windows do not accidentally share the latest selection state. The agent itself remains globally limited to one active run at a time.

## Aider editing engine / Aider-Editing-Engine

LocalCode routes real multi-file code changes through a managed Aider subprocess. The supervisor remains authoritative for task intent, approvals, cancellation, MCP calls, tool discovery, output events, context compaction, and UI state. The Aider adapter owns isolated installation, CLI construction, task-specific history, repository-map configuration, edit format, lint/test handoff, project fingerprints, backup manifests, and guarded restore.

LocalCode leitet echte mehrdateilige Codeänderungen an einen verwalteten Aider-Unterprozess weiter. Der Supervisor bleibt für Aufgabenabsicht, Genehmigungen, Abbruch, MCP-Aufrufe, Werkzeugerkennung, Ausgabenereignisse, Kontextkomprimierung und UI-Zustand maßgeblich. Der Aider-Adapter verwaltet isolierte Installation, CLI-Erzeugung, aufgabenbezogenen Verlauf, Repository-Map-Konfiguration, Edit-Format, Lint-/Testübergabe, Projekt-Fingerprints, Backup-Manifeste und geschützte Wiederherstellung.

