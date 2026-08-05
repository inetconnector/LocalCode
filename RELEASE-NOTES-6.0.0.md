# LocalCode 6.0.0 – Aider editing engine

[Deutsch](#deutsch) · [English](#english)

## Deutsch

LocalCode 6.0.0 ersetzt für echte Quellcodeänderungen die bisherige primäre Eigenimplementierung durch eine kontrollierte Integration von Aider. Die LocalCode-Oberfläche und alle Sicherheits-, Genehmigungs-, MCP-, Werkzeug-, Backup- und Abbruchmechanismen bleiben erhalten.

### Kernfunktionen

- Aider 0.86.2, fest angeheftet und über eine isolierte benutzerlokale `uv tool`-Umgebung installiert.
- Direkte Ollama-Anbindung mit `ollama_chat/<modell>`.
- Repository Map und aufgabenbezogene Auswahl editierbarer Dateien.
- `AGENTS.md`, `README.md` und `STATE.md` als read-only Kontext.
- `diff`, `whole`, `udiff`, `editor-diff` und `editor-whole`.
- Optionaler Architect/Editor-Modus mit getrennten Modellen.
- Persistenter Verlauf pro LocalCode-Aufgabe.
- Automatisches Linting und Testen mit Projektprofilerkennung.
- Vorher-Backups und sichere Undo-Prüfung über Datei-Hashes.
- Kontrollierte Installation, Statusprüfung, Test und Wiederherstellung über die Einstellungen.
- Ollama-Modellnamen mit Unterpfaden wie `hf.co/...` werden korrekt über `ollama_chat/` geroutet.
- Repository-Map-, Lint- und Testläufe verwenden dieselbe isolierte Konfiguration, leere `.env` und Abbruchlogik wie Bearbeitungsläufe.
- Harte Zeitlimits und Prozessgruppen-Abbruch verhindern hängende Aider-, Lint- und Testprozesse.
- Native LocalCode-Engine als wählbarer Fallback.

### Sicherheit

LocalCode startet Aider nicht unkontrolliert. Vor schreibenden Läufen gilt weiterhin LocalCodes Genehmigungsmodell. Der Aider-Prozess erhält eine explizite verwaltete Konfiguration, eine leere `.env`, definierte Historienpfade, ein Timeout und einen kontrollierten Prozessbaum-Abbruch.

## English

LocalCode 6.0.0 replaces the previous primary in-house editing path for real source changes with a controlled Aider integration. The LocalCode UI and all approval, MCP, tool, backup, and cancellation mechanisms remain in place.

### Core capabilities

- Aider 0.86.2, pinned and installed through an isolated per-user `uv tool` environment.
- Direct Ollama connection using `ollama_chat/<model>`.
- Repository maps and task-based selection of editable files.
- `AGENTS.md`, `README.md`, and `STATE.md` supplied as read-only context.
- `diff`, `whole`, `udiff`, `editor-diff`, and `editor-whole` formats.
- Optional architect/editor mode with separate models.
- Persistent history per LocalCode task.
- Automatic linting and testing with project-profile detection.
- Pre-run backups and hash-based safe undo.
- Controlled installation, status, integration test, and restore controls in Settings.
- Ollama model names containing paths such as `hf.co/...` are routed correctly through `ollama_chat/`.
- Repository-map, lint, and test runs use the same isolated configuration, empty `.env`, and cancellation behavior as edit runs.
- Hard deadlines and process-group termination prevent stuck Aider, lint, and test processes.
- Native LocalCode engine retained as an explicit fallback.

### Security

LocalCode does not start Aider without control. LocalCode's approval policy remains active before write-capable runs. The Aider subprocess receives an explicit managed configuration, an empty `.env`, defined history paths, a timeout, and controlled process-tree termination.
