# MCP configuration examples / MCP-Konfigurationsbeispiele

LocalCode supports local `stdio` servers and remote Streamable HTTP servers. Secrets should be referenced through environment variables rather than stored directly in configuration files.

LocalCode unterstützt lokale `stdio`-Server und entfernte Streamable-HTTP-Server. Geheimnisse sollen über Umgebungsvariablen referenziert und nicht direkt in Konfigurationsdateien gespeichert werden.

## stdio

```json
{
  "name": "filesystem",
  "enabled": true,
  "transport": "stdio",
  "command": "npx.cmd",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "C:\\Users\\frede\\Projekte"],
  "env": {},
  "headers": {},
  "timeout_sec": 60
}
```

## Streamable HTTP

```json
{
  "name": "remote-tools",
  "enabled": false,
  "transport": "http",
  "url": "https://example.com/mcp",
  "headers": {"Authorization": "Bearer ${MCP_TOKEN}"},
  "timeout_sec": 60
}
```
