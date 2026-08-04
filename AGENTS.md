# LocalCodex Repository Instructions

- Lies `README.md`, `STATE.md`, `docs/ARCHITECTURE.md` und `docs/SECURITY.md` vor Änderungen.
- Halte `STATE.md` nach jeder abgeschlossenen Änderung vollständig aktuell.
- Verwende ausschließlich die Go-Standardbibliothek, sofern eine neue Abhängigkeit nicht ausdrücklich begründet und dokumentiert wird.
- Führe vor Abschluss `go fmt ./...`, `go test ./...`, `go vet ./...` und einen Windows-amd64-Cross-Build aus.
- Erhalte die Ein-Datei-Weboberfläche unter `src/static/index.html`, sofern keine bewusste Build-Pipeline eingeführt wird.
- Speichere keine Passwörter, Tokens oder privaten Schlüssel in Konfigurationsdateien. Nutze Umgebungsvariablenreferenzen.
- Änderungen an Datei-, Shell-, Git-, Netzwerk- oder MCP-Zugriff müssen Genehmigungen, Sandboxgrenzen, Timeouts und Protokollierung berücksichtigen.
- Destruktive Git- und Systembefehle bleiben blockiert oder müssen in ein sichtbares, interaktives Terminal ausgelagert werden.
- Behaupte keine Codex-Parität ohne nachprüfbare Tests. Dokumentiere Grenzen offen.
