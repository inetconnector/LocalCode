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
- `src/agent_mission_status.go` – begrenzte, ephemere Mission-/Scheduler-Telemetrie für Desktop; keine Recovery-Autorität.
- `src/agent_orchestration_diagnostics.go` – abgeleitete Desktop-Orchestrierungsdiagnostik für Backend, Queue und Ressourcen; keine Scheduler-Policy.
- `src/agent_orchestration_parallelism_benchmark_test.go` – synthetischer Dispatcher- und opt-in Ollama-Parallelitätsbenchmark.
- `src/run_journal.go` – dauerhafte aktive Run-/Recovery-Autorität.
- `src/static/mission_status.js` – read-only Mission- und Orchestrierungsdiagnostik im Desktop-Output-Inspector.
- `src/static/*` – weitere Desktop-/Remote-Weboberflächen und DE/EN-Kataloge.
- `src/remote_mission_status_contract.md` – Source-Level-Vertrag für die schmale Mobile-Mission-Anzeige.
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproduzierbare Benchmark-Kommandos, Messwerte und Interpretationsgrenzen.
- `android/app/.../MainActivity.java` – native Android-Hülle.

### Native Agent Teams

Aktuell ausführbare Child-Rollen sind ausschließlich **Explorer**, **Planner** und **Reviewer**. Ihr Action-Schema ist read-only: Projektbaum, Dateien, Textsuche, genehmigungsfreies LSP und strukturiertes `finish`. Mutation, Shell, Git, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory, Approval-Requests und rekursives Spawning fehlen absichtlich.

`AgentTaskGraph` enthält eine stabile Mission-ID und stabile Task-IDs. Dependencies, Zyklen und Zustände werden validiert. Der `AgentScheduler` hält eine begrenzte Ready-Queue und Ressourcenklassen; read-only Child-Tasks verwenden derzeit standardmäßig `model-inference`, dessen Default-Limit eins ist.

Die Scheduler-Abnahme enthält inzwischen größere Sättigungs-/Fairness-Fälle: mehrere Ressourcenklassen können gleichzeitig gesättigt sein, zulässige Arbeit einer anderen Klasse darf blockierte ältere Arbeit umgehen, FIFO bleibt innerhalb derselben Ressourcenklasse erhalten und ein 14-Task-Fan-out/Fan-in-DAG muss ohne Starvation oder Ressourcenleck vollständig drainen.

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

Cancellation-first verwirft verspätete Child-Resultate. Completion-first bleibt erfolgreich. Wenn eine komplette Mission über Parent-Context bzw. `StopAgent` abgebrochen wird, terminalisiert die Mission-Grenze nach Ende des synchronen Dispatches zusätzlich alle noch unfertigen `ready`/`blocked`/sonst nicht terminalen Tasks als `cancelled`; bereits terminale erfolgreiche oder fehlgeschlagene Tasks bleiben unverändert. Der abschließende Scheduler-Snapshot wird danach erneut erzeugt, sodass Graph, Mission-Resultat und Desktop-Status denselben terminalen Zustand zeigen.

### Desktop Mission-Status

Der Desktop verwendet weiterhin `/api/status` als kanonische Statusquelle. `Status.MarshalJSON` ergänzt nur dann ein `mission`-Objekt, wenn zur aktuellen execution-scoped `RunID` passende Mission-Telemetrie vorliegt. Normale Runs oder fremde RunIDs erhalten kein Mission-Objekt.

Während einer read-only Mission publiziert ein begrenzter In-Memory-Monitor:

- stabile `MissionID` und execution-scoped `RunID`,
- Mission-State/-Reason,
- Queue-/Running-Zahlen,
- Ressourcenklasse, Limit und Belegung,
- Task-State, Queue-Position, Admission-Block-Grund und Task-Budget,
- Mission-Budget und Usage.

