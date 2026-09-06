# Security model / Sicherheitsmodell

## Deutsch

LocalCode setzt auf mehrere Anwendungsschutzschichten statt auf eine einzige allmächtige Sandbox. Die Grenzen gelten unabhängig davon, ob LocalCode Native oder eine externe Engine verwendet wird.

### Grundregeln

- Datei-, Befehls-, Netzwerk- und Installationsaktionen laufen durch LocalCode-Policy und Genehmigungen.
- Projekt-/Workspace-Pfade werden kanonisch geprüft; Symlink- und NTFS-Junction-Ausbrüche werden abgewiesen.
- Dateiänderungen verwenden Versions-/SHA-Preconditions, konfliktbewusste/atomare Schreibpfade und Postconditions.
- Destruktive oder riskante System-/Git-Befehle werden blockiert oder auf einen strengeren sichtbaren Pfad delegiert.
- Eigene Prozesse besitzen Timeouts, Cancellation und Windows-Prozessbaum-Abbruch.
- Es gibt keinen standardmäßigen oder dauerhaft still aktivierten `danger-full-access`-Modus.
- Prompts, Regeldateien, Slash-Commands, Skills, Memories oder Planner-Ausgaben können Berechtigungen nicht selbst erweitern.
- Geheimnisse sollen nicht in normaler Konfiguration, Memories oder Recovery-Metadaten persistiert werden.

### Planner, Capabilities und read-only Agent Teams

`RequestedCapabilities` aus Planner-/Task-Vorschlägen sind reine Planungsdaten. Ausführbare `Capabilities` müssen von einer vertrauenswürdigen Governance-/Parent-Grenze explizit vergeben werden.

Aktuell ausführbare Child-Rollen sind Explorer, Planner und Reviewer. Ihr Schema enthält ausschließlich:

- Projektbaum lesen,
- Datei lesen,
- Textsuche,
- genehmigungsfreies read-only LSP,
- strukturiertes `finish`.

Nicht enthalten sind Datei-Mutation, Shell/Befehle, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Child-Spawning.

Persistierte Requested-/Granted-Capabilities sind Recovery-Zustand, keine ausführbare Authority. Recovery regeneriert den kanonischen Rollen-Envelope und akzeptiert keine Rechteeskalation allein aus persistenten Daten.

### Scheduler- und Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und tatsächliche Ressourcenadmission. Standardmäßig gibt es nur einen aktiven lokalen Model-Inference-Slot.

Für laufende Scheduled Children gilt:

1. `prepareScheduledAgentTask` prüft den Lease unter Scheduler-Mutex.
2. Das Child erhält eine abgetrennte Task-Kopie.
3. Das Modell läuft außerhalb des Mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder unter derselben Sperre.
5. Genau ein terminaler Gewinner darf Zustand/Resultat festschreiben und den Lease freigeben.

Cancellation-first verwirft verspätete Child-Resultate und deren Usage. Completion-first bleibt erfolgreich. Vollständiger Mission-Cancel terminalisiert verbleibende unfertige Tasks kontrolliert als `cancelled`.

### Desktop Status und Orchestrierungsdiagnostik

Mission-Status und Orchestrierungsdiagnostik unter `/api/status` sind Beobachtungsgrenzen. Die ephemere In-Memory-Registry ist kein Journal, keine Capability-Quelle und kein Recovery-Mechanismus. Angezeigte Queue-/Ressourcen-/Budgetdaten dürfen Scheduler-Limits nicht verändern.

`at_capacity` bedeutet vollständige Belegung. `saturated` wird nur gemeldet, wenn die Ressource voll ist und passende Arbeit tatsächlich wartet. Diagnose oder Benchmark-Ausgabe darf niemals automatisch Scheduler-Policy ändern.

### Single durable recovery authority

`run_journal.go` bleibt die **einzige dauerhafte Recovery-Autorität** für aktive Runs und read-only Missions. Der bestehende `RunRecoveryState` enthält einen begrenzten strukturierten Mission-Checkpoint; ein zweites aktives Mission-Journal ist verboten.

