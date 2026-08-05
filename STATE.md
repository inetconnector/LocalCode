# LocalCode project state / Projektstatus

**Version:** 6.0.0  
**License / Lizenz:** Apache-2.0  
**Status:** Aider editing engine integrated; bilingual source package, approvals, backups, undo, MCP, tool discovery, context compaction, tests, and Windows cross-build completed

## Deutsch

### Abgeschlossene Änderungen in 6.0.0

- Aider 0.86.2 als standardmäßige produktive Editing Engine integriert.
- Fest angeheftete, benutzerlokale Installation über `uv tool`, isoliertes Python 3.12, eigene Tool-, Cache- und Python-Verzeichnisse.
- Ollama-Anbindung über `ollama_chat/<modell>` und `OLLAMA_API_BASE`.
- Repository Map, aufgabenbezogene Dateivorauswahl und read-only Einbindung von `AGENTS.md`, `README.md` und `STATE.md`.
- Konfigurierbare Edit-Formate sowie optionaler Architect/Editor-Zweischritt.
- Dauerhafte, aufgabenbezogene Aider-Chat-, Eingabe- und LLM-Verläufe.
- Automatische, projektspezifische Lint- und Testbefehle für Go, Rust, Gradle/Android, .NET, Python und Node.js; manuelle Befehle bleiben konfigurierbar.
- Lokales Vorher-Backup und Hash-Manifest bei Bearbeitung, Lint und Test. Sichere Wiederherstellung verweigert das Überschreiben späterer manueller Änderungen.
- Kontrollierter Timeout, Prozessbaum-Abbruch und keine sichtbaren Konsolenfenster.
- Verwaltete leere Aider-Konfiguration und `.env`, damit externe Aider-Konfiguration oder Projektgeheimnisse nicht unbemerkt geladen werden.
- Aider-Status, Installation/Reparatur, Integrationstest und Undo in der Einstellungsseite.
- Native LocalCode-Edit-Schleife als explizit wählbarer Fallback erhalten.
- Bestehende MCP-, Werkzeugerkennungs-, Web-, Git-, Build-, Android-, Genehmigungs- und Kontextkomprimierungsfunktionen bleiben erhalten.
- Deutsche und englische Texte, README, Git-Anleitung, Architektur-, Sicherheits-, Aider- und Lizenzdokumentation gleichzeitig aktualisiert.

### Bekannte Betriebsgrenzen

- Die erstmalige Aider-/Python-Installation benötigt Internetzugang und ausdrückliche Genehmigung.
- Die tatsächliche Editierqualität bleibt vom ausgewählten lokalen Ollama-Modell abhängig.
- Eine echte Installation und ein realer Ollama-Lauf müssen auf dem Zielrechner verifiziert werden; der Quelltest verwendet zusätzlich einen kontrollierten Aider-Testprozess.

## English

### Completed changes in 6.0.0

- Integrated Aider 0.86.2 as the default production editing engine.
- Added a pinned per-user installation through `uv tool`, isolated Python 3.12, and dedicated tool, cache, and Python directories.
- Connected Ollama through `ollama_chat/<model>` and `OLLAMA_API_BASE`.
- Added repository maps, task-based edit-file selection, and read-only inclusion of `AGENTS.md`, `README.md`, and `STATE.md`.
- Added configurable edit formats and an optional two-stage architect/editor workflow.
- Added persistent task-specific Aider chat, input, and LLM histories.
- Added automatic project-specific lint and test commands for Go, Rust, Gradle/Android, .NET, Python, and Node.js; explicit commands remain configurable.
- Added a local pre-run backup and hash manifest for edits, linting, and testing. Safe restore refuses to overwrite later manual changes.
- Added controlled timeouts, process-tree cancellation, and hidden subprocess windows.
- Added a managed empty Aider configuration and `.env` file so external Aider configuration or repository secrets are not loaded silently.
- Added Aider status, install/repair, integration test, and undo controls to Settings.
- Preserved the native LocalCode edit loop as an explicit fallback.
- Preserved existing MCP, tool discovery, web, Git, build, Android, approval, and context-compaction features.
- Updated German and English UI text, README, Git guide, architecture, security, Aider, and licensing documentation together.

### Known operational boundaries

- The first Aider/Python installation requires internet access and explicit approval.
- Actual editing quality still depends on the selected local Ollama model.
- A real installation and Ollama run must be verified on the target computer; source tests additionally use a controlled Aider test process.

<!-- LOCALCODE:STATE:BEGIN -->
Managed runtime state is written here when this repository itself is selected in LocalCode.
Verwalteter Laufzeitstatus wird hier geschrieben, wenn dieses Repository selbst in LocalCode ausgewählt ist.
<!-- LOCALCODE:STATE:END -->
