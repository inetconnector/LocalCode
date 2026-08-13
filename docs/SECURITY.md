# Security model / Sicherheitsmodell

## Deutsch

LocalCode kombiniert folgende Anwendungsschutzschichten:

- Genehmigungsmodi für Dateiänderungen, Befehle, Werkzeuginstallationen und Netzwerkaktionen
- Projekt-, Workspace- und unbeschränkte Pfadmodi sowie zusätzliche freigegebene Wurzeln
- kanonische Pfadprüfung nach Auflösung vorhandener Symlinks und NTFS-Junctions, auch bei neu anzulegenden Zieldateien
- blockierte Befehlsmuster und harte Schutzregeln für destruktive Git-/Systemaktionen
- Zeitlimits, Prozessgruppen und vollständigen Prozessbaum-Abbruch unter Windows
- einen globalen Netzwerkschalter
- Schutz vor Loopback-, Link-local-, privaten und sonstigen nichtöffentlichen Zieladressen
- DNS-Rebinding-Schutz: verbunden wird mit genau der zuvor beim Dial validierten IP, nicht mit einer zweiten Namensauflösung
- explizite MCP-Konfiguration und kontrolliertes Warten auf beendete stdio-Prozesse
- keine Speicherung von Passwörtern oder Tokens in normalen Einstellungen; dauerhafte Agentenerinnerungen lehnen Inhalte ab, die wie Passwörter, Tokens, private Schlüssel oder API-Keys aussehen
- lokale Regel-/Skill-Dateien erweitern nur den Modellkontext und umgehen keine Genehmigungen, Sandboxgrenzen oder blockierten Befehle; Skills mit deklarierten Skripten oder nicht-read-only Berechtigungen werden nicht automatisch eingebettet
- validierte SVG-/Icon-Erzeugung blockiert Skripte, Event-Handler und `javascript:`-URLs, bevor Asset-Dateien geschrieben werden
- validierte Raster-/Icon-Erzeugung dekodiert Data-URL/Base64, begrenzt die Dateigröße, prüft Format-Signaturen und Dimensionen und schreibt nur unterstützte Bildendungen
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

### Aider-Unterprozessgrenze

LocalCode übergibt Aider eine explizite Minimal-Konfiguration und eine absichtlich leere Umgebungsdatei. Analytics, Update-Prüfung, Browseraufforderungen, Shell-Vorschläge, URL-Erkennung, Benachrichtigungen, Dateiüberwachung und Prompt-Caching sind deaktiviert. Historien liegen außerhalb des Repositories. Vor Bearbeitungs-, Lint- und Testläufen werden Backups und Hash-Manifeste erzeugt; Undo überschreibt keine später manuell geänderten Dateien. Aiders `--yes-always` gilt nur innerhalb des bereits durch LocalCode genehmigten Bearbeitungslaufs.

Hintergrundbefehle starten unter Windows ohne sichtbare Konsolenfenster. Interaktive Logins werden bewusst in einem sichtbaren Terminal geöffnet. Diese Kontrollen sind Anwendungsschutz und keine identische Betriebssystem- oder Virtualisierungssandbox der proprietären Codex-Infrastruktur.

Agentenerinnerungen sind lokale Konfigurationsdaten. Sie besitzen Projekt- oder Global-Scope, werden atomar mit der Konfiguration geschrieben und können nur über eine konkrete Memory-ID gelöscht werden. Sie sind nicht für Zugangsdaten, Geheimnisse oder vertrauliche Schlüssel bestimmt.

Globale und projektbezogene Anweisungen sowie lokale und globale Skills werden als unprivilegierter Text in den Agentenkontext eingebettet oder per `skill_read` gelesen. Skill-Frontmatter mit `permissions`, `allowed-tools`, `tools`, `scripts` oder `commands` ist nur Metadaten: nicht-read-only Berechtigungen und deklarierte Skripte markieren den Skill als `approval-required`, verhindern automatische Einbettung und ändern keine Werkzeugfreigabe. Zusätzliche Skill-Ressourcen können nur relativ zum jeweiligen Skill-Verzeichnis gelesen werden; Pfad-Traversal und direkte Nutzung von `SKILL.md` über den Ressourcenpfad werden blockiert. Binäre oder anderweitig projektbenötigte Skill-Ressourcen können mit `skill_copy_resource` nur nach Genehmigung kopiert werden: Die Quelle muss eine reguläre Datei innerhalb des Skill-Verzeichnisses bleiben, Symlink-/Junction-Ausbrüche werden verworfen, die Größe ist begrenzt, und das Ziel läuft durch die normale Projektpfadgrenze mit Backup und Postcondition. `skill_run_script` akzeptiert nur einen exakten `scripts`-/`commands`-Eintrag aus dem Skill, begrenzt Argumente, prüft die Command-Blockliste und verlangt eine eigene Genehmigung; relative Skriptdateien müssen innerhalb des Skill-Verzeichnisses bleiben. Ein Skill oder eine Regel kann keine LocalCode-Policy direkt ändern; Datei-, Shell-, Netzwerk-, MCP- und externe Engine-Aktionen laufen weiterhin durch die normalen Validierungs- und Genehmigungspfade.

