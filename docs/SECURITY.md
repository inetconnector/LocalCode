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

Aktuell ausführbare Child-Rollen sind Explorer, Planner und Reviewer. Das Child-Action-Schema enthält ausschließlich read-only Operationen:

- Projektbaum lesen,
- Datei lesen,
- Textsuche,
- genehmigungsfreies read-only LSP,
- strukturiertes `finish`.

Nicht enthalten sind Datei-Mutation, Shell/Befehle, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Child-Spawning. Damit kann ein Child diese Rechte nicht allein durch Modelltext anfordern.

### Scheduler / Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und tatsächliche Ressourcenadmission. Standardmäßig gibt es nur einen aktiven lokalen Model-Inference-Slot.

Ein besonders wichtiger Race-Schutz gilt für laufende Scheduled Children:

1. `prepareScheduledAgentTask` prüft den Lease unter dem Scheduler-Mutex.
2. Das Child erhält eine **abgetrennte Kopie** des Tasks, keinen Pointer in den gemeinsam mutierbaren Graphen.
3. Das Modell läuft außerhalb des Mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder unter derselben Scheduler-Sperre.
5. Genau ein terminaler Gewinner darf Zustand/Resultat festschreiben und den Lease freigeben.

Wenn Cancellation zuerst gewinnt, werden verspätete Child-Resultate/Usage nicht in den Task geschrieben. Wenn Completion zuerst erfolgreich finalisiert wurde, ändert ein späteres Cancel den erfolgreichen Task nicht mehr. Parent-Context-Cancellation wird ebenfalls als Cancellation behandelt. Bei vollständigem Mission-Cancel werden nach beendetem synchronen Dispatch alle noch nicht terminalen Tasks als `cancelled` terminalisiert und der abschließende Scheduler-Snapshot erneut erstellt. Diese Grenzen besitzen absichtliche Konkurrenz- und Produktgrenzentests unter Go's Race Detector.

### Desktop Mission-Status

Die Desktop-Mission-Anzeige ist eine reine **Beobachtungsgrenze**:

- `/api/status` bleibt die vorhandene Loopback-Statusquelle; Mission-Daten werden nur für die exakt passende execution-scoped `RunID` ergänzt.
- Die stabile, vom Aufrufer gewählte `MissionID` wird nicht als Run-/Journal-Identifier verwendet.
- Mission-Telemetrie liegt ausschließlich in einer begrenzten In-Memory-Registry; alte Einträge werden entfernt.
- Die Registry ist kein Journal, kein Resume-Mechanismus und keine Berechtigungsquelle.
- Die Desktop-Card liest Mission-/Scheduler-/Budget-/Task-Zustände, besitzt aber keinen Mission-Start-, Datei-, Shell-, Git-, Approval-, Projektmutations- oder Terminal-Command-Pfad.
- Das Anzeigen eines Planner-/Task-Status kann keine `RequestedCapabilities` in ausführbare Rechte umwandeln.
- Mobile/Remote erhält durch diese Desktop-Erweiterung keine neue API oder Authority.

Damit kann Statusbeobachtung weder neue Arbeit starten noch bestehende Sicherheitsgrenzen umgehen. Durable Mission-Recovery-Metadaten sind davon getrennt und laufen ausschließlich über `run_journal.go`. Eine spätere Mission-Steuerung muss als eigene, separat geprüfte Governance-Grenze implementiert werden.

### Durable Mission-Metadaten, Restart-Reconciliation und Transition-Planung

`run_journal.go` bleibt die **einzige** dauerhafte Recovery-Autorität. Der vorhandene `RunRecoveryState` enthält für read-only Missions einen optionalen, begrenzten strukturierten Mission-Checkpoint; es wird kein zweites Mission-Journal erzeugt.

Persistiert werden nur recovery-relevante strukturierte Fakten: Mission-ID, Objective, direkte Projekt-/Scope-Identität, Modell, begrenzte Constraints/Success Criteria, Mission-Budget, DAG-/Task-Identität und -Zustand, Requested-/Granted-Capabilities, Task-Budgets, Scheduler-Ressourcen-/Queue-/Running-/Budget-Snapshots sowie Completion-/Lifecycle-/Verification-Evidenz, finaler Mission-State/-Reason, Accounting und ausschließlich scheduler-akzeptierte Usage.