Die Registry ist auf wenige Einträge begrenzt und entfernt alte Beobachtungsdaten. Sie schreibt **nichts** dauerhaft und kann keine Mission starten, fortsetzen, wiederaufnehmen oder autorisieren. `src/static/mission_status.js` liest ausschließlich diese Statusdaten und rendert sie DE/EN im bestehenden Output-Inspector. Die Oberfläche besitzt in diesem Slice keinen Mission-Start-/Mutation-/Approval-Pfad.

### Orchestrierungs- und Sättigungsdiagnostik

`/api/status` enthält zusätzlich ein maschinenlesbares `orchestration`-Objekt. Es wird aus dem bereits gelesenen Ollama-/Modellzustand und – falls vorhanden – dem aktuellen ephemeren Mission-/Scheduler-Snapshot abgeleitet; es ist keine zweite Scheduler- oder Recovery-Quelle.

Die Diagnose unterscheidet `ready`, `active`, `saturated`, `backend_unavailable` und `model_unavailable`. Gründe unterscheiden insbesondere Ollama offline, kein ausgewähltes Modell, ein lokal nicht vorhandenes ausgewähltes Modell, eine laufende Mission, ein erreichtes Queue-Limit und wartende Arbeit auf einer ausgelasteten Ressourcenklasse.

Pro Ressourcenklasse werden Limit, Belegung, freie Slots und wartende Tasks ausgewiesen. **`at_capacity` und `saturated` sind absichtlich verschieden:** `at_capacity` bedeutet nur, dass alle Slots belegt sind; `saturated` gilt erst, wenn zusätzlich passende Arbeit auf diese volle Ressource wartet. Queue-Auslastung sowie logische Ready-/Running-/Blocked-Zahlen werden separat ausgewiesen. Tatsächliche normalisierte Mission-Ressourcenlimits werden im ephemeren Mission-Status mitgeführt, damit die Diagnose während einer Mission nicht still Standardlimits annimmt.

Die Diagnose verändert weder Queue-Limits noch Admission noch Modellparallelität. Sie startet keine Arbeit und ist keine Performance-Evidenz. Benchmark-Messung erfolgt separat über die nachfolgend beschriebene Benchmark-Grenze. `src/static/mission_status.js` rendert die Diagnose read-only im bestehenden Desktop-Output-Inspector; Mobile erhält diesen Desktop-Payload nicht.

### Orchestrierungs-Benchmarks

`BenchmarkScheduledReadOnlyDispatcherParallelism` trennt bewusst drei Ebenen: vier gleichzeitig logisch bereite read-only Tasks, konfigurierte `model-inference`-Slot-Kapazität und tatsächlich beobachtete Executor-Überlappung. Die aktuelle `runScheduledReadOnlyAgentGraphWithExecutor`-Schleife ruft den Executor synchron auf. Ein Model-Inference-Limit größer eins ist deshalb heute allein kein Beleg paralleler Child-Modellaufrufe. Der synthetische Executor enthält eine feste kurze Verzögerung, sodass eine spätere echte Überlappung im selben Messformat sichtbar würde.

`TestOllamaConcurrencyBenchmarkOptIn` misst zusätzlich ein festes reales Ollama-Arbeitsvolumen bei Client-Konkurrenz 1/2/4 über denselben produktiven `OllamaClient.Chat`-Pfad. Dieser Lauf ist explizit opt-in, akzeptiert nur Loopback, verlangt ein bereits installiertes exaktes Modell und ruft weder `EnsureRunning` noch `Pull` auf. Er misst End-to-End-Wall-Time, Latenzen, Requests/Sekunde und Client-Overlap. Daraus wird **keine** gleichzeitige GPU-Kernel- oder Token-Inferenz abgeleitet, da Ollama intern queuen oder batchen kann.

Benchmarkresultate sind Evidenz, aber keine Scheduler-Policy. Eine spätere Änderung von Model-Slots ist ein separater, reviewpflichtiger Change. Befehle, Parametergrenzen und Interpretationsregeln stehen in `docs/ORCHESTRATION_BENCHMARKS.md`.

### Mobile Mission-Status

