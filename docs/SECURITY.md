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
- Prompts, Regeln, Skills, Memories oder Planner-Ausgaben können keine Berechtigungen selbst erweitern.
- Geheimnisse sollen nicht in normaler Konfiguration, Memories oder Recovery-Metadaten persistiert werden.

### Planner, Capabilities und Agent Teams

`RequestedCapabilities` aus Planner-/Task-Vorschlägen sind reine Planungsdaten. Ausführbare `Capabilities` werden davon getrennt gehalten und müssen von einer vertrauenswürdigen Governance-/Parent-Grenze vergeben werden.

Aktuell ausführbare Child-Rollen sind Explorer, Planner und Reviewer. Das Child-Action-Schema enthält ausschließlich Projektbaum-/Datei-/Text-/LSP-Lesen und strukturiertes `finish`. Datei-Mutation, Shell, Git-Mutation, Web/Netzwerk, MCP-Tool-Aufrufe, Installation, Memory-Schreiben, Approval-Requests und rekursives Child-Spawning fehlen.

### Scheduler / Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und Ressourcenadmission; lokale Modellinferenz besitzt standardmäßig einen aktiven Slot. Scheduled Children erhalten abgetrennte Task-Kopien. Preparation, Finalize und Cancellation werden an derselben Scheduler-Sperre serialisiert. Cancellation-first verwirft verspätete Child-Resultate/Usage; Completion-first bleibt erfolgreich.

Bei vollständigem Mission-Cancel werden nach beendetem synchronen Dispatch alle noch nicht terminalen Tasks als `cancelled` terminalisiert und der abschließende Scheduler-Snapshot erneut erstellt. Konkurrenz- und Produktgrenzentests laufen unter Go's Race Detector.

### Desktop Mission-Status

Die Desktop-Mission-Anzeige ist eine reine **Beobachtungsgrenze**:

- `/api/status` bleibt die Loopback-Statusquelle; Mission-Daten werden nur für die passende execution-scoped `RunID` ergänzt.
- Die stabile `MissionID` wird nicht als Run-/Journal-Identifier verwendet.
- Mission-Telemetrie liegt ausschließlich in einer begrenzten In-Memory-Registry.
- Die Registry ist kein Journal, Resume-Mechanismus oder Berechtigungsquelle.
- Die Desktop-Card besitzt keinen Mission-Start-, Datei-, Shell-, Git-, Approval-, Projektmutations- oder Terminal-Command-Pfad.
- Statusanzeige kann `RequestedCapabilities` nicht in ausführbare Rechte umwandeln.

### Mobile Remote / Android

Desktop und Mobile sind getrennte Servergrenzen. Die Desktop-API bleibt Loopback-orientiert. Remote besitzt eine schmalere API und erweitert keine Werkzeugrechte.

Wichtige Remote-Schutzmaßnahmen:

- Pairing erzeugt ein zufälliges Gerätetoken; persistent gespeichert wird nur dessen SHA-256-Hash.
- Dauerhafte Tokens stehen nicht in SSE-URLs; Streams verwenden kurzlebige Tickets.
- Cross-Origin-/Fetch-Site-Prüfungen begrenzen unerwünschte Browser-POSTs.
- LAN-Remote verwendet HTTPS; Android pinnt den erwarteten TLS-SHA-256-Fingerprint.
- mDNS/QR/Deep-Link-Discovery transportiert Endpoint/Fingerprint, verleiht aber keine zusätzlichen Rechte.
- Die JavaScript-Brücke bleibt auf Dateipicker und Android Speech Recognition begrenzt und führt keine LocalCode-Werkzeuge aus.
- Attachments laufen durch normale Remote-/Backend-Validierung.

#### Mobile Mission-Anzeige

Die Mobile-Mission-Anzeige ist absichtlich **narrow read-only observation**:

- Sie verwendet ausschließlich die bereits vorhandene authentifizierte `/remote/api/status`-Antwort.
- Relevant sind nur `running` und `run_phase`.
- Eine Mission wird nur angezeigt, wenn `running == true` und `run_phase == "mission-read-only"`.
- Es wird kein `/remote/api/mission`-Endpunkt hinzugefügt.
- Das reichere Desktop-`mission`-Objekt wird Remote nicht bereitgestellt.
- Die UI erhält dadurch keine Mission-/Task-IDs, Scheduler-/Queue-/Ressourcendaten, Budgets, Usage/Accounting oder neue Mission-Control-Aktionen.
- Es werden keine neuen Tool-, Datei-, Shell-, Git-, Netzwerk-, Approval- oder Mutation-Rechte vergeben.
- Das bereits vorhandene Remote-Stop-Verhalten bleibt unverändert; dieser Slice fügt keinen neuen Steuerpfad hinzu.

`remote_mission_status_test.go` und `remote_mission_status_contract.md` schützen diese Grenze explizit gegen spätere unbemerkte Ausweitung.

### Netzwerk, MCP, Skills und Memories