Zusätzlich wird beim Missionsstart eine begrenzte Projekt-/Git-Baseline gespeichert:

- SHA-256 der kanonischen Projektidentität,
- Git-Beobachtungszustand,
- SHA-256 der Git-Root-Identität statt eines zusätzlichen rohen Repository-Pfads,
- exaktes Git `HEAD`,
- SHA-256 der Bytes von `git status --porcelain=v1 -z --untracked-files=all`,
- Erfassungszeitpunkt.

Die rohe Porcelain-Ausgabe und damit Dateipfade werden **nicht** dauerhaft gespeichert. Die Baseline ist Evidenz, keine Berechtigungsquelle.

Der Reconciliation-Git-Observer ist absichtlich enger als ein allgemeiner Shell-/Git-Toolpfad. Er akzeptiert keinen frei formulierten Befehl und führt ausschließlich die fest kodierten read-only Aufrufe `rev-parse --show-toplevel`, `rev-parse --verify HEAD` und `status --porcelain=v1 -z --untracked-files=all` aus. Der Beobachtungspfad ist mit einem Drei-Sekunden-Timeout begrenzt und besitzt keine Mutationsoperation.

Beim nächsten Start einer unterbrochenen, nicht-terminalen Mission wird die aktuelle Projekt-/Git-Sicht gegen die Baseline klassifiziert als `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable` oder `insufficient_evidence`. Ältere Mission-Journale ohne Baseline und nicht ausreichend beobachtbare/non-Git Baselines werden konservativ `insufficient_evidence`; fehlende Evidenz wird nie als Übereinstimmung interpretiert.

Task-Reconciliation bleibt ebenfalls konservativ:

- Ein beim Prozessabbruch `running` markierter Task wird immer `interrupted_unknown`; ein laufender Zustand ist **niemals** Erfolgsbeweis.
- Durable `succeeded`-/legacy-`completed`-Tasks bleiben bei fehlender oder fehlgeschlagener Verifikation `verify_postconditions`; ein `verified` Erfolg ist nur bei **aktuell** weiter `matched` Projekt-/Git-Reconciliation terminal/wiederverwendbar.
- Noch nicht gestartete/nicht-terminale Arbeit wird nur bei vollständigem Projekt-/Git-Match als `pending` klassifiziert.
- Bei Drift, fehlender oder unzureichender Evidenz bleibt potenziell wiederverwendbare Arbeit `blocked_reconciliation`, auch wenn ein früherer Verification-State `verified` war.
- Bereits `failed`/`cancelled` bleibt terminal.

Weitere Grenzen:

- Freitext läuft durch die bestehende Secret-Redaction und harte Längen-/Mengenbegrenzungen.
- Rohe Child-/Modellantworten, Findings und Tool-Transcripts werden nicht als zweites Transcript in Mission-Metadaten kopiert.
- Durable Checkpoints, Reconciliation und Postcondition-Verifikation vergeben keine Capabilities und verändern weder Scheduler-Limits noch Admission.
- Die interne Postcondition-Verifikation ruft kein Modell und keinen Child erneut auf. Sie verwendet nur die vorhandene feste read-only Projekt-/Git-Beobachtung und feste Recovery-Checks.
- Verification-Evidenz persistiert nur SHA-256-Digest, Check-Anzahl, State und Zeitstempel; rohe Pfade, Child-/Modelloutput und rohe Verifikationsausgabe werden nicht gespeichert.
- Nach der frischen Projekt-/Git-Beobachtung wird das Journal erneut geladen; eine zwischenzeitliche Änderung des Mission-Recovery-Zustands verhindert den Verification-Write.
- Ein historisches `verified` überstimmt niemals aktuelle Drift. Der terminale Verification-State wird bei Drift nicht zurückgestuft, aber die aktuelle Reconciliation blockiert Wiederverwendung.
- Persistierte Requested-/Granted-Capabilities dokumentieren Zustand; aus dem Journal entsteht keine ausführbare Authority.
- Unterbrochene Missionen werden erkannt und reconciliert, aber **nicht** automatisch resumed, retried oder replayed.
- Der normale Chat-Recovery-Pfad `Weiter`/`Continue` verweigert Mission-Journale ausdrücklich, damit eine strukturierte Mission nicht als normaler Prompt blind erneut ausgeführt wird.
- Eine unterbrochene Mission bleibt als Recovery-Evidenz sichtbar, wenn das Projektverzeichnis fehlt; diese Situation wird blockierend als `project_unavailable` klassifiziert und erweitert keine Dateirechte.
- Späte/stale Child-Resultate bleiben nicht autoritativ; finale Usage wird nur aus scheduler-akzeptierten Resultaten abgeleitet.

