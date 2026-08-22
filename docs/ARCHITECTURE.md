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
- `src/agent_scheduler_dispatch.go` – tatsächliche Scheduler-Ausführung autorisierter read-only Tasks und durable Mission-Checkpoint-Hooks.
- `src/agent_scheduler_finalize.go` – serialisierte Vorbereitung/Finalisierung gegen Cancel-Races.
- `src/agent_mission.go` – explizite Governance-/Mission-Einstiegsgrenze.
- `src/agent_mission_accounting.go` – missionweite Usage, Wall-Time, Budget und Terminalgründe.
- `src/agent_mission_cancel.go` – Produktgrenzen-Cancel für noch nicht terminale Mission-Tasks.
- `src/run_journal_mission.go` – begrenzte strukturierte Mission-Metadaten, Projekt-/Git-Baseline und Journal-Checkpoint-Abbildung; kein Auto-Resume.
- `src/run_journal_mission_reconcile.go` – read-only Restart-Reconciliation gegen Projektidentität, Git-HEAD und gehashten Worktree-Zustand; keine Resume-/Retry-Authority.
- `src/run_journal_mission_postcondition_verify.go` – deterministische read-only Postcondition-Verifikation für durable erfolgreiche Recovery-Tasks; keine Child-/Modell-Ausführung und keine Resume-/Retry-Authority.
- `src/run_journal_mission_transition_plan.go` – deterministische Recovery-Transition-Klassifikation mit Dependency- und Attempt-Grenzen; reine Planung ohne Scheduler-Admission oder Ausführung.
- `src/agent_mission_status.go` – begrenzte, ephemere Mission-/Scheduler-Telemetrie für Desktop; keine Recovery-Autorität.
- `src/agent_orchestration_diagnostics.go` – abgeleitete Desktop-Orchestrierungsdiagnostik für Backend, Queue und Ressourcen; keine Scheduler-Policy.
- `src/agent_orchestration_parallelism_benchmark_test.go` – synthetischer Dispatcher- und opt-in Ollama-Parallelitätsbenchmark.
- `src/run_journal.go` – einzige dauerhafte aktive Run-/Mission-Recovery-Autorität (`active-run.json`) und Startup-Einstieg für die Mission-Reconciliation.
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

Die Status-Registry ist auf wenige Einträge begrenzt und entfernt alte Beobachtungsdaten. Sie schreibt **selbst nichts dauerhaft** und kann keine Mission starten, fortsetzen, wiederaufnehmen oder autorisieren. Durable Mission-Checkpoints laufen getrennt davon ausschließlich über `run_journal.go`. `src/static/mission_status.js` liest nur Statusdaten und rendert sie DE/EN im bestehenden Output-Inspector.

### Durable Mission-Checkpoints, Verifikation und Transition-Planung

Eine validierte read-only Mission hängt ihre Recovery-Metadaten an denselben `RunRecoveryState`, den normale Runs bereits unter `active-run.json` verwenden. Es gibt kein zweites Mission-Journal.

Der optionale Mission-Checkpoint enthält begrenzt und strukturiert:

- stabile `MissionID`, Objective, direkte Projekt-/Scope-Identität, Model, Constraints und Success Criteria,
- Mission-Budget,
- Task-ID, Parent/Dependencies, Rolle, Zustand/Grund, Requested-/Granted-Capabilities, Model und Task-Budget,
- Scheduler-Ressourcenklasse, Queue-Position, Running-Flag und Budget-Snapshot,
- Completion-/Lifecycle-/Verification-Evidenz,
- finalen Mission-State/-Reason, Mission-Accounting und ausschließlich vom Scheduler akzeptierte `UsageByTask`.

Beim Missionsstart wird zusätzlich eine begrenzte Reconciliation-Baseline festgehalten: SHA-256 der kanonischen Projektidentität, Git-Beobachtungszustand, SHA-256 der Git-Root-Identität, exaktes `HEAD`, SHA-256 der Bytes aus `git status --porcelain=v1 -z --untracked-files=all` sowie der Erfassungszeitpunkt. Rohe Porcelain-Ausgabe und damit Dateipfade werden nicht in den Checkpoint geschrieben.

Der Git-Observer ist ein privater, fest verdrahteter read-only Pfad. Er akzeptiert keinen freien Command-Text und führt mit einem Drei-Sekunden-Timeout ausschließlich `rev-parse --show-toplevel`, `rev-parse --verify HEAD` und den genannten Porcelain-Status aus. Diese Beobachtung erteilt keine Git-, Shell- oder Scheduler-Berechtigung.

