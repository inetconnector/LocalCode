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
- validierte SVG-/Icon-Erzeugung blockiert Skripte, Event-Handler und `javascript:`-URLs, bevor Asset-Dateien geschrieben werden

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

`create_svg_asset` ist ein lokales Vektor-Asset-Werkzeug, kein privilegierter Browser-Renderer. Es schreibt nur `.svg`-Dateien nach XML-Prüfung und ersetzt keine allgemeine Rasterbild- oder Medien-Sandbox.

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
- validated SVG/icon creation blocks scripts, event handlers, and `javascript:` URLs before asset files are written

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

`create_svg_asset` is a local vector-asset tool, not a privileged browser renderer. It writes only `.svg` files after XML validation and does not replace a general raster-image or media sandbox.
