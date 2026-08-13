# Architecture / Architektur

## Deutsch

LocalCode ist eine einzelne Go-Anwendung mit eingebettetem Web-Frontend und einer ausschließlich an `127.0.0.1` gebundenen HTTP-/SSE-API.

- `main.go`: Versionsübergabe, kompaktes Startfenster, automatische Laufzeiteinrichtung und lokaler Serverstart.
- `runtime_bootstrap.go`: Fortschrittsgesteuerte Prüfung und automatische Vervollständigung von Ollama, konfigurierten Modellen und der ausgewählten Coding-Agent-Engine.
- `ollama.go`: Dienstsuche, Modellinventar, Modell-Download, Chat und lokale Bildanalyse.
- `server.go`: API, SSE, Projekte, Aufgaben, Einstellungen und Genehmigungen.
- `project_catalog.go` und `history.go`: Projekt-Aliase, Anheften, Entfernen, Wiederherstellen und persistente Aufgabenaktionen.
- `agent.go`, `agent_supervisor.go` und `continuation.go`: Agentenschleife, deterministische Steuerung, Abschlussprüfung/-Review, Fortsetzungen und Abbruch.
- `instruction_context.go`: globale und projektbezogene Regelkette, Cursor-Regeln sowie lokale Skill-Indizes und relevante Skill-Inhalte für den Agentenstart.
- `memory.go`: lokale dauerhafte Agentenerinnerungen, Scope-Filterung, Secret-Blockade und Kontext-Zusammenfassung.
- `asset_tools.go`: validierte lokale SVG-/Icon-Erzeugung, Raster-/Icon-Asset-Erzeugung, Rendering und Konvertierung mit XML-, Signatur-, Größen- und Dimensionsprüfung vor dem Schreiben.
- `aider_engine.go`: isolierte Aider-/uv-Installation, Aufrufparameter, Verlauf, Backups und geschütztes Undo.
- `mcp.go` und `mcp_builtin.go`: eingebaute und externe MCP-Sitzungen einschließlich kontrollierter Prozessfreigabe.
- `path_tools.go`: Dateioperationen und kanonische Sandboxprüfung einschließlich Symlinks und NTFS-Junctions.
- `web_tools.go`: öffentliche Webabrufe mit IP-Prüfung beim Verbindungsaufbau.
- `tool_registry.go`, `tool_install.go`, `project_automation.go`: Werkzeugerkennung, Installation, Build und Deployment.
- `static/index.html` und `static/i18n.js`: Desktop-Oberfläche und vollständige deutsche/englische Sprachkataloge.

### Start- und Bootstrap-Ablauf

1. Konfiguration und frühere Produktdaten werden aus portablen oder benutzerlokalen Verzeichnissen geladen.
2. Eine bereits laufende ältere LocalCode-Instanz wird kontrolliert abgelöst.
3. Ein ausschließlich an Loopback gebundenes, token-geschütztes Startfenster zeigt alle weiteren Schritte und bietet bei Fehlern Wiederholen, Log-Ordner, eingeschränkten Start und Beenden.
4. Ollama wird gesucht und gestartet. Fehlt es unter Windows, lädt LocalCode den offiziellen Installer, prüft dessen Authenticode-Signatur und installiert ihn unbeaufsichtigt für den Benutzer.
5. Fehlende für LocalCode und die ausgewählte Engine benötigte Modelle werden über die Ollama-API geladen. Auf einem frischen System wird nur das konfigurierte Standardmodell geladen, damit ein alter gespeicherter Modellname keinen zweiten großen Download auslöst.
6. Die ausgewählte externe Engine wird geprüft; Aider wird gegen die angeheftete Version geprüft. Falls nötig, installiert LocalCode das geprüfte portable `uv`, eine isolierte Python-3.12-Laufzeit und `aider-chat==0.86.2`.
7. Erst nach erfolgreicher Verifikation startet die lokale UI-API. Details und Fehler werden im LocalCode-Log festgehalten.

