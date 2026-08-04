# MCP Configuration Examples

## stdio

```json
{
  "name": "example-stdio",
  "enabled": true,
  "transport": "stdio",
  "command": "python.exe",
  "args": ["C:\\tools\\my_mcp_server.py"],
  "env": {"API_TOKEN": "${MY_API_TOKEN}"},
  "headers": {},
  "timeout_sec": 60
}
```

## Streamable HTTP

```json
{
  "name": "example-http",
  "enabled": true,
  "transport": "http",
  "url": "https://mcp.example.com/mcp",
  "headers": {"Authorization": "Bearer ${MCP_TOKEN}"},
  "timeout_sec": 60
}
```

The Test button calls `tools/list`. The agent can also list/read resources, list/get prompts and call tools.
