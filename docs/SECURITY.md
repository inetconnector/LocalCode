# Security model / Sicherheitsmodell

## Deutsch

LocalCode setzt auf mehrere Anwendungsschutzschichten statt auf eine einzige allmächtige Sandbox. Die Sicherheitsgrenzen gelten unabhängig davon, ob LocalCode Native oder eine externe Engine verwendet wird.

### Grundregeln

- Datei-, Befehls-, Netzwerk- und Installationsaktionen laufen durch LocalCode-Policy und Genehmigungen.
- Projekt-/Workspace-Pfade werden kanonisch geprüft; Symlink- und NTFS-Junction-Ausbrüche werden abgewiesen.
- Dateiänderungen verwenden Versions-/SHA-Preconditions, konfliktbewusste/atomare Schreibpfade und Postconditions.
- Destruktive oder riskante System-/Git-Befehle werden blockiert oder benötigen einen strengeren Pfad.
- Eigene Prozesse besitzen Timeouts, Cancellation und Windows-Prozessbaum-Abbruch.
- Es gibt keinen standardmäßigen oder dauerhaft still aktivierten `danger-full-access`-Modus.
- Prompts, Regeldateien, Slash-Commands, Skills, Memories oder Planner-Ausgaben können keine Berechtigungen selbst erweitern.
- Geheimnisse sollen nicht in normaler Konfiguration, Memories oder Recovery-Metadaten persistiert werden.

### Planner, Capabilities und Agent Teams

`RequestedCapabilities` aus Planner-/Task-Vorschlägen sind reine Planungsdaten. Ausführbare `Capabilities` werden davon getrennt gehalten und müssen von einer vertrauenswürdigen Governance-/Parent-Grenze explizit vergeben werden.

Aktuell ausführbare Child-Rollen sind Explorer, Planner und Reviewer. Das Child-Action-Schema enthält ausschließlich read-only Operationen: Projektbaum lesen, Datei lesen, Textsuche, genehmigungsfreies read-only LSP und strukturiertes `finish`. Nicht enthalten sind Datei-Mutation, Shell/Befehle, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Child-Spawning.

### Scheduler / Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und tatsächliche Ressourcenadmission. Standardmäßig gibt es nur einen aktiven lokalen Model-Inference-Slot.

Ein besonders wichtiger Race-Schutz gilt für laufende Scheduled Children:

1. `prepareScheduledAgentTask` prüft den Lease unter dem Scheduler-Mutex.
2. Das Child erhält eine **abgetrennte Kopie** des Tasks, keinen Pointer in den gemeinsam mutierbaren Graphen.
3. Das Modell läuft außerhalb des Mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder unter derselben Scheduler-Sperre.
5. Genau ein terminaler Gewinner darf Zustand/Resultat festschreiben und den Lease freigeben.

Wenn Cancellation zuerst gewinnt, werden verspätete Child-Resultate/Usage nicht in den Task geschrieben. Wenn Completion zuerst erfolgreich finalisiert wurde, ändert ein späteres Cancel den erfolgreichen Task nicht mehr. Parent-Context-Cancellation wird ebenfalls als Cancellation behandelt. Bei vollständigem Mission-Cancel werden nach beendetem synchronen Dispatch alle noch nicht terminalen Tasks als `cancelled` terminalisiert und der abschließende Scheduler-Snapshot erneut erstellt.

### Desktop Mission-Status

Die Desktop-Mission-Anzeige ist eine reine **Beobachtungsgrenze**. `/api/status` bleibt die vorhandene Loopback-Statusquelle; Mission-Daten werden nur für die exakt passende execution-scoped `RunID` ergänzt. Die stabile `MissionID` wird nicht als Run-/Journal-Identifier verwendet. Die begrenzte In-Memory-Registry ist kein Journal, kein Resume-Mechanismus und keine Berechtigungsquelle. Die Desktop-Card besitzt keinen Mission-Start-, Datei-, Shell-, Git-, Approval-, Projektmutations- oder Terminal-Command-Pfad. Mobile/Remote erhält dadurch keine neue API oder Authority.

### Durable Mission-Metadaten, Verifikation und Transition-Planung

