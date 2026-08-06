# LocalCode 6.2.0 – 80% test coverage and race-safe chat finalization

## Deutsch

Diese Version erweitert die automatisierte Prüfung von LocalCode auf mehr als 80 Prozent Statement-Coverage und behebt dabei zusätzliche reale Fehler.

### Testabdeckung

- 151 Go-Testfunktionen.
- Exakte Statement-Coverage: **5.873 von 7.316 Statements = 80,276107 %**.
- Neue Tests für Agentenaktionen und Fehlerpfade, Genehmigungen, Webzugriffe, Aider, MCP, Git, PowerShell, Tool-Installation, Ollama- und Modell-Bootstrap, Projektkatalog, Datei-Sandbox, HTTP-Endpunkte, Migrationen und Windows-spezifische Werkzeugauflösung.
- Erfolgs-, Abbruch-, Timeout-, ungültige Eingabe-, fehlende Werkzeug- und Prozessfreigabepfade werden kontrolliert geprüft.

### Behobene Produktfehler

- **Race Condition beim Chatwechsel:** Ein Agent konnte nach dem Umschalten auf „nicht laufend“ noch sein abschließendes Statusereignis in einen gerade neu angelegten Chat schreiben. Die Finalisierung hält den Lauf nun bis zum letzten Statusereignis aktiv.
- **Ungeschützter Thread-Zugriff:** `NewChat` erzeugt seine Rückgabe jetzt noch unter dem State-Lock; parallele Ereignisaktualisierungen können die Threaddaten nicht mehr gleichzeitig verändern.
- **Aider-Backup-Pfad:** Erstellung und Wiederherstellung verwenden konsistent den konfigurierbaren LocalCode-Cachepfad.
- **Tool-Reparatur:** Ein typisierter `nil`-Fehler wird nicht mehr als scheinbar vorhandener Fehler weitergereicht.
- Die früheren Windows-Korrekturen für ADB-Tabulatorausgabe, MCP-Prozessfreigabe und isolierte Testprofile bleiben enthalten.

### Automatische Einrichtung

LocalCode erkennt und installiert weiterhin fehlende Komponenten selbstständig:

- unterstützte Go-Werkzeugkette für den Build,
- Ollama unter Windows,
- konfigurierte Ollama-Modelle,
- `uv`, Python und Aider 0.86.2,
- unterstützte Projekt- und MCP-Werkzeuge nach Genehmigung.

## English

This release raises LocalCode's automated verification above 80 percent statement coverage and fixes additional real defects discovered by the expanded tests.

### Test coverage

- 151 Go test functions.
- Exact statement coverage: **5,873 of 7,316 statements = 80.276107%**.
- New tests cover agent actions and failures, approvals, web access, Aider, MCP, Git, PowerShell, managed tool installation, Ollama/model bootstrap, the project catalog, file sandboxing, HTTP endpoints, migrations, and Windows-specific tool resolution.
- Success, cancellation, timeout, invalid-input, missing-tool, and process-release paths are exercised deterministically.

### Product fixes

- **Race condition during chat switching:** an agent could mark itself not running and then write its final status event into a newly selected chat. Finalization now remains active until the final event has been recorded.
- **Unprotected thread access:** `NewChat` now constructs its return summary while holding the state lock, preventing concurrent event updates from mutating the same thread data.
- **Aider backup path:** backup creation and restoration now consistently use the configurable LocalCode cache root.
- **Tool repair:** a typed nil error is no longer returned as an apparently non-nil error.
- Previous Windows fixes for tab-delimited ADB output, MCP process cleanup, and isolated test profiles remain included.

### Automatic setup

LocalCode continues to detect and install missing components automatically:

- a supported Go toolchain for builds,
- Ollama on Windows,
- configured Ollama models,
- `uv`, Python, and Aider 0.86.2,
- supported project and MCP tools after approval.
