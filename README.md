# LocalCode 4.8.0

[Deutsch](#deutsch) · [English](#english)

LocalCode is a local Windows coding-agent application for Ollama. It provides project-based chats, controlled tool execution, Git, builds, Android deployment, web research, MCP, attachments, approvals, context compaction, and a desktop-style user interface. LocalCode is an independent project and is not OpenAI Codex.

---

## Deutsch

### Schnellstart

1. Das ZIP vollständig in einen neuen Ordner entpacken.
2. `BUILD-AND-RUN.bat` doppelklicken.
3. Falls Go fehlt, lädt das Skript eine portable offizielle Go-Version.
4. LocalCode öffnet sich unter Windows bevorzugt in Edge oder Chrome im App-Modus.

Voraussetzung: Eine erreichbare lokale Ollama-Installation mit mindestens einem geeigneten Modell, beispielsweise `qwen2.5-coder:14b` oder `gpt-oss:20b`.

### Sprache

- `Automatisch (Windows)` verwendet die Windows-Anzeigesprache: Deutsch bei einem deutschen Windows, andernfalls Englisch.
- Deutsch und Englisch können in **Einstellungen → Allgemein → Sprache** manuell gewählt werden.
- Oberfläche, Dialoge, Genehmigungen, Statusmeldungen, Projektvorlagen und zentrale Dokumentation werden in beiden Sprachen gepflegt.
- Ein automatischer Test stellt sicher, dass alle Sprachkataloge dieselben Schlüssel enthalten.

### Bedienung und Kontrolle

- Projekt- und Chatnavigation links, Arbeitsverlauf in der Mitte, Ausgaben und Quellen rechts.
- Verschiebbare Splitter für linke und rechte Seitenleiste sowie das integrierte Terminal.
- Offene Genehmigungen erscheinen dauerhaft zusätzlich als Leiste unten in der Mitte – auch in Einstellungen oder anderen Ansichten.
- Der Projektordner kann über einen sichtbaren Windows-Ordnerdialog oder durch direkte Pfadeingabe geändert werden.
- Enter sendet; Umschalt+Enter erzeugt eine neue Zeile.
- Nach Abschluss oder kontrolliertem Abbruch wird das Eingabefeld wieder fokussiert.
- Modell- und Werkzeugaufrufe besitzen Zeitlimits und können kontrolliert beendet werden.

### Agent und Kontext

Der Supervisor erkennt typische Aufgaben wie Analyse, Build, Android-Deployment, Git-Initialisierung und Webrecherche. Wiederholte identische Fragen oder Werkzeugaktionen werden blockiert. Vor Erreichen des Kontextlimits wird der Verlauf komprimiert; erhalten bleiben Aufgabe, Entscheidungen, gelesene und geänderte Dateien, Befehle, Fehler, offene Punkte und die nächste geplante Aktion.

### Werkzeuge

LocalCode sucht Werkzeuge über:

- Projekt-Wrapper und lokale Binärdateien
- konfigurierte absolute Werkzeugpfade
- `PATH` und relevante Umgebungsvariablen
- Android-SDK-Verzeichnisse
- Visual-Studio-Installationen und `vswhere.exe`
- bekannte Windows-Installationsorte
- den benutzerlokalen LocalCode-Werkzeugordner

Fehlende unterstützte Werkzeuge werden nicht nur behauptet: LocalCode zeigt die durchsuchten Pfade, fragt nach Installationsgenehmigung, installiert das Werkzeug aus einer dokumentierten Quelle, verifiziert es und setzt anschließend die ursprüngliche Aktion fort. Unterstützt sind unter anderem Git/MinGit, Android Platform-Tools, .NET SDK, Visual Studio Build Tools sowie mehrere WinGet-Pakete.

### Dateien und Anhänge

Bis zu 20 Dateien pro Anfrage, insgesamt bis 96 MiB:

- Bilder über ein lokales Vision-Modell
- Text, Quellcode, JSON, XML, CSV, Markdown und Konfigurationen
- DOCX, PPTX, XLSX/XLSM
- ZIP, JAR, APK, AAB
- PDF über `pdftotext`, sofern vorhanden, sonst konservativer lokaler Fallback

Dateien können ausgewählt, per Drag-and-drop oder aus der Zwischenablage eingefügt werden.

### Git und Projektstatus

Das Quellpaket enthält `.gitignore`, `.gitattributes`, `.editorconfig`, `GIT-SETUP.md` und `COMMIT_MESSAGE.txt`. LocalCode kann in Projekten fehlende `README.md` und `AGENTS.md` anlegen und einen markierten Bereich in `STATE.md` aktuell halten, ohne manuelle Abschnitte zu überschreiben.

### Build und Qualität

`BUILD.bat` führt aus:

```text
go fmt ./...
go test -count=1 ./...
go vet ./...
Windows-amd64-GUI-Build
Windows-amd64-Diagnose-Build
SHA-256-Erzeugung
```

Der Release-Testbericht befindet sich in `TEST-REPORT.txt`.

### Sicherheit und Grenzen

Standardmäßig gelten projektbezogene Pfadgrenzen und Genehmigungen für Änderungen, Befehle und Netzwerkaktionen. Destruktive Git- und Systembefehle werden blockiert oder in ein sichtbares interaktives Terminal ausgelagert. Die native Anwendungssandbox ist nicht identisch mit der proprietären Codex-Infrastruktur. Eine vollständige Modell- oder Cloud-Parität kann mit einem lokalen 14B-/20B-Modell nicht seriös garantiert werden.

### Lizenz

Apache License 2.0. Siehe `LICENSE`, `NOTICE` und `THIRD_PARTY_NOTICES.md`.

---

## English

### Quick start

1. Extract the ZIP completely into a new directory.
2. Double-click `BUILD-AND-RUN.bat`.
3. If Go is missing, the script downloads an official portable Go distribution.
4. On Windows, LocalCode preferably opens in Edge or Chrome application mode.

Requirement: a reachable local Ollama installation with at least one suitable model, for example `qwen2.5-coder:14b` or `gpt-oss:20b`.

### Language

- `Automatic (Windows)` follows the Windows display language: German on German Windows, English otherwise.
- German and English can be selected manually under **Settings → General → Language**.
- The interface, dialogs, approvals, status messages, project templates, and central documentation are maintained in both languages.
- An automated test enforces identical keys in all language catalogs.

### Operation and control

- Project and chat navigation on the left, work history in the center, output and sources on the right.
- Resizable split panels for both sidebars and the integrated terminal.
- Pending approvals are always duplicated in a persistent bottom-center decision bar, including while Settings or another view is open.
- The project root can be changed through a visible Windows folder dialog or by entering a path directly.
- Enter sends; Shift+Enter inserts a new line.
- The prompt is focused again after completion or controlled cancellation.
- Model and tool calls have time limits and can be stopped in a controlled manner.

### Agent and context

The supervisor detects common tasks such as analysis, builds, Android deployment, Git initialization, and web research. Repeated identical questions or tool actions are blocked. Before the context limit is reached, older history is compacted while preserving the task, decisions, files read or changed, commands, failures, open items, and the next planned action.

### Tools

LocalCode searches for tools through:

- project wrappers and project-local binaries
- configured absolute tool paths
- `PATH` and relevant environment variables
- Android SDK directories
- Visual Studio installations and `vswhere.exe`
- known Windows installation locations
- the per-user LocalCode tools directory

For supported missing tools, LocalCode shows every searched path, requests installation approval, installs from a documented source, verifies the result, and then retries the original action. Supported installers include Git/MinGit, Android Platform-Tools, the .NET SDK, Visual Studio Build Tools, and several WinGet packages.

### Files and attachments

Up to 20 files per request with a combined 96 MiB limit:

- images through a local vision model
- text, source code, JSON, XML, CSV, Markdown, and configuration files
- DOCX, PPTX, XLSX/XLSM
- ZIP, JAR, APK, and AAB
- PDF through `pdftotext` when available, otherwise a conservative local fallback

Files can be selected, dragged and dropped, or pasted from the clipboard.

### Git and project state

The source package includes `.gitignore`, `.gitattributes`, `.editorconfig`, `GIT-SETUP.md`, and `COMMIT_MESSAGE.txt`. LocalCode can create missing `README.md` and `AGENTS.md` files in projects and keep a marked section of `STATE.md` current without overwriting manual sections.

### Build and quality

`BUILD.bat` runs:

```text
go fmt ./...
go test -count=1 ./...
go vet ./...
Windows-amd64 GUI build
Windows-amd64 diagnostic build
SHA-256 generation
```

The release verification report is stored in `TEST-REPORT.txt`.

### Security and limitations

Project path boundaries and approvals for changes, commands, and network actions are enabled by default. Destructive Git and system commands are blocked or moved to a visible interactive terminal. The native application-level sandbox is not identical to proprietary Codex infrastructure. Complete model or cloud-service parity cannot be honestly guaranteed with a local 14B/20B model.

### License

Apache License 2.0. See `LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md`.
