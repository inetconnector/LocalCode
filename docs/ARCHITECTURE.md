# Architecture / Architektur

## Deutsch

### Systemübersicht

LocalCode ist eine Windows-first Go-Anwendung mit eingebettetem Web-Frontend.

Zentraler Desktop-Pfad:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native oder externe Engine -> kontrollierte Tools -> Verifikation/Recovery`

Mobile-Pfad:

`Android/Browser Remote -> separater token-geschützter Remote-Server -> engere AppState-Oberfläche`

Native Agent Teams:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

Langfristiges Ziel:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolierte Worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

LocalCode trennt **logische Task-Parallelität** von **tatsächlicher Modellinferenzparallelität**. Viele Tasks können logisch bereit sein; lokale Modellinferenz besitzt standardmäßig einen aktiven Model-Slot. Der aktuelle Dispatcher ist synchron.

### Zentrale Komponenten

- `src/types.go` – `Config`, `AppState`, gemeinsame Laufzeittypen.
- `src/server.go` – Desktop Loopback HTTP/SSE API.
- `src/remote_server.go` – separater Mobile-Remote-Server.
- `src/remote_secure_server.go` / `src/remote_firewall_windows.go` – LAN-Remote-HTTPS, Discovery und nicht-erhöhende Firewall-Prüfung.
- `android/app/src/main/java/com/inetconnector/localcode/MainActivity.java` – Android-WebView-Hülle mit gespeicherter Wiederverbindung, mDNS und LAN-Fallback-Probe.
- `src/agent.go` – LocalCode-Native-Hauptschleife und Werkzeugdispatch.
- `src/agent_team_types.go` – Rollen, Capabilities, Budget, Usage, Tasks und `AgentResult`.
- `src/agent_task_graph.go` – DAG-Validierung und Dependency-Zustände.
- `src/agent_scheduler.go` – Queue, Ressourcenlimits, Admission, Cancellation und Snapshots.
- `src/agent_scheduler_dispatch.go` – tatsächliche read-only Child-Ausführung und Scheduler-Checkpoint-Hooks.
- `src/agent_scheduler_finalize.go` – serialisierte Finalisierung gegen Cancel-Races.
- `src/agent_mission.go` – explizite Mission-Governance-/Startgrenze.
- `src/agent_mission_accounting.go` – Mission-Usage, aktive Wall-Time, Budget und Terminalgründe.
- `src/run_journal.go` – einzige dauerhafte aktive Run-/Mission-Recovery-Autorität (`active-run.json`).
- `src/run_journal_mission.go` – begrenzte strukturierte Mission-Metadaten und Scheduler-Checkpoint-Abbildung.
- `src/run_journal_mission_reconcile.go` – Restart-Reconciliation gegen aktuelle Projekt-/Git-Evidenz.
- `src/run_journal_mission_postcondition_verify.go` – deterministische read-only Postcondition-Verifikation.
- `src/run_journal_mission_transition_plan.go` – deterministische Recovery-Transition-Klassifikation.
- `src/run_journal_mission_control.go` – stabiler read-only Recovery-Control-Snapshot.
- `src/run_journal_mission_continuation.go` – bounded Continuation-Materialisierung für einen expliziten Kandidaten.
- `src/run_journal_mission_admission.go` – atomare Recovery-Reservation, Scheduler-Ausführung und subset-sichere Finalisierung.
- `src/agent_factory.go` – Agent Factory, rollenspezifische Sicherheitsprofile und inert Dynamic Role Quarantine.
- `src/agent_mission_replanning.go` – begrenztes DAG-Replanning, Reparatur-Subgraphen und Stagnationsschutz.
- `src/computemesh.go` – dezentrales ComputeMesh-Cluster-Subsystem, Provider-Self-Compute und Auto-Discovery.
- `src/doctor.go` – Systemdiagnose für GPU/VRAM, Cluster, Ollama, Engines, MCP und Worktrees.
- `scripts/test-automation-service.ps1` – E2E-Automations- und Fernsteuerungstest für Windows und Android.
- `src/desktop_mission_recovery.go` – Desktop-only Inspection/Continue-Transport und AppState-Admission-Größe.
- `src/static/mission_status.js` – Mission-/Diagnostik-/Recovery-UI im Desktop Output-Inspector.
- `src/remote_mission_status_contract.md` – schmale Mobile-Mission-Beobachtungsgrenze.
- `docs/ORCHESTRATION_BENCHMARKS.md` – reproduzierbare Parallelitätsmessung und Interpretationsgrenzen.

### Native Agent Teams

Aktuell ausführbare Child-Rollen sind ausschließlich **Explorer**, **Planner** und **Reviewer**. Ihr Action-Schema ist read-only: Projektbaum, Dateien, Textsuche, genehmigungsfreies LSP und strukturiertes `finish`. Mutation, Shell, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Spawning fehlen absichtlich.

Planner-`RequestedCapabilities` sind Planungsdaten. Ausführbare Capabilities werden erst an einer vertrauenswürdigen Governance-Grenze vergeben. Persistierte Capabilities werden bei Recovery nicht als Autorität wiederverwendet; die Runtime regeneriert den erlaubten Rollen-Envelope.

### Mission Manager, Scheduler und Accounting

`MissionID` ist stabile Produktidentität. `AppState.RunID` ist dagegen execution-scoped und wird bei einer Recovery-Fortsetzung neu erzeugt.

`AgentScheduler` trennt logische Ready-Zustände von tatsächlicher Ressourcenadmission. `prepareScheduledAgentTask` erstellt unter Scheduler-Lock eine abgetrennte Task-Kopie; der Child läuft außerhalb des Locks; Finalisierung und Cancellation konkurrieren wieder an derselben Lock-Grenze. Nur der terminale Gewinner darf Resultat/Usage festschreiben und den Lease freigeben.

Mission-Usage stammt ausschließlich aus scheduler-akzeptiertem `UsageByTask`. Model-/Tool-Calls und geschätzte Tokens bleiben additiv; aktive Mission-Wall-Time wird separat verfolgt. Mission-Budgets dürfen Child-Budgets nur einschränken, niemals erweitern.

### Durable Mission-Recovery

Eine validierte read-only Mission hängt ihre Recovery-Metadaten an denselben `RunRecoveryState`, den normale Runs bereits als `active-run.json` verwenden. Es gibt kein zweites Mission-Journal.

Persistiert werden nur begrenzte strukturierte Recovery-Fakten: Mission-/Task-Identität, DAG, Rollen/Requested-/Granted-Capabilities als Zustand, Budgets, Scheduler-Fakten, Completion-/Lifecycle-/Verification-Evidenz, Mission-State/-Reason, Accounting und scheduler-akzeptierte Usage.

Mission-Start erfasst zusätzlich eine begrenzte Reconciliation-Baseline aus:

- SHA-256 der kanonischen Projektidentität,
- Git-Beobachtungszustand,
- SHA-256 der Git-Root-Identität,
- exaktem `HEAD`,
- SHA-256 der Bytes aus `git status --porcelain=v1 -z --untracked-files=all`,
- Erfassungszeitpunkt.

Rohe Porcelain-Ausgabe und Dateipfade werden nicht als Recovery-Transcript persistiert.

Nach Unterbrechung wird die aktuelle Sicht klassifiziert als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence`. Crash-running Arbeit wird niemals als Erfolg behandelt. Durable erfolgreiche Tasks benötigen aktuelle, bounded Postcondition-Verifikation. Historisches `verified` überstimmt keine aktuelle Drift.