`run_journal.go` bleibt die **einzige** dauerhafte Recovery-Autorität. Der vorhandene `RunRecoveryState` enthält für read-only Missions einen optionalen, begrenzten strukturierten Mission-Checkpoint; es wird kein zweites Mission-Journal erzeugt.

Persistiert werden nur recovery-relevante strukturierte Fakten: Mission-ID, Objective, direkte Projekt-/Scope-Identität, Modell, begrenzte Constraints/Success Criteria, Mission-Budget, DAG-/Task-Identität und -Zustand, Requested-/Granted-Capabilities, Task-Budgets, Scheduler-Ressourcen-/Queue-/Running-/Budget-Snapshots, Completion-/Lifecycle-/Verification-Evidenz sowie finaler Mission-State/-Reason, Accounting und ausschließlich scheduler-akzeptierte Usage.

Beim Missionsstart wird eine begrenzte Projekt-/Git-Baseline gespeichert: SHA-256 der kanonischen Projektidentität, Git-Beobachtungszustand, SHA-256 der Git-Root-Identität, exaktes `HEAD`, SHA-256 der Bytes von `git status --porcelain=v1 -z --untracked-files=all` und Erfassungszeitpunkt. Die rohe Porcelain-Ausgabe und damit Dateipfade werden **nicht** dauerhaft gespeichert.

Der Reconciliation-Git-Observer akzeptiert keinen frei formulierten Befehl und führt ausschließlich die fest kodierten read-only Aufrufe `rev-parse --show-toplevel`, `rev-parse --verify HEAD` und `status --porcelain=v1 -z --untracked-files=all` mit begrenztem Timeout aus. Er besitzt keine Mutationsoperation.

