# LocalCode Repository Instructions
- Externe Programme dürfen erst nach `discover_tool`/`run_tool` oder gleichwertiger absoluter Pfadauflösung als fehlend bezeichnet werden.
- Werkzeugfehler müssen mit Pfad, Exitcode, STDOUT und STDERR diagnostiziert werden; identische Aktionen und Rückfragen dürfen nicht ohne neue Evidenz wiederholt werden.
- Bei unbekannter Bedienung ist zuerst offizielle Herstellerdokumentation zu recherchieren; Drittquellen sind nur ergänzend zulässig.

- Lies `README.md`, `STATE.md`, `docs/ARCHITECTURE.md` und `docs/SECURITY.md` vor Änderungen.
- Halte `STATE.md` nach jeder abgeschlossenen Änderung vollständig aktuell.
- Verwende ausschließlich die Go-Standardbibliothek, sofern eine neue Abhängigkeit nicht ausdrücklich begründet und dokumentiert wird.
- Führe vor Abschluss `go fmt ./...`, `go test ./...`, `go vet ./...` und einen Windows-amd64-Cross-Build aus.
- Erhalte die Ein-Datei-Weboberfläche unter `src/static/index.html`, sofern keine bewusste Build-Pipeline eingeführt wird.
- Speichere keine Passwörter, Tokens oder privaten Schlüssel in Konfigurationsdateien. Nutze Umgebungsvariablenreferenzen.
- Änderungen an Datei-, Shell-, Git-, Netzwerk- oder MCP-Zugriff müssen Genehmigungen, Sandboxgrenzen, Timeouts und Protokollierung berücksichtigen.
- Destruktive Git- und Systembefehle bleiben blockiert oder müssen in ein sichtbares, interaktives Terminal ausgelagert werden.
- Behaupte keine Codex-Parität ohne nachprüfbare Tests. Dokumentiere Grenzen offen.