Der Transition-Planer rekonstruiert und validiert den durable DAG. Duplicate IDs, fehlende Dependencies, Zyklen, mehr als 64 Tasks, ungültige Task-Metadaten oder inkonsistente Lifecycle-Zähler führen fail-closed zu `invalid_recovery_state`. Feste Obergrenzen sind drei tatsächliche gestartete Attempts pro Task und 192 pro Mission.

Mögliche Klassifikationen umfassen `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, `interrupted_review_required`, `preserve_terminal` und blockierende Reconciliation-/Dependency-/Attempt-Zustände. Planung erzeugt keinen Scheduler-Lease und keine Ausführungsberechtigung.

### Atomare Recovery-Admission

Die erste ausführungsfähige Recovery-Grenze liegt in `run_journal_mission_admission.go`.

Für einen expliziten Kandidaten wird die #67-Materialisierung **unter dem AppState-Run-Gate frisch neu berechnet**. Danach werden Task-/Mission-Budgets aus durablem Evidence konservativ rekonstruiert, der exakte Journal-Fingerprint und die File-Version geprüft und ein frischer execution-scoped `RunID` durch eine versionsgebundene atomare Journal-Schreiboperation reserviert.

Die Reservation wird persistiert, bevor ein Scheduler existiert. Erst danach wird `AppState.Running=true` gesetzt und der Continuation-Scheduler erzeugt.

`AttemptReserved` beschreibt Admission-Intent. `AttemptCount` steigt erst mit dem ersten durable Scheduler-`Running`-Checkpoint. Ein Crash nach Reservation, aber vor Scheduler-Start, verbraucht daher keinen weiteren Retry.

Historische Usage wird in den Scheduler seedend übernommen und nur um scheduler-akzeptierte neue Usage ergänzt. Die Continuation-Finalisierung merged nur den bounded Teilgraphen zurück und erhält alle unrelated Mission-Tasks. Die gewöhnliche Whole-Mission-Finalisierung wird dafür bewusst nicht wiederverwendet.

### Desktop Recovery Control

Seit PR #69 besitzt der Desktop eine explizite produktseitige Recovery-Steuerung:

- `GET /api/mission-recovery` erzeugt einen bounded Inspection-DTO aus dem trusted Recovery-Control-Snapshot.
- `POST /api/mission-recovery/continue` akzeptiert eine explizite Run/Mission/Task/Action-Auswahl plus inspizierte Hash-Preconditions.
- Request-Daten sind **keine** Authority. Vor Admission werden aktuelle Projekt-/Git-Evidenz, Transition-Plan, Task-/Mission-Budgets, Modell-/Capability-Governance und der aktuelle Journal-Fingerprint erneut vertrauenswürdig berechnet/geprüft.
- `SnapshotSHA256` beschreibt die beobachtete Snapshot-Form und ist wegen des gebundenen Beobachtungszeitpunkts kein wiederverwendbares Autorisierungstoken.
- `JournalSHA256` bindet die durable Stale-State-Precondition; die eigentliche Admission führt zusätzlich den exakten Journal/File-CAS aus.
- `202 Accepted` kommt erst nach durable Reservation + AppState-Ownership. Die Ausführung lebt danach unabhängig vom HTTP-Request unter AppState/Scheduler und `StopAgent`.

Die Desktop-Karte zeigt nur current `resume_candidate`/`retry_candidate`-Aktionen und startet niemals automatisch.

### Desktop / Mobile Trennung

Desktop-Recovery-Routen existieren nur auf dem Loopback-Server und erben dessen Host-, Origin- und `Sec-Fetch-Site`-Prüfungen.

`RemoteServer` besitzt keine Mission-Recovery-Route. Mobile erhält nur den schmalen aktiven Mission-Indikator aus `running` und `run_phase`; keine Mission-/Task-Recovery-IDs, keinen Transition-Plan, keine Scheduler-/Budget-/Accounting-Daten und keine Resume-/Retry-Autorität.

Startup bleibt passiv. Es gibt kein automatisches Resume, Retry oder Replay.

### Orchestrierungsdiagnostik und Benchmarks

`/api/status` enthält abgeleitete Orchestrierungsdiagnostik für Backend, Queue, logische Task-Zustände und Ressourcen. `at_capacity` bedeutet nur vollständige Belegung; `saturated` zusätzlich tatsächlich wartende passende Arbeit. Diagnostik verändert keine Scheduler-Policy.

Der synthetische Parallelitätsbenchmark und der opt-in Ollama-Benchmark liefern Messdaten, keine Policy. Der Ollama-Benchmark akzeptiert nur Loopback, verlangt ein bereits installiertes exaktes Modell und startet oder lädt nichts.

### Mission Memory & Persistente Knowledge-Speicherung

`src/agent_mission_knowledge.go` implementiert ein zweistufiges Wissensmodell:
1. **Laufzeit-Wissen (Mission-Scope)**: Gespeichert in `RunJournalState.Mission.Knowledge` (max. 64 Einträge pro aktiver Mission) für strukturierte Architekturentscheidungen (`architecture_decision`), Schnittstellenverträge (`subsystem_contract`), bekannte Fehler/Gotchas (`known_failure`) und Testbelege (`test_evidence`).
2. **Projektweit persistente Knowledge-Ablage**: Gespeichert unter `%LOCALAPPDATA%\LocalCode\knowledge\<project_hash>.json` (`schema_version: 1`).
   - Feste Obergrenzen: Maximal 64 Einträge pro Projekt, FIFO-Eviction älterer Einträge.
   - Byte-Budget-Kompaktierung: Maximal 128 KiB JSON-Dateigröße; ältere Einträge werden automatisch verworfen, wenn das Budget überschritten wird.
   - Datenschutz & Secret-Redaktion: Sanitisierung aller Titel, Zusammenfassungen und Tags (`sanitizeMissionKnowledgeItem`), automatische Maskierung von API-Schlüsseln, Passwörtern und Authentifizierungs-Headern.
   - Atomare Speicherung: Sicheres Schreiben über temporäre `.tmp`-Dateien und `os.Rename`.
   - Reiner Kontextcharakter: Mission Knowledge informiert Planer und Kontextprompts (`formatMissionKnowledgeForPrompt`), stellt jedoch **keine Ausführungsautorität**, keinen Scheduler-Lease und keine Wiederherstellungsautorität dar. `run_journal.go` bleibt die einzige Wiederherstellungsautorität.

---

## English

### System overview

LocalCode is a Windows-first Go application with an embedded web frontend.

Desktop runtime:

`Desktop UI -> Loopback API -> AppState/Supervisor -> Native or external engine -> controlled tools -> verification/recovery`

Mobile runtime:

`Android/browser Remote -> separate token-protected Remote server -> narrower AppState surface`

Native Agent Teams:

`Governance -> Mission Manager -> Task DAG -> Scheduler/Resource Manager -> read-only Child Runtime`

Long-term target:

`Governance -> Mission Manager -> Planner -> Task DAG -> Scheduler -> Agent Factory -> Explorer/Builder/Test/Reviewer -> isolated worktrees -> Integrator -> Acceptance Gate -> Mission Memory/Recovery/Replanning`

Logical task parallelism is deliberately separated from actual model-inference parallelism. The current dispatcher is synchronous and local inference defaults to one active model slot.

### Core components

The main boundaries are `src/server.go` (Desktop loopback), `src/remote_server.go` (Mobile Remote), `src/remote_secure_server.go` / `src/remote_firewall_windows.go` (LAN Remote HTTPS, discovery and non-elevating firewall checks), `android/app/src/main/java/com/inetconnector/localcode/MainActivity.java` (Android WebView shell with saved reconnection, mDNS and LAN fallback probing), `src/agent_factory.go` (Agent Factory & governance), `src/agent_mission_replanning.go` (DAG repair & replanning), `src/agent_mission_knowledge.go` (Mission Knowledge & persistent storage), `src/computemesh.go` (decentralized cluster subsystem), `src/doctor.go` (system health diagnostics), `scripts/test-automation-service.ps1` (E2E automation harness), `src/agent_mission.go` (Mission governance), `src/agent_scheduler*.go` (queue/admission/finalization), `src/run_journal.go` plus `src/run_journal_mission*.go` (single durable recovery authority and recovery layers), `src/desktop_mission_recovery.go` (Desktop inspection/continue transport), and `src/static/mission_status.js` (Desktop Mission/recovery UI).

### Native Agent Teams

The only executable child roles are **Explorer**, **Planner**, and **Reviewer**. Their action schema is read-only: project tree, files, text search, approval-free LSP, and structured `finish`. Mutation, shell, Git mutation, web/network, MCP tool calls, installation, memory writes, approvals and recursive spawning are absent.

Planner `RequestedCapabilities` are inert planning data. Executable capabilities are granted only by trusted governance. Persisted capabilities are not reused as recovery authority; the runtime rebuilds the trusted role envelope.

### Mission Manager, Scheduler, and accounting

`MissionID` is stable product identity while `AppState.RunID` is execution-scoped and rotates for recovery continuation.

Scheduler admission and cancellation share serialized authority. Children execute on detached task copies outside the scheduler lock; only the terminal winner may persist result/usage and release the lease.

Mission usage is derived only from Scheduler-accepted `UsageByTask`. Mission budgets may narrow child budgets but never widen them.

### Durable Mission recovery

Read-only Mission recovery metadata lives in the existing `active-run.json`; `run_journal.go` is the **only durable recovery authority**.

Mission start stores bounded project/Git evidence from canonical project identity, repository identity, exact `HEAD`, a SHA-256 fingerprint of porcelain worktree bytes, and timestamps. Raw porcelain paths are not persisted as a second transcript.

After interruption, current state is reconciled as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Crash-running work is never inferred successful. Durable success requires bounded current postcondition verification, and historical verification never overrides current drift.

The transition planner validates the durable DAG and lifecycle evidence. Invalid graphs/counters fail closed. Fixed limits are three actually started attempts per task and 192 per Mission. Planning may emit `reuse_verified`, `verify_postconditions`, `resume_candidate`, `retry_candidate`, `interrupted_review_required`, terminal or blocking states, but planning grants no lease or execution authority.

### Atomic recovery admission

For an explicit candidate, trusted continuation materialization is recomputed under the AppState run gate. Task/Mission budgets are conservatively restored, exact journal fingerprint/file version are checked, and a new execution-scoped RunID is durably reserved before any Scheduler exists.

`AttemptReserved` is admission intent; `AttemptCount` increases only when a durable Scheduler `Running` checkpoint proves execution started. Historical usage is seeded cumulatively. Continuation finalization merges only the bounded subgraph back into the full durable Mission.

### Desktop recovery control

Since PR #69, Desktop has explicit Mission recovery control:

- `GET /api/mission-recovery` returns a bounded inspection DTO.
- `POST /api/mission-recovery/continue` accepts one explicit Run/Mission/task/action choice plus inspected hash preconditions.
- Request hashes are preconditions, not authority. Current project/Git state, transition plan, capability/model governance, budgets and journal fingerprint are recomputed immediately before admission.
- `SnapshotSHA256` is not a reusable authorization token; `JournalSHA256` binds durable stale-state evidence and admission still performs the exact journal/file CAS.
- `202 Accepted` is emitted only after durable reservation and AppState ownership. Execution then lives under AppState/Scheduler and remains cancellable via `StopAgent`.

The Desktop card never auto-posts and only exposes current Resume/Retry candidates.

### Desktop / Mobile separation

Recovery routes exist only on the Desktop loopback server and inherit its Host, Origin and `Sec-Fetch-Site` checks.

`RemoteServer` has no recovery route. Mobile receives only the narrow active-Mission indicator from `running`/`run_phase`, with no recovery identifiers, transition plan, scheduler/budget/accounting payload or Resume/Retry authority.

Startup stays passive: no automatic resume, retry, or replay.

### Diagnostics and benchmarks

`/api/status` exposes observation-only orchestration diagnostics. Synthetic and opt-in Ollama benchmarks provide evidence but never modify Scheduler policy. The real Ollama benchmark is loopback-only, requires an already-installed exact model and starts/downloads nothing.

### Mission Memory & Persistent Knowledge Store

`src/agent_mission_knowledge.go` implements a two-tier knowledge model:
1. **Runtime Mission Knowledge**: Held in `RunJournalState.Mission.Knowledge` (max 64 items per active mission) for structured architecture decisions (`architecture_decision`), subsystem contracts (`subsystem_contract`), known failures/gotchas (`known_failure`), and test evidence (`test_evidence`).
2. **Project-Persistent Knowledge Store**: Stored at `%LOCALAPPDATA%\LocalCode\knowledge\<project_hash>.json` (`schema_version: 1`).
   - Strict bounds: Maximum 64 items per project, FIFO eviction of older items.
   - Byte budget compaction: Hard cap of 128 KiB total JSON size; older items are dropped if the byte budget is exceeded.
   - Data protection & secret redaction: Sanitization across titles, summaries, and tags (`sanitizeMissionKnowledgeItem`), with automatic redaction of API keys, tokens, passwords, and authorization headers.
   - Atomic writes: Safe writes via temporary `.tmp` files and `os.Rename`.
   - Contextual-only role: Mission Knowledge informs planners and prompt context (`formatMissionKnowledgeForPrompt`), but constitutes **no execution authority**, no scheduler lease, and no recovery authority. `run_journal.go` remains the single canonical recovery authority.