Öffentliche Webabrufe prüfen Zieladressen und blockieren nichtöffentliche Ziele; DNS-Rebinding wird durch Dialing der validierten IP begrenzt. MCP ist explizit konfiguriert und besitzt Timeouts/kontrollierten Prozesslebenszyklus. Regel-/Skill-/Command-/Memory-Inhalte erweitern Modellkontext, nicht Policy oder Authority.

### Recovery

`run_journal.go` ist die Recovery-Autorität für aktive Runs. Zukünftige Mission-Persistenz muss in diese Autorität integriert werden. Desktop- und Mobile-Mission-Anzeigen sind ausdrücklich nicht persistent und dürfen nicht als Recovery-Ersatz verwendet werden.

### Zukünftige Mutation-Agenten

Builder-/Worktree-Mutation ist noch nicht implementiert. Wenn sie eingeführt wird, gelten weiterhin kontrollierte Workspaces/Worktrees, keine unsupervised concurrent mutation desselben Workspace, normale Genehmigungen und SHA-Preconditions, diff-reviewbare Resultate, Verifikation nach der letzten Mutation, Integrator als kontrollierte Zusammenführungsgrenze und sichere Cancellation/Recovery ohne blindes destruktives Reset/Clean.

---

## English

LocalCode uses multiple application-level protection layers rather than relying on one all-powerful sandbox. These boundaries apply to both LocalCode Native and external coding engines.

### Baseline rules

- File, command, network and installation actions pass through LocalCode policy and approvals.
- Project/workspace paths are canonicalized; symlink and NTFS-junction escapes are rejected.
- File mutations use version/SHA preconditions, conflict-aware atomic writes and postconditions.
- Destructive/high-risk operations are blocked or routed through stricter paths.
- Owned processes have timeouts, cancellation and Windows process-tree termination.
- There is no default or silently persistent `danger-full-access` mode.
- Prompts, rules, skills, memories and Planner output cannot self-escalate authority.
- Secrets should not be persisted in ordinary configuration, memory or recovery metadata.

### Planner, capabilities and Agent Teams

`RequestedCapabilities` from Planner/task proposals are planning data only. Executable capabilities remain separate and must be granted by a trusted governance/parent boundary.

Executable child roles are Explorer, Planner and Reviewer. Their action schema is limited to project-tree/file/text/LSP reads and structured `finish`; file mutation, shell, Git mutation, web/network, MCP tool calls, installation, memory writes, approval requests and recursive spawning are absent.

### Scheduler / cancellation safety

The Scheduler separates ready queue and resource admission; local model inference defaults to one slot. Scheduled children receive detached task copies. Preparation, finalization and cancellation are serialized at the same scheduler-lock boundary. Cancellation-first discards late results/usage; completion-first remains successful.

Whole-Mission cancellation terminalizes every remaining nonterminal task as `cancelled` after synchronous dispatch stops and refreshes the terminal scheduler snapshot. Race and product-boundary tests exercise these guarantees under Go's race detector.

### Desktop Mission status

Desktop Mission status is **observation only**. `/api/status` attaches Mission data only for the matching execution-scoped `RunID`; telemetry is bounded/in-memory, not a journal or authorization source. The Desktop card has no Mission-start, file, shell, Git, approval, project-mutation or terminal-command path.

### Mobile Remote / Android

Desktop and Mobile are separate server boundaries. Remote exposes a narrower API and grants no additional tool authority. Pairing tokens are hashed at rest, SSE uses short-lived tickets, origin/fetch-site checks protect mutations, LAN Remote uses HTTPS with Android TLS fingerprint pinning, and the JavaScript bridge is limited to file picking and speech input.

#### Mobile Mission display

Mobile Mission status is deliberately **narrow read-only observation**:

- it reuses only the existing authenticated `/remote/api/status` response;
- only `running` and `run_phase` are needed;
- an active Mission is shown only for `running == true && run_phase == "mission-read-only"`;
- no `/remote/api/mission` endpoint is added;
- the richer Desktop `mission` object is not exposed to Remote;
- Mobile receives no Mission/task IDs, scheduler/queue/resource details, budgets, usage/accounting or new Mission-control actions;
- no new tool/file/shell/Git/network/approval/mutation authority is granted;
- existing Remote stop behavior is unchanged and this slice adds no new control path.

`remote_mission_status_test.go` and `remote_mission_status_contract.md` explicitly guard this boundary against accidental widening.

### Network, MCP, skills and memories

Public web fetches validate destinations and reject non-public targets; DNS-rebinding protection dials the validated IP. MCP is explicit and lifecycle-controlled. Rules, skills, commands and memories extend model context, not policy or authority.

### Recovery

`run_journal.go` remains the recovery authority for active runs. Future Mission persistence must integrate with it. Desktop and Mobile Mission displays are non-durable observation only and must not become recovery substitutes.

### Future mutation agents

Builder/worktree mutation is not implemented. When introduced, existing controls remain: managed workspaces/worktrees, no unsupervised concurrent mutation of the same workspace, approvals/SHA preconditions, diff-reviewable results, post-mutation verification, controlled integration and safe cancellation/recovery without blind destructive reset/clean shortcuts.