Zustände werden unter einem Mutex verwaltet. Ereignisse werden im Speicher sofort veröffentlicht und durch einen zusammenfassenden Hintergrundschreiber atomar persistiert. Offene Genehmigungen sind Backendzustand und erscheinen zusätzlich als feste Entscheidungsleiste. Jede UI-Instanz fordert ihren Snapshot mit konkreter Projekt- und Aufgaben-ID an; ein Agentenlauf bleibt global auf eine gleichzeitig aktive Ausführung begrenzt.

Vor dem Abschluss einer Editieraufgabe prüft LocalCode nicht nur die Modellmeldung, sondern den beobachteten Arbeitszustand: geänderte und erwähnte Dateien, erkannte Funktionsmarker, Postconditions von Dateiwerkzeugen und den Nachweis einer passenden Prüfung nach der letzten Code-/App-/Tool-Änderung. Reine Dokumentationsänderungen erfüllen keine Implementierungsaufgabe; Dokumentationsaufgaben und reine Dateioperationen werden gesondert erkannt.

Der Agentenstart baut eine begrenzte Instruktionskette. Pro globalem Verzeichnis gilt `AGENTS.override.md` vor `AGENTS.md`; unterstützt werden LocalCode-Konfiguration und `CODEX_HOME`. Projektseitig gilt ebenfalls Override vor Basis, danach Fallback-Dateien wie `CLAUDE.md`, `README.md` und `STATE.md`. `.cursor/rules` werden eingebettet, wenn sie `alwaysApply` setzen, per `globs` zu genannten Projektdateien passen oder textlich zur Nutzeraufgabe passen. Frontmatter unterstützt neben Inline-Werten auch einfache YAML-Listen. Skills werden in Projekt- und globalen Verzeichnissen indexiert; relevante `SKILL.md`-Dateien werden zusätzlich vollständig begrenzt eingebettet, außer sie deklarieren nicht-read-only `permissions`/`tools` oder `scripts`/`commands`. Solche Skills bleiben als `approval-required` im Index sichtbar und können per `skill_read` gelesen werden, erweitern aber keine Berechtigungen. Die read-only Aktionen `skill_list`, `skill_read`, `skill_list_resources` und `skill_read_resource` erlauben progressives Nachladen einzelner Skills und ihrer Textressourcen während des Agentenlaufs. Die schreibende Aktion `skill_copy_resource` kopiert binäre oder anderweitig projektbenötigte Skill-Ressourcen erst nach normaler Dateigenehmigung in einen Projektpfad.

Agentenerinnerungen werden als normalisierte Einträge in der lokalen Konfiguration gespeichert. Jeder neue native Agentenlauf erhält globale Erinnerungen und Erinnerungen des aktiven Projekts im eingebetteten Kontext. Schreibende Memory-Aktionen speichern atomar über denselben Konfigurationspfad; Löschungen verlangen eine konkrete ID.

SVG-/Icon-Ressourcen laufen über ein eigenes Werkzeug, wenn der native Agent `create_svg_asset` nutzt. Der Pfad muss auf `.svg` enden, der Inhalt muss gültiges XML mit `<svg>`-Wurzel und Größenangabe sein, und Skripte, Event-Handler sowie `javascript:`-URLs werden vor der Dateioperation blockiert.

Raster- und Icon-Dateien laufen über `create_image_asset`, wenn der Agent vollständige Bildbytes als Data-URL oder Base64 besitzt. Unterstützt werden `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico` und `.bmp`. LocalCode dekodiert die Binärdaten, begrenzt sie auf 16 MiB, prüft Format-Signaturen und Dimensionen und schreibt danach über dieselbe kanonische Projektpfadgrenze mit Backup und Datei-Postcondition.