Nach einem Prozessabbruch wird eine nicht-terminale Mission gegen die aktuelle Projekt-/Git-Sicht abgeglichen. Der Zustand ist `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence`. Ein beim Abbruch laufender Task bleibt `interrupted_unknown`. Durable erfolgreiche Tasks benötigen begrenzte Verifikation; ein historisches `verified` ist nur wiederverwendbar, wenn die **aktuelle** Reconciliation weiterhin `matched` ist.

Der interne Postcondition-Verifier führt eine frische read-only Projekt-/Git-Beobachtung durch und prüft feste Recovery-Fakten: aktuelle Reconciliation `matched`, Task nicht laufend, durable Success-State, vorhandene Completion-Evidenz, erfolgreicher Completion-Status und kanonischer Completion-SHA-256. Der Verification-Digest bindet Mission-/Task-ID, Completion-Digest, aktuelle gehashte Projekt-/Git-Identität, `HEAD`, gehashten Porcelain-Status und feste Check-Bools. Rohpfade, Child-/Modelloutput und rohe Verifikationsausgabe werden nicht persistiert. Eine optimistische Journal-Precondition verhindert einen Write, wenn sich der Mission-Recovery-Zustand während der Beobachtung geändert hat.

Auf dieser Evidenz erzeugt der Transition-Planer ausschließlich Klassifikation. Er rekonstruiert den durable DAG und validiert ihn mit der bestehenden Graph-Validierung. Duplicate IDs, fehlende Dependencies, Zyklen, ungültige Task-Metadaten, mehr als 64 Tasks oder inkonsistente/überhöhte Lifecycle-Zähler führen zu einem invaliden Plan; dann wird ausschließlich `invalid_recovery_state` ausgegeben.