Der Recovery-Transition-Planer bleibt strikt außerhalb der Ausführungsautorität:

- Er führt keine Mission-/Task-Arbeit aus, ruft kein Modell oder Tool auf, vergibt keine Capability und beantragt keinen Scheduler-Lease.
- Er rekonstruiert den durable DAG und verwendet vor jeder Kandidatur die bestehende Task-Graph-Validierung. Doppelte IDs, fehlende Dependencies, Zyklen, ungültige Task-Metadaten oder mehr als 64 Tasks invalidieren den Plan vollständig.
- Vorhandene Lifecycle-Zähler müssen nichtnegativ, intern konsistent und höchstens drei Attempts pro Task sein. Fehlende Lifecycle-Evidenz bleibt für Legacy-Zustände zulässig, macht `failed`/`retryable` Arbeit aber nicht retryfähig.
- Die feste Planungsobergrenze ist drei Attempts pro Task und 192 Attempts pro Mission (`64 × 3`). Prospektive Reservations sind nur Planungsfakten und niemals Scheduler-Leases.
- Crash-running Arbeit wird `interrupted_review_required` und nie direkt `resume_candidate` oder `retry_candidate`.
- `reuse_verified`, `resume_candidate` und `retry_candidate` setzen voraus, dass jede Dependency aktuell `verified` und wiederverwendbar ist; andernfalls gilt `blocked_dependency`.
- Aktuelle Reconciliation ungleich `matched` blockiert potenziell wiederverwendbare oder fortsetzbare Arbeit auch bei historischem `verified`.
- Manipulierte oder inkonsistente Recovery-Strukturen erzeugen ausschließlich `invalid_recovery_state`; fail-open Kandidaturen sind unzulässig.
- Der Plan verändert weder historische scheduler-akzeptierte Usage noch Mission-Accounting und stellt keine Admission-/Ausführungsberechtigung dar.

### Orchestrierungsdiagnostik

Die Desktop-Orchestrierungsdiagnostik ist ebenfalls ausschließlich **Beobachtung** und keine Steuerungsgrenze:

- `/api/status` ergänzt maschinenlesbare Backend-, Queue-, logische Task- und Ressourceninformationen, ohne Scheduler-Konfiguration zu verändern.
- `at_capacity` bedeutet nur, dass das jeweilige Ressourcenlimit vollständig belegt ist. `saturated` wird erst gemeldet, wenn die Ressource voll ist und passende Arbeit tatsächlich darauf wartet.
- Die Diagnostik kann Ollama offline, kein gewähltes Modell, ein lokal fehlendes gewähltes Modell, Queue-Limit und Ressourcenwartedruck unterscheiden.
- Angezeigte Limits stammen während einer Mission aus den tatsächlich normalisierten Scheduler-Limits; die Anzeige darf sie weder erhöhen noch automatisch auf Hardware vermeintlich „optimieren“.
- Diagnostikdaten sind nicht persistent, kein Recovery-Speicher, keine Capability-Quelle und kein Mission-Control-Pfad.
- Die Desktop-Diagnoseoberfläche besitzt keinen Chat-, Approval-, Projektmutations-, Terminal- oder Scheduler-Policy-Endpunkt.
- Mobile Remote erhält durch diese Diagnostik keinen erweiterten Payload und keine zusätzliche Authority.
- Ein beobachteter Sättigungszustand ist **keine** Benchmark- oder Performance-Evidenz und rechtfertigt für sich keine Änderung der Modellparallelität. Der dedizierte Benchmark-Pfad liefert Messdaten, ändert aber ebenfalls niemals automatisch Scheduler-Policy oder Capabilities.