Die Mobile Remote bleibt absichtlich enger als Desktop. Sie verwendet **keinen** neuen Mission-Endpunkt und bekommt nicht das Desktop-`mission`-Objekt. Stattdessen nutzt `src/static/remote.html` ausschließlich die bereits authentifizierten Felder `running` und `run_phase` aus `/remote/api/status`.

Wenn `running == true` und `run_phase == "mission-read-only"`, zeigt die Remote-Oberfläche im Header sowie in der Tasks-Ansicht lediglich an, dass eine read-only Mission läuft. Nicht übertragen werden Mission-/Task-IDs, Scheduler-/Queue-/Ressourcendetails, Mission-/Task-Budgets, Usage/Accounting oder neue Mission-Start-/Spawn-/Retry-/Resume-Aktionen. Das bestehende Remote-Stop-Verhalten bleibt unverändert; dieser Slice fügt keine neue Authority hinzu.

### Recovery und nächste Stufen

`run_journal.go` bleibt die einzige Recovery-Autorität. Missionen besitzen noch keine dauerhafte eigene Recovery-Persistenz; die spätere Phase muss in diesen Pfad integriert werden und darf kein konkurrierendes Journal erzeugen. Die Desktop-Telemetrie aus `agent_mission_status.go`, die Orchestrierungsdiagnostik und die Mobile-Anzeige sind ausdrücklich keine Recovery-Speicher.

Als Nächstes folgt dauerhafte Mission-Metadaten-/Recovery-Integration in `run_journal.go` mit Restart-Reconciliation. Mutation-capable Builder in isolierten Git-Worktrees kommen erst danach.

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
- `src/agent_mission_status.go` – bounded ephemeral Mission/scheduler telemetry for Desktop; not a recovery authority.
- `src/agent_orchestration_diagnostics.go` – derived Desktop orchestration diagnostics for backend, queue and resources; not Scheduler policy.
- `src/agent_orchestration_parallelism_benchmark_test.go` – synthetic dispatcher and opt-in Ollama parallelism benchmark.
- `src/run_journal.go` – durable active-run recovery authority.
- `src/static/mission_status.js` – read-only Mission and orchestration diagnostics in the Desktop Output inspector.
- `src/static/*` – other Desktop/Remote UIs and DE/EN catalogs.
- `src/remote_mission_status_contract.md` – source-level contract for the narrow Mobile Mission display.
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproducible benchmark commands, measurements and interpretation boundaries.
- `android/app/.../MainActivity.java` – native Android shell.

### Native Agent Teams

The only executable child roles are **Explorer**, **Planner** and **Reviewer**. Their action schema is read-only: project tree, file reads, text search, approval-free LSP and structured `finish`. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approval requests and recursive spawning are intentionally absent.

`AgentTaskGraph` contains a stable Mission ID and stable task IDs. Dependencies, cycles and states are validated. `AgentScheduler` owns a bounded ready queue and resource classes; read-only children currently default to `model-inference`, whose default limit is one.

Scheduler acceptance now includes larger saturation/fairness cases: multiple resource classes can be saturated simultaneously, admissible work in another class may bypass older work blocked by its saturated class, FIFO is preserved within a resource class, and a 14-task fan-out/fan-in DAG must drain without starvation or resource leakage.

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

Cancellation-first discards late child results. Completion-first remains successful. When a whole Mission is cancelled through its parent context or `StopAgent`, the Mission boundary also terminalizes every still-unfinished `ready`/`blocked`/other non-terminal task as `cancelled` after synchronous dispatch has stopped; already-terminal successful or failed work is preserved. The terminal scheduler snapshot is refreshed afterwards so the graph, Mission result and Desktop status expose the same terminal state.

### Desktop Mission status

Desktop continues to use `/api/status` as its canonical status source. `Status.MarshalJSON` adds a `mission` object only when Mission telemetry matches the current execution-scoped `RunID`. Normal runs and unrelated RunIDs do not receive Mission data.

