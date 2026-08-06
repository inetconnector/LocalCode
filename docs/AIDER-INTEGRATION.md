# Aider integration / Aider-Integration

[Deutsch](#deutsch) · [English](#english)

## Deutsch

### Architektur

LocalCode integriert Aider als verwalteten, nichtinteraktiven Unterprozess. Aiders Python-Code wird weder kopiert noch neu implementiert. LocalCode kontrolliert Installation, Version, Argumente, Arbeitsverzeichnis, Umgebungsvariablen, Genehmigungen, Zeitlimit, Prozessabbruch, Backup, Ergebnisdarstellung und Wiederherstellung.

Ablauf:

1. Beim Programmstart prüft LocalCode, ob die festgelegte Aider-Version einsatzbereit ist. Eine fehlende oder abweichende Version wird bei aktivierter automatischer Einrichtung repariert.
2. Bei einem echten Codeänderungsauftrag wertet LocalCode seine Genehmigungsregeln aus.
3. Vor dem Aider-Lauf erzeugt LocalCode Projekt-Fingerprints und ein lokales Textdatei-Backup.
4. Aufgabenrelevante Dateien werden als bearbeitbar vorausgewählt; die übrige Struktur liefert Aiders Repository Map. `README.md`, `AGENTS.md` und `STATE.md` können schreibgeschützt als Kontext einfließen.
5. Aider verarbeitet genau eine Nachricht aus einer geschützten Message-Datei und beendet sich danach.
6. LocalCode ermittelt die tatsächlich veränderten Dateien, schreibt ein Hash-Manifest und zeigt Ausgabe, Exitcode, Dauer und Backup-Pfad.
7. Undo stellt nur Dateien wieder her, die noch exakt dem Zustand direkt nach dem Aider-Lauf entsprechen.

### Automatische Installation

- Fest angeheftete Version: `aider-chat==0.86.2`.
- Python: isolierte, von `uv` verwaltete Python-3.12-Laufzeit.
- Installationswurzel: `%APPDATA%\LocalCode\tools\aider` beziehungsweise das konfigurierte portable LocalCode-Datenverzeichnis.
- Die portable Windows-Ausgabe von `uv` wird nur bei Bedarf geladen und vor Ausführung gegen den im Quellcode festgelegten SHA-256-Wert geprüft.
- `uv tool install --force --python 3.12` installiert Aider in eigene Tool-, Python- und Cache-Verzeichnisse.
- Nach jeder Installation wird `aider --version` ausgeführt. LocalCode akzeptiert die Laufzeit nur, wenn die erwartete Version gemeldet wird.
- Globale Python-Installationen, globale Python-Pakete und Projekt-Virtual-Environments werden nicht verändert.

### Ollama- und Modellübergabe

- Vor Aider werden Ollama und die benötigten Modelle verifiziert. Fehlende konfigurierte Modelle werden bei aktivierter Auto-Pull-Option über `/api/pull` geladen.
- Auf einer frischen Installation wird standardmäßig nur `qwen2.5-coder:14b` geladen. Explizit konfigurierte Haupt-, Architect- oder Editor-Modelle werden zusätzlich berücksichtigt.
- Ollama-Modelle werden als `ollama_chat/<modell>` an Aider übergeben.
- `OLLAMA_API_BASE` verweist auf die tatsächlich von LocalCode gefundene oder konfigurierte Ollama-Adresse.
- `--map-tokens` und `--max-chat-history-tokens` sind einstellbar.
- Historien werden pro LocalCode-Aufgabe im LocalCode-Anwendungsdatenverzeichnis gespeichert, nicht im Projekt.

### Bearbeitung, Qualität und Git

- `diff` ist Standard; bei schwächeren lokalen Modellen kann `whole` zuverlässiger sein.
- Architect/Editor kann dasselbe Modell zweimal oder getrennte Modelle verwenden.
- Lint- und Testbefehle werden konservativ aus Projektdateien erkannt; explizite Befehle überschreiben die Erkennung.
- Bearbeitungs-, Lint- und Test-Reparaturläufe erhalten denselben Backup- und Undo-Schutz.
- Aider erhält `--git` nur, wenn das Projekt bereits ein Repository ist und die Option aktiviert wurde. Automatische Aider-Commits sind standardmäßig ausgeschaltet.

### Konfigurations- und Geheimnisgrenze

LocalCode übergibt eine verwaltete Minimal-Konfiguration, eine absichtlich leere `.env` und explizite Historienpfade. Dadurch werden `.aider.conf.yml`, `.env` oder andere Benutzer-/Projektkonfigurationen nicht unbemerkt übernommen. Gewollte Variablen müssen in LocalCode eingetragen sein. Analytics, Update-Prüfung, Browseröffnung, Shell-Vorschläge, URL-Erkennung, Benachrichtigungen, Dateiüberwachung und Prompt-Caching werden deaktiviert.

### Abbruch unter Windows

Aider wird in einer eigenen Windows-Prozessgruppe gestartet. Bei Abbruch beendet LocalCode zuerst den vollständigen Prozessbaum und wartet anschließend kontrolliert auf die Prozessfreigabe. Dadurch kehrt die Oberfläche zeitnah zurück und temporäre Dateien bleiben nicht durch einen weiterlaufenden Python-, CMD- oder Wrapper-Prozess gesperrt. Bereits angelegte Backups bleiben verfügbar.

## English

### Architecture

LocalCode integrates Aider as a managed, non-interactive subprocess. It neither copies nor reimplements Aider's Python internals. LocalCode controls installation, version, arguments, working directory, environment, approvals, timeout, process cancellation, backup, result presentation, and restoration.

Workflow:

1. At application startup LocalCode verifies the pinned Aider version. A missing or mismatched version is repaired when automatic setup is enabled.
2. For an actual source-editing task, LocalCode evaluates its approval policy.
3. Before Aider starts, LocalCode creates project fingerprints and a local text-file backup.
4. Task-relevant files are preselected as editable; Aider's repository map supplies the rest of the structure. `README.md`, `AGENTS.md`, and `STATE.md` may be included read-only.
5. Aider processes exactly one message from a protected message file and exits.
6. LocalCode detects actual changes, writes a hash manifest, and reports output, exit code, duration, and backup location.
7. Undo restores only files that still match the exact state immediately after the Aider run.

### Automatic installation

- Pinned version: `aider-chat==0.86.2`.
- Python: an isolated Python 3.12 runtime managed by `uv`.
- Installation root: `%APPDATA%\LocalCode\tools\aider`, or the configured portable LocalCode data directory.
- Portable Windows `uv` is downloaded only when required and checked against the SHA-256 value pinned in source before execution.
- `uv tool install --force --python 3.12` uses dedicated tool, Python, and cache directories.
- `aider --version` is executed after installation. LocalCode accepts the runtime only when the expected version is reported.
- Global Python installations, global packages, and project virtual environments are not modified.

### Ollama and model handoff

- Ollama and required models are verified before Aider. Missing configured models are pulled through `/api/pull` when auto-pull is enabled.
- A fresh installation pulls only `qwen2.5-coder:14b` by default. Explicit main, architect, or editor models are added as required.
- Ollama model names are passed as `ollama_chat/<model>`.
- `OLLAMA_API_BASE` points to the endpoint actually discovered or configured by LocalCode.
- `--map-tokens` and `--max-chat-history-tokens` are configurable.
- Histories are stored per LocalCode task in the application-data directory, not in the repository.

### Editing, quality, and Git

- `diff` is the default; `whole` may be more reliable for weaker local models.
- Architect/editor can use the same model twice or distinct models.
- Lint and test commands are detected conservatively from project files; explicit commands override detection.
- Edit, lint, and test repair runs receive the same backup and undo protection.
- Aider receives `--git` only when the project already is a repository and the option is enabled. Automatic Aider commits are disabled by default.

### Configuration and secret boundary

LocalCode supplies a managed minimal configuration, an intentionally empty `.env`, and explicit history paths. Project or user `.aider.conf.yml`, `.env`, and similar configuration therefore cannot be loaded silently. Intended variables must be entered in LocalCode. Analytics, update checks, browser opening, shell suggestions, URL detection, notifications, file watching, and prompt caching are disabled.

### Windows cancellation

Aider starts in a dedicated Windows process group. On cancellation LocalCode first terminates the complete process tree and then waits for process reaping. This returns control promptly and prevents temporary files from remaining locked by a surviving Python, CMD, or wrapper process. Existing backups remain available.
