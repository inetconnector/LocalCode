# Aider integration / Aider-Integration

[Deutsch](#deutsch) · [English](#english)

## Deutsch

### Architektur

LocalCode integriert Aider als nichtinteraktiven Unterprozess. Das ist bewusst keine Kopie oder Neuimplementierung von Aiders internem Python-Code. LocalCode kontrolliert Installation, Argumente, Arbeitsverzeichnis, Umgebungsvariablen, Genehmigung, Timeout, Backup, Ergebnisdarstellung und Wiederherstellung.

Ablauf eines Bearbeitungsauftrags:

1. LocalCode erkennt einen echten Codeänderungsauftrag.
2. Die LocalCode-Genehmigungsregeln werden ausgewertet.
3. Fehlt Aider, wird eine ausdrückliche Installationsgenehmigung angefordert.
4. LocalCode erzeugt Fingerprints und ein Textdatei-Backup des Projekts.
5. Aufgabenrelevante Dateien werden vorausgewählt; die restliche Struktur liefert Aiders Repository Map.
6. Aider verarbeitet genau eine Nachricht aus einer geschützten Message-Datei und beendet sich anschließend.
7. LocalCode ermittelt tatsächlich veränderte Dateien, schreibt ein Hash-Manifest und zeigt vollständige Ausgabe, Exitcode, Dauer und Backup-Pfad.
8. Bei Undo wird nur wiederhergestellt, wenn jede betroffene Datei noch exakt dem Zustand nach dem Aider-Lauf entspricht.

### Installation

- Fest angeheftete Version: `aider-chat==0.86.2`.
- Python: isolierte, von `uv` verwaltete Python-3.12-Laufzeit.
- Installationsort: `%LOCALAPPDATA%\LocalCode\tools\aider`.
- Die portable Windows-Ausgabe von `uv` wird nur bei Bedarf geladen und vor Ausführung per SHA-256 geprüft.
- Globale Python-Installationen und globale Python-Pakete werden nicht verändert.

### Modell- und Kontextübergabe

- Ollama-Modelle werden als `ollama_chat/<modell>` übergeben.
- `OLLAMA_API_BASE` verweist auf die in LocalCode konfigurierte Ollama-Adresse.
- Aider steuert das Ollama-Kontextfenster passend zum einzelnen Request; LocalCode behält zusätzlich seine eigene übergeordnete Aufgaben- und Kontextkomprimierung.
- `--map-tokens` und `--max-chat-history-tokens` sind einstellbar.
- Historien werden pro LocalCode-Aufgabe unter dem LocalCode-Anwendungsdatenverzeichnis gespeichert, nicht im Projekt.

### Bearbeitung und Qualität

- `diff` ist Standard. Für schwächere lokale Modelle kann `whole` zuverlässiger sein.
- Architect/Editor kann dasselbe Modell zweimal oder unterschiedliche Modelle verwenden.
- Automatische Lint- und Testbefehle werden konservativ aus Projektdateien erkannt. Benutzerdefinierte Befehle überschreiben die Erkennung.
- Lint- und Test-Reparaturläufe erhalten denselben Backup- und Undo-Schutz wie normale Bearbeitungen.

### Git

Aider erhält `--git` nur, wenn das Projekt bereits ein Git-Repository ist und die Einstellung aktiv ist. Automatische Commits sind standardmäßig aus. LocalCodes eigene deterministische Git-Werkzeuge und Genehmigungen bleiben weiterhin verfügbar.

### Geheimnisse und Konfiguration

LocalCode gibt Aider eine explizite minimale Konfigurationsdatei und eine absichtlich leere `.env`. Dadurch werden `.aider.conf.yml` und `.env` aus Projekt oder Benutzerprofil nicht unbemerkt übernommen. Erforderliche Variablen müssen explizit in LocalCode konfiguriert werden.

### Abbruch

Ein Abbruch beendet den vollständigen Aider-Prozessbaum. Die zuvor angelegten Backups bleiben erhalten. Teiländerungen werden sichtbar als veränderte Dateien gemeldet und können über den sicheren Undo-Ablauf zurückgesetzt werden.

## English

### Architecture

LocalCode integrates Aider as a non-interactive subprocess. It deliberately does not copy or reimplement Aider's internal Python code. LocalCode controls installation, arguments, working directory, environment, approvals, timeout, backup, result presentation, and restoration.

Editing workflow:

1. LocalCode recognizes a real source-editing task.
2. LocalCode approval rules are evaluated.
3. If Aider is missing, LocalCode requests explicit installation approval.
4. LocalCode creates project fingerprints and a text-file backup.
5. Task-relevant files are preselected; Aider's repository map supplies the remaining structure.
6. Aider processes one message from a protected message file and exits.
7. LocalCode detects actual file changes and writes a hash manifest, then displays full output, exit code, duration, and backup location.
8. Undo restores files only when every affected file still matches the exact post-Aider state.

### Installation

- Pinned version: `aider-chat==0.86.2`.
- Python: an isolated Python 3.12 runtime managed by `uv`.
- Installation root: `%LOCALAPPDATA%\LocalCode\tools\aider`.
- The portable Windows build of `uv` is downloaded only when required and is verified by SHA-256 before execution.
- Global Python installations and packages are not modified.

### Model and context handoff

- Ollama models are passed as `ollama_chat/<model>`.
- `OLLAMA_API_BASE` points to LocalCode's configured Ollama endpoint.
- Aider sizes Ollama's context for an individual request; LocalCode additionally keeps its higher-level task state and context compaction.
- `--map-tokens` and `--max-chat-history-tokens` are configurable.
- Histories are stored per LocalCode task under the LocalCode application-data directory, not inside the project.

### Editing and quality

- `diff` is the default. `whole` can be more reliable for weaker local models.
- Architect/editor can use the same model twice or separate models.
- Automatic lint and test commands are detected conservatively from project files. Explicit commands override detection.
- Lint and test repair runs use the same backup and undo protection as normal edits.

### Git

Aider receives `--git` only when the project is already a Git repository and the option is enabled. Automatic commits are disabled by default. LocalCode's deterministic Git tools and approvals remain available.

### Secrets and configuration

LocalCode provides an explicit minimal configuration file and an intentionally empty `.env`. This prevents project or home `.aider.conf.yml` and `.env` files from being loaded silently. Required variables must be configured explicitly in LocalCode.

### Cancellation

Cancellation terminates the complete Aider process tree. Pre-run backups remain available. Partial changes are reported as changed files and can be reverted through the guarded undo workflow.
