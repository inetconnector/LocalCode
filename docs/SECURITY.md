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

Aktuell ausführbare Child-Rollen sind Explorer, Planner und Reviewer. Das Child-Action-Schema enthält ausschließlich read-only Operationen: Projektbaum lesen, Datei lesen, Textsuche, genehmigungsfreies read-only LSP und strukturiertes `finish`.

Nicht enthalten sind Datei-Mutation, Shell/Befehle, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Child-Spawning. Damit kann ein Child diese Rechte nicht allein durch Modelltext anfordern.

### Scheduler / Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und tatsächliche Ressourcenadmission. Standardmäßig gibt es nur einen aktiven lokalen Model-Inference-Slot.

Ein wichtiger Race-Schutz gilt für laufende Scheduled Children:

1. `prepareScheduledAgentTask` prüft den Lease unter dem Scheduler-Mutex.
2. Das Child erhält eine **abgetrennte Kopie** des Tasks, keinen Pointer in den gemeinsam mutierbaren Graphen.
3. Das Modell läuft außerhalb des Mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder unter derselben Scheduler-Sperre.
5. Genau ein terminaler Gewinner darf Zustand/Resultat festschreiben und den Lease freigeben.

Wenn Cancellation zuerst gewinnt, werden verspätete Child-Resultate/Usage nicht in den Task geschrieben. Completion-first bleibt erfolgreich. Bei vollständigem Mission-Cancel werden nach beendetem synchronen Dispatch alle noch nicht terminalen Tasks als `cancelled` terminalisiert und der abschließende Scheduler-Snapshot erneut erstellt. Diese Grenzen besitzen Konkurrenz- und Produktgrenzentests unter Go's Race Detector.

### Desktop Mission-Status

Die Desktop-Mission-Anzeige ist eine reine **Beobachtungsgrenze**:

- `/api/status` bleibt die vorhandene Loopback-Statusquelle; Mission-Daten werden nur für die exakt passende execution-scoped `RunID` ergänzt.
- Die stabile `MissionID` wird nicht als Run-/Journal-Identifier verwendet.
- Desktop-Telemetrie liegt in einer begrenzten In-Memory-Registry und ist kein Journal, Resume-Mechanismus oder Berechtigungsquelle.
- Die Desktop-Card besitzt keinen Mission-Start-, Datei-, Shell-, Git-, Approval-, Projektmutations- oder Terminal-Command-Pfad.
- Das Anzeigen von Planner-/Task-Status kann keine `RequestedCapabilities` in ausführbare Rechte umwandeln.
- Mobile/Remote erhält dadurch keine neue API oder Authority.

Durable Mission-Recovery-Metadaten sind davon getrennt und laufen ausschließlich über `run_journal.go`.

### Durable Mission-Metadaten und Recovery-Grenze

`run_journal.go` bleibt die **einzige** dauerhafte Recovery-Autorität. PR #60 erweitert den vorhandenen `RunRecoveryState` um einen optionalen, begrenzten strukturierten Mission-Checkpoint; es wird kein zweites Mission-Journal erzeugt.

Persistiert werden nur recovery-relevante strukturierte Fakten: Mission-ID, Objective, direkte Projekt-/Scope-Identität, Modell, begrenzte Constraints/Success Criteria, Mission-Budget, DAG-/Task-Identität und -Zustand, Requested-/Granted-Capabilities, Task-Budgets, Scheduler-Ressourcen-/Queue-/Running-/Budget-Snapshots sowie finaler Mission-State/-Reason, Accounting und ausschließlich scheduler-akzeptierte Usage.

Sicherheitsgrenzen:

- Freitext läuft durch die bestehende Secret-Redaction und harte Längen-/Mengenbegrenzungen.
- Rohe Child-/Modellantworten, Findings und Tool-Transcripts werden nicht als zweites Transcript in Mission-Metadaten kopiert.
- Durable Checkpoints vergeben keine Capabilities und verändern weder Scheduler-Limits noch Admission.
- Ein Journal-Task kann Requested-/Granted-Capabilities nur als Zustand dokumentieren; daraus entsteht keine neue ausführbare Authority.
- Unterbrochene Missionen werden erkannt, aber in diesem Slice **nicht** automatisch resumed, retried oder replayed.
- Der normale Chat-Recovery-Pfad `Weiter`/`Continue` verweigert Mission-Journale ausdrücklich, damit eine strukturierte Mission nicht als normaler Prompt blind erneut ausgeführt wird.
- Eine spätere Wiederaufnahme muss zuerst aktuelle Projekt-/Git-/Task-Postconditions rekonstruieren und gegen den Journal-Checkpoint abgleichen.
- Spätes/stales Child-Resultat bleibt nicht autoritativ; finale Usage wird nur aus scheduler-akzeptierten Resultaten abgeleitet.

