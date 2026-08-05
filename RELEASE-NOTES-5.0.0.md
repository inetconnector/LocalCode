# LocalCode 5.0.0 – managed MCP tool suite

## Deutsch

LocalCode 5.0.0 ergänzt eine verwaltete MCP-Laufzeit, damit Dateioperationen, PowerShell, Git, Webabruf, GitHub und Browserautomation nicht nur konfiguriert, sondern dauerhaft verbunden, geprüft und kontrolliert ausgeführt werden können.

### Enthaltene MCP-Server

- **Filesystem** – direkt in LocalCode implementiert, auf das aktive Projekt beschränkt, mit Lesen, Schreiben, Suchen, Kopieren, Verschieben, Metadaten und kontrolliertem Löschen.
- **PowerShell** – direkt in LocalCode implementiert, ohne sichtbare Konsolenfenster, mit Skriptausführung, `Get-Command` und `Get-Help`.
- **Git** – direkt in LocalCode implementiert, mit Status, Diff, Historie, Initialisierung, Staging, Commit, Branch, Checkout, Pull, Push und Show. Destruktive Git-Operationen bleiben blockiert.
- **Fetch** – offizieller MCP-Referenzserver über `uvx mcp-server-fetch`.
- **GitHub** – offizieller gehosteter GitHub MCP Server über Streamable HTTP. Die Authentifizierung verwendet `GITHUB_PAT_TOKEN` oder den Token einer bestehenden GitHub-CLI-Anmeldung.
- **Playwright Browser** – offizieller Microsoft Playwright MCP Server. Der stdio-Prozess und das Browserprofil bleiben über mehrere Werkzeugaufrufe erhalten.

### Laufzeit und Zuverlässigkeit

- Persistente stdio-Sitzungen statt eines neuen MCP-Prozesses pro Werkzeugaufruf.
- Persistente Streamable-HTTP-Sitzungen mit `Mcp-Session-Id`.
- MCP-Protokollverhandlung mit aktuellen und kompatiblen älteren Versionen.
- Unterstützung für Serveranfragen wie `roots/list` und `ping`.
- Vollständige Unterstützung für `tools/list`, `tools/call`, Ressourcen und Prompts.
- Kontrollierter Timeout, Sitzungsreset und Prozessbaum-Abbruch.
- Projektplatzhalter `${PROJECT_ROOT}`, `${APP_DATA}` und `${USER_HOME}`.
- GitHub-Token werden nicht in der LocalCode-Konfiguration gespeichert.

### Automatische Einrichtung

Unter **Einstellungen → Plugins** besitzt jeder verwaltete Server eine echte Statuskarte mit:

- Aktivieren/Deaktivieren
- Installieren
- Anmelden
- Verbindung testen
- Sitzung zurücksetzen
- Anzeige der tatsächlich erkannten Werkzeuge und Fehler

Fehlende Laufzeiten können nach ausdrücklicher Genehmigung benutzerlokal eingerichtet werden:

- `uv`/`uvx` für Fetch
- offizielles portables Node.js LTS für Playwright
- offizielle portable GitHub CLI für GitHub

### Sicherheit

- Dateiwerkzeuge bleiben in der Projektwurzel beziehungsweise den ausdrücklich erlaubten Wurzeln.
- Schreibende MCP-Werkzeuge durchlaufen weiterhin LocalCodes Genehmigungsregeln.
- PowerShell wird ohne sichtbares Konsolenfenster und mit Timeout ausgeführt.
- Force-Push, `git reset --hard`, `git clean -fdx` und vergleichbar destruktive Git-Aktionen bleiben blockiert.
- Browser- und Netzwerkwerkzeuge sind weiterhin von Netzwerk- und Genehmigungseinstellungen abhängig.

## English

LocalCode 5.0.0 adds a managed MCP runtime so file operations, PowerShell, Git, web fetching, GitHub, and browser automation are not merely configurable: they stay connected, are verified, and run under explicit control.

### Included MCP servers

- **Filesystem** – implemented directly in LocalCode, restricted to the active project, with read, write, search, copy, move, metadata, and controlled deletion tools.
- **PowerShell** – implemented directly in LocalCode, without visible console windows, with script execution, `Get-Command`, and `Get-Help`.
- **Git** – implemented directly in LocalCode, with status, diff, history, initialization, staging, commit, branch, checkout, pull, push, and show. Destructive Git operations remain blocked.
- **Fetch** – official MCP reference server through `uvx mcp-server-fetch`.
- **GitHub** – official hosted GitHub MCP server with the complete toolset over Streamable HTTP. Authentication uses `GITHUB_PAT_TOKEN` or the token from an existing GitHub CLI sign-in.
- **Playwright Browser** – official Microsoft Playwright MCP server. The stdio process and browser profile persist across tool calls.

### Runtime and reliability

- Persistent stdio sessions instead of spawning a new MCP process per call.
- Persistent Streamable HTTP sessions using `Mcp-Session-Id`.
- MCP protocol negotiation with the current and compatible earlier versions.
- Support for server-to-client requests such as `roots/list` and `ping`.
- Full `tools/list`, `tools/call`, resources, and prompts support.
- Controlled timeout, session reset, and process-tree termination.
- `${PROJECT_ROOT}`, `${APP_DATA}`, and `${USER_HOME}` placeholders.
- GitHub tokens are never persisted in the LocalCode configuration.

### Managed setup

Under **Settings → Plugins**, every managed server has a real status card with:

- Enable/disable
- Install
- Sign in
- Connection test
- Session reset
- Display of discovered tools and concrete errors

After explicit approval, missing runtimes can be installed for the current user:

- `uv`/`uvx` for Fetch
- official portable Node.js LTS for Playwright
- official portable GitHub CLI for GitHub

### Security

- Filesystem tools remain inside the project root or explicitly allowed roots.
- Mutating MCP tools continue to use LocalCode approval rules.
- PowerShell runs without visible console windows and with a timeout.
- Force push, `git reset --hard`, `git clean -fdx`, and similarly destructive Git actions remain blocked.
- Browser and network tools remain governed by network and approval settings.
