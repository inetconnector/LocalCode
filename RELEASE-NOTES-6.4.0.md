# LocalCode 6.4.0 release notes

## Startup setup and progress window

- Added a compact startup splash window before the main LocalCode UI.
- The splash reports Ollama discovery, installation, service start, installed-model checks, per-model download status and size progress, selected engine setup, and final readiness.
- Failure actions include retry, opening the log folder, starting in limited mode, and exiting.
- On Windows, Edge or Chrome app mode is opened at a compact splash size and expands before the main UI is shown.

## Fixed first-run network deadlock

- Automatic dependency downloads are now controlled by `setup_downloads_enabled`, independently from agent/web `network_enabled`.
- Configuration schema 10 migrates existing schema-9 installations with setup downloads enabled, while preserving the user's agent/web network preference.
- Disabling web search therefore no longer prevents LocalCode from installing Ollama, configured models, Aider, Claude Code, OpenCode, or explicitly enabled managed tools.
- Startup setup failures are localized and remain actionable in the splash instead of terminating behind a generic message box.

## Additional correctness and security fixes

- Ollama URLs preserve HTTPS, reject unsupported schemes, and correctly normalize IPv6 hosts.
- Only models required by the selected external engine are downloaded; inactive Aider/OpenCode models no longer trigger unnecessary multi-gigabyte pulls.
- Selecting an engine that is disabled now falls back explicitly to LocalCode native instead of leaving an unusable engine selected.
- The startup server validates loopback Host headers, uses bounded HTTP timeouts, and avoids double-escaping displayed paths and versions.
- MCP uvx setup now uses the same pinned, SHA-256-verified uv Windows archive instead of executing a downloaded PowerShell installer script.
- Project STATE output distinguishes agent network access from automatic setup downloads.

## Verification

The final package is rebuilt from this source revision and includes source tests, race-detector partitions, randomized test orders, vet, UI/API simulation, translation/DOM checks, statement coverage above 80%, and Windows-amd64 cross-build verification. Native execution of the Windows batch files and PE binaries remains a target-machine verification boundary; `REBUILD-NATIVE.txt` forces a complete first build on Windows.