Damit ist Persistenz eine Recovery-Evidenzgrenze, keine Ausführungs- oder Berechtigungsgrenze.

### Orchestrierungsdiagnostik

Die Desktop-Orchestrierungsdiagnostik ist ausschließlich **Beobachtung** und keine Steuerungsgrenze:

- `/api/status` ergänzt Backend-, Queue-, logische Task- und Ressourceninformationen, ohne Scheduler-Konfiguration zu verändern.
- `at_capacity` bedeutet nur, dass ein Ressourcenlimit vollständig belegt ist. `saturated` wird erst gemeldet, wenn passende Arbeit tatsächlich wartet.
- Angezeigte Limits stammen während einer Mission aus den normalisierten Scheduler-Limits; die Anzeige darf sie weder erhöhen noch automatisch „optimieren“.
- Diagnostikdaten sind kein Recovery-Speicher, keine Capability-Quelle und kein Mission-Control-Pfad.
- Mobile Remote erhält dadurch keinen erweiterten Payload und keine zusätzliche Authority.
- Beobachtete Sättigung rechtfertigt keine Änderung der Modellparallelität; der Benchmark-Pfad liefert Messdaten, ändert aber ebenfalls niemals automatisch Scheduler-Policy oder Capabilities.

### Sicherheit der Orchestrierungs-Benchmarks

Die Benchmark-Pfade sind Messgrenzen, keine neuen Laufzeitrechte:

- Der synthetische Dispatcher-Benchmark verwendet ausschließlich bereits autorisierte read-only Child-Tasks und einen lokalen synthetischen Executor. Er verändert keine Scheduler-Defaults oder Produktkonfiguration.
- Der reale Ollama-Benchmark ist standardmäßig deaktiviert und läuft nur bei explizitem `LOCALCODE_BENCH_OLLAMA=1`.
- Er akzeptiert ausschließlich Loopback-Ollama-Endpunkte und verlangt den exakten Namen eines bereits installierten Modells.
- Er ruft weder `EnsureRunning` noch `Pull` oder einen Installer auf und startet, lädt oder installiert daher nichts.
- Benchmarkresultate können keine Capabilities vergeben, keine Admission-Grenze umgehen und kein Ressourcenlimit selbst erhöhen.
- Gemessene Client-Request-Überlappung oder höherer Durchsatz beweisen keine gleichzeitige GPU-Kernel- oder Token-Inferenz.
- Änderungen an Model-Slot-Limits oder Scheduler-Policy benötigen einen separaten Review mit VRAM-/RAM-Druck, Fairness, Cancellation und Stabilität als Abnahmekriterien.

### Mobile Remote / Android

Desktop und Mobile sind getrennte Servergrenzen. Die Desktop-API bleibt Loopback-orientiert. Remote besitzt eine schmalere API und erweitert keine Werkzeugrechte.

Wichtige Remote-Schutzmaßnahmen:

- Pairing wird über die Desktop-Seite initiiert und erzeugt ein zufälliges Gerätetoken.
- LocalCode persistiert nur den SHA-256-Hash des Gerätetokens.
- Dauerhafte Tokens werden nicht in SSE-URLs geschrieben; Streams verwenden kurzlebige Tickets.
- Cross-Origin-/Fetch-Site-Prüfungen begrenzen unerwünschte Browser-POSTs.
- LAN-Remote verwendet HTTPS; die Android-Hülle pinnt den erwarteten TLS-SHA-256-Fingerprint.
- Manuelle Android-Ziele müssen private HTTPS-IP-Ziele mit passendem Fingerprint sein.
- mDNS/QR/Deep-Link-Discovery transportiert Endpoint/Fingerprint, verleiht aber keine zusätzlichen Rechte.
- Die JavaScript-Brücke ist eng: Dateipicker und Android Speech Recognition. Sie führt keine LocalCode-Werkzeuge aus.
- Pending WebView-Dateiauswahl-Callbacks werden bei Ersatz, Fehler und Activity-Abbau abgeschlossen/cancelled.
- Speech Recognition wird nur gestartet, wenn ein passender Android-Handler verfügbar ist; Fehler werden sichtbar in der Remote-Ansicht angezeigt.
- Attachments laufen danach durch die normale Remote-/Backend-Validierung.