While a read-only Mission is executing, a bounded in-memory monitor publishes:

- stable `MissionID` and execution-scoped `RunID`,
- Mission state/reason,
- queued/running counts,
- resource class, limits and usage,
- task state, queue position, admission-block reason and task budget,
- Mission budget and usage.

The registry retains only a bounded number of observations and evicts old data. It writes **nothing** durably and cannot start, continue, resume or authorize a Mission. `src/static/mission_status.js` only reads this status data and renders a DE/EN card in the existing Output inspector. This slice contains no Mission-start, mutation or approval path.

### Orchestration and saturation diagnostics

`/api/status` also contains a machine-readable `orchestration` object. It is derived from the already-read Ollama/model state and, when present, the current ephemeral Mission/Scheduler snapshot; it is not a second Scheduler or recovery source.

Diagnostics distinguish `ready`, `active`, `saturated`, `backend_unavailable` and `model_unavailable`. Reasons separately identify Ollama offline, no selected model, a selected model missing locally, a running Mission, a reached queue limit and queued work waiting on a full resource class.

For each resource class, diagnostics report limit, in-use, available and waiting work. **`at_capacity` intentionally differs from `saturated`:** `at_capacity` only means every slot is occupied; `saturated` requires both a full resource and compatible work waiting for it. Queue utilization and logical ready/running/blocked counts are reported separately. Actual normalized Mission resource limits are retained in ephemeral Mission status so active-Mission diagnostics do not silently assume defaults.

Diagnostics do not alter queue limits, admission or model concurrency. They cannot start work and are not performance evidence. Benchmark measurement is handled separately by the benchmark boundary below. `src/static/mission_status.js` renders these diagnostics read-only in the existing Desktop Output inspector; Mobile does not receive this Desktop payload.

### Orchestration benchmarks

`BenchmarkScheduledReadOnlyDispatcherParallelism` deliberately separates three layers: four simultaneously logically-ready read-only tasks, configured `model-inference` slot capacity and actually observed executor overlap. The current `runScheduledReadOnlyAgentGraphWithExecutor` loop invokes the executor synchronously. A model-inference limit greater than one therefore is not by itself evidence of parallel child-model calls. The synthetic executor has a fixed short delay so future real overlap becomes visible in the same measurement format.

`TestOllamaConcurrencyBenchmarkOptIn` additionally measures a fixed real Ollama workload at client concurrency 1/2/4 through the same production `OllamaClient.Chat` path. This run is explicitly opt-in, accepts loopback only, requires an exact already-installed model and calls neither `EnsureRunning` nor `Pull`. It measures end-to-end wall time, latency, requests/second and client overlap. It does **not** infer simultaneous GPU-kernel or token inference because Ollama may queue or batch internally.

Benchmark results are evidence, not Scheduler policy. A later model-slot change is a separate reviewed change. Commands, parameter bounds and interpretation rules are documented in `docs/ORCHESTRATION_BENCHMARKS.md`.

### Mobile Mission status

Mobile Remote deliberately remains narrower than Desktop. It adds **no** Mission endpoint and does not receive the Desktop `mission` object. `src/static/remote.html` uses only the already-authenticated `running` and `run_phase` fields from `/remote/api/status`.

When `running == true && run_phase == "mission-read-only"`, Remote only indicates an active read-only Mission in its header and Tasks view. It does not receive Mission/task IDs, scheduler/queue/resource details, Mission/task budgets, usage/accounting or new Mission start/spawn/retry/resume actions. Existing Remote stop behavior is unchanged; this slice adds no new authority.

### Recovery and next layers

`run_journal.go` remains the sole recovery authority. Missions do not yet have durable recovery persistence; the later Mission-recovery phase must integrate with this path rather than create a competing journal. Desktop telemetry, orchestration diagnostics and the Mobile indicator are explicitly not recovery stores.

Next is durable Mission metadata/recovery integration with `run_journal.go` plus restart reconciliation. Mutation-capable Builders in isolated Git worktrees come later.