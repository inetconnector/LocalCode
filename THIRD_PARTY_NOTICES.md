# Third-party notices / Hinweise zu Drittkomponenten

LocalCode itself is licensed under the Apache License, Version 2.0. See [`LICENSE`](LICENSE).
LocalCode selbst steht unter der Apache License, Version 2.0. Siehe [`LICENSE`](LICENSE).

## Go toolchain and standard library / Go-Werkzeugkette und Standardbibliothek

LocalCode is built with the Go toolchain and standard library. Go is distributed under a BSD-style license. The Go toolchain is not bundled in the release executable; the build scripts can download an official portable Go distribution when needed.

LocalCode wird mit der Go-Werkzeugkette und Standardbibliothek gebaut. Go wird unter einer BSD-artigen Lizenz verteilt. Die Go-Werkzeugkette ist nicht in der Release-EXE enthalten; die Build-Skripte können bei Bedarf eine offizielle portable Go-Distribution laden.


## Aider editing engine / Aider-Bearbeitungs-Engine

LocalCode can install or repair `aider-chat==0.86.2` automatically at startup as an external, separately executed editing engine when runtime auto-setup is enabled. Aider is licensed under Apache-2.0. It is not redistributed in this source package or executable. See `NOTICE-AIDER.md` and `licenses/aider-LICENSE.txt`.

LocalCode kann bei aktivierter automatischer Laufzeiteinrichtung `aider-chat==0.86.2` beim Start automatisch als externe, separat ausgeführte Bearbeitungs-Engine installieren oder reparieren. Aider steht unter Apache-2.0 und wird weder in diesem Quellpaket noch in der EXE weiterverteilt. Siehe `NOTICE-AIDER.md` und `licenses/aider-LICENSE.txt`.

## Managed MCP servers / Verwaltete MCP-Server

The source package contains LocalCode client integrations and built-in providers, but it does not redistribute the following upstream server packages or their runtimes. When the user explicitly approves installation or sign-in, LocalCode downloads or starts them from their official distribution channels. Their own licenses and terms apply.

Das Quellpaket enthält LocalCode-Clientintegrationen und integrierte Provider, verteilt die folgenden externen Serverpakete oder Laufzeiten jedoch nicht mit. Nach ausdrücklicher Zustimmung des Benutzers lädt oder startet LocalCode sie aus ihren offiziellen Distributionskanälen. Es gelten deren jeweilige Lizenzen und Bedingungen.

- Model Context Protocol reference Fetch server (`mcp-server-fetch`)
- Microsoft Playwright MCP server (`@playwright/mcp`)
- GitHub MCP Server, including GitHub's hosted remote endpoint
- Astral `uv` / `uvx`
- Node.js, npm and npx
- GitHub CLI (`gh`)
- Git / MinGit
- PowerShell

The built-in Filesystem, PowerShell and Git MCP providers are LocalCode code and use the application's own approval, sandbox, timeout, backup and tool-discovery mechanisms.

Die integrierten Filesystem-, PowerShell- und Git-MCP-Provider sind Bestandteil von LocalCode und verwenden die Genehmigungs-, Sandbox-, Timeout-, Backup- und Werkzeugerkennungsmechanismen der Anwendung.

## External applications and services / Externe Anwendungen und Dienste

The following components can be detected or invoked by LocalCode but are not bundled with the source package or executable:

Die folgenden Komponenten können von LocalCode erkannt oder aufgerufen werden, sind aber nicht im Quellpaket oder in der EXE enthalten:

- Ollama and locally installed Ollama models; when enabled, LocalCode can download the official Windows installer and pull configured models automatically
- Microsoft Edge, Google Chrome, Visual Studio and Visual Studio Code
- Android SDK, ADB and Gradle
- Windows Terminal, OpenSSH and WSL
- user-configured MCP servers and command-line tools

## Web content / Webinhalte

Search results and fetched web pages remain subject to the copyright, licenses and terms of their respective publishers. LocalCode does not grant rights to third-party web content.

Suchergebnisse und abgerufene Webseiten unterliegen weiterhin den Urheberrechten, Lizenzen und Bedingungen ihrer jeweiligen Herausgeber. LocalCode räumt keine Rechte an Drittinhalten ein.