Nach einer Unterbrechung wird die aktuelle Projekt-/Git-Sicht als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence` klassifiziert. Ein beim Prozessabbruch laufender Task wird immer `interrupted_unknown`; laufender Zustand ist **niemals** Erfolgsbeweis. Durable erfolgreiche Tasks bleiben ohne erfolgreiche Verifikation `verify_postconditions`; ein `verified` Erfolg ist nur bei **aktuell** weiter `matched` Projekt-/Git-Reconciliation wiederverwendbar. Aktuelle Drift überstimmt historische Verifikation.

Die interne Postcondition-Verifikation ruft kein Modell und keinen Child erneut auf. Sie verwendet nur die feste read-only Projekt-/Git-Beobachtung und feste Recovery-Checks. Verification-Evidenz persistiert nur SHA-256-Digest, Check-Anzahl, State und Zeitstempel; rohe Pfade, Child-/Modelloutput und rohe Verifikationsausgabe werden nicht gespeichert. Nach der frischen Beobachtung wird das Journal erneut geladen; eine zwischenzeitliche Mission-Recovery-Änderung verhindert einen stale Verification-Write.

Der Recovery-Transition-Planer ist ebenfalls **keine** Steuerungsgrenze. Er führt keine Mission-/Task-Arbeit aus, ruft kein Modell/Tool auf, vergibt keine Capability und beantragt keinen Scheduler-Lease. Er rekonstruiert den durable DAG und verwendet die bestehende Graph-Validierung, bevor irgendeine Kandidatur entsteht.

Sicherheitsregeln des Transition-Plans:

- Duplicate Task-IDs, fehlende Dependencies, Dependency-Zyklen, ungültige Task-Metadaten oder mehr als 64 Tasks invalidieren den Plan vollständig.
- Vorhandene Lifecycle-Zähler müssen nichtnegativ, intern konsistent und höchstens `3` Attempts pro Task sein. Fehlende Lifecycle-Evidenz bleibt für Legacy-Zustände zulässig, macht failed/retryable Arbeit aber **nicht** retryfähig.
- Die feste Mission-Obergrenze ist `192` Attempts (`64 × 3`). Prospective Reservations sind nur Planungsfakten und niemals Scheduler-Leases.
- Crash-running Arbeit wird `interrupted_review_required` und nie direkt `resume_candidate`/`retry_candidate`.
- `reuse_verified`, `resume_candidate` und `retry_candidate` setzen voraus, dass alle Dependencies aktuell `verified` und wiederverwendbar sind; sonst gilt `blocked_dependency`.
- Bei aktueller Reconciliation ungleich `matched` wird potenziell wiederverwendbare/fortsetzbare Arbeit blockiert, auch bei historischem `verified`.
- Manipulierte oder inkonsistente Recovery-Strukturen erzeugen ausschließlich `invalid_recovery_state`; fail-open Kandidaturen sind unzulässig.
- Der Plan verändert historische Usage/Accounting nicht und darf daraus keine neue Ausführungsberechtigung ableiten.

Weitere Grenzen:

- Freitext läuft durch die bestehende Secret-Redaction und harte Längen-/Mengenbegrenzungen.
- Rohe Child-/Modellantworten, Findings und Tool-Transcripts werden nicht als zweites Transcript in Mission-Metadaten kopiert.
- Durable Checkpoints, Reconciliation, Postcondition-Verifikation und Transition-Planung vergeben keine Capabilities und verändern weder Scheduler-Limits noch Admission.
- Persistierte Requested-/Granted-Capabilities dokumentieren Zustand; aus dem Journal entsteht keine ausführbare Authority.
- Unterbrochene Missionen werden erkannt und klassifiziert, aber **nicht** automatisch resumed, retried oder replayed.
- Der normale Chat-Recovery-Pfad `Weiter`/`Continue` verweigert Mission-Journale ausdrücklich.
- Eine unterbrochene Mission bleibt als Recovery-Evidenz sichtbar, wenn das Projektverzeichnis fehlt; diese Situation erweitert keine Dateirechte.
- Späte/stale Child-Resultate bleiben nicht autoritativ; finale Usage wird nur aus scheduler-akzeptierten Resultaten abgeleitet.

### Orchestrierungsdiagnostik und Benchmarks

Desktop-Orchestrierungsdiagnostik ist ausschließlich Beobachtung. Sie kann Backend-, Queue- und Ressourcenzustand anzeigen, verändert aber keine Scheduler-Konfiguration. `at_capacity` und `saturated` bleiben verschieden; Sättigungsbeobachtung ist keine Performance-Evidenz.

Die synthetischen und opt-in Ollama-Benchmarks sind Messgrenzen, keine Laufzeitrechte. Der reale Ollama-Benchmark ist standardmäßig deaktiviert, Loopback-only, verlangt ein bereits installiertes exaktes Modell und ruft weder `EnsureRunning` noch `Pull` oder Installer auf. Benchmarkresultate können keine Capabilities vergeben oder Scheduler-Limits selbst erhöhen.

### Mobile Remote / Android

Desktop und Mobile sind getrennte Servergrenzen. Remote besitzt eine schmalere API und erweitert keine Werkzeugrechte. Pairing erzeugt zufällige Gerätetokens, dauerhaft wird nur der SHA-256-Hash gespeichert; Streams verwenden kurzlebige Tickets. LAN-Remote verwendet HTTPS, Android pinnt den erwarteten TLS-SHA-256-Fingerprint. Die JavaScript-Brücke bleibt auf Dateipicker und Speech Recognition begrenzt und führt keine LocalCode-Werkzeuge aus.

Die Mobile-Mission-Anzeige verwendet nur die bereits authentifizierten `running`- und `run_phase`-Felder. Es gibt keinen neuen Mobile-Mission-Endpunkt, keine Mission-/Task-IDs, keine Scheduler-/Ressourcen-/Budget-/Accounting-Daten und keine neuen Mission-Control-Aktionen. Das bestehende Remote-Stop-Verhalten bleibt unverändert.

### Netzwerk, MCP, Skills und Memories

Öffentliche Webabrufe prüfen Ziele und blockieren Loopback, Link-local, private und sonstige nichtöffentliche Ziele; DNS-Rebinding wird durch Verbindung zur zuvor validierten IP begrenzt. MCP ist explizit konfiguriert und läuft mit Timeouts/kontrollierter Prozessbeendigung. Regel-/Skill-/Slash-Command-/Memory-Inhalte erweitern Kontext, nicht Policy. Persistente Memories lehnen secret-ähnliche Inhalte ab und erweitern keine Werkzeugrechte.

### Recovery

`run_journal.go` ist die Recovery-Autorität für aktive Runs und read-only Missions. Restart-Reconciliation, interne Postcondition-Verifikation und Transition-Planung sind read-only Evidenz-/Planungsschichten. Automatisches Mission-Resume/Retry/Replay ist weiterhin nicht implementiert. Eine spätere Recovery-Control-Grenze muss unmittelbar vor jeder Ausführung aktuelle Reconciliation, erforderliche Verifikation, Dependency-Eignung und Attempt-Limits erneut prüfen und weiterhin normale Scheduler-/Budget-/Cancellation-Grenzen einhalten.

### Zukünftige Mutation-Agenten

Builder-/Worktree-Mutation ist noch nicht implementiert. Wenn sie eingeführt wird, gelten weiterhin sämtliche bestehenden LocalCode-Grenzen: kontrollierte Workspaces/Worktrees, keine unsupervised concurrent mutation desselben Workspace, normale Genehmigungen und SHA-Preconditions, diff-reviewbare Resultate, Verifikation nach der letzten Mutation, kontrollierte Integrator-Grenze und sichere Cancellation/Recovery ohne blindes `reset --hard`/`clean`.

---

## English

LocalCode uses multiple application-level protection layers rather than relying on one all-powerful sandbox. These boundaries apply whether LocalCode Native or an external coding engine is selected.

### Baseline rules

- File, command, network and installation actions pass through LocalCode policy and approvals.
- Project/workspace paths are canonicalized; symlink and NTFS-junction escapes are rejected.
- File mutations use version/SHA preconditions, conflict-aware atomic write paths and postconditions.
- Destructive or high-risk system/Git commands are blocked or routed through a stricter path.
- Owned processes have timeouts, cancellation and Windows process-tree termination.
- There is no default or silently persistent `danger-full-access` mode.
- Prompts, rule files, slash commands, skills, memories and Planner output cannot self-escalate authority.
- Secrets should not be persisted in normal configuration, memories or recovery metadata.

### Planner, capabilities and Agent Teams

`RequestedCapabilities` from Planner/task proposals are planning data only. Executable `Capabilities` remain separate and must be explicitly granted by a trusted governance/parent boundary. Current executable child roles are Explorer, Planner and Reviewer. Their action schema is read-only: project-tree/file/text-search/LSP reads plus structured `finish`; mutation, shell, Git, network/web, MCP tools, installation, memory writes, approvals and recursive spawning are absent.

### Scheduler / cancellation safety

Scheduler readiness is separate from resource admission. Local model inference defaults to one active slot. Scheduled Children receive detached task copies; finalization and cancellation compete under the same Scheduler lock so exactly one terminal outcome wins. Late results after cancellation are non-authoritative; successful completion cannot be rewritten by later cancel.

### Desktop Mission status

Desktop Mission status is observation only. Mission telemetry is scoped to the matching execution `RunID`, lives in a bounded in-memory registry, and is neither journal nor authorization source. The UI has no Mission-start, file, shell, Git, approval, project-mutation or terminal-command path. Mobile gets no new authority from Desktop status.

### Durable Mission metadata, verification and transition planning

`run_journal.go` remains the **sole** durable recovery authority. Read-only Missions attach bounded structured metadata to the existing `RunRecoveryState`; no second Mission journal is introduced.

Durable state is limited to recovery-relevant structured facts: Mission identity/objective/direct project scope/model/bounded constraints and success criteria, Mission budget, DAG/task state and capabilities, task budgets, Scheduler resource/queue/running/budget snapshots, completion/lifecycle/verification evidence, final Mission state/reason/accounting and scheduler-accepted usage.

Mission start stores a bounded project/Git baseline: SHA-256 canonical project identity, Git observation state, SHA-256 Git-root identity, exact `HEAD`, SHA-256 of `git status --porcelain=v1 -z --untracked-files=all` bytes and capture time. Raw porcelain output and file paths are not persisted. The fixed Git observer accepts no arbitrary command and has no mutation operation.

After interruption, current project/Git state is classified as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` or `insufficient_evidence`. Crash-running state is never success evidence. Durable successful work remains `verify_postconditions` until verified; historical `verified` is reusable only while **current** reconciliation remains `matched`.

