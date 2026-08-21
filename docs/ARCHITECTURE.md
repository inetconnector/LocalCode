# Architecture / Architektur

## Deutsch

### Systemübersicht

LocalCode ist eine Windows-first Go-Anwendung mit eingebettetem Web-Frontend. Die zentralen Laufzeitpfade sind:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native oder externe Engine -> kontrollierte Tools -> Verifikation/Recovery`

und für Mobilgeräte:

`Android/Browser Remote -> separater token-geschützter Remote-Server -> dieselben AppState-Operationen mit engerer Berechtigungsoberfläche`

Die Anwendung trennt bewusst **logische Agentenparallelität** von **tatsächlicher Modellinferenzparallelität**. Viele DAG-Tasks können bereit sein; der Scheduler begrenzt lokale Inferenz standardmäßig auf einen aktiven Model-Slot.

### Zentrale Komponenten

- `src/types.go` – `Config`, `AppState`, gemeinsame Laufzeittypen.
- `src/server.go` – Desktop Loopback HTTP/SSE API.
- `src/remote_server.go` – separater Mobile-Remote-Server, Pairing/Token/SSE/Remote-Aktionen.
- `src/agent.go` – Hauptschleife von LocalCode Native und Werkzeugdispatch.
- `src/agent_supervisor.go`, `src/edit_reliability.go`, `src/agent_loop_guard.go` – deterministische Steuerung, Edit-Preflight, Abschluss-/No-Progress-Schutz.
- `src/subagent.go` – deterministischer read-only Repository-Handoff/Fallback.
- `src/subagent_model.go` – modellgestützte read-only Explorer/Planner/Reviewer-Child-Runtime.
- `src/agent_team_types.go` – Rollen, Capabilities, Budget, Usage, Task und strukturierter `AgentResult`.
- `src/agent_task_graph.go` – Task-DAG-Validierung, Dependencies, Readiness und Zustandspropagation.
- `src/agent_scheduler.go` – Queue, Ressourcenlimits, Admission, Cancellation und Scheduler-Snapshots.
- `src/agent_scheduler_dispatch.go` – tatsächliche Scheduler-Ausführung von autorisierten read-only Child-Tasks.
- `src/agent_scheduler_finalize.go` – serialisierte Vorbereitung/Finalisierung gegen Cancel-Races.
- `src/run_journal.go` – dauerhafte aktive Run-/Recovery-Autorität.
- `src/path_tools.go` und Dateiwerkzeuge – kanonische Pfad-/Mutation-Grenzen.
- `src/mcp*.go`, `src/web_tools.go`, `src/tool_*` – externe Werkzeug-/Netzwerkgrenzen.
- `src/static/*` – Desktop-/Remote-Weboberflächen und DE/EN-Kataloge.
- `android/app/.../MainActivity.java` – native Android-Hülle mit Discovery, TLS-Pinning, WebView, Datei- und Speech-Brücke.

### Desktop und externe Engines

Die Desktop-Oberfläche spricht ausschließlich die lokale Loopback-API an. LocalCode hält Projekt, Thread, Modell, Engine, laufenden Run, Genehmigungen und Events im zentralen Zustand. Externe Engines Aider, Claude Code, OpenCode und Claw Code werden als kontrollierte Unterprozesse/Integrationen unter LocalCode gestartet; LocalCode Native nutzt die eigene strukturierte Werkzeugschleife.

### Mobile Remote und Android

Remote ist ein eigener Server mit schmaler API. Pairing erzeugt ein Gerätetoken; LocalCode persistiert nur dessen Hash. Der langlebige Token wird nicht als SSE-URL-Parameter verwendet; Streams werden über kurzlebige Tickets autorisiert.

Die native Android-Hülle ist bereits implementiert. Sie:

- entdeckt LocalCode per mDNS,
- kann Pair-/QR-/Deep-Link-Daten übernehmen,
- akzeptiert nur private HTTPS-Ziele mit erwartetem TLS-SHA-256-Fingerprint,
- verbindet WebView-Dateiinputs mit Androids Dateipicker,
- stellt eine enge Speech-Brücke zu `RecognizerIntent` bereit,
- räumt WebView-/Chooser-Ressourcen beim Activity-Abbau auf.

Die Android-Brücke führt keine LocalCode-Werkzeuge aus. Sie liefert nur Dateien bzw. erkannten Text an die bereits geladene Remote-Web-App. Werkzeugrechte bleiben auf dem Windows-Host.

### Native Agent Teams

#### Child Runtime

Aktuell ausführbare Rollen:

- Explorer
- Planner
- Reviewer

Ein Child-Task besitzt Objective, Role, Capabilities, Budget und strukturiertes Resultat. Das Child-Schema erlaubt nur:

- Projektbaum lesen,
- Datei lesen,
- Textsuche,
- genehmigungsfreies read-only LSP,
- strukturiertes `finish`.

Mutation, Shell, Git, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory, Approval-Requests und rekursives Spawning sind nicht Teil des Child-Schemas.

#### Task DAG

`AgentTaskGraph` enthält eine Mission-ID und stabile Task-IDs. Dependencies werden validiert; Zyklen, fehlende/self/duplizierte Abhängigkeiten und inkonsistente Zustände werden abgewiesen. Readiness und Blockierung werden deterministisch aus Dependency-Zuständen abgeleitet.

#### Scheduler / Resource Manager

`AgentScheduler` trennt die logische Ready-Queue von Ressourcenadmission. Ressourcenklassen sind derzeit unter anderem:

- `model-inference`
- `read-cpu`
- `build`
- `exclusive-integration`

Noch werden read-only Child-Tasks standardmäßig als Model-Inference klassifiziert. Default ist ein aktiver Model-Slot. Planner-`RequestedCapabilities` werden niemals selbst zu ausführbaren `Capabilities`; Admission verlangt eine bekannte Runtime-Rolle und tatsächlich gewährte Capabilities.

#### Actual read-only dispatch

`runScheduledReadOnlyAgentGraph` verbindet DAG und Scheduler mit der bestehenden Child-Runtime. Ablauf:

1. Ready-Tasks werden in die begrenzte Queue aufgenommen.
2. Scheduler wählt einen zulässigen Lease.
3. `prepareScheduledAgentTask` prüft den aktiven Lease unter Scheduler-Lock, normalisiert das Task-Budget und erstellt eine **abgetrennte Kopie** für den Child.
4. Der Child läuft außerhalb des Locks.
5. `finalizeScheduledAgentTask` konkurriert mit `CancelTask`/`CancelMission` wieder unter demselben Scheduler-Lock.
6. Nur der terminale Gewinner darf Resultat und Zustand festschreiben; der Lease wird exakt einmal freigegeben.
7. Danach werden Dependencies reconciled und der nächste Task kann admitted werden.

Damit greift ein Child während der Modellarbeit nicht auf den gemeinsam mutierbaren Graph-Pointer zu. Ein Cancel, der zuerst gewinnt, verwirft verspätete Child-Ergebnisse. Ein bereits erfolgreich finalisierter Task bleibt erfolgreich.

### Recovery

`run_journal.go` ist die bestehende Recovery-Autorität für aktive Hauptläufe. Zukünftige dauerhafte Mission-Persistenz muss diesen Pfad erweitern oder integrieren; ein zweites konkurrierendes Journal ist architektonisch nicht zulässig.

### Nächste Architekturstufe

Noch fehlt der **produktseitige Mission Manager** zwischen Main-Agent/Planner und Task-DAG/Scheduler. Diese Ebene muss explizit validieren, welche Planner-Vorschläge tatsächlich als Mission ausgeführt werden dürfen. Danach folgen Mission-Budgets, UI/Remote-Status, dauerhafte Mission-Recovery und erst später mutation-capable Builder in isolierten Git-Worktrees.

---

## English

### System overview

LocalCode is a Windows-first Go application with an embedded web frontend. Its main runtime paths are:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native or external engine -> controlled tools -> verification/recovery`

and for phones:

`Android/browser Remote -> separate token-protected Remote server -> the same AppState operations through a narrower permission surface`

The design deliberately separates **logical agent parallelism** from **actual model-inference parallelism**. Many DAG tasks may be ready while the Scheduler defaults to a single active local model-inference slot.

### Core components

- `src/types.go` – `Config`, `AppState`, shared runtime types.
- `src/server.go` – Desktop loopback HTTP/SSE API.
- `src/remote_server.go` – separate Mobile Remote server for pairing/token/SSE/Remote actions.
- `src/agent.go` – LocalCode Native main loop and tool dispatch.
- `src/agent_supervisor.go`, `src/edit_reliability.go`, `src/agent_loop_guard.go` – deterministic supervision, edit preflight, completion/no-progress controls.
- `src/subagent.go` – deterministic read-only repository handoff/fallback.
- `src/subagent_model.go` – model-backed read-only Explorer/Planner/Reviewer runtime.
- `src/agent_team_types.go` – roles, capabilities, budgets, usage, tasks and structured `AgentResult`.
- `src/agent_task_graph.go` – DAG validation, dependencies, readiness and state propagation.
- `src/agent_scheduler.go` – queue, resource limits, admission, cancellation and scheduler snapshots.
- `src/agent_scheduler_dispatch.go` – actual scheduled execution of authorized read-only children.
- `src/agent_scheduler_finalize.go` – serialized preparation/finalization against cancellation races.
- `src/run_journal.go` – durable active-run recovery authority.
- `src/path_tools.go` and file tools – canonical path/mutation boundaries.
- `src/mcp*.go`, `src/web_tools.go`, `src/tool_*` – external tool/network boundaries.
- `src/static/*` – Desktop/Remote web UIs and DE/EN catalogs.
- `android/app/.../MainActivity.java` – native Android shell with discovery, TLS pinning, WebView, file and speech bridge.

### Desktop and external engines

The Desktop UI talks only to the local loopback API. LocalCode owns project, thread, model, engine, active run, approvals and events. Aider, Claude Code, OpenCode and Claw Code run as controlled external integrations; LocalCode Native uses the built-in structured tool loop.

### Mobile Remote and Android

Remote is a separate server with a narrow API. Pairing creates a device token while LocalCode persists only its hash. The long-lived token is not placed in SSE URLs; streams use short-lived tickets.

The native Android shell is already implemented. It:

- discovers LocalCode via mDNS,
- consumes pair/QR/deep-link data,
- accepts only private HTTPS endpoints with the expected TLS SHA-256 fingerprint,
- connects WebView file inputs to Android's file picker,
- exposes a narrow `RecognizerIntent` speech bridge,
- cleans up WebView/file-chooser resources during Activity teardown.

The Android bridge does not execute LocalCode tools. It only returns files or recognized text to the loaded Remote web app. Tool authority remains on the Windows host.

### Native Agent Teams

#### Child runtime

Currently executable roles:

- Explorer
- Planner
- Reviewer

A child task has an objective, role, capabilities, budget and structured result. Its schema permits only project-tree reads, file reads, text search, approval-free read-only LSP and structured finish. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approval requests and recursive spawning are absent.

#### Task DAG

`AgentTaskGraph` contains a Mission ID and stable task IDs. Dependencies are validated; cycles, missing/self/duplicate dependencies and inconsistent states are rejected. Readiness/blocking is deterministically derived from dependency state.

#### Scheduler / Resource Manager

`AgentScheduler` separates the logical ready queue from resource admission. Resource classes currently include `model-inference`, `read-cpu`, `build` and `exclusive-integration`. Read-only children currently default to model inference, with one active model slot by default. Planner `RequestedCapabilities` never self-grant executable `Capabilities`; admission requires a known executable role plus actually granted capabilities.

#### Actual read-only dispatch

`runScheduledReadOnlyAgentGraph` connects the DAG/Scheduler to the child runtime:

1. ready tasks enter the bounded queue;
2. the scheduler chooses an admissible lease;
3. `prepareScheduledAgentTask` validates the lease under the scheduler lock, normalizes budget and creates a **detached task copy**;
4. the child executes outside the lock;
5. `finalizeScheduledAgentTask` competes with `CancelTask`/`CancelMission` under the same scheduler lock;
6. only the terminal winner may persist result/state and the lease is released exactly once;
7. dependencies are reconciled and the next task may be admitted.

The child therefore does not retain a pointer into the shared mutable graph during model execution. Cancellation-first discards late child results; completion-first remains successful.

### Recovery

`run_journal.go` is the existing recovery authority for active main-agent runs. Future durable Mission storage must integrate with this authority rather than creating a competing journal.

### Next architecture layer

The missing layer is a **product-level Mission Manager** between the main agent/Planner and DAG/Scheduler. It must explicitly validate which Planner proposals are allowed to execute. Mission budgets, UI/Remote state and durable Mission recovery come next; mutation-capable Builders in isolated Git worktrees come later.