Vorhandene Rasterdateien laufen über `convert_image_asset`, wenn ein lokales Bild in ein anderes Projektbild umgewandelt werden soll. Quellen sind decodierbare PNG/JPG/JPEG/GIF-Dateien und PNG-basierte ICO-Dateien; Ziele sind `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico`. Optional angegebene Zielgrößen werden mit einer deterministischen lokalen Skalierung angewendet. PNG/JPEG/ICO werden lokal encodiert; WebP wird aus einer temporären lokalen HTML/PNG-Zwischenquelle über denselben Headless-Browser-Renderer erzeugt. Nach dem Encoding wird die Zieldatei erneut per Signatur und Dimensionen validiert.

SVG-/HTML-/Canvas-Rendering läuft über `render_asset`. Die Quelle muss lokal im Projekt liegen und auf `.svg`, `.html` oder `.htm` enden; Ziele sind `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico`. SVG wird vorher mit dem SVG-Validator geprüft. HTML-Quellen mit externen HTTP(S)-Referenzen werden abgelehnt. Der Renderer nutzt ein vorhandenes Edge/Chrome im Headless-Modus mit temporärem Profil, totem lokalen Proxy und Dimensionsvorgabe. PNG wird direkt aus dem Browser-Screenshot validiert; JPEG und ICO werden aus dem validierten PNG erzeugt und anschließend über dieselbe Bildsignaturprüfung validiert. WebP wird direkt als Headless-Browser-Screenshot erzeugt und über RIFF/WebP-Signatur sowie Dimensionen geprüft.

MCP-Sitzungen speichern serverweite `instructions` aus der Initialisierung. Die Agentenzusammenfassung enthält diese Hinweise zusammen mit den aktivierten Servern; eingebaute MCP-Server liefern eigene kurze Nutzungsregeln.

## English

LocalCode is a single Go application with an embedded web frontend and an HTTP/SSE API bound exclusively to `127.0.0.1`.

- `main.go`: version handover, compact startup window, automatic runtime setup, and local server startup.
- `runtime_bootstrap.go`: progress-driven verification and automatic completion of Ollama, configured models, and the selected coding-agent engine.
- `ollama.go`: service discovery, model inventory, model pulls, chat, and local image analysis.
- `server.go`: API, SSE, projects, tasks, settings, and approvals.
- `project_catalog.go` and `history.go`: aliases, pin/remove/restore behavior, and persistent task actions.
- `agent.go`, `agent_supervisor.go`, and `continuation.go`: agent loop, deterministic supervision, completion guard/review, continuations, and cancellation.
- `instruction_context.go`: global and project rule chain, Cursor rules, plus local skill indexes and relevant skill contents for agent startup.
- `memory.go`: local durable agent memories, scope filtering, secret blocking, and context summarization.
- `asset_tools.go`: validated local SVG/icon creation plus raster/icon asset creation, rendering, and conversion with XML, signature, size, and dimension checks before writing.
- `aider_engine.go`: isolated Aider/uv setup, invocation arguments, histories, backups, and guarded undo.
- `mcp.go` and `mcp_builtin.go`: built-in and external MCP sessions with controlled process reaping.
- `path_tools.go`: file operations and canonical sandbox checks including symlinks and NTFS junctions.
- `web_tools.go`: public web fetches with IP validation at connection time.
- `tool_registry.go`, `tool_install.go`, and `project_automation.go`: tool discovery, installation, build, and deployment.
- `static/index.html` and `static/i18n.js`: desktop UI and complete German/English catalogs.

### Startup and bootstrap flow

