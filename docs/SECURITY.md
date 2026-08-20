# Security model / Sicherheitsmodell

## Deutsch

LocalCode kombiniert folgende Anwendungsschutzschichten:

- Genehmigungsmodi für Dateiänderungen, Befehle, Werkzeuginstallationen und Netzwerkaktionen
- Projekt-, Workspace- und unbeschränkte Pfadmodi sowie zusätzliche freigegebene Wurzeln
- kanonische Pfadprüfung nach Auflösung vorhandener Symlinks und NTFS-Junctions, auch bei neu anzulegenden Zieldateien
- blockierte Befehlsmuster und harte Schutzregeln für destruktive Git-/Systemaktionen
- Zeitlimits, Prozessgruppen und vollständigen Prozessbaum-Abbruch unter Windows
- Task- und Tool-Hooks laufen als normale nicht-interaktive Projektbefehle mit denselben Timeouts, Umgebungsregeln, Blocklisten und Prozessabbrüchen; Before-Tool-Fehler stoppen die Werkzeugaktion
- einen globalen Netzwerkschalter
- Schutz vor Loopback-, Link-local-, privaten und sonstigen nichtöffentlichen Zieladressen
- DNS-Rebinding-Schutz: verbunden wird mit genau der zuvor beim Dial validierten IP, nicht mit einer zweiten Namensauflösung
- explizite MCP-Konfiguration und kontrolliertes Warten auf beendete stdio-Prozesse
- keine Speicherung von Passwörtern oder Tokens in normalen Einstellungen; dauerhafte Agentenerinnerungen lehnen Inhalte ab, die wie Passwörter, Tokens, private Schlüssel oder API-Keys aussehen
- Slash-/Projekt-Commands sind read-only Text-Templates und erweitern keine Genehmigungen, Sandboxgrenzen oder Werkzeugrechte
- lokale Regel-/Skill-Dateien erweitern nur den Modellkontext und umgehen keine Genehmigungen, Sandboxgrenzen oder blockierten Befehle; Skills mit deklarierten Skripten oder nicht-read-only Berechtigungen werden nicht automatisch eingebettet
- read-only Subagent-Handoffs lesen nur Projektinfo, Projektbaum, erwähnte Textdateien und Suchtreffer; sie starten keine Shell-, Netzwerk-, MCP- oder Schreibaktionen
- validierte SVG-/Icon-Erzeugung blockiert Skripte, Event-Handler und `javascript:`-URLs, bevor Asset-Dateien geschrieben werden
- validierte Raster-/Icon-Erzeugung dekodiert Data-URL/Base64, begrenzt die Dateigröße, prüft Format-Signaturen und Dimensionen und schreibt nur unterstützte Bildendungen
- lokale Bildmodell-Erzeugung akzeptiert nur Loopback-Endpunkte für AUTOMATIC1111-/Forge-kompatible APIs, verlangt normale Dateigenehmigung und validiert das erzeugte Zielbild erneut
- lokale Raster-/Icon-Konvertierung verlangt Genehmigung, bleibt innerhalb der Projektpfadgrenze, nutzt nur lokale Quell- und Zieldateien und validiert das neu encodierte Bild erneut
- lokales SVG-/HTML-Rendering in Raster-/Icon-Ziele verlangt Genehmigung, bleibt innerhalb der Projektpfadgrenze, lehnt externe HTML-Netzwerkreferenzen ab und startet Edge/Chrome mit temporärem Profil und blockiertem Proxy

### Automatische Laufzeitinstallation

Die Kernlaufzeit wird standardmäßig selbstständig vervollständigt. Dabei gelten zusätzliche Integritätsprüfungen:

- Go wird vom Build-Skript aus der offiziellen `go.dev`-Release-Liste bezogen und gegen den dort veröffentlichten SHA-256-Wert geprüft.
- Der Ollama-Installer wird ausschließlich von `ollama.com` geladen und muss eine gültige Windows-Authenticode-Signatur besitzen.
- Das portable `uv` ist versions- und SHA-256-gebunden.
- Aider ist auf `aider-chat==0.86.2` festgelegt und wird nach der Installation durch `aider --version` verifiziert.
- Python, uv und Aider liegen in LocalCode-eigenen Benutzerverzeichnissen; globale Python-Pakete werden nicht verändert.
- Setup-Downloads besitzen einen eigenen Schalter und sind vom Agenten-/Web-Netzwerkzugriff getrennt. Dadurch kann Webrecherche deaktiviert bleiben, ohne die ausdrücklich aktivierte Ersteinrichtung zu blockieren.
- Das temporäre Startfenster ist ausschließlich an `127.0.0.1` gebunden, verwendet ein zufälliges Zugriffstoken, prüft den Host-Header und lädt keine externen Ressourcen.
- MCP-`uvx` wird aus demselben versions- und SHA-256-gebundenen uv-Archiv installiert; es wird kein heruntergeladenes Installationsskript direkt ausgeführt.

### Handy-Remote

Der Desktop-Server bleibt an Loopback gebunden. Die Handy-Fernsteuerung läuft auf einem separaten Remote-Server, der nur die Remote-Web-App und eine kleine Remote-API bereitstellt. Remote ist standardmäßig deaktiviert und standardmäßig ebenfalls nur an Loopback gebunden; eine LAN-Bindung muss bewusst aktiviert werden. Der Pairing-Code kann nur über die loopback-geschützte Desktop-API erzeugt werden, besteht aus acht kryptografisch gleichverteilten Dezimalziffern, ist drei Minuten gültig und wird nach fünf Fehlversuchen gesperrt. Erst nach erfolgreichem Pairing erhält das Handy ein zufälliges 256-Bit-Token; LocalCode speichert davon ausschließlich einen SHA-256-Hash. Ein gekoppeltes Gerätetoken läuft nach 30 Tagen ab und kann über die Desktop-API gezielt widerrufen werden.

Remote-Aufrufe für Status, Projekte, Aufgaben, Chat, Stop und Genehmigungen benötigen das Gerätetoken ausschließlich per `X-LocalCode-Remote-Token` oder Bearer-Header. Für EventSource fordert die authentifizierte Remote-App zuerst ein kurzlebiges, einmal verwendbares Stream-Ticket mit 45 Sekunden TTL an; das langlebige Gerätetoken wird nicht in die SSE-URL geschrieben. Die Remote-App erweitert keine Genehmigungen, Sandboxgrenzen, Werkzeugrechte oder Projektgrenzen. Sie ruft dieselben Backendfunktionen wie die Desktop-Oberfläche auf; Datei-Anhänge laufen durch dieselbe Größen- und Typvalidierung. Cross-Origin-POSTs werden über Origin- und Fetch-Site-Prüfung abgelehnt. Bei bewusst aktivierter LAN-Bindung verwendet der Remote-Server derzeit HTTP; bis eine TLS-Transportgrenze vorhanden ist, sollte LAN-Remote deshalb nur in einem vertrauenswürdigen lokalen Netz verwendet werden. Windows-Firewall- und Netzwerksegment-Regeln bleiben zusätzliche Betriebssystem-/Netzwerkaufgaben.

### Aider-Unterprozessgrenze

LocalCode übergibt Aider eine explizite Minimal-Konfiguration und eine absichtlich leere Umgebungsdatei. Analytics, Update-Prüfung, Browseraufforderungen, Shell-Vorschläge, URL-Erkennung, Benachrichtigungen, Dateiüberwachung und Prompt-Caching sind deaktiviert. Historien liegen außerhalb des Repositories. Vor Bearbeitungs-, Lint- und Testläufen werden Backups und Hash-Manifeste erzeugt; Undo überschreibt keine später manuell geänderten Dateien. Aiders `--yes-always` gilt nur innerhalb des bereits durch LocalCode genehmigten Bearbeitungslaufs.