### Sicherheit der Orchestrierungs-Benchmarks

Die Benchmark-Pfade sind Messgrenzen, keine neuen Laufzeitrechte:

- Der synthetische Dispatcher-Benchmark verwendet ausschließlich bereits autorisierte read-only Child-Tasks und einen lokalen synthetischen Executor. Er verändert keine Scheduler-Defaults oder Produktkonfiguration.
- Der reale Ollama-Benchmark ist standardmäßig deaktiviert und läuft nur bei explizitem `LOCALCODE_BENCH_OLLAMA=1`.
- Er akzeptiert ausschließlich Loopback-Ollama-Endpunkte und verlangt den exakten Namen eines bereits installierten Modells.
- Er ruft weder `EnsureRunning` noch `Pull` oder einen Installer auf und startet, lädt oder installiert daher nichts.
- Benchmarkresultate können keine Capabilities vergeben, keine Admission-Grenze umgehen und kein Ressourcenlimit selbst erhöhen.
- Gemessene Client-Request-Überlappung oder höherer Durchsatz beweisen keine gleichzeitige GPU-Kernel- oder Token-Inferenz; Hardware-/Backend-Parallelität darf daraus nicht ungeprüft abgeleitet werden.
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
- Pending WebView-Dateiauswahl-Callbacks werden bei Ersatz, Fehler und Activity-Abbau abgeschlossen/cancelled, damit kein hängender Callback zurückbleibt.
- Speech Recognition wird nur gestartet, wenn ein passender Android-Handler verfügbar ist; Launch-/Picker-Fehler werden sichtbar in der Remote-Ansicht angezeigt.
- Attachments laufen danach durch die normale Remote-/Backend-Validierung.

Die Mobile-Mission-Anzeige erweitert diese Grenze nicht: Sie verwendet ausschließlich die bereits authentifizierten `/remote/api/status`-Felder `running` und `run_phase`. Nur für `running == true` und `run_phase == "mission-read-only"` wird ein read-only Mission-Hinweis angezeigt. Es gibt keinen neuen `/remote/api/mission`-Endpunkt, kein Mobile-`mission`-Payload, keine Mission-/Task-IDs, keine Scheduler-/Ressourcen-/Budget-/Accounting-Daten und keine neuen Mission-Control-Aktionen. Das bereits vorhandene Remote-Stop-Verhalten bleibt unverändert. `remote_mission_status_test.go` und `remote_mission_status_contract.md` sichern diese schmale Beobachtungsgrenze ab.

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

`run_journal.go` ist die Recovery-Autorität für aktive Runs und read-only Missions. Persistiert werden nur recovery-relevante, begrenzte Metadaten; Roh-Toolausgaben und Zugangsdaten sollen nicht als zweites Transcript gespeichert werden.

Mission-Persistenz und die Projekt-/Git-Baseline sind in diese bestehende Recovery-Autorität integriert. Ein konkurrierendes zweites Journal bleibt unzulässig. Die Desktop-Mission-Status-Registry, die Orchestrierungsdiagnostik und die Mobile-Mission-Anzeige sind weiterhin nicht autoritative Beobachtungsflächen und dürfen nicht als Recovery-Ersatz verwendet werden.

Restart-Reconciliation, interne Postcondition-Verifikation und Transition-Planung sind read-only Evidenz-/Planungsschichten. Automatisches Mission-Resume/Retry/Replay ist weiterhin nicht implementiert. Ein durable erfolgreicher Task darf nur dann als wiederverwendbar gelten, wenn seine begrenzte Verification-Evidenz `verified` ist **und** die aktuelle Projekt-/Git-Reconciliation weiterhin `matched` ist. Eine spätere Recovery-Control-Grenze muss direkt vor jeder Ausführung Reconciliation, notwendige Verifikation, Dependency-Eignung und Attempt-Limits neu prüfen; blindes Replay bleibt unzulässig.

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

Currently executable child roles are Explorer, Planner and Reviewer. Their action schema is read-only and contains only project-tree reads, file reads, text search, approval-free read-only LSP and structured `finish`.

