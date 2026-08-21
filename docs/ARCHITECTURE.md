# Architecture / Architektur

## Deutsch

### Systemübersicht

LocalCode ist eine Windows-first Go-Anwendung mit eingebettetem Web-Frontend. Die zentralen Laufzeitpfade sind:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native oder externe Engine -> kontrollierte Tools -> Verifikation/Recovery`

und für Mobilgeräte:

`Android/Browser Remote -> separater token-geschützter Remote-Server -> dieselben AppState-Operationen mit engerer Berechtigungsoberfläche`

Für Native Agent Teams gilt:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

LocalCode trennt bewusst **logische Agentenparallelität** von **tatsächlicher Modellinferenzparallelität**. Viele DAG-Tasks können bereit sein; lokale Modellinferenz bleibt standardmäßig auf einen aktiven Model-Slot begrenzt.

### Zentrale Komponenten

- `src/types.go` – `Config`, `AppState`, gemeinsame Laufzeittypen.
- `src/server.go` – Desktop Loopback HTTP/SSE API.
- `src/remote_server.go` – separater Mobile-Remote-Server.
- `src/agent.go` – Hauptschleife von LocalCode Native und Werkzeugdispatch.
- `src/agent_supervisor.go`, `src/edit_reliability.go`, `src/agent_loop_guard.go` – deterministische Steuerung und Reliability-Grenzen.
- `src/subagent_model.go` – modellgestützte read-only Explorer/Planner/Reviewer-Runtime.
- `src/agent_team_types.go` – Rollen, Capabilities, Budget, Usage, Task und `AgentResult`.
- `src/agent_task_graph.go` – DAG-Validierung, Dependencies und Zustandspropagation.
- `src/agent_scheduler.go` – Queue, Ressourcenlimits, Admission, Cancellation und Snapshots.
- `src/agent_scheduler_dispatch.go` – tatsächliche Scheduler-Ausführung autorisierter read-only Tasks.
- `src/agent_scheduler_finalize.go` – serialisierte Vorbereitung/Finalisierung gegen Cancel-Races.
- `src/agent_mission.go` – explizite Governance-/Mission-Einstiegsgrenze.
- `src/agent_mission_accounting.go` – missionweite Usage, Wall-Time, Budget und Terminalgründe.
- `src/agent_mission_cancel.go` – Produktgrenzen-Cancel für noch nicht terminale Mission-Tasks.
- `src/run_journal.go` – dauerhafte aktive Run-/Recovery-Autorität.
- `src/static/*` – Desktop-/Remote-Weboberflächen und DE/EN-Kataloge.
- `android/app/.../MainActivity.java` – native Android-Hülle.

### Native Agent Teams

Aktuell ausführbare Child-Rollen sind ausschließlich **Explorer**, **Planner** und **Reviewer**. Ihr Action-Schema ist read-only: Projektbaum, Dateien, Textsuche, genehmigungsfreies LSP und strukturiertes `finish`. Mutation, Shell, Git, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory, Approval-Requests und rekursives Spawning fehlen absichtlich.

`AgentTaskGraph` enthält eine stabile Mission-ID und stabile Task-IDs. Dependencies, Zyklen und Zustände werden validiert. Der `AgentScheduler` hält eine begrenzte Ready-Queue und Ressourcenklassen; read-only Child-Tasks verwenden derzeit standardmäßig `model-inference`, dessen Default-Limit eins ist.

### Governed Mission Manager

Der produktseitige read-only Mission-Einstieg ist implementiert. Eine Mission wird **nicht** allein deshalb ausgeführt, weil ein Planner Tasks vorgeschlagen hat. Ein expliziter Mission-Request wird zuerst auf Mission-/Task-IDs, direkte Projektgrenze, DAG, ausführbare Rollen und Requested-Capability-Envelope geprüft. Erst diese vertrauenswürdige Grenze vergibt die festen read-only Runtime-Capabilities.

`MissionID` ist stabile Produktidentität. Der gemeinsame `AppState.RunID` ist dagegen ein frischer execution-scoped Token für Stop-/Journal-Hooks. Dadurch kann eine vom Aufrufer gewählte Mission-ID nicht mit einem älteren Run-Journal-Identifier kollidieren.

Missionweite Usage wird ausschließlich aus vom Scheduler akzeptiertem `UsageByTask` aggregiert. Model-/Tool-Calls und geschätzte Tokens sind additiv; echte Mission-Wall-Time wird getrennt von aufsummierter Child-Arbeitszeit geführt. Optionale Mission-Budgets dürfen normalisierte Child-Budgets nur weiter einschränken, niemals erhöhen.

### Dispatch und Cancellation

`runScheduledReadOnlyAgentGraph` verbindet DAG und Scheduler mit der bestehenden Child-Runtime:

1. Ready-Tasks werden gequeue-t.
2. Der Scheduler vergibt einen zulässigen Lease.
3. `prepareScheduledAgentTask` erstellt unter Scheduler-Lock eine abgetrennte Task-Kopie.
4. Der Child läuft außerhalb des Locks.
5. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder an derselben Lock-Grenze.
6. Nur der terminale Gewinner darf Resultat/Usage festschreiben und den Lease freigeben.

Cancellation-first verwirft verspätete Child-Resultate. Completion-first bleibt erfolgreich. Wenn eine komplette Mission über Parent-Context bzw. `StopAgent` abgebrochen wird, terminalisiert die Mission-Grenze nach Ende des synchronen Dispatches zusätzlich alle noch unfertigen `ready`/`blocked`/sonst nicht terminalen Tasks als `cancelled`; bereits terminale erfolgreiche oder fehlgeschlagene Tasks bleiben unverändert.

### Recovery und nächste Stufen

`run_journal.go` bleibt die einzige Recovery-Autorität. Missionen besitzen noch keine dauerhafte eigene Recovery-Persistenz; die spätere Phase muss in diesen Pfad integriert werden und darf kein konkurrierendes Journal erzeugen.

Als Nächstes folgen größere DAG-Saturation/Fairness-Tests, ein stabiler Desktop-Mission-Status, danach eine engere read-only Mobile-Ansicht, Ressourcen-Diagnostik/Benchmarks und dauerhafte Mission-Recovery. Mutation-capable Builder in isolierten Git-Worktrees kommen erst danach.

---

## English

### System overview

LocalCode is a Windows-first Go application with an embedded web frontend. Its main runtime paths are:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native or external engine -> controlled tools -> verification/recovery`

and for phones:

`Android/browser Remote -> separate token-protected Remote server -> the same AppState operations through a narrower permission surface`

For Native Agent Teams the current path is:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

LocalCode deliberately separates **logical agent parallelism** from **actual model-inference parallelism**. Many DAG tasks may be ready while local inference defaults to one active model slot.

### Core components

- `src/types.go` – `Config`, `AppState`, shared runtime types.
- `src/server.go` – Desktop loopback HTTP/SSE API.
- `src/remote_server.go` – separate Mobile Remote server.
- `src/agent.go` – LocalCode Native main loop and tool dispatch.
- `src/agent_supervisor.go`, `src/edit_reliability.go`, `src/agent_loop_guard.go` – deterministic supervision and reliability boundaries.
- `src/subagent_model.go` – model-backed read-only Explorer/Planner/Reviewer runtime.
- `src/agent_team_types.go` – roles, capabilities, budgets, usage, tasks and `AgentResult`.
- `src/agent_task_graph.go` – DAG validation, dependencies and state propagation.
- `src/agent_scheduler.go` – queue, resource limits, admission, cancellation and snapshots.
- `src/agent_scheduler_dispatch.go` – actual scheduled execution of authorized read-only tasks.
- `src/agent_scheduler_finalize.go` – serialized preparation/finalization against cancellation races.
- `src/agent_mission.go` – explicit governance/Mission entry boundary.
- `src/agent_mission_accounting.go` – Mission usage, wall time, budget and terminal reasons.
- `src/agent_mission_cancel.go` – product-boundary cancellation for unfinished Mission tasks.
- `src/run_journal.go` – durable active-run recovery authority.
- `src/static/*` – Desktop/Remote UIs and DE/EN catalogs.
- `android/app/.../MainActivity.java` – native Android shell.

### Native Agent Teams

The only executable child roles are **Explorer**, **Planner** and **Reviewer**. Their action schema is read-only: project tree, file reads, text search, approval-free LSP and structured `finish`. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approval requests and recursive spawning are intentionally absent.

`AgentTaskGraph` contains a stable Mission ID and stable task IDs. Dependencies, cycles and states are validated. `AgentScheduler` owns a bounded ready queue and resource classes; read-only children currently default to `model-inference`, whose default limit is one.

### Governed Mission Manager

The product-level read-only Mission entry is implemented. A Mission is **not** executed merely because a Planner proposed tasks. An explicit Mission request is first validated for Mission/task IDs, direct project boundary, DAG, executable roles and requested-capability envelope. Only this trusted boundary grants the fixed read-only runtime capabilities.

`MissionID` is stable product identity. The shared `AppState.RunID` is instead a fresh execution-scoped token used by stop/journal hooks, preventing a caller-selected Mission ID from colliding with an older run-journal identifier.

Mission-wide usage is aggregated only from scheduler-accepted `UsageByTask`. Model/tool calls and estimated tokens are additive; actual Mission wall time is kept separate from summed child-work time. Optional Mission budgets may only further constrain normalized child budgets and can never widen them.

### Dispatch and cancellation

`runScheduledReadOnlyAgentGraph` connects DAG/Scheduler to the existing child runtime:

1. ready tasks enter the queue;
2. the scheduler grants an admissible lease;
3. `prepareScheduledAgentTask` creates a detached task copy under the scheduler lock;
4. the child executes outside the lock;
5. `finalizeScheduledAgentTask`, `CancelTask` and `CancelMission` compete again at the same lock boundary;
6. only the terminal winner may persist result/usage and release the lease.

Cancellation-first discards late child results. Completion-first remains successful. When a whole Mission is cancelled through its parent context or `StopAgent`, the Mission boundary also terminalizes every still-unfinished `ready`/`blocked`/other non-terminal task as `cancelled` after synchronous dispatch has stopped; already-terminal successful or failed work is preserved.

### Recovery and next layers

`run_journal.go` remains the sole recovery authority. Missions do not yet have durable recovery persistence; the later Mission-recovery phase must integrate with this path rather than create a competing journal.

Next come larger DAG saturation/fairness tests, a stable Desktop Mission status surface, then a narrower read-only Mobile view, resource diagnostics/benchmarks and durable Mission recovery. Mutation-capable Builders in isolated Git worktrees come later.
