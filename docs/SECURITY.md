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

Nicht enthalten sind:

- Datei-Mutation,
- Shell/Befehle,
- Git-Mutation,
- Web/Netzwerk,
- MCP-Tool-Aufrufe,
- Installation,
- Memory-Schreiben,
- Approval-Requests,
- rekursives Child-Spawning.

Damit kann ein Child diese Rechte nicht allein durch Modelltext anfordern.

### Scheduler / Cancellation-Sicherheit

Der Scheduler trennt Ready-Queue und tatsächliche Ressourcenadmission. Standardmäßig gibt es nur einen aktiven lokalen Model-Inference-Slot.

Ein besonders wichtiger Race-Schutz gilt für laufende Scheduled Children:

1. `prepareScheduledAgentTask` prüft den Lease unter dem Scheduler-Mutex.
2. Das Child erhält eine **abgetrennte Kopie** des Tasks, keinen Pointer in den gemeinsam mutierbaren Graphen.
3. Das Modell läuft außerhalb des Mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` und `CancelMission` konkurrieren wieder unter derselben Scheduler-Sperre.
5. Genau ein terminaler Gewinner darf Zustand/Resultat festschreiben und den Lease freigeben.

Wenn Cancellation zuerst gewinnt, werden verspätete Child-Resultate/Usage nicht in den Task geschrieben. Wenn Completion zuerst erfolgreich finalisiert wurde, ändert ein späteres Cancel den erfolgreichen Task nicht mehr. Parent-Context-Cancellation wird ebenfalls als Cancellation behandelt. Diese Grenze besitzt absichtliche Konkurrenztests unter Go's Race Detector.

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

`run_journal.go` ist die Recovery-Autorität für aktive Runs. Persistiert werden nur recovery-relevante, begrenzte Metadaten; Roh-Toolausgaben und Zugangsdaten sollen nicht als zweites Transcript gespeichert werden.

Zukünftige Mission-Persistenz muss in diese Recovery-Autorität integriert werden. Ein konkurrierendes zweites Journal würde widersprüchliche Wiederaufnahmeentscheidungen ermöglichen und ist daher nicht vorgesehen.

### Zukünftige Mutation-Agenten

Builder-/Worktree-Mutation ist noch nicht implementiert. Wenn sie eingeführt wird, gelten weiterhin sämtliche bestehenden LocalCode-Grenzen:

- eigener kontrollierter Workspace/Worktree,
- keine unsupervised concurrent mutation desselben Workspace,
- normale Genehmigungen und SHA-Preconditions,
- diff-reviewbare Resultate,
- Verifikation nach der letzten Mutation,
- Integrator als kontrollierte Zusammenführungsgrenze,
- sichere Cancellation/Recovery ohne blindes `reset --hard`/`clean`.

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

The schema does not contain file mutation, shell/commands, Git mutation, web/network, MCP tool calls, installation, memory writes, approval requests or recursive child spawning. The model therefore cannot obtain those rights merely by requesting them in text.

### Scheduler / cancellation safety

The Scheduler separates the ready queue from actual resource admission. Local model inference defaults to one active slot.

A critical race boundary protects scheduled children:

1. `prepareScheduledAgentTask` validates the lease under the scheduler mutex.
2. The child receives a **detached task copy**, not a pointer into the shared mutable graph.
3. Model execution happens outside the mutex.
4. `finalizeScheduledAgentTask`, `CancelTask` and `CancelMission` compete under the same scheduler lock.
5. Exactly one terminal winner may persist state/result and release the lease.

If cancellation wins first, late child results/usage are discarded. If successful completion finalizes first, later cancellation cannot rewrite that successful task. Parent-context cancellation is handled as cancellation as well. Deliberate competing tests run under Go's race detector.

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

### Network and MCP

Public web fetches validate destinations and reject loopback, link-local, private and other non-public addresses. DNS rebinding is mitigated by dialing the exact IP that was validated before connection.

MCP is explicitly configured. Stdio/HTTP sessions run with timeouts and controlled subprocess lifecycle. Skill/prompt metadata cannot self-enable MCP authority.

### Skills, commands and memories

- Project/slash commands are text templates and do not execute shell commands by themselves.
- Rule/skill files extend model context, not policy.
- Skills declaring non-read-only tool authority or scripts/commands do not become automatically privileged instructions.
- Skill resources remain subject to path, size and approval boundaries.
- Durable memories reject secret-like content and do not expand tool authority.

### Recovery

`run_journal.go` is the recovery authority for active runs. Only bounded recovery-relevant metadata is persisted; raw tool output and credentials should not become a second transcript.

Future Mission persistence must integrate with this recovery authority. A competing second journal would permit contradictory resume decisions and is intentionally avoided.

### Future mutation agents

Builder/worktree mutation is not implemented yet. When introduced, all existing LocalCode boundaries continue to apply: controlled workspaces/worktrees, no unsupervised concurrent mutation of the same workspace, normal approvals and SHA preconditions, diff-reviewable results, verification after the last mutation, a controlled Integrator boundary and safe cancellation/recovery without blind destructive reset/clean shortcuts.