`create_svg_asset` ist ein lokales Vektor-Asset-Werkzeug. Es schreibt nur `.svg`-Dateien nach XML-Prüfung. `create_image_asset` ist die separate lokale Binärgrenze für `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico` und `.bmp`; es validiert Signatur, Größe und Dimensionen, erzeugt aber selbst keine KI-Bilder. `convert_image_asset` decodiert unterstützte lokale Quellbilder, encodiert nur `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico` und validiert die Zieldatei nach dem Schreiben; WebP nutzt dafür eine temporäre lokale HTML/PNG-Zwischenquelle im blockierten Headless-Browser. `render_asset` ist der lokale Browser-Renderer für `.svg`, `.html` und `.htm` nach `.png`, `.jpg`, `.jpeg`, `.webp` oder `.ico`. HTML mit externen HTTP(S)-Referenzen wird abgelehnt; der Headless-Browser läuft mit temporärem Profil und blockiertem Proxy. Das ist weiterhin eine Anwendungsschutzgrenze, keine vollständige Betriebssystem-Sandbox.

## English

LocalCode combines these application-level controls:

- approval modes for file changes, commands, tool installations, and network actions
- project, workspace, and unrestricted path modes plus explicitly allowed roots
- canonical path checks after resolving existing symlinks and NTFS junctions, including destinations that do not exist yet
- blocked command patterns and hard guards for destructive Git/system operations
- timeouts, process groups, and complete Windows process-tree termination
- a global network switch
- rejection of loopback, link-local, private, and other non-public destinations
- DNS-rebinding protection by dialing the exact IP validated during connection setup rather than performing a second lookup
- explicit MCP configuration and controlled reaping of stdio subprocesses
- no normal settings storage for passwords or tokens; durable agent memories reject content that looks like passwords, tokens, private keys, or API keys
- local rule/skill files extend only model context and do not bypass approvals, sandbox boundaries, or blocked commands; skills with declared scripts or non-read-only permissions are not embedded automatically
- validated SVG/icon creation blocks scripts, event handlers, and `javascript:` URLs before asset files are written
- validated raster/icon creation decodes data URLs/Base64, limits file size, checks format signatures and dimensions, and writes only supported image extensions
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

### Aider subprocess boundary

LocalCode passes an explicit minimal configuration and an intentionally empty environment file. Analytics, update checks, browser prompts, shell suggestions, URL detection, notifications, file watching, and prompt caching are disabled. Histories remain outside the repository. Backups and hash manifests are created before edit, lint, and test runs; undo never overwrites later manual changes. Aider's `--yes-always` applies only inside an editing run already approved by LocalCode.

Background commands start without visible console windows on Windows. Interactive logins deliberately open in a visible terminal. These controls are application-level protections, not an identical operating-system or virtualization sandbox to proprietary Codex infrastructure.

Agent memories are local configuration data. They have project or global scope, are written atomically with the configuration, and can only be deleted through a concrete memory ID. They are not intended for credentials, secrets, or confidential keys.

Global and project instructions plus local and global skills are embedded as unprivileged text in the agent context or read through `skill_read`. Skill frontmatter with `permissions`, `allowed-tools`, `tools`, `scripts`, or `commands` is metadata only: non-read-only permissions and declared scripts mark the skill as `approval-required`, prevent automatic embedding, and do not change tool access. Additional skill resources can only be read relative to their skill directory; path traversal and direct `SKILL.md` access through the resource path are blocked. Binary or otherwise project-needed skill resources can be copied with `skill_copy_resource` only after approval: the source must remain a regular file inside the skill directory, symlink/junction escapes are rejected, size is capped, and the destination passes through the normal project path boundary with backup and postcondition. `skill_run_script` accepts only an exact `scripts`/`commands` entry from the skill, caps arguments, checks the command blocklist, and requires its own approval; relative script files must remain inside the skill directory. A skill or rule cannot directly change LocalCode policy; file, shell, network, MCP, and external engine actions still pass through the normal validation and approval paths.

`create_svg_asset` is a local vector-asset tool. It writes only `.svg` files after XML validation. `create_image_asset` is the separate local binary boundary for `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.ico`, and `.bmp`; it validates signatures, size, and dimensions, but does not generate AI images by itself. `convert_image_asset` decodes supported local source images, encodes only `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`, and validates the target file after writing; WebP uses a temporary local HTML/PNG intermediate in the blocked headless browser. `render_asset` is the local browser renderer for `.svg`, `.html`, and `.htm` into `.png`, `.jpg`, `.jpeg`, `.webp`, or `.ico`. HTML with external HTTP(S) references is rejected; the headless browser runs with a temporary profile and blocked proxy. This remains an application-level control, not a full operating-system sandbox.