Bei gültigem Plan sind unter anderem `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, `interrupted_review_required`, `preserve_terminal` sowie blockierende Reconciliation-/Dependency-/Attempt-Zustände möglich. Für jede Wiederverwendungs- oder spätere Ausführungskandidatur müssen alle Dependencies aktuell `verified` und wiederverwendbar sein. Die Planungsobergrenzen sind drei Attempts pro Task und 192 Attempts pro Mission (64×3). Failed/retryable Legacy-Tasks ohne Lifecycle-Evidenz werden nicht als Retry-Kandidaten eingestuft, weil das verbleibende Attempt-Budget nicht beweisbar ist.

Der Planer reserviert keine echte Scheduler-Ressource. `RequiresNewAttempt` und prospective Attempt-Zählung sind Planungsfakten, keine Leases, keine Admission und keine Ausführungsberechtigung. Er startet weder Child noch Modell und verändert keine Mission.

Freitext wird über die bestehende Journal-Redaction sanitisiert und hart begrenzt. Rohes Child-/Modellresultat, Findings und Tool-Transcript werden nicht als zweites Transcript persistiert. Der normale Chat-Recovery-Pfad `Weiter`/`Continue` verweigert Mission-Journale weiterhin ausdrücklich. Reconciliation, Postcondition-Verifikation und Transition-Planung sind keine Resume-/Retry-/Replay-Autorität.

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

`run_journal.go` bleibt die einzige Recovery-Autorität. Read-only Missions besitzen begrenzte strukturierte durable Metadaten, Projekt-/Git-Baselines, Scheduler-/Accounting-Checkpoints, Completion-/Lifecycle-/Verification-Evidenz, Restart-Reconciliation, interne read-only Postcondition-Verifikation und deterministische Transition-Planung im bestehenden Recovery-Pfad. Desktop-Telemetrie, Orchestrierungsdiagnostik und Mobile-Anzeige bleiben nicht autoritativ.

Automatische Mission-Wiederaufnahme, Retry und Replay sind weiterhin nicht implementiert. Als Nächstes folgt eine explizite kontrollierte read-only Recovery-Control-Grenze, die unmittelbar vor jeder Pause/Resume/Retry-Entscheidung Reconciliation, notwendige Verifikation und Transition-Plan frisch berechnet und weiterhin normale Scheduler-/Budget-/Cancellation-Grenzen einhält. Mutation-capable Builder in isolierten Git-Worktrees kommen erst nach belastbarer Mission-Recovery.

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
- `src/agent_scheduler_dispatch.go` – actual scheduled execution of authorized read-only tasks plus durable Mission checkpoint hooks.
- `src/agent_scheduler_finalize.go` – serialized preparation/finalization against cancellation races.
- `src/agent_mission.go` – explicit governance/Mission entry boundary.
- `src/agent_mission_accounting.go` – Mission usage, wall time, budget and terminal reasons.
- `src/agent_mission_cancel.go` – product-boundary cancellation for unfinished Mission tasks.
- `src/run_journal_mission.go` – bounded structured Mission metadata, project/Git baseline and journal-checkpoint mapping; no auto-resume.
- `src/run_journal_mission_reconcile.go` – read-only restart reconciliation against project identity, Git HEAD and hashed worktree state; no resume/retry authority.
- `src/run_journal_mission_postcondition_verify.go` – deterministic read-only postcondition verification for durable successful recovery tasks; no Child/model execution and no resume/retry authority.
- `src/run_journal_mission_transition_plan.go` – deterministic recovery-transition classification with dependency and attempt limits; planning only, without Scheduler admission or execution.
- `src/agent_mission_status.go` – bounded ephemeral Mission/scheduler telemetry for Desktop; not a recovery authority.
- `src/agent_orchestration_diagnostics.go` – derived Desktop orchestration diagnostics for backend, queue and resources; not Scheduler policy.
- `src/agent_orchestration_parallelism_benchmark_test.go` – synthetic dispatcher and opt-in Ollama parallelism benchmark.
- `src/run_journal.go` – sole durable active-run/Mission recovery authority (`active-run.json`) and startup entry for Mission reconciliation.
- `src/static/mission_status.js` – read-only Mission and orchestration diagnostics in the Desktop Output inspector.
- `src/static/*` – other Desktop/Remote UIs and DE/EN catalogs.
- `src/remote_mission_status_contract.md` – source-level contract for the narrow Mobile Mission display.
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproducible benchmark commands, measurements and interpretation boundaries.
- `android/app/.../MainActivity.java` – native Android shell.

### Native Agent Teams

The only executable child roles are **Explorer**, **Planner** and **Reviewer**. Their action schema is read-only: project tree, file reads, text search, approval-free LSP and structured `finish`. Mutation, shell, Git, web/network, MCP tool calls, installation, memory, approval requests and recursive spawning are intentionally absent.

`AgentTaskGraph` contains a stable Mission ID and stable task IDs. Dependencies, cycles and states are validated. `AgentScheduler` owns a bounded ready queue and resource classes; read-only children currently default to `model-inference`, whose default limit is one.

Scheduler acceptance includes larger saturation/fairness cases: multiple resource classes may be saturated simultaneously, admissible work in another class may bypass older work blocked by its saturated class, FIFO is preserved within a resource class, and a 14-task fan-out/fan-in DAG must drain without starvation or resource leakage.

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

Cancellation-first discards late child results. Completion-first remains successful. Whole-Mission cancellation terminalizes unfinished work after synchronous dispatch stops and refreshes the terminal Scheduler snapshot so graph, Mission result and Desktop status agree.

### Desktop Mission status

Desktop continues to use `/api/status` as its canonical status source. `Status.MarshalJSON` adds a `mission` object only when Mission telemetry matches the current execution-scoped `RunID`. Normal runs and unrelated RunIDs do not receive Mission data.

While a read-only Mission is executing, a bounded in-memory monitor publishes stable Mission/Run identity, Mission state/reason, queued/running counts, resource state, task state/queue/admission/budget facts and Mission budget/usage.

The status registry is bounded and evicts old observations. It **does not itself write durable state** and cannot start, continue, resume or authorize a Mission. Durable Mission checkpoints are separate and flow only through `run_journal.go`. `src/static/mission_status.js` only reads status data.

### Durable Mission checkpoints, verification and transition planning

A validated read-only Mission attaches its recovery metadata to the same `RunRecoveryState` already persisted as `active-run.json` for normal runs. No second Mission journal is introduced.

The Mission checkpoint stores bounded structured identity/objective/scope/model/constraints/success criteria, Mission budget, DAG/task state and capabilities, Scheduler resource/queue/running/budget snapshots, completion/lifecycle/verification evidence, final Mission state/reason/accounting and only scheduler-accepted usage.

Mission start captures a bounded reconciliation baseline: SHA-256 canonical project identity, Git observation state, SHA-256 Git-root identity, exact `HEAD`, SHA-256 of `git status --porcelain=v1 -z --untracked-files=all` bytes and capture time. Raw porcelain output and file paths are not persisted. The Git observer is fixed-function read-only and grants no Git, shell or Scheduler authority.

After interruption, current project/Git state is reconciled as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` or `insufficient_evidence`. Crash-running work stays `interrupted_unknown`. Durable success needs bounded verification, and historical `verified` is reusable only while current reconciliation remains `matched`.

The internal postcondition verifier freshly observes current project/Git state and checks fixed recovery facts: matched reconciliation, non-running task, durable success, completion evidence, successful completion status and canonical completion digest. Its SHA-256 evidence binds Mission/task identity and current hashed/HEAD Git observation. Raw paths, Child/model output and raw verification output are not persisted. An optimistic journal precondition prevents stale writes after concurrent recovery-state changes.

The transition planner then produces classification only. It reconstructs and validates the durable DAG. Duplicate IDs, missing dependencies, cycles, invalid task metadata, more than 64 tasks or inconsistent/over-limit lifecycle counters invalidate the whole plan; every task becomes `invalid_recovery_state` and no candidate is executable.

Valid plans may classify `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, `interrupted_review_required`, `preserve_terminal` and blocking reconciliation/dependency/attempt outcomes. Any reusable or future executable candidate requires every dependency to be currently verified and reusable. Fixed planning bounds are three attempts per task and 192 aggregate Mission attempts (64×3). Failed/retryable legacy work without lifecycle evidence is not retryable because its remaining attempt budget cannot be proven.

The planner reserves no Scheduler resource. Prospective attempts and `RequiresNewAttempt` are planning facts, not leases, admission or execution authority. It starts no Child/model and mutates no Mission. Normal chat `Continue` still rejects Mission journals. Reconciliation, verification and transition planning are not resume/retry/replay authority.

### Orchestration and saturation diagnostics

`/api/status` also contains a machine-readable `orchestration` object derived from current Ollama/model state and the ephemeral Mission/Scheduler snapshot. It is not a second Scheduler or recovery source.

Diagnostics distinguish `ready`, `active`, `saturated`, `backend_unavailable` and `model_unavailable`; `at_capacity` only means every slot is occupied while `saturated` additionally requires matching waiting work. Diagnostics do not alter queue limits, admission or model concurrency and are not performance evidence.

### Orchestration benchmarks

`BenchmarkScheduledReadOnlyDispatcherParallelism` deliberately separates logical readiness, configured `model-inference` slot capacity and observed executor overlap. The current dispatch loop invokes the executor synchronously; a model-inference limit greater than one is therefore not by itself evidence of parallel child-model calls.

`TestOllamaConcurrencyBenchmarkOptIn` measures a fixed real Ollama workload at client concurrency 1/2/4 through the production `OllamaClient.Chat` path. It is explicitly opt-in, loopback-only, requires an exact already-installed model and calls neither `EnsureRunning` nor `Pull`. Client overlap or throughput is not treated as proof of simultaneous GPU/token inference.

Benchmark results are evidence, not Scheduler policy. Model-slot changes require a separate reviewed change. Commands, bounds and interpretation rules are documented in `docs/ORCHESTRATION_BENCHMARKS.md`.

### Mobile Mission status

Mobile Remote deliberately remains narrower than Desktop. It adds **no** Mission endpoint and does not receive the Desktop `mission` object. `src/static/remote.html` uses only authenticated `running` and `run_phase` fields. Existing Remote stop behavior is unchanged and no Mission control authority is added.

### Recovery and next layers

`run_journal.go` remains the sole recovery authority. Read-only Missions now have bounded structured durable metadata, project/Git baselines, Scheduler/accounting checkpoints, completion/lifecycle/verification evidence, restart reconciliation, internal read-only postcondition verification and deterministic transition planning in the existing recovery path. Desktop telemetry, orchestration diagnostics and the Mobile indicator remain non-authoritative.

Automatic Mission resume, retry and replay remain absent. Next comes an explicit controlled read-only recovery-control boundary that recomputes reconciliation, required verification and the transition plan immediately before any pause/resume/retry decision and still obeys normal Scheduler, budget and cancellation boundaries. Mutation-capable Builders in isolated Git worktrees come only after durable Mission recovery is sound.
