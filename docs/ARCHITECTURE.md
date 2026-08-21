# Architecture / Architektur

## Deutsch

### Systemübersicht

LocalCode ist eine Windows-first Go-Anwendung mit eingebettetem Web-Frontend.

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native oder externe Engine -> kontrollierte Tools -> Verifikation/Recovery`

`Android/Browser Remote -> separater token-geschützter Remote-Server -> engere AppState-/Statusoberfläche`

Für Native Agent Teams gilt:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

LocalCode trennt bewusst **logische Agentenparallelität** von **tatsächlicher Modellinferenzparallelität**. Viele DAG-Tasks können bereit sein; lokale Modellinferenz bleibt standardmäßig auf einen aktiven Model-Slot begrenzt.

### Zentrale Komponenten

- `src/server.go` – Desktop Loopback HTTP/SSE API.
- `src/remote_server.go` – separater Mobile-Remote-Server.
- `src/agent.go` – LocalCode-Native-Hauptschleife und Werkzeugdispatch.
- `src/subagent_model.go` – read-only Explorer/Planner/Reviewer-Runtime.
- `src/agent_team_types.go` – Rollen, Capabilities, Budget, Usage, Task und `AgentResult`.
- `src/agent_task_graph.go` – DAG-Validierung, Dependencies und Zustandspropagation.
- `src/agent_scheduler.go` – Queue, Ressourcenlimits, Admission, Cancellation und Snapshots.
- `src/agent_scheduler_dispatch.go` / `src/agent_scheduler_finalize.go` – tatsächliche, race-sichere Child-Ausführung.
- `src/agent_mission.go` – explizite Governance-/Mission-Einstiegsgrenze.
- `src/agent_mission_accounting.go` – Mission-Usage, Wall-Time, Budget und Terminalgründe.
- `src/agent_mission_cancel.go` – Produktgrenzen-Cancel für unfertige Mission-Tasks.
- `src/agent_mission_status.go` – begrenzte ephemere Desktop-Mission-Telemetrie.
- `src/run_journal.go` – einzige dauerhafte aktive Run-/Recovery-Autorität.
- `src/static/mission_status.js` – read-only Desktop-Mission-Card.
- `src/static/remote.html` – schmalere Mobile-Remote-Oberfläche.
- `src/remote_mission_status_contract.md` – Source-Level-Vertrag für die schmale Mobile-Mission-Anzeige.

### Native Agent Teams und Mission Manager

Aktuell ausführbare Child-Rollen sind ausschließlich **Explorer**, **Planner** und **Reviewer**. Ihr Action-Schema ist read-only: Projektbaum, Dateien, Textsuche, genehmigungsfreies LSP und strukturiertes `finish`. Mutation, Shell, Git, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory, Approval-Requests und rekursives Spawning fehlen absichtlich.

Der produktseitige read-only Mission-Einstieg validiert Mission-/Task-IDs, direkte Projektgrenze, DAG, ausführbare Rollen und Requested-Capability-Envelope. Planner-Vorschläge sind ohne diese Governance-Grenze nicht ausführbar.

`MissionID` ist stabile Produktidentität. `AppState.RunID` ist ein frischer execution-scoped Token für aktive Run-/Stop-/Journal-Hooks. Missionweite Usage wird aus Scheduler-akzeptiertem `UsageByTask` aggregiert; Mission-Budgets dürfen Child-Budgets nur weiter einschränken.

### Scheduler, Fairness und Cancellation

Ready-Queue und Ressourcenadmission sind getrennt. Sättigungs-/Fairness-Tests prüfen Cross-Class-Bypass, FIFO innerhalb derselben Ressourcenklasse und einen 14-Task-Fan-out/Fan-in-DAG ohne Starvation.

Scheduled Children laufen auf abgetrennten Task-Kopien. Preparation, Finalize und Cancellation konkurrieren an derselben Scheduler-Lock-Grenze. Cancellation-first verwirft verspätete Child-Resultate; Completion-first bleibt erfolgreich. Ein vollständiger Mission-Cancel terminalisiert alle noch unfertigen Tasks kontrolliert als `cancelled` und erzeugt danach einen konsistenten finalen Scheduler-Snapshot.

### Desktop Mission-Status

Desktop verwendet `/api/status` als kanonische Statusquelle. Ein `mission`-Objekt wird nur ergänzt, wenn Mission-Telemetrie zur aktuellen execution-scoped `RunID` passt. Die bounded In-Memory-Registry enthält Live-/Terminalzustand, Scheduler-Ressourcen, Tasks und Budgets, schreibt aber nichts dauerhaft und kann keine Mission starten, fortsetzen oder autorisieren.

### Mobile Mission-Status

Mobile bleibt absichtlich **schmaler als Desktop**. Die Remote-UI nutzt ausschließlich die bereits vorhandenen authentifizierten `/remote/api/status`-Felder:

- `running`
- `run_phase`

Nur wenn `running == true` und `run_phase == "mission-read-only"`, zeigt Mobile im Header und in der Tasks-Ansicht an, dass eine read-only Mission läuft. Es gibt dafür keinen neuen Remote-Endpunkt und keinen neuen Control-Pfad.

Nicht an Mobile übertragen werden durch diese Erweiterung:

- `MissionID` oder Task-IDs,
- Scheduler-/Queue-/Ressourcendetails,
- Mission-/Task-Budgets oder Accounting,
- Mission-Start-/Spawn-/Retry-/Resume-Aktionen,
- zusätzliche Tool-, Datei-, Shell-, Git-, Netzwerk- oder Approval-Rechte.

Das bereits existierende Remote-Stop-Verhalten bleibt unverändert und ist keine neue Authority dieses Slices.

### Recovery und nächste Stufen

`run_journal.go` bleibt die einzige Recovery-Autorität. Desktop- und Mobile-Mission-Anzeigen sind reine, ephemere Beobachtung.

Als Nächstes folgen Modell-/Ressourcen-Sättigungsdiagnostik, reproduzierbare Parallelitätsbenchmarks und danach dauerhafte Mission-Recovery integriert in `run_journal.go`. Mutation-capable Builder in isolierten Git-Worktrees kommen erst danach.

---

## English

### System overview

LocalCode is a Windows-first Go application with an embedded web frontend.

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native or external engine -> controlled tools -> verification/recovery`

`Android/browser Remote -> separate token-protected Remote server -> narrower AppState/status surface`

For Native Agent Teams:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

LocalCode deliberately separates **logical agent parallelism** from **actual model-inference parallelism**. Many DAG tasks may be ready while local inference defaults to one active model slot.

### Core components

- `src/server.go` – Desktop loopback HTTP/SSE API.
- `src/remote_server.go` – separate Mobile Remote server.
- `src/agent.go` – LocalCode Native main loop/tool dispatch.
- `src/subagent_model.go` – read-only Explorer/Planner/Reviewer runtime.
- `src/agent_team_types.go` – roles, capabilities, budgets, usage, tasks and `AgentResult`.
- `src/agent_task_graph.go` – DAG validation/dependency/state propagation.
- `src/agent_scheduler.go` – queue, resource limits, admission, cancellation and snapshots.
- `src/agent_scheduler_dispatch.go` / `src/agent_scheduler_finalize.go` – actual race-safe child execution.
- `src/agent_mission.go` – explicit governance/Mission entry boundary.
- `src/agent_mission_accounting.go` – Mission usage, wall time, budgets and terminal reasons.
- `src/agent_mission_cancel.go` – product-boundary cancellation of unfinished Mission tasks.
- `src/agent_mission_status.go` – bounded ephemeral Desktop Mission telemetry.
- `src/run_journal.go` – sole durable active-run recovery authority.
- `src/static/mission_status.js` – read-only Desktop Mission card.
- `src/static/remote.html` – narrower Mobile Remote UI.
- `src/remote_mission_status_contract.md` – source-level contract for Mobile Mission observation.

### Native Agent Teams and Mission Manager

The only executable child roles are **Explorer**, **Planner** and **Reviewer**. Their action schema is read-only: project tree, file reads, text search, approval-free LSP and structured `finish`. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approval requests and recursive spawning are intentionally absent.

The product-level read-only Mission entry validates Mission/task IDs, direct project boundary, DAG, executable roles and requested-capability envelope. Planner proposals are not executable without this governance boundary.

`MissionID` is stable product identity while `AppState.RunID` is a fresh execution-scoped token. Mission usage is aggregated from scheduler-accepted `UsageByTask`; Mission budgets may only tighten child budgets.

### Scheduler, fairness and cancellation

Ready queue and resource admission are separate. Acceptance tests cover cross-class bypass, FIFO inside a resource class and a 14-task fan-out/fan-in DAG without starvation.

Scheduled children execute on detached task copies. Preparation, finalization and cancellation meet at the same scheduler-lock boundary. Cancellation-first discards late child results; completion-first remains successful. Whole-Mission cancellation terminalizes every unfinished task as `cancelled` and then refreshes the terminal scheduler snapshot.

### Desktop Mission status

Desktop continues to use `/api/status` as the canonical status source. A `mission` object is attached only when telemetry matches the current execution-scoped `RunID`. The bounded in-memory registry contains live/terminal state, scheduler resources, tasks and budgets but is non-durable and cannot start, continue or authorize a Mission.

### Mobile Mission status

Mobile deliberately remains **narrower than Desktop**. Remote uses only the already-authenticated `/remote/api/status` fields `running` and `run_phase`.

Only when `running == true` and `run_phase == "mission-read-only"` does Mobile show an active read-only Mission indicator in its header and Tasks view. No new Remote endpoint or control path is added.

This extension does not expose Mission/task IDs, scheduler/queue/resource details, budgets/accounting, Mission start/spawn/retry/resume actions, or additional tool/file/shell/Git/network/approval authority. Existing Remote stop behavior is unchanged and is not new authority from this slice.

### Recovery and next layers

`run_journal.go` remains the sole recovery authority. Desktop and Mobile Mission displays are ephemeral observation only.

Next come model/resource saturation diagnostics, reproducible parallelism benchmarks and then durable Mission recovery integrated with `run_journal.go`. Mutation-capable Builders in isolated Git worktrees come later.
