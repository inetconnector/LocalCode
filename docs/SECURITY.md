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
- keine Speicherung von Passwörtern oder Tokens in normalen Einstellungen

### Automatische Laufzeitinstallation

Die Kernlaufzeit wird standardmäßig selbstständig vervollständigt. Dabei gelten zusätzliche Integritätsprüfungen:

- Go wird vom Build-Skript aus der offiziellen `go.dev`-Release-Liste bezogen und gegen den dort veröffentlichten SHA-256-Wert geprüft.
- Der Ollama-Installer wird ausschließlich von `ollama.com` geladen und muss eine gültige Windows-Authenticode-Signatur besitzen.
- Das portable `uv` ist versions- und SHA-256-gebunden.
- Aider ist auf `aider-chat==0.86.2` festgelegt und wird nach der Installation durch `aider --version` verifiziert.
- Python, uv und Aider liegen in LocalCode-eigenen Benutzerverzeichnissen; globale Python-Pakete werden nicht verändert.
- Der globale Netzwerkschalter und die jeweiligen Auto-Installationsoptionen können die Einrichtung ausdrücklich unterbinden.

### Aider-Unterprozessgrenze

LocalCode übergibt Aider eine explizite Minimal-Konfiguration und eine absichtlich leere Umgebungsdatei. Analytics, Update-Prüfung, Browseraufforderungen, Shell-Vorschläge, URL-Erkennung, Benachrichtigungen, Dateiüberwachung und Prompt-Caching sind deaktiviert. Historien liegen außerhalb des Repositories. Vor Bearbeitungs-, Lint- und Testläufen werden Backups und Hash-Manifeste erzeugt; Undo überschreibt keine später manuell geänderten Dateien. Aiders `--yes-always` gilt nur innerhalb des bereits durch LocalCode genehmigten Bearbeitungslaufs.

Hintergrundbefehle starten unter Windows ohne sichtbare Konsolenfenster. Interaktive Logins werden bewusst in einem sichtbaren Terminal geöffnet. Diese Kontrollen sind Anwendungsschutz und keine identische Betriebssystem- oder Virtualisierungssandbox der proprietären Codex-Infrastruktur.

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
- no normal settings storage for passwords or tokens

### Automatic runtime installation

The core runtime is completed automatically by default with additional integrity checks:

- Go is downloaded by the build script from the official `go.dev` release feed and verified against its published SHA-256 value.
- The Ollama installer is downloaded only from `ollama.com` and must carry a valid Windows Authenticode signature.
- Portable `uv` is pinned by version and SHA-256.
- Aider is pinned to `aider-chat==0.86.2` and verified with `aider --version` after installation.
- Python, uv, and Aider are stored in LocalCode-owned user directories; global Python packages are not modified.
- The global network switch and individual auto-install options can explicitly block setup.

### Aider subprocess boundary

LocalCode passes an explicit minimal configuration and an intentionally empty environment file. Analytics, update checks, browser prompts, shell suggestions, URL detection, notifications, file watching, and prompt caching are disabled. Histories remain outside the repository. Backups and hash manifests are created before edit, lint, and test runs; undo never overwrites later manual changes. Aider's `--yes-always` applies only inside an editing run already approved by LocalCode.

Background commands start without visible console windows on Windows. Interactive logins deliberately open in a visible terminal. These controls are application-level protections, not an identical operating-system or virtualization sandbox to proprietary Codex infrastructure.
