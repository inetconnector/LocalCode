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