The schema does not contain file mutation, shell/commands, Git mutation, web/network, MCP tool calls, installation, memory writes, approval requests or recursive spawning. The model therefore cannot obtain those rights merely by requesting them in text.

### Scheduler / cancellation safety

The Scheduler separates the ready queue from actual resource admission. Local model inference defaults to one active slot.

A critical race boundary protects scheduled children:

1. `prepareScheduledAgentTask` validates the lease under the scheduler mutex.
2. The child receives a **detached task copy**, not a pointer into the shared mutable graph.
3. Model execution happens outside the mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` and `CancelMission` compete under the same scheduler lock.
5. Exactly one terminal winner may persist state/result and release the lease.

If cancellation wins first, late child results/usage are discarded. If successful completion finalizes first, later cancellation cannot rewrite that successful task. Parent-context cancellation is handled as cancellation as well. Whole-Mission cancellation terminalizes every still-nonterminal task after synchronous dispatch stops and refreshes the terminal scheduler snapshot. Deliberate race and product-boundary tests exercise these guarantees under Go's race detector.

### Desktop Mission status

The Desktop Mission display is an **observation-only boundary**:

- `/api/status` remains the existing loopback status source; Mission data is attached only for the exactly matching execution-scoped `RunID`.
- The caller-selected stable `MissionID` is never used as a run/journal identifier.
- Mission telemetry lives only in a bounded in-memory registry and old entries are evicted.
- The registry is not a journal, resume mechanism or source of authorization.
- The Desktop card reads Mission/scheduler/budget/task state but has no Mission-start, file, shell, Git, approval, project-mutation or terminal-command path.
- Displaying Planner/task status cannot convert `RequestedCapabilities` into executable authority.
- This Desktop extension grants no new Mobile/Remote API or authority.

Status observation therefore cannot start new work or bypass existing safety boundaries. Durable Mission recovery metadata is separate and flows only through `run_journal.go`. Any future Mission-control surface must be a separate, reviewed governance boundary.

### Durable Mission metadata, restart reconciliation and transition planning

`run_journal.go` remains the **sole** durable recovery authority. The existing `RunRecoveryState` contains an optional bounded structured Mission checkpoint for read-only Missions; no second Mission journal is introduced.

Persisted data is limited to recovery-relevant structured facts: Mission identity/objective/direct project scope/model/bounded constraints and success criteria, Mission budget, DAG/task identity and state, requested/granted capabilities, task budgets, Scheduler resource/queue/running/budget snapshots, completion/lifecycle/verification evidence, final Mission state/reason/accounting and scheduler-accepted usage.

Mission start also persists a bounded project/Git baseline:

- SHA-256 of canonical project identity,
- Git observation state,
- SHA-256 of Git-root identity instead of another raw repository path,
- exact Git `HEAD`,
- SHA-256 of the bytes from `git status --porcelain=v1 -z --untracked-files=all`,
- capture timestamp.

Raw porcelain output and therefore file paths are **not** durably stored. The baseline is evidence, not an authorization source.

The reconciliation Git observer is deliberately narrower than a general shell/Git tool path. It accepts no arbitrary command text and runs only the hard-coded read-only calls `rev-parse --show-toplevel`, `rev-parse --verify HEAD`, and `status --porcelain=v1 -z --untracked-files=all`. The observer is bounded by a three-second timeout and has no mutation operation.

On the next startup after an interrupted non-terminal Mission, current project/Git observation is classified against the baseline as `matched`, `project_unavailable`, `project_mismatch`, `git_changed`, `git_unavailable`, or `insufficient_evidence`. Older Mission journals without a baseline and insufficiently observable/non-Git baselines conservatively become `insufficient_evidence`; missing evidence is never treated as a match.

Task reconciliation is equally conservative:

- A task marked `running` at process interruption always becomes `interrupted_unknown`; running state is **never** evidence of success.
- Durable `succeeded`/legacy `completed` tasks remain `verify_postconditions` when verification is absent or failed; a `verified` success is terminal/reusable only while the **current** project/Git reconciliation remains `matched`.
- Not-started/non-terminal work becomes `pending` only after a complete project/Git match.
- Drift, unavailable or insufficient evidence leaves potentially reusable/pending work as `blocked_reconciliation`, including work with a historical `verified` state.
- Existing `failed`/`cancelled` work remains terminal.

Additional boundaries:

- Free text passes through existing secret redaction and strict count/length bounds.
- Raw Child/model responses, findings and tool transcripts are not copied into Mission recovery metadata.
- Durable checkpoints, reconciliation and postcondition verification cannot grant capabilities or change Scheduler limits/admission.
- The internal postcondition verifier never calls a model or re-executes a Child. It uses only the existing fixed read-only project/Git observer and fixed recovery checks.
- Verification evidence persists only SHA-256 digest, check count, state and timestamps; raw paths, Child/model output and raw verification output are not stored.
- After fresh project/Git observation, the run journal is reloaded; any intervening Mission recovery-state change prevents the verification write.
- Historical `verified` never overrides current drift. Drift does not regress the terminal verification record, but current reconciliation blocks reuse.
- Persisted requested/granted capabilities describe state only and do not become executable authority by themselves.
- Interrupted Missions are detected and reconciled but are **not** automatically resumed, retried or replayed.
- Normal chat `Continue` recovery explicitly rejects Mission journal entries so structured Mission work cannot be blindly replayed as an ordinary prompt.
- An interrupted Mission remains visible as recovery evidence if the project directory disappeared; that case is blocking `project_unavailable` and grants no new filesystem authority.
- Late/stale Child results remain non-authoritative; terminal usage is based only on Scheduler-accepted results.

The recovery transition planner remains strictly outside execution authority:

- It executes no Mission/task work, calls no model or tool, grants no capability and requests no Scheduler lease.
- It reconstructs the durable DAG and runs existing graph validation before any candidate is emitted. Duplicate IDs, missing dependencies, cycles, invalid task metadata or more than 64 tasks invalidate the whole plan.
- Present lifecycle counters must be nonnegative, internally consistent and no greater than three attempts per task. Missing lifecycle evidence remains allowed for legacy states but cannot make `failed`/`retryable` work retryable.
- Fixed planning bounds are three attempts per task and 192 attempts per Mission (`64 × 3`). Prospective reservations are planning facts only and never Scheduler leases.
- Crash-running work becomes `interrupted_review_required`, never directly `resume_candidate` or `retry_candidate`.
- `reuse_verified`, `resume_candidate` and `retry_candidate` require every dependency to be currently `verified` and reusable; otherwise `blocked_dependency` applies.
- Current reconciliation other than `matched` blocks potentially reusable/continuable work even when historical verification says `verified`.
- Malformed or inconsistent recovery structures produce only `invalid_recovery_state`; fail-open candidates are forbidden.
- The plan does not modify historical Scheduler-accepted usage or Mission accounting and grants no admission/execution authority.

### Orchestration diagnostics

Desktop orchestration diagnostics are likewise an **observation-only** surface, not a control boundary:

- `/api/status` adds machine-readable backend, queue, logical-task and resource facts without changing Scheduler configuration.
- `at_capacity` only means a resource limit is fully occupied. `saturated` is reported only when that resource is full and matching work is actually waiting for it.
- Diagnostics distinguish Ollama offline, no selected model, selected model missing locally, queue-limit pressure and resource waiting pressure.
- During a Mission, reported limits come from the actually normalized Scheduler limits; the display must not widen them or automatically “optimize” them from hardware guesses.
- Diagnostic data is non-durable, not a recovery store, not a capability source and not a Mission-control path.
- The Desktop diagnostics UI contains no chat, approval, project-mutation, terminal or Scheduler-policy endpoint.
- Mobile Remote receives no broader payload or authority from this feature.
- Observed saturation is **not** benchmark or performance evidence and by itself does not justify changing model concurrency. The dedicated benchmark path provides measurement data but likewise never changes Scheduler policy or capabilities automatically.

### Orchestration benchmark safety

The benchmark paths are measurement boundaries, not new runtime authority:

- The synthetic dispatcher benchmark uses only already-authorized read-only child tasks plus a local synthetic executor. It changes no Scheduler default or product configuration.
- The real Ollama benchmark is disabled by default and runs only with explicit `LOCALCODE_BENCH_OLLAMA=1` opt-in.
- It accepts loopback Ollama endpoints only and requires the exact name of an already-installed model.
- It never calls `EnsureRunning`, `Pull` or an installer, so it starts, downloads and installs nothing.
- Benchmark output cannot grant capabilities, bypass admission boundaries or widen a resource limit by itself.
- Measured client-request overlap or increased throughput does not prove simultaneous GPU-kernel execution or token inference; hardware/backend parallelism must not be inferred without separate evidence.
- Any change to model-slot limits or Scheduler policy requires a separate review with VRAM/RAM pressure, fairness, cancellation and stability as acceptance criteria.

### Mobile Remote / Android

Desktop and Mobile are separate server boundaries. The Desktop API remains loopback-oriented. Remote exposes a narrower API and grants no additional tool authority.

Important protections include:

- pairing initiated from Desktop creates a random device token;
- LocalCode persists only the token's SHA-256 hash;
- long-lived tokens are not placed in SSE URLs; streams use short-lived tickets;
- Origin/Fetch-Site checks limit unwanted browser POSTs;
- LAN Remote uses HTTPS and the Android shell pins the expected TLS SHA-256 fingerprint;
- manual Android endpoints must be private HTTPS IP endpoints with the matching fingerprint;
- mDNS/QR/deep-link discovery transports endpoint/fingerprint data but grants no new authority;
- the JavaScript bridge is deliberately narrow: file picker and Android speech recognition only;
- the bridge never executes LocalCode tools;
- pending WebView file-chooser callbacks are closed/cancelled on replacement, failure and Activity teardown;
- speech recognition starts only when Android has a compatible handler; picker/speech launch failures are surfaced visibly in Remote;
- attachments then pass through normal Remote/backend validation.

The Mobile Mission indicator does not widen this boundary. It uses only the existing authenticated `/remote/api/status` fields `running` and `run_phase`, and is shown only for `running == true && run_phase == "mission-read-only"`. No new `/remote/api/mission` endpoint or Mobile `mission` payload is added; Mobile receives no Mission/task IDs, scheduler/resource/budget/accounting data or new Mission-control actions. Existing Remote stop behavior is unchanged. `remote_mission_status_test.go` and `remote_mission_status_contract.md` guard this narrow observation surface.

### Network and MCP

Public web fetches validate destinations and reject loopback, link-local, private and other non-public addresses. DNS rebinding is mitigated by dialing the exact IP that was validated before connection.

MCP is explicitly configured. Stdio/HTTP sessions run with timeouts and controlled subprocess lifecycle. Skill/prompt metadata cannot self-enable MCP authority.

### Skills, commands and memories

- Project/slash commands are text templates and do not execute shell commands by themselves.
- Rule/skill files extend model context, not policy.
- Skills declaring non-read-only tool authority or scripts/commands do not become automatically privileged instructions.
- Skill resources remain subject to path, size and approval boundaries.
- Durable memories reject secret-like contents and do not expand tool authority.

### Recovery

`run_journal.go` is the recovery authority for active runs and read-only Missions. Only bounded recovery-relevant metadata is persisted; raw tool output and credentials should not become a second transcript.

Mission persistence and the project/Git baseline are integrated with this existing recovery authority. A competing second journal remains forbidden. The Desktop Mission status registry, orchestration diagnostics and Mobile Mission indicator remain non-authoritative observation surfaces and must not be used as recovery substitutes.

Restart reconciliation, the internal postcondition verifier and transition planning are read-only evidence/planning layers. Automatic Mission resume/retry/replay remains unimplemented. A durable successful task may be considered reusable only when its bounded verification evidence is `verified` **and** current project/Git reconciliation remains `matched`. Any future recovery-control boundary must freshly re-evaluate reconciliation, required verification, dependency eligibility and attempt limits immediately before execution and must still obey normal Scheduler, budget and cancellation boundaries; blind replay remains forbidden.

### Future mutation agents

Builder/worktree mutation is not implemented yet. When introduced, all existing LocalCode boundaries continue to apply: controlled workspaces/worktrees, no unsupervised concurrent mutation of the same workspace, normal approvals and SHA preconditions, diff-reviewable results, verification after the last mutation, a controlled Integrator boundary and safe cancellation/recovery without blind destructive reset/clean shortcuts.