Persistiert werden ausschließlich begrenzte recovery-relevante strukturierte Fakten: Mission-/Task-Identität, DAG-Zustand, Rolle, Requested-/Granted-Capabilities als Zustand, Budgets, Scheduler-Fakten, Completion-/Lifecycle-/Verification-Evidenz, Mission-State/-Reason, Accounting und scheduler-akzeptierte Usage.

Rohes Child-/Modellresultat, Findings, Tool-Transcript und rohe `git status`-Pfade werden nicht als zweites Recovery-Transcript persistiert.

### Projekt-/Git-Reconciliation

Mission-Start speichert eine begrenzte Baseline:

- SHA-256 der kanonischen Projektidentität,
- Git-Beobachtungszustand,
- SHA-256 der Git-Root-Identität,
- exaktes `HEAD`,
- SHA-256 von `git status --porcelain=v1 -z --untracked-files=all`,
- Erfassungszeitpunkt.

Der Recovery-Git-Observer akzeptiert keinen freien Command-Text. Er führt nur fest kodierte read-only Git-Abfragen mit engem Timeout aus und erteilt dadurch weder Git-, Shell- noch Scheduler-Authority.

Nach Unterbrechung wird die aktuelle Sicht als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence` klassifiziert. Fehlende Evidenz wird nie als Match interpretiert.

Ein beim Crash `running` markierter Task ist immer unbekannt/nicht erfolgreich. Historisches `verified` überstimmt niemals aktuelle Drift. Durable erfolgreiche Tasks sind nur wiederverwendbar, wenn aktuelle Reconciliation weiterhin `matched` ist und bounded Postcondition-Verifikation gültig ist.

### Transition-Planer und Continuation-Materialisierung

Der Recovery-Transition-Planer ist reine Klassifikation. Er ruft kein Modell/Tool auf, vergibt keine Capability und beantragt keinen Scheduler-Lease.

Der durable DAG wird vor Kandidatur erneut validiert. Duplicate IDs, fehlende Dependencies, Zyklen, ungültige Task-Metadaten, mehr als 64 Tasks oder inkonsistente Lifecycle-Zähler führen fail-closed zu `invalid_recovery_state`.

Feste Grenzen sind drei tatsächlich gestartete Attempts pro Task und 192 pro Mission. Crash-running Arbeit wird nicht direkt Resume-/Retry-Kandidat. `reuse_verified`, `resume_candidate` und `retry_candidate` verlangen aktuell verifizierte/reusable Dependencies.

Die #67-Continuation-Materialisierung enthält nur einen expliziten aktuellen Resume-/Retry-Kandidaten plus dessen transitiv `reuse_verified` Dependency-Closure. Persistierte Capabilities werden nicht als ausführbare Rechte übernommen; Rollen-/Capability-Governance und Modellidentität werden neu geprüft. Fehlende oder widersprüchliche historische Usage-/Budget-Evidenz führt fail-closed.

### Atomare Recovery-Admission

Die erste ausführungsfähige Recovery-Grenze (#68) recomputet die trusted Materialisierung **unter dem AppState-Run-Gate**. Ein zuvor gezeigtes Snapshot-/Materialisierungsobjekt ist kein Autorisierungstoken.

Vor Scheduler-Erzeugung werden:

- current project/Git evidence und Transition-Eligibility frisch geprüft,
- Task-/Mission-Budgets aus durablem Evidence konservativ rekonstruiert,
- exakter Journal-Fingerprint und Dateiversion geprüft,
- ein neuer execution-scoped RunID mit versionsgebundener atomarer Journal-Schreiboperation reserviert.

Die Reservation wird dauerhaft geschrieben, **bevor** ein Scheduler existiert. Ein stale/konkurrierend geänderter Journalzustand darf keinen Executor starten und keinen Lease erhalten.

`AttemptReserved` bedeutet Admission-Intent, nicht gestartete Ausführung. `AttemptCount` steigt nur beim ersten durable Scheduler-`Running`-Checkpoint. Ein Crash zwischen Reservation und tatsächlicher Admission verbraucht keinen neuen Retry.

Historische scheduler-akzeptierte Usage wird kumulativ geseedet. Recovery darf kein frisches Budget minten. Mission-Wall-Time verwendet durable aktive Zeit; Crash-/Offline-Downtime wird nicht als aktive Ausführungszeit berechnet.

Continuation-Finalisierung merged nur den bounded Teilgraphen in die volle durable Mission. Unrelated Tasks müssen erhalten bleiben. Late cancelled Child-Ergebnisse bleiben non-authoritative.

### Explizite Desktop Recovery-Steuerung (#69)

Desktop besitzt zwei Loopback-only Recovery-Routen:

- `GET /api/mission-recovery` – bounded Inspection.
- `POST /api/mission-recovery/continue` – explizite Fortsetzung genau eines aktuellen Resume-/Retry-Kandidaten.

Der Inspection-DTO ist absichtlich schmal. Er enthält nur die zur Entscheidung erforderlichen Run-/Mission-/Task-/Transition-/Hash-Fakten. Er enthält **keine** Projektpfade, Objectives, Capabilities, rohe Child-/Modellantworten, Usage oder Accounting.

POST-Requests sind body-bounded und strikt dekodiert; unbekannte Felder werden abgewiesen. Die angeforderten Run/Mission/Task/Action-IDs und Hashes sind **Stale-State-Preconditions**, keine Authority.

`SnapshotSHA256` beschreibt die von der UI beobachtete Snapshot-Form und ist kein wiederverwendbares Autorisierungstoken, weil der Digest den Beobachtungszeitpunkt bindet. `JournalSHA256` bindet den inspizierten durable Zustand. Unmittelbar vor Admission wird trusted Recovery-Governance trotzdem vollständig frisch berechnet und die #68 Journal/File-CAS-Grenze erneut durchlaufen.

`202 Accepted` wird erst nach erfolgreicher durable Reservation und AppState-Ownership ausgegeben. Danach hängt die Ausführung nicht mehr an der Lebenszeit des HTTP-Requests; AppState/Scheduler und `StopAgent` besitzen Cancellation.

Die Desktop-UI darf niemals automatisch POSTen. Nur explizite Benutzeraktion auf einem aktuell dargestellten `resume_candidate`/`retry_candidate` startet den Admission-Pfad.

### Desktop-/Remote-Trennung

Desktop-Recovery-Routen laufen ausschließlich auf dem bestehenden Loopback-Server und erben dessen Host-, Origin- und `Sec-Fetch-Site`-Prüfungen.

`RemoteServer` besitzt **keine** Mission-Recovery-Route. Tests verlangen für `/remote/api/mission-recovery` und `/remote/api/mission-recovery/continue` `404`.

Mobile erhält nur den bestehenden schmalen aktiven Mission-Indikator aus authentifiziertem `running`/`run_phase`. Keine Recovery-IDs, kein Transition-Plan, keine Scheduler-/Budget-/Accounting-Daten und keine Resume-/Retry-Autorität werden an Mobile erweitert. Bestehendes Remote-Stop-Verhalten bleibt unverändert.

Die Android-Hülle speichert nur die zuletzt akzeptierte Remote-URL und den TLS-Fingerprint in privaten App-Preferences. Automatische Suche nutzt mDNS und einen begrenzten privaten LAN-Probe gegen die Remote-Discovery-/Ping-Endpunkte; sie erweitert keine Remote-Autorität über das bestehende Pairing-Token hinaus.

Der Windows-Startpfad erzeugt keine Firewall-Regeln per automatischer UAC-Eskalation. Eine vorhandene passende Regel wird nicht-erhöhend geprüft; eine fehlende Regel wird geloggt und blockiert den App-Start nicht.

Projektöffner starten Visual Studio nur mit tatsächlich vorhandenen Solution-/Projektdateien. Fehlende IDEs, fehlende Projektdateien oder Startfehler werden in LocalCode geloggt und fallen auf Explorer zurück, damit fremde "Datei nicht gefunden"-Dialoge nicht aus einem falschen Pfad entstehen.

Startup bleibt passiv. Automatisches Mission-Resume, Retry oder Replay ist weiterhin **nicht erlaubt**.

### Netzwerk und MCP

Public-Web-Fetches validieren Ziele und lehnen Loopback, Link-Local, private und andere nichtöffentliche Adressen ab. DNS-Rebinding wird durch Verbindung mit der zuvor validierten IP begrenzt.

MCP wird explizit konfiguriert. Stdio-/HTTP-Sessions besitzen Timeouts und kontrollierten Prozesslebenszyklus. Skill-/Prompt-Metadaten können MCP-Authority nicht selbst aktivieren.

### Skills, Commands und Memories

- Project-/Slash-Commands sind Texttemplates und führen keine Shell-Kommandos von selbst aus.
- Rule-/Skill-Dateien erweitern Modellkontext, nicht Policy.
- Skills mit non-read-only Tool-Authority werden nicht automatisch privilegiert.
- Skill-Ressourcen bleiben unter Pfad-, Größen- und Approval-Grenzen.
- Bestehende dauerhafte Memories dürfen keine Secrets enthalten und erweitern Tool-Authority nicht.

### Nächste Sicherheitsgrenze: Mission Memory/Knowledge

Persistente Mission Memory/Knowledge ist noch nicht implementiert. Vor Einführung müssen feste Regeln definiert werden:

- versioniertes Schema,
- maximale Entries/Bytes und per-field Caps,
- deterministische Retention/Eviction/Compaction,
- Secret-/Credential-Redaction,
- kein rohes Child-/Modell-Transcript, keine beliebige Tool-Ausgabe und keine unbounded File-Inhalte.

Mission Memory darf Planung/Kontext informieren, aber **niemals** Capabilities vergeben, Postconditions erfüllen, Recovery autorisieren, Attempts verändern, Scheduler-Leases erzeugen oder aktuelle project/Git Reconciliation überstimmen. Persistenz darf keine zweite aktive Recovery-Autorität neben `run_journal.go` schaffen.

### Future mutation agents

Builder-/Worktree-Mutation ist noch nicht implementiert. Wenn sie eingeführt wird, gelten weiterhin kontrollierte Workspaces, keine unbeaufsichtigte parallele Mutation desselben Workspace, normale Approvals und SHA-Preconditions, diff-reviewbare Resultate, Verifikation nach der letzten Mutation, kontrollierte Integrator-Grenze und sichere Cancellation/Recovery ohne blindes `reset/clean`.

---

## English

LocalCode uses layered application controls rather than one all-powerful sandbox. The boundaries apply to Native and external engines alike.

### Core rules

- File, command, network and installation actions remain behind LocalCode policy/approvals.
- Canonical project/workspace containment rejects symlink/NTFS-junction escapes.
- File mutation uses version/SHA preconditions, conflict-aware atomic writes and postconditions.
- Owned processes have timeouts, cancellation and Windows process-tree termination.
- There is no default or silently persistent unrestricted mode.
- Prompts, rules, slash commands, skills, memories and Planner output cannot self-escalate authority.
- Secrets should not be persisted in ordinary configuration, memories or recovery metadata.

### Planner, capabilities and read-only teams

Planner `RequestedCapabilities` are inert planning data. Executable capabilities are granted only by trusted governance. Explorer/Planner/Reviewer children have a read-only schema containing project-tree/file/text/LSP reads and structured `finish`; mutation, shell, Git mutation, network/web, MCP tools, installation, memory writes, approvals and recursive spawning are absent.

Persisted requested/granted capabilities describe historical state only. Recovery reconstructs trusted role authority instead of trusting persistent capability fields.

### Scheduler and cancellation

Ready state is separate from resource admission. Children run on detached task copies outside the scheduler lock; finalization and cancellation compete at the same serialized authority boundary. Cancellation-first discards late results/usage. Completion-first remains successful.

### Desktop observation

Mission status and orchestration diagnostics are observation-only. Their bounded in-memory state is not recovery storage, cannot grant capabilities and cannot modify Scheduler policy. Benchmark output is evidence only and never auto-tunes concurrency.

### Single durable recovery authority

`run_journal.go` is the **only durable recovery authority** for active runs and read-only Missions. The existing `RunRecoveryState` contains bounded structured Mission recovery facts; no second active Mission journal is allowed.

Raw Child/model responses, findings, tool transcripts and raw porcelain paths are not persisted as a second transcript.

### Project/Git reconciliation

Mission start stores bounded canonical project/Git identity, exact `HEAD`, hashed porcelain state and capture time. The recovery Git observer is fixed-function read-only and grants no Git/shell/Scheduler authority.

Interrupted state is classified conservatively. Missing evidence never becomes a match. Crash-running work is never inferred successful. Historical verification never overrides current drift.

### Transition planning and materialization

Transition planning executes no Mission work and grants no lease/authority. Durable DAG/lifecycle corruption fails closed. Fixed limits remain three actually started attempts per task and 192 per Mission.

Continuation materialization contains only one explicit current Resume/Retry candidate plus transitively verified dependencies. Capability/model governance and historical usage/budget evidence are revalidated; corrupt or missing evidence fails closed.

### Atomic recovery admission

Trusted continuation materialization is recomputed under the AppState run gate. Current project/Git eligibility, budgets, exact journal fingerprint and file version are checked before a new execution-scoped RunID is durably reserved. The reservation is committed before a Scheduler exists.

`AttemptReserved` is admission intent; `AttemptCount` increases only at a durable Scheduler `Running` checkpoint. Historical accepted usage remains cumulative and crash/offline downtime is excluded from active Mission execution time.

Continuation finalization merges only the bounded subgraph and preserves unrelated durable Mission tasks.

### Explicit Desktop recovery control

Desktop exposes loopback-only `GET /api/mission-recovery` and `POST /api/mission-recovery/continue`.

The inspection DTO is deliberately bounded and excludes project paths, objectives, capabilities, raw Child/model output, usage and accounting.

POST input is strictly decoded and size-bounded. Run/Mission/task/action identifiers and hashes are stale-state preconditions, not authority. `SnapshotSHA256` is not a reusable authorization token; `JournalSHA256` binds the inspected durable state, and trusted recovery governance plus the exact journal/file CAS are still recomputed immediately before admission.

`202 Accepted` is emitted only after durable reservation and AppState ownership. Execution then belongs to AppState/Scheduler and remains cancellable through `StopAgent`, independent of HTTP request lifetime.

The Desktop UI never auto-posts; only explicit user action on a current Resume/Retry candidate can initiate admission.

### Desktop / Remote separation

Recovery routes exist only on the Desktop loopback server and inherit Host, Origin and `Sec-Fetch-Site` protections.

`RemoteServer` has no recovery route. Regression tests require the corresponding Remote paths to remain `404`. Mobile receives only the narrow active-Mission indicator and no recovery IDs, plan, Scheduler/budget/accounting payload or Resume/Retry authority.

The Android shell stores only the last accepted Remote URL and TLS fingerprint in private app preferences. Automatic discovery uses mDNS plus a bounded private-LAN probe against the Remote discovery/ping endpoints; it does not widen authority beyond the existing pairing token.

The Windows startup path does not create firewall rules through automatic UAC elevation. It checks for an existing matching rule without elevation; a missing rule is logged and does not block app startup.

Project openers launch Visual Studio only with existing solution/project files. Missing IDEs, missing project files, or launch failures are logged in LocalCode and fall back to Explorer so external "file not found" dialogs are not caused by a bad path.

Startup remains passive. Automatic Mission resume, retry or replay is **not allowed**.

### Network, MCP, skills and memory

Public web fetches reject non-public destinations and mitigate DNS rebinding by dialing the validated IP. MCP is explicitly configured and controlled. Rules/skills/commands extend context, not authority. Existing durable memories reject secret-like content and cannot widen tool permissions.

### Next security boundary: Mission Memory/Knowledge

Persistent Mission Memory/Knowledge is not implemented yet. Before persistence it requires a versioned schema, strict entry/byte/per-field caps, deterministic retention/eviction, secret redaction and exclusion of raw transcripts/unbounded tool or file content.

Mission Memory may inform planning/context but must **never** grant capabilities, satisfy postconditions, authorize recovery, alter attempts, create Scheduler leases or override current project/Git reconciliation. It must not become a second active recovery authority beside `run_journal.go`.

### Future mutation agents

Builder/worktree mutation is not implemented yet. When introduced, controlled workspaces, approval/SHA boundaries, diff-reviewable results, post-mutation verification, a controlled Integrator and safe cancellation/recovery remain mandatory.