Hintergrundbefehle starten unter Windows ohne sichtbare Konsolenfenster. Interaktive Logins werden bewusst in einem sichtbaren Terminal geöffnet. Diese Kontrollen sind Anwendungsschutz und keine identische Betriebssystem- oder Virtualisierungssandbox der proprietären Codex-Infrastruktur.

Hooks sind Automatisierung innerhalb derselben Befehlsgrenze. Sie erhalten Aktionsmetadaten über `LOCALCODE_HOOK_PHASE`, `LOCALCODE_ACTION`, `LOCALCODE_ACTION_MESSAGE`, `LOCALCODE_ACTION_PATH` und `LOCALCODE_ACTION_COMMAND`, aber keine zusätzliche Berechtigung. Before-Tool-Hooks können eine Werkzeugaktion durch Fehler abbrechen; After-Tool-Hooks werden als eigenes Ereignis protokolliert und verändern das ursprüngliche Werkzeugresultat nicht.

Projekt-Commands sind reine Text-Templates. LocalCode liest nur `.md`- und `.txt`-Dateien mit validierten Command-Namen aus bekannten Command-Verzeichnissen, begrenzt die Größe, prüft Textkodierung und verwirft Symlink-/Junction-Ausbrüche aus dem jeweiligen Command-Root. Ein Slash-Command kann den Modellauftrag präzisieren, aber keine Genehmigungsregel, Sandboxgrenze oder Werkzeugberechtigung ändern und führt selbst keinen Shell-Befehl aus.

Agentenerinnerungen sind lokale Konfigurationsdaten. Sie besitzen Projekt- oder Global-Scope, werden atomar mit der Konfiguration geschrieben und können über eine konkrete Memory-ID oder einen eindeutigen Inhaltstreffer gelöscht werden. Direkte natürliche Memory-Befehle werden vor dem Modelllauf verarbeitet; globale Erinnerungen werden später als Kontext angewendet, erweitern aber keine Genehmigungen, Sandboxgrenzen oder Werkzeugrechte. Sie sind nicht für Zugangsdaten, Geheimnisse oder vertrauliche Schlüssel bestimmt.

Globale und projektbezogene Anweisungen sowie lokale und globale Skills werden als unprivilegierter Text in den Agentenkontext eingebettet oder per `skill_read` gelesen. Skill-Frontmatter mit `permissions`, `allowed-tools`, `tools`, `scripts` oder `commands` ist nur Metadaten: nur bekannte read-only Aliase wie `Read`, `Glob`, `Grep`, `skill_read`, `command_read` und MCP-List/Read-Aktionen bleiben automatisch einbettbar; Schreib-, Editier-, Shell-, Netzwerk-, Build-, Installations-, Memory- oder MCP-Tool-Call-Rechte sowie deklarierte Skripte markieren den Skill als `approval-required`, verhindern automatische Einbettung und ändern keine Werkzeugfreigabe. Zusätzliche Skill-Ressourcen können nur relativ zum jeweiligen Skill-Verzeichnis gelesen werden; Pfad-Traversal und direkte Nutzung von `SKILL.md` über den Ressourcenpfad werden blockiert. Binäre oder anderweitig projektbenötigte Skill-Ressourcen können mit `skill_copy_resource` nur nach Genehmigung kopiert werden: Die Quelle muss eine reguläre Datei innerhalb des Skill-Verzeichnisses bleiben, Symlink-/Junction-Ausbrüche werden verworfen, die Größe ist begrenzt, und das Ziel läuft durch die normale Projektpfadgrenze mit Backup und Postcondition. `skill_run_script` akzeptiert nur einen exakten `scripts`-/`commands`-Eintrag aus dem Skill, begrenzt Argumente, prüft die Command-Blockliste und verlangt eine eigene Genehmigung; relative Skriptdateien müssen innerhalb des Skill-Verzeichnisses bleiben. Ein Skill oder eine Regel kann keine LocalCode-Policy direkt ändern; Datei-, Shell-, Netzwerk-, MCP- und externe Engine-Aktionen laufen weiterhin durch die normalen Validierungs- und Genehmigungspfade.

`subagent_analyze` startet optional einen isolierten modellgestützten read-only Child-Agenten. Sein JSON-Schema enthält ausschließlich `list_files`, `read_file`, `search_text`, freigabefreies `lsp` und `finish`; Datei-Mutation, Shell, Git, Netzwerk/Web, MCP, Installation, Memory, Genehmigungsanforderungen und rekursives Spawning können vom Child daher nicht angefordert werden. LSP wird zusätzlich abgewiesen, wenn die aktuelle Approval-Policy eine interaktive Bestätigung verlangen würde. Modell-, Tool-, Zeit- und geschätzte Tokenbudgets begrenzen jeden Child-Lauf. Modellfehler oder Budgetende führen nur zu deterministischer read-only Analyse und erweitern keine Berechtigung. Builder-/Worktree-Mutation ist in dieser Stufe ausdrücklich nicht implementiert.

`create_svg_asset` ist ein lokales Vektor-Asset-Werkzeug. Es schreibt nur `.svg`-Dateien nach XML-Prüfung. `create_image_asset` ist die separate lokale Binärgrenze für `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico` und `.bmp`; es validiert Signatur, Größe und Dimensionen. `generate_image_asset` ist die lokale KI-Bildgrenze für `.png`, `.jpg`, `.jpeg`, `.webp` und `.ico`: Der Provider muss auf `localhost`, `127.0.0.1` oder `::1` zeigen, der Prompt wird begrenzt, und die Antwort wird vor dem Schreiben dekodiert, konvertiert und validiert. `convert_image_asset` decodiert unterstützte lokale Quellbilder, encodiert nur `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico` und validiert die Zieldatei nach dem Schreiben; WebP nutzt dafür eine temporäre lokale HTML/PNG-Zwischenquelle im blockierten Headless-Browser. `render_asset` ist der lokale Browser-Renderer für `.svg`, `.html` und `.htm` nach `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico`. HTML mit externen HTTP(S)-Referenzen wird abgelehnt; der Headless-Browser läuft mit temporärem Profil und blockiertem Proxy. Das ist weiterhin eine Anwendungsschutzgrenze, keine vollständige Betriebssystem-Sandbox.

## English

LocalCode combines these application-level controls:

- approval modes for file changes, commands, tool installations, and network actions
- project, workspace, and unrestricted path modes plus explicitly allowed roots
- canonical path checks after resolving existing symlinks and NTFS junctions, including destinations that do not exist yet
- blocked command patterns and hard guards for destructive Git/system operations
- timeouts, process groups, and complete Windows process-tree termination
- task and tool hooks run as normal non-interactive project commands with the same timeouts, environment rules, blocklists, and process cancellation; before-tool failures stop the tool action
- a global network switch
- rejection of loopback, link-local, private, and other non-public destinations
- DNS-rebinding protection by dialing the exact IP validated during connection setup rather than performing a second lookup
- explicit MCP configuration and controlled reaping of stdio subprocesses
- no normal settings storage for passwords or tokens; durable agent memories reject content that looks like passwords, tokens, private keys, or API keys
- slash/project commands are read-only text templates and do not extend approvals, sandbox boundaries, or tool permissions
- local rule/skill files extend only model context and do not bypass approvals, sandbox boundaries, or blocked commands; skills with declared scripts or non-read-only permissions are not embedded automatically
- read-only subagent handoffs only read project info, the project tree, mentioned text files, and search hits; they start no shell, network, MCP, or write actions
- validated SVG/icon creation blocks scripts, event handlers, and `javascript:` URLs before asset files are written
- validated raster/icon creation decodes data URLs/Base64, limits file size, checks format signatures and dimensions, and writes only supported image extensions
- local image-model generation accepts only loopback endpoints for AUTOMATIC1111/Forge-compatible APIs, requires normal file approval, and validates the generated target image again
- local raster/icon conversion requires approval, stays inside the project path boundary, uses only local source and target files, and validates the newly encoded image again
- local SVG/HTML rendering into raster/icon targets requires approval, stays inside the project path boundary, rejects external HTML network references, and starts Edge/Chrome with a temporary profile and blocked proxy

### Automatic runtime installation

The core runtime is completed automatically by default with additional integrity checks:

- Go is downloaded by the build script from the official `go.dev` release feed and verified against its published SHA-256 value.
- The Ollama installer is downloaded only from `ollama.com` and must carry a valid Windows Authenticode signature.
- Portable `uv` is pinned by version and SHA-256.
- Aider is pinned to `aider-chat==0.86.2` and verified with `aider --version` after installation.
- Python, uv, and Aider are stored in LocalCode-owned user directories; global Python packages are not modified.
- Setup downloads have their own switch and are separate from agent/web network access. Web research can remain disabled without blocking explicitly enabled first-run setup.
- The temporary startup window binds only to `127.0.0.1`, uses a random access token, validates the Host header, and loads no external resources.
- MCP `uvx` is installed from the same version- and SHA-256-pinned uv archive; no downloaded installer script is executed directly.

### Phone Remote

The desktop server remains bound to loopback. Phone remote control runs on a separate Remote server that serves only the Remote web app and a small Remote API. Remote is disabled by default and also binds to loopback by default; LAN binding must be enabled deliberately. Pairing codes can be created only through the loopback-protected desktop API, contain eight cryptographically uniform decimal digits, expire after three minutes, and lock after five failed attempts. After successful pairing the phone receives a random 256-bit token; LocalCode stores only its SHA-256 hash. A paired-device token expires after 30 days and can be revoked explicitly through the desktop API.

Remote calls for status, projects, tasks, chat, stop, and approvals require the device token only through `X-LocalCode-Remote-Token` or a Bearer header. For EventSource, the authenticated Remote app first requests a short-lived, single-use stream ticket with a 45-second TTL; the long-lived device token is not placed in the SSE URL. The Remote app does not extend approvals, sandbox boundaries, tool rights, or project boundaries. It calls the same backend functions as the desktop UI; file attachments pass through the same size and type validation. Cross-origin POSTs are rejected through Origin and Fetch-Site checks. When LAN binding is deliberately enabled, the Remote server currently uses HTTP; until a TLS transport boundary is available, LAN Remote should therefore be used only on a trusted local network. Windows Firewall and network-segment controls remain additional operating-system and network responsibilities.

### Aider subprocess boundary

LocalCode passes an explicit minimal configuration and an intentionally empty environment file. Analytics, update checks, browser prompts, shell suggestions, URL detection, notifications, file watching, and prompt caching are disabled. Histories remain outside the repository. Backups and hash manifests are created before edit, lint, and test runs; undo never overwrites later manual changes. Aider's `--yes-always` applies only inside an editing run already approved by LocalCode.

Background commands start without visible console windows on Windows. Interactive logins deliberately open in a visible terminal. These controls are application-level protections, not an identical operating-system or virtualization sandbox to proprietary Codex infrastructure.

Hooks are automation inside the same command boundary. They receive action metadata through `LOCALCODE_HOOK_PHASE`, `LOCALCODE_ACTION`, `LOCALCODE_ACTION_MESSAGE`, `LOCALCODE_ACTION_PATH`, and `LOCALCODE_ACTION_COMMAND`, but no additional permission. Before-tool hooks can abort a tool action by failing; after-tool hooks are logged as separate events and do not change the original tool result.

Project commands are plain text templates. LocalCode reads only `.md` and `.txt` files with validated command names from known command directories, caps file size, checks text encoding, and rejects symlink/junction escapes from the respective command root. A slash command can refine the model task, but it cannot change approval rules, sandbox boundaries, or tool permissions and does not execute shell code by itself.

Agent memories are local configuration data. They have project or global scope, are written atomically with the configuration, and can be deleted through a concrete memory ID or a unique content match. Direct natural-language memory commands are handled before the model run; global memories are later applied as context, but they do not extend approvals, sandbox boundaries, or tool permissions. They are not intended for credentials, secrets, or confidential keys.

Global and project instructions plus local and global skills are embedded as unprivileged text in the agent context or read through `skill_read`. Skill frontmatter with `permissions`, `allowed-tools`, `tools`, `scripts`, or `commands` is metadata only: only known read-only aliases such as `Read`, `Glob`, `Grep`, `skill_read`, `command_read`, and MCP list/read actions remain eligible for automatic embedding; write, edit, shell, network, build, install, memory, or MCP tool-call permissions plus declared scripts mark the skill as `approval-required`, prevent automatic embedding, and do not change tool access. Additional skill resources can only be read relative to their skill directory; path traversal and direct `SKILL.md` access through the resource path are blocked. Binary or otherwise project-needed skill resources can be copied with `skill_copy_resource` only after approval: the source must remain a regular file inside the skill directory, symlink/junction escapes are rejected, size is capped, and the destination passes through the normal project path boundary with backup and postcondition. `skill_run_script` accepts only an exact `scripts`/`commands` entry from the skill, caps arguments, checks the command blocklist, and requires its own approval; relative script files must remain inside the skill directory. A skill or rule cannot directly change LocalCode policy; file, shell, network, MCP, and external engine actions still pass through the normal validation and approval paths.

`subagent_analyze` can start an isolated model-backed read-only child agent. Its JSON schema contains only `list_files`, `read_file`, `search_text`, approval-free `lsp`, and `finish`; file mutation, shell, Git, network/web, MCP, installation, memory, approval requests, and recursive spawning therefore cannot be requested by the child. LSP is additionally rejected whenever the current approval policy would require interactive confirmation. Hard model-call, tool-call, time, and estimated-token budgets bound each child run. Model failures or budget exhaustion fall back only to deterministic read-only analysis and grant no additional capability. Builder/worktree mutation is explicitly not implemented at this stage.

`create_svg_asset` is a local vector-asset tool. It writes only `.svg` files after XML validation. `create_image_asset` is the separate local binary boundary for `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico`, and `.bmp`; it validates signatures, size, and dimensions. `generate_image_asset` is the local AI-image boundary for `.png`, `.jpg`, `.jpeg`, `.webp`, and `.ico`: the provider must point to `localhost`, `127.0.0.1`, or `::1`, the prompt is capped, and the response is decoded, converted, and validated before writing. `convert_image_asset` decodes supported local source images, encodes only `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`, and validates the target file after writing; WebP uses a temporary local HTML/PNG intermediate in the blocked headless browser. `render_asset` is the local browser renderer for `.svg`, `.html`, and `.htm` into `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`. HTML with external HTTP(S) references is rejected; the headless browser runs with a temporary profile and blocked proxy. This remains an application-level control, not a full operating-system sandbox.
## Native Agent Task DAG security boundary / Sicherheitsgrenze

Task-DAG validation does not widen the child-agent trust boundary. Planner proposals may name dynamic roles and request capabilities, but these values are inert until a later governance layer explicitly maps them to an allowed executable role and grants capabilities. Graph construction copies requests into `RequestedCapabilities` and leaves granted `Capabilities` empty. Invalid IDs, missing/duplicate/self dependencies, cycles, inconsistent mission/parent metadata, and invalid task states fail closed.

Deutsch: Der DAG vergibt keine Rechte. Dynamische Rollen und angeforderte Capabilities aus einem Plan werden nicht automatisch ausführbar. Bestehende Sandbox-, Approval-, SHA-/Precondition-, Atomic-Write-, Mobile- und Prozessgrenzen bleiben unverändert; insbesondere führt dieser Phase-4-Slice keine mutierenden Child-Agents, Worktrees oder parallelen Schreibzugriffe ein.