Die Mobile-Mission-Anzeige verwendet ausschließlich die authentifizierten `/remote/api/status`-Felder `running` und `run_phase`. Es gibt keinen neuen `/remote/api/mission`-Endpunkt, kein Mobile-`mission`-Payload, keine Mission-/Task-IDs, keine Scheduler-/Ressourcen-/Budget-/Accounting-Daten und keine neuen Mission-Control-Aktionen. Das vorhandene Remote-Stop-Verhalten bleibt unverändert.

### Netzwerk und MCP

Öffentliche Webabrufe prüfen Zieladressen und blockieren Loopback, Link-local, private und sonstige nichtöffentliche Ziele. DNS-Rebinding wird verhindert, indem die zuvor validierte IP für den tatsächlichen Verbindungsaufbau verwendet wird.

MCP ist explizit konfiguriert. Stdio-/HTTP-Sitzungen laufen mit Timeouts und kontrollierter Prozessbeendigung. Skill-/Prompt-Metadaten können keine MCP-Rechte selbst aktivieren.

### Skills, Commands und Memories

- Projekt-/Slash-Commands sind Text-Templates; sie führen selbst keine Shell-Befehle aus.
- Regel-/Skill-Dateien erweitern Modellkontext, nicht Policy.
- Skills mit nicht-read-only Toolrechten oder deklarierten Scripts/Commands werden nicht automatisch als privilegierte Arbeitsanweisung ausgeführt.
- Skill-Ressourcen unterliegen Pfad-/Größen-/Genehmigungsgrenzen.
- Persistente Memories lehnen secret-ähnliche Inhalte ab und erweitern keine Werkzeugrechte.

### Recovery

`active-run.json` bleibt der eine aktive Recovery-Datensatz. Normale Runs und read-only Missions teilen dieselbe `RunRecoveryState`-Autorität. Desktop-Telemetrie, Orchestrierungsdiagnostik und Mobile-Mission-Anzeige bleiben nicht autoritative Beobachtungsflächen.

Automatisches Mission-Resume ist noch nicht implementiert. Restart-Reconciliation muss vor jeder späteren Wiederaufnahme den aktuellen beobachtbaren Zustand prüfen; blindes Replay ist unzulässig.

### Zukünftige Mutation-Agenten

Builder-/Worktree-Mutation ist noch nicht implementiert. Wenn sie eingeführt wird, gelten weiterhin sämtliche bestehenden LocalCode-Grenzen: eigener kontrollierter Workspace/Worktree, keine unsupervised concurrent mutation desselben Workspace, normale Genehmigungen und SHA-Preconditions, diff-reviewbare Resultate, Verifikation nach der letzten Mutation, Integrator als kontrollierte Zusammenführungsgrenze und sichere Cancellation/Recovery ohne blindes `reset --hard`/`clean`.

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

`RequestedCapabilities` from Planner/task proposals are planning data only. Executable `Capabilities` remain separate and must be explicitly granted by a trusted governance/parent boundary.

Currently executable child roles are Explorer, Planner and Reviewer. Their action schema contains only project-tree reads, file reads, text search, approval-free read-only LSP and structured `finish`. It does not contain file mutation, shell/commands, Git mutation, web/network, MCP tool calls, installation, memory writes, approval requests or recursive spawning.

### Scheduler / cancellation safety

The Scheduler separates the ready queue from actual resource admission. Local model inference defaults to one active slot. Scheduled children receive detached task copies; preparation/finalization/cancellation share the Scheduler lock boundary so cancellation-first cannot be overwritten by a late Child result and completion-first remains terminal. Whole-Mission cancellation terminalizes unfinished work and refreshes the terminal Scheduler snapshot. These boundaries are exercised under Go's race detector.

### Desktop Mission status

