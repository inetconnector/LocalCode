# LocalCode repository instructions / Repository-Regeln

## Deutsch

- Lies vor Änderungen `README.md`, `STATE.md`, `docs/ARCHITECTURE.md` und `docs/SECURITY.md`.
- Halte `STATE.md` nach jeder abgeschlossenen Änderung vollständig aktuell.
- **Sprachpflege ist verpflichtend:** Jede neue oder geänderte sichtbare Zeichenfolge muss gleichzeitig auf Deutsch und Englisch gepflegt werden. Alle Sprachkataloge müssen identische Schlüssel besitzen. Neue Sprachen müssen in Tests, Dokumentation, Systemerkennung und manueller Auswahl vollständig ergänzt werden.
- Standardverhalten: Windows-Anzeigesprache verwenden; Deutsch bei deutschem Windows, sonst Englisch. Die manuelle Auswahl muss diese Automatik überschreiben können.
- Bezeichne externe Programme erst nach vollständiger Werkzeugerkennung als fehlend. Dokumentiere Pfad, Exitcode, STDOUT, STDERR und durchsuchte Orte.
- Recherchiere bei unbekannter Werkzeugbedienung zuerst offizielle Herstellerdokumentation.
- Führe vor Abschluss `go fmt ./...`, `go test -race -count=1 ./...`, `go vet ./...`, JavaScript-Syntaxprüfungen und Windows-amd64-Builds aus.
- Änderungen an Datei-, Shell-, Git-, Netzwerk- oder MCP-Zugriff müssen Genehmigungen, Sandboxgrenzen, Timeouts, Abbruch und Protokollierung berücksichtigen.
- Destruktive Git- und Systembefehle bleiben blockiert oder werden in ein sichtbares interaktives Terminal ausgelagert.
- Behaupte keine vollständige Codex-Parität ohne nachprüfbare Tests; dokumentiere Grenzen offen.

## English

- Read `README.md`, `STATE.md`, `docs/ARCHITECTURE.md`, and `docs/SECURITY.md` before changing code.
- Keep `STATE.md` fully current after every completed change.
- **Localization maintenance is mandatory:** every new or changed user-visible string must be maintained in German and English at the same time. All language catalogs must contain identical keys. New languages must be added completely to tests, documentation, system detection, and manual selection.
- Default behavior: follow the Windows display language; use German on German Windows and English otherwise. Manual selection must override automatic detection.
- Do not declare an external tool missing before full tool discovery. Record path, exit code, STDOUT, STDERR, and searched locations.
- When tool usage is unknown, research official vendor documentation first.
- Before completion, run `go fmt ./...`, `go test -race -count=1 ./...`, `go vet ./...`, JavaScript syntax checks, and Windows-amd64 builds.
- Changes to file, shell, Git, network, or MCP access must preserve approvals, sandbox boundaries, timeouts, cancellation, and logging.
- Destructive Git and system commands remain blocked or are delegated to a visible interactive terminal.
- Do not claim full Codex parity without verifiable tests; document limitations honestly.
