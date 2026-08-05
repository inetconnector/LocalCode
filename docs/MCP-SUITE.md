# Managed MCP suite / Verwaltete MCP-Suite

## Deutsch

LocalCode verwaltet sechs MCP-Funktionsbereiche. Die drei für lokale Entwicklungsarbeit zentralen Server (Filesystem, PowerShell und Git) sind als integrierte MCP-Provider implementiert. Dadurch benötigen sie keine npm- oder Python-Pakete und verwenden unmittelbar LocalCodes Sandbox-, Genehmigungs-, Backup-, Werkzeugerkennungs- und Timeout-Mechanismen.

Fetch und Playwright laufen als persistente stdio-Server. GitHub verwendet den offiziellen gehosteten Streamable-HTTP-Endpunkt. LocalCode hält Sitzungen über mehrere Werkzeugaufrufe offen, verarbeitet Serveranfragen wie `roots/list` und beendet Prozessbäume kontrolliert beim Reset, Konfigurationswechsel oder Programmende.

### Standardserver

| Name | Transport | Standard | Voraussetzung |
|---|---|---:|---|
| Filesystem | integriert | aktiv | keine |
| PowerShell | integriert | aktiv | Windows PowerShell oder PowerShell 7; genehmigte PowerShell-7-Installation verfügbar |
| Git | integriert | aktiv | Git; LocalCode kann MinGit installieren |
| Fetch | stdio | aktiv | uv/uvx; verwaltete Installation verfügbar |
| GitHub | Streamable HTTP | inaktiv bis Anmeldung | PAT oder `gh auth login`; Standardendpunkt: `https://api.githubcopilot.com/mcp/x/all` |
| Playwright | stdio | aktiv | Node.js/npx und Edge; verwaltete Node-LTS-Installation verfügbar |

### GitHub-Anmeldung

LocalCode verwendet in dieser Reihenfolge:

1. die Umgebungsvariable `GITHUB_PAT_TOKEN`,
2. den flüchtig aus `gh auth token` gelesenen Token einer vorhandenen GitHub-CLI-Sitzung.

Der Token wird nicht in `config.json` geschrieben. Über **Anmelden** öffnet LocalCode ein sichtbares Terminal mit `gh auth login`.

### Eigene Server

Die erweiterte JSON-Konfiguration bleibt verfügbar. Unterstützte Transporte:

- `builtin`
- `stdio`
- `streamable-http`

Unterstützte Platzhalter:

- `${PROJECT_ROOT}`
- `${APP_DATA}`
- `${USER_HOME}`
- beliebige Umgebungsvariablen

## English

LocalCode manages six MCP capability areas. The three core local development servers (Filesystem, PowerShell, and Git) are implemented as built-in MCP providers. They therefore require no npm or Python packages and directly reuse LocalCode's sandbox, approval, backup, tool-discovery, and timeout mechanisms.

Fetch and Playwright run as persistent stdio servers. GitHub uses the official hosted Streamable HTTP endpoint. LocalCode keeps sessions alive across tool calls, handles server requests such as `roots/list`, and terminates process trees in a controlled manner when a session is reset, configuration changes, or the application exits.

### Default servers

| Name | Transport | Default | Requirement |
|---|---|---:|---|
| Filesystem | built-in | enabled | none |
| PowerShell | built-in | enabled | Windows PowerShell or PowerShell 7; approved PowerShell 7 installation available |
| Git | built-in | enabled | Git; LocalCode can install MinGit |
| Fetch | stdio | enabled | uv/uvx; managed installation available |
| GitHub | Streamable HTTP | disabled until sign-in | PAT or `gh auth login`; default endpoint: `https://api.githubcopilot.com/mcp/x/all` |
| Playwright | stdio | enabled | Node.js/npx and Edge; managed Node LTS installation available |

### GitHub authentication

LocalCode uses, in order:

1. the `GITHUB_PAT_TOKEN` environment variable,
2. a token read transiently from an existing GitHub CLI session through `gh auth token`.

The token is not written to `config.json`. The **Sign in** action opens a visible terminal running `gh auth login`.

### Custom servers

The advanced JSON configuration remains available. Supported transports:

- `builtin`
- `stdio`
- `streamable-http`

Supported placeholders:

- `${PROJECT_ROOT}`
- `${APP_DATA}`
- `${USER_HOME}`
- arbitrary environment variables