Desktop Mission status is **observation only**. `/api/status` attaches Mission data only to the matching execution `RunID`; the caller-selected stable `MissionID` is not a journal identifier. The bounded in-memory status registry is not a journal, resume mechanism or source of authorization. The Desktop UI has no Mission-start, file, shell, Git, approval, project-mutation or terminal-command path. Mobile gains no new authority from it.

Durable Mission recovery metadata is separate and flows only through `run_journal.go`.

### Durable Mission metadata and recovery boundary

`run_journal.go` remains the **sole** durable recovery authority. PR #60 adds an optional bounded structured Mission checkpoint to the existing `RunRecoveryState`; it does not create a second Mission journal.

Persisted data is limited to recovery-relevant structured facts: Mission identity/objective/direct project scope/model/bounded constraints and success criteria, Mission budget, DAG/task identity and state, requested/granted capabilities, task budgets, Scheduler resource/queue/running/budget snapshots, final Mission state/reason/accounting and scheduler-accepted usage.

Safety rules:

- Free text passes through existing secret redaction and strict count/length bounds.
- Raw Child/model responses, findings and tool transcripts are not copied into Mission recovery metadata.
- Durable checkpoints cannot grant capabilities or change Scheduler limits/admission.
- Persisted requested/granted capabilities describe state only and are not executable authority by themselves.
- Interrupted Missions are detected but are **not** automatically resumed, retried or replayed in this slice.
- Normal chat `Continue` recovery explicitly rejects Mission journal entries so structured Mission work cannot be blindly replayed as an ordinary prompt.
- Future resume must first reconstruct current project/Git/task postconditions and reconcile them against the checkpoint.
- Late/stale Child results remain non-authoritative; terminal usage is based only on Scheduler-accepted results.

Persistence is therefore a recovery-evidence boundary, not an execution or authorization boundary.

### Orchestration diagnostics

Desktop orchestration diagnostics are observation-only. They report backend, queue, logical-task and resource facts without changing Scheduler configuration. `at_capacity` only means a limit is fully occupied; `saturated` additionally requires matching waiting work. Diagnostics are not recovery state, capability authority or Mission control, and Mobile receives no broader payload from them.

### Orchestration benchmark safety

The benchmark paths are measurement boundaries, not new runtime authority. The synthetic benchmark uses only authorized read-only Children and changes no product default. The real Ollama benchmark requires explicit `LOCALCODE_BENCH_OLLAMA=1`, loopback, and an exact already-installed model; it calls neither `EnsureRunning` nor `Pull`. Benchmark output cannot grant capabilities, bypass admission or widen resource limits. Any model-slot policy change requires separate review with memory pressure, fairness, cancellation and stability evidence.

### Mobile Remote / Android

Desktop and Mobile are separate server boundaries. Desktop remains loopback-oriented; Remote exposes a narrower API and grants no extra tool authority. Pairing uses random device tokens whose hashes are persisted, long-lived tokens are excluded from SSE URLs, HTTPS/fingerprint pinning protects LAN Remote, and the Android bridge is limited to file picker and speech recognition. Mission display uses only authenticated `running`/`run_phase` and adds no Mission payload or new control authority.

### Network and MCP

Public web fetches validate destinations and reject loopback, link-local, private and other non-public addresses. DNS rebinding is mitigated by dialing the exact validated IP. MCP is explicitly configured; sessions have timeouts and controlled subprocess lifecycle, and prompt/skill metadata cannot self-enable MCP authority.

### Skills, commands and memories

Project/slash commands are text templates, rules/skills extend context rather than policy, privileged skill declarations are not automatically trusted, skill resources remain bounded, and durable memories reject secret-like contents and do not expand tool authority.

### Recovery

`active-run.json` remains the single active recovery record. Normal runs and read-only Missions share the same `RunRecoveryState` authority. Desktop telemetry, orchestration diagnostics and Mobile Mission indicators remain non-authoritative observation surfaces.

Automatic Mission resume is not implemented yet. Restart reconciliation must verify current observable state before any later resume; blind replay is forbidden.

### Future mutation agents

Builder/worktree mutation is not implemented yet. When introduced, existing LocalCode boundaries continue to apply: controlled workspaces/worktrees, no unsupervised concurrent mutation of the same workspace, normal approvals and SHA preconditions, diff-reviewable results, verification after the last mutation, a controlled Integrator boundary and safe cancellation/recovery without blind destructive reset/clean shortcuts.