1. Configuration and legacy product data are loaded from portable or per-user directories.
2. A running older LocalCode instance is replaced in a controlled manner.
3. A loopback-only, token-protected startup window displays every following step and offers retry, log-folder access, limited mode, and exit on failure.
4. Ollama is discovered and started. If missing on Windows, LocalCode downloads the official installer, validates its Authenticode signature, and installs it unattended for the current user.
5. Missing models required by LocalCode and the selected engine are pulled through the Ollama API. On a fresh system only the configured default is pulled so a stale historical model does not trigger a second large download.
6. The selected external engine is checked; Aider is checked against the pinned version. When needed, LocalCode installs the verified portable `uv`, an isolated Python 3.12 runtime, and `aider-chat==0.86.2`.
7. The loopback UI API starts only after successful verification. Details and failures are written to the LocalCode log.

State is mutex-protected. Events are published immediately in memory and atomically persisted by one coalescing background writer. Pending approvals are backend state and are duplicated in a fixed decision bar. Every UI instance requests an explicit project/task snapshot; agent execution remains globally limited to one active run.

Before completing an editing task, LocalCode checks the observed working state rather than only the model's message: changed and mentioned files, requested capability markers, file-tool postconditions, and proof of a suitable check after the last code/app/tool change. Documentation-only edits do not satisfy implementation tasks; documentation tasks and pure file operations are classified separately.

Agent startup builds a bounded instruction chain. For each global directory, `AGENTS.override.md` wins over `AGENTS.md`; LocalCode configuration and `CODEX_HOME` are supported. Project-side discovery uses the same override-before-base rule, then fallback files such as `CLAUDE.md`, `README.md`, and `STATE.md`. `.cursor/rules` are embedded when they set `alwaysApply`, match mentioned project files through `globs`, or textually match the user task. Frontmatter supports inline values plus simple YAML lists. Skills are indexed from project and global directories; relevant `SKILL.md` files are also embedded with limits unless they declare non-read-only `permissions`/`tools` or `scripts`/`commands`. Those skills remain visible as `approval-required` in the index and can be read through `skill_read`, but they do not grant permissions. The read-only `skill_list`, `skill_read`, `skill_list_resources`, and `skill_read_resource` actions let an agent progressively load individual skills and their text resources during a run. The mutating `skill_copy_resource` action copies binary or otherwise project-needed skill resources into a project path only after normal file-change approval.

Agent memories are stored as normalized entries in the local configuration. Every new native agent run receives global memories and memories for the active project in the embedded context. Writing memory actions persist atomically through the same configuration path; deletion requires a concrete ID.

SVG/icon resources use a dedicated tool when the native agent calls `create_svg_asset`. The path must end in `.svg`, content must be valid XML with an `<svg>` root and size metadata, and scripts, event handlers, and `javascript:` URLs are blocked before the file operation.

Raster and icon files use `create_image_asset` when the agent has complete image bytes as a data URL or Base64. Supported extensions are `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico`, and `.bmp`. LocalCode decodes the binary data, caps it at 16 MiB, checks format signatures and dimensions, and then writes through the same canonical project path boundary with backup and file postcondition.

Existing raster files use `convert_image_asset` when a local image should be converted into another project image. Sources are decodable PNG/JPG/JPEG/GIF files and PNG-backed ICO files; targets are `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`. Optional target dimensions are applied with deterministic local scaling. PNG/JPEG/ICO are encoded locally; WebP is produced from a temporary local HTML/PNG intermediate through the same headless-browser renderer. After encoding, the target file is validated again by signature and dimensions.

SVG/HTML/canvas rendering uses `render_asset`. The source must be local to the project and end in `.svg`, `.html`, or `.htm`; targets are `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`. SVG is checked with the SVG validator first. HTML sources with external HTTP(S) references are rejected. Rendering uses an installed Edge/Chrome browser in headless mode with a temporary profile, dead local proxy, and explicit dimensions. PNG is validated from the browser screenshot; JPEG and ICO are produced from the validated PNG and then checked through the same image-signature validation. WebP is produced directly as a headless-browser screenshot and checked through RIFF/WebP signature and dimensions.

MCP sessions store server-wide `instructions` from initialization. The agent summary includes these hints together with the enabled servers; built-in MCP servers provide their own short usage rules.
