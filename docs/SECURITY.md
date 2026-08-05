# Security model / Sicherheitsmodell

## Deutsch

LocalCode kombiniert:

- Genehmigungsmodi für Änderungen, Befehle und Netzwerkaktionen
- Projekt-, Workspace- und unbeschränkte Pfadmodi
- zusätzliche freigegebene Verzeichniswurzeln
- blockierte Befehlsmuster und harte Schutzregeln für destruktive Git-Aktionen
- Zeitlimits und kontrollierten Prozessbaum-Abbruch
- einen Netzwerk-Hauptschalter
- Schutz vor Abrufen lokaler und privater Netzwerkadressen
- explizite MCP-Konfiguration
- keine Speicherung von Passwörtern oder Tokens in normalen Einstellungen

Hintergrundbefehle werden unter Windows ohne sichtbare Konsolenfenster gestartet. Interaktive Logins werden bewusst in einem sichtbaren Terminal geöffnet. Die Schutzschicht ist anwendungsbasiert und keine identische Betriebssystem-Sandbox der proprietären Codex-Infrastruktur.

## English

LocalCode combines:

- approval modes for changes, commands, and network actions
- project, workspace, and unrestricted path modes
- additional explicitly allowed roots
- blocked command patterns and hard guards for destructive Git operations
- time limits and controlled process-tree termination
- a global network switch
- protection against fetching loopback and private-network addresses
- explicit MCP configuration
- no normal settings storage for passwords or tokens

Background commands start without visible console windows on Windows. Interactive logins deliberately open in a visible terminal. These controls are application-level protections, not an identical operating-system sandbox to proprietary Codex infrastructure.

## Aider subprocess boundary / Aider-Unterprozessgrenze

Aider is an external Python application installed only after explicit approval. LocalCode passes an explicit minimal config and an empty environment file, disables Aider analytics, update checks, browser prompts, shell suggestions, URL detection, notifications, file watching, and prompt caching, and uses explicit history paths outside the repository. The selected task and files remain subject to LocalCode's project boundary and approval policy. Aider's `--yes-always` applies only inside the already approved subprocess.

Aider ist eine externe Python-Anwendung und wird erst nach ausdrücklicher Zustimmung installiert. LocalCode übergibt eine explizite Minimal-Konfiguration und eine leere Umgebungsdatei, deaktiviert Aider-Analytics, Update-Prüfung, Browseraufforderungen, Shell-Vorschläge, URL-Erkennung, Benachrichtigungen, Dateiüberwachung und Prompt-Caching und verwendet explizite Historienpfade außerhalb des Projekts. Aufgabe und Dateien bleiben der LocalCode-Projektgrenze und Genehmigungspolitik unterworfen. Aiders `--yes-always` gilt nur innerhalb des bereits genehmigten Unterprozesses.

