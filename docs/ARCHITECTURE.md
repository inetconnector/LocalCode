# Architecture / Architektur

## Deutsch

LocalCode besteht aus einer einzelnen Go-Anwendung mit eingebettetem Web-Frontend.

- `main.go`: Start, Ollama-Erkennung, Versionswechsel und lokaler HTTP-Server.
- `server.go`: ausschließlich lokale API auf `127.0.0.1`, SSE, Projekte, Chats, Einstellungen und Genehmigungen.
- `agent.go` und `agent_supervisor.go`: Agentenschleife, deterministische Aufgabensteuerung, Fortsetzungen und Abbruch.
- `context_compaction.go`: kontrollierte Kontextkomprimierung.
- `tool_registry.go`, `tool_install.go`, `project_automation.go`: Werkzeugerkennung, Installation, Build und Deployment.
- `static/index.html` und `static/i18n.js`: Desktop-Oberfläche und Sprachkataloge.

Zustände werden unter einem Mutex verwaltet. Chatereignisse werden im Speicher sofort veröffentlicht und über einen einzelnen, zusammenfassenden Hintergrundschreiber atomar gespeichert. Dadurch blockieren Dateischreibvorgänge weder Agent noch Oberfläche.

Offene Genehmigungen gehören zum Backendzustand. Die Oberfläche liest sie aus Snapshots und Ereignissen und zeigt sie zusätzlich in einer festen Leiste unten mittig an. Die Projektwurzel kann über eine API manuell gesetzt oder durch einen Windows-Ordnerdialog gewählt werden.

## English

LocalCode is a single Go application with an embedded web frontend.

- `main.go`: startup, Ollama discovery, version handover, and local HTTP server.
- `server.go`: loopback-only API, SSE, projects, chats, settings, and approvals.
- `agent.go` and `agent_supervisor.go`: agent loop, deterministic supervision, continuations, and cancellation.
- `context_compaction.go`: controlled context compaction.
- `tool_registry.go`, `tool_install.go`, `project_automation.go`: tool discovery, installation, builds, and deployment.
- `static/index.html` and `static/i18n.js`: desktop UI and language catalogs.

State is protected by a mutex. Chat events are published immediately in memory and persisted by one coalescing background writer using atomic file replacement. Disk writes therefore do not block the agent or the UI.

Pending approvals are backend state. The client restores them from snapshots and events and duplicates them in a fixed bottom-center decision bar. The project root can be entered manually through the API or selected with the Windows folder dialog.