The internal postcondition verifier calls no model and re-executes no Child. It uses only fixed read-only project/Git observation and fixed recovery checks. Verification persistence is bounded to SHA-256 digest, check count, state and timestamps. Raw paths, Child/model output and raw verification output are excluded. A journal optimistic-precondition check prevents stale writes after concurrent recovery-state changes.

The recovery transition planner is likewise **not** a control boundary. It executes no Mission/task work, calls no model/tool, grants no capability and requests no Scheduler lease. It reconstructs the durable DAG and runs existing graph validation before emitting any candidate.

Transition-plan safety rules:

- duplicate task IDs, missing dependencies, dependency cycles, invalid task metadata or more than 64 tasks invalidate the whole plan;
- present lifecycle counters must be nonnegative, internally consistent and at most `3` attempts per task; missing legacy lifecycle evidence is allowed but cannot make failed/retryable work retryable;
- the fixed Mission bound is `192` attempts (`64 × 3`); prospective reservations are planning facts, never Scheduler leases;
- crash-running work is `interrupted_review_required`, never directly resumable/retryable;
- `reuse_verified`, `resume_candidate` and `retry_candidate` require every dependency to be currently verified and reusable, otherwise `blocked_dependency` applies;
- current reconciliation other than `matched` blocks potentially reusable/continuable work even when historical verification says `verified`;
- malformed/inconsistent recovery state produces only `invalid_recovery_state`, never fail-open candidates;
- planning does not alter historical usage/accounting and cannot derive execution authority from it.

Durable checkpoints, reconciliation, verification and transition planning cannot change Scheduler limits/admission or grant capabilities. Interrupted Missions are still not automatically resumed, retried or replayed. Normal chat `Continue` continues to reject Mission journals.

### Orchestration diagnostics and benchmarks

Desktop orchestration diagnostics are observation only and cannot change Scheduler configuration. Synthetic and opt-in Ollama benchmarks are measurement boundaries, not runtime authority. The real Ollama benchmark is disabled by default, loopback-only, requires an exact installed model and never pulls/installs one. Benchmark results cannot grant capabilities or automatically widen Scheduler limits.

### Mobile Remote / Android

Desktop and Mobile are separate server boundaries. Remote exposes a narrower API and grants no additional tool authority. Pairing uses random device tokens with only their SHA-256 hash persisted; streams use short-lived tickets. LAN Remote uses HTTPS with Android TLS fingerprint pinning. The JavaScript bridge is limited to file picking and speech recognition and never executes LocalCode tools.

The Mobile Mission indicator uses only authenticated `running` and `run_phase`. There is no new Mobile Mission endpoint, Mission/task IDs, Scheduler/resource/budget/accounting payload or new Mission-control action. Existing Remote stop behavior is unchanged.

### Network, MCP, skills and memories

Public web targets are validated and non-public destinations rejected; DNS rebinding is constrained by dialing the validated IP. MCP is explicit and bounded by timeouts/controlled process lifecycle. Rules, skills, slash commands and memories extend context rather than policy. Durable memories reject secret-like content and grant no tool authority.

### Recovery

`run_journal.go` is the recovery authority for active runs and read-only Missions. Restart reconciliation, internal postcondition verification and transition planning are read-only evidence/planning layers. Automatic Mission resume/retry/replay remains unimplemented. Any future recovery-control boundary must freshly re-evaluate reconciliation, required verification, dependency eligibility and attempt limits immediately before execution and must still obey normal Scheduler, budget and cancellation boundaries.

### Future mutation agents

Builder/worktree mutation is not implemented yet. When introduced, existing LocalCode boundaries continue to apply: controlled workspaces/worktrees, no unsupervised concurrent mutation of the same workspace, normal approvals and SHA preconditions, diff-reviewable results, verification after the last mutation, a controlled Integrator boundary and safe cancellation/recovery without blind destructive reset/clean shortcuts.
