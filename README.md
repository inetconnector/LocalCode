# LocalCode 6.4.3

[Deutsch](#deutsch) · [English](#english)

LocalCode is a local Windows coding-agent application for Ollama. It provides project-based chats, controlled tool execution, Git, builds, Android deployment, web research, MCP, attachments, approvals, context compaction, and a desktop-style user interface. LocalCode is an independent project and is not OpenAI Codex.

---

## Deutsch

### Schnellstart

1. Das ZIP vollständig in einen neuen Ordner entpacken.
2. `START.bat` doppelklicken. Die Markierung `dist\REBUILD-NATIVE.txt` erzwingt vor dem ersten Programmstart einen vollständigen nativen Windows-Build und wird erst nach erfolgreicher Prüfung entfernt.
3. Fehlt eine unterstützte Go-Version, lädt das Build-Skript die aktuelle stabile Windows-Version direkt von `go.dev`, prüft deren offiziellen SHA-256-Wert und verwendet sie projektlokal.
4. Vor der eigentlichen Oberfläche öffnet LocalCode ein kompaktes Startfenster. Es zeigt die aktuelle Prüfung, Installation und den Fortschritt großer Modelldownloads sowie bei Fehlern Wiederholen, Log-Ordner, eingeschränkten Start und Beenden.
5. Beim ersten Programmstart prüft LocalCode Ollama, das konfigurierte Coding-Modell und die ausgewählte Coding-Agent-Engine. Fehlende unterstützte Komponenten werden automatisch benutzerlokal installiert und anschließend verifiziert.
6. LocalCode öffnet sich unter Windows bevorzugt in Edge oder Chrome im App-Modus.

Standardmodell für eine neue Installation: `qwen2.5-coder:14b`. Bereits vorhandene Ollama-Installationen und Modelle werden weiterverwendet.

### Sprache

- `Automatisch (Windows)` verwendet die Windows-Anzeigesprache: Deutsch bei einem deutschen Windows, andernfalls Englisch.
- Deutsch und Englisch können in **Einstellungen → Allgemein → Sprache** manuell gewählt werden.
- Oberfläche, Dialoge, Genehmigungen, Statusmeldungen, Projektvorlagen und zentrale Dokumentation werden in beiden Sprachen gepflegt.
- Ein automatischer Test stellt sicher, dass alle Sprachkataloge dieselben Schlüssel enthalten.


### Projekte, Aufgaben und Kontextmenüs

- Unter jedem aufgeklappten Projekt steht dauerhaft **Neue Aufgabe starten**. Zusätzlich gibt es den Plus-Knopf in der Projektzeile und **Neue Aufgabe** im Kontextmenü.
- Ein Rechtsklick oder der Drei-Punkte-Knopf am Projekt öffnet funktionierende Aktionen für neue Aufgaben, Anzeigenamen, Standardeditor, Visual Studio, Visual Studio Code, integriertes Terminal, Datei-Explorer, Anheften/Lösen und Entfernen aus der Seitenleiste. Entfernte Projekte können unter **Einstellungen → Archiviert** wiederhergestellt werden.
- Ein Rechtsklick oder der Drei-Punkte-Knopf an einer Aufgabe ermöglicht Umbenennen, Öffnen in einem neuen LocalCode-Fenster, Duplizieren, Archivieren und Löschen.
- Neue Aufgabenfenster laden ihren eigenen Aufgabenverlauf. Prompts enthalten immer die konkrete Projekt- und Aufgaben-ID, damit ein zweites Fenster nicht versehentlich in die zuletzt von einem anderen Fenster ausgewählte Aufgabe schreibt. Werkzeugläufe bleiben absichtlich global auf einen aktiven Agenten begrenzt.
- Kontextmenüs unterstützen Maus, Rechtsklick, Fokus, Pfeiltasten, Enter und Escape. Untermenüs werden am Fensterrand automatisch auf die andere Seite gespiegelt.
- Genehmigungen bieten **Einmal zulassen**, **Immer für dieses Projekt zulassen**, **Immer global zulassen** und **Ablehnen**. Dauerhafte Regeln können in den Einstellungen geprüft und gelöscht werden.

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

Der Startkontext des nativen Agenten folgt jetzt einer mehrstufigen Regelkette. LocalCode lädt globale LocalCode- und kompatible Codex-Anweisungen, projektnahe `AGENTS.override.md`/`AGENTS.md`-Dateien, Fallback-Anweisungen wie `CLAUDE.md`, `README.md` und `STATE.md`, relevante `.cursor/rules` sowie lokale Skills aus `.codex/skills`, `.cursor/skills`, `.opencode/skills` oder `skills`. Skills werden als Index aufgeführt; zur Aufgabe passende Skills werden vollständig und begrenzt eingebettet.

Die native LocalCode-Werkzeugschleife validiert strukturierte Agentenaktionen aktionsspezifisch. `write_file` wird ohne relativen Pfad und vollständigen nicht-leeren Inhalt abgelehnt; wiederholt unvollständige Schreibaktionen werden gezielt in eine Inhaltserzeugung repariert. Bei Editieraufgaben blockiert eine Abschlussprüfung zu frühes `finish`, wenn angeforderte Dateien fehlen oder leer sind, offensichtliche Platzhalter enthalten, verlangte Funktionsmerkmale nicht in den geänderten Dateien erkennbar sind oder nach der letzten Änderung eine ausdrücklich verlangte Prüfung nicht tatsächlich ausgeführt wurde. Ein zusätzlicher Abschluss-Review blockiert Implementierungsaufgaben, die nur Dokumentation ändern, und verlangt nach Code-, App- oder Tool-Änderungen eine passende Prüfung nach der letzten Änderung. Der verwaltete Abschnitt von `STATE.md` ist zusätzlich gegen versehentliches Überschreiben geschützt.

Für Herstellungs- und Änderungsaufgaben erzeugt LocalCode zusätzlich interne, sprach- und domänenneutrale Qualitätsanforderungen. Diese gelten für Go, Python, JavaScript/TypeScript, HTML/CSS, Java/Kotlin, C/C++, C#, Rust, Shell, Konfigurationen, Dokumente, Dateioperationen und visuelle Assets. Der Agent soll aus der Aufgabe selbst Abnahmekriterien ableiten, echte Logik statt README-Behauptungen umsetzen, UI-Kontrollen verdrahten, Dateioperationen durch Existenz-/Inhaltsprüfung bestätigen und Bild-/Diagrammaufgaben als konkrete darstellbare Artefakte erzeugen. Verifikationen werden nicht auf JavaScript verengt; erkannt werden unter anderem `go test`, `python -m py_compile`, `cargo check`, `javac`, `dotnet test`, `mvn test`, `gradle build`, `php -l`, `ruby -c`, `bash -n`, C/C++-Syntaxchecks und Browser-/DOM-Smokes, sofern passend. Globale Installer wie `npm install -g`, `pip install --user`, `cargo install`, `go install`, `winget install`, `choco install` oder `scoop install` werden bei normalen Aufgaben nicht als Reflex-Reparatur akzeptiert; der Agent muss vorhandene, projektlokale, verwaltete oder nicht-invasive Prüfwege bevorzugen, sofern der Nutzer keine Installation verlangt.

Bild-, Zeichen-, Diagramm- und Asset-Aufgaben müssen eine konkrete Artefaktdatei ändern, etwa SVG, HTML/Canvas, CSS oder eine Bilddatei. Eine Beschreibung oder ein SVG-Beispiel nur in einer README zählt nicht als fertiges Bild. Für lokale Icons, Diagramme und Vektorbilder stellt die native Werkzeugschleife `create_svg_asset` bereit: Das Modell liefert vollständiges SVG, LocalCode validiert XML-Struktur, Größenangabe und blockiert Skripte/Event-Handler, bevor die Datei geschrieben wird. Explizit genannte Bildpfade wie `assets/castle.svg` oder `preview.webp` werden als verpflichtende Projektdateien erkannt.

Für lokale Modelle budgetiert LocalCode den Modellkontext aggressiver als die sichtbare UI-Ausgabe. Große Werkzeugresultate werden im Ausgabenbereich weiter angezeigt, aber im nächsten Modellaufruf gekürzt. Nach einer Kontextkomprimierung reduziert LocalCode die unveränderten Recent-Messages adaptiv, kürzt sehr große neuere Nachrichten und fällt notfalls auf wenige jüngste Schritte plus faktischen Arbeitszustand zurück. So bleibt genug Kontextfenster für den nächsten Reparatur- oder Implementierungsschritt frei.

Der native Agent besitzt lokale, dauerhafte Erinnerungen. Mit `memory_remember`, `memory_list` und `memory_forget` kann er wichtige Projekt- oder Nutzerfakten speichern, im nächsten Agentenlauf wieder in den Kontext bekommen und per konkreter ID löschen. Projektbezogene Erinnerungen sind der Standard; globale Erinnerungen sind für ausdrücklich projektübergreifende Präferenzen gedacht. Inhalte, die wie Passwörter, Tokens, private Schlüssel oder API-Keys aussehen, werden abgelehnt.

Direkte Dateioperationen liefern überprüfbare Postconditions zurück. `write_file`, `delete_file`, `copy_path` und `move_path` melden nach der Ausführung Existenz, Typ, Größe und bei Dateien den SHA-256-Hash; Verschieben und Löschen melden ausdrücklich, ob die Quelle beziehungsweise das Ziel fehlt. Dadurch sieht das lokale Modell im nächsten Schritt nicht nur eine Erfolgsmeldung, sondern konkrete Fakten über den Dateisystemzustand.

Die Werkzeugerkennung umfasst Windows-Standardpfade, benutzerlokale Installationen und WinGet-Paketpfade für Node.js/npm/npx. Technologiewörter wie `Node.js` werden in Aufgaben nicht als Projektdatei fehlinterpretiert.

### Umschaltbare Coding-Agent-Engines

Unter **Einstellungen → Konfiguration → Coding-Agent-Engine** kann LocalCode zwischen drei vollständig angebundenen externen Bearbeitungs-Engines umschalten:

- **Aider 0.86.2** – Standard für lokale Ollama-Modelle. LocalCode installiert Aider reproduzierbar über `uv tool` mit Python 3.12 und verwendet `ollama_chat/<modell>`.
- **Claude Code** – Anthropics native CLI. Unter Windows verwendet LocalCode den offiziellen benutzerlokalen PowerShell-Installer, prüft Version und Anmeldung und startet Aufgaben nichtinteraktiv über `claude -p`. Stable-, Latest- oder konkrete Versionskanäle sind konfigurierbar. Für die Nutzung ist ein geeigneter Claude-/Anthropic-Zugang oder eine unterstützte Provider-Konfiguration erforderlich.
- **OpenCode** – provideroffene CLI. LocalCode installiert `opencode-ai` benutzerlokal über ein verwaltetes Node.js/npm, unterstützt `provider/modell`, Anmeldung über `opencode auth login` und lokale Ollama-Modelle. Für Ollama erzeugt LocalCode pro Prozess eine passende OpenCode-Providerkonfiguration, ohne die globale OpenCode-Konfiguration zu überschreiben.

Zusätzlich bleibt **LocalCode nativ** als interne Werkzeugschleife verfügbar. Diese Option ist keine vierte externe Engine.

Für jede externe Engine sind Statusprüfung, Installation/Reparatur, Anmeldung (soweit erforderlich), Repository-Analysetest, kontrollierter Abbruch, Ausgabeerfassung und Wiederherstellung der letzten Änderung integriert. Der Supervisor verwendet für mehrdateilige Änderungen, Repository-Analyse, Linting und Tests immer die aktuell ausgewählte Engine. Die alten `aider_*`-Agentenaktionen bleiben als kompatible Aliase erhalten.

Vor bearbeitenden Läufen erstellt LocalCode ein projektbezogenes Backup und ermittelt geänderte Dateien anhand von Fingerprints. **Letzte Engine-Änderung zurücksetzen** schützt spätere manuelle Änderungen durch Hash-Prüfung. LocalCodes Genehmigungsgrenze liegt vor dem Start der externen Engine; die externe Engine läuft anschließend mit ihren eigenen Werkzeug- und Berechtigungsregeln. Der gefährliche Claude-Modus `bypassPermissions` wird in LocalCode bewusst nicht angeboten. OpenCodes automatische Genehmigung kann in den Einstellungen ausgeschaltet werden.

Aider-spezifische Details stehen in `docs/AIDER-INTEGRATION.md`; Vergleich, Installation, Modelle, Anmeldung und Sicherheitsgrenzen aller Engines stehen in `docs/CODING-ENGINES.md`.

### Werkzeuge

LocalCode sucht Werkzeuge über:

- Projekt-Wrapper und lokale Binärdateien
- konfigurierte absolute Werkzeugpfade
- `PATH` und relevante Umgebungsvariablen
- Android-SDK-Verzeichnisse
- Visual-Studio-Installationen und `vswhere.exe`
- bekannte Windows-Installationsorte
- den benutzerlokalen LocalCode-Werkzeugordner

Die Kernlaufzeit (Ollama, konfigurierte Ollama-Modelle und die ausgewählte Engine) wird beim Start automatisch vervollständigt. **Downloads für automatische Einrichtung** ist dabei bewusst vom Agenten-/Web-Schalter **Netzwerkzugriff** getrennt: Eine deaktivierte Websuche blockiert die Ersteinrichtung nicht. Für Aider gehören `uv` und Python 3.12 dazu; für OpenCode bei Bedarf ein verwaltetes Node.js/npm; Claude Code verwendet den offiziellen nativen Windows-Installer. Weitere projektspezifische Werkzeuge werden bedarfsgesteuert erkannt; LocalCode zeigt die durchsuchten Pfade, holt für eingriffsreiche Installationen eine Genehmigung ein, installiert aus einer dokumentierten Quelle, verifiziert das Ergebnis und setzt die ursprüngliche Aktion fort. Unterstützt sind unter anderem Git/MinGit, Android Platform-Tools, .NET SDK, Visual Studio Build Tools sowie mehrere WinGet-Pakete.


### Verwaltete MCP-Suite

LocalCode 6.4.3 enthält sechs verwaltete MCP-Funktionsbereiche:

- **Filesystem MCP** – integrierte, projektgebundene Datei- und Verzeichniswerkzeuge.
- **PowerShell MCP** – integrierte Skriptausführung, Cmdlet-Erkennung und Hilfe ohne sichtbare Konsolenfenster.
- **Git MCP** – integrierte Git-Werkzeuge mit sicherer Argumentübergabe, Genehmigungen und Blockierung destruktiver Befehle.
- **Fetch MCP** – offizieller Referenzserver über `uvx mcp-server-fetch`.
- **GitHub MCP** – offizieller gehosteter Streamable-HTTP-Server mit PAT oder GitHub-CLI-Anmeldung.
- **Playwright MCP** – offizieller Microsoft-Server mit persistentem Browserprofil und persistenter stdio-Sitzung.

Unter **Einstellungen → Plugins** lassen sich Installation, Anmeldung, Aktivierung, Verbindungstest und Sitzungsreset pro Server steuern. LocalCode kann fehlendes uv, portables Node.js LTS und GitHub CLI nach ausdrücklicher Genehmigung benutzerlokal installieren. Eigene stdio- und Streamable-HTTP-Server bleiben über die erweiterte JSON-Konfiguration möglich. Details stehen in `docs/MCP-SUITE.md`.

Beim Initialisieren von MCP-Servern übernimmt LocalCode serverweite `instructions` in den Agentenkontext. Auch die eingebauten Filesystem-, PowerShell- und Git-MCP-Server liefern kurze Nutzungsregeln, damit das lokale Modell Werkzeuggrenzen und Ergebnisprüfung nicht nur aus Toolnamen ableitet.

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
go test -count=1 -timeout=180s ./...
go vet ./...
go test -shuffle=on -count=1 -timeout=180s ./...
Windows-amd64-GUI-Build
Windows-amd64-Diagnose-Build
SHA-256-Erzeugung
```

Der Release-Testbericht befindet sich in `TEST-REPORT.txt`.

### Sicherheit und Grenzen

Standardmäßig gelten projektbezogene Pfadgrenzen und Genehmigungen für Änderungen, Befehle und Netzwerkaktionen. Pfade werden einschließlich Symlinks und NTFS-Junctions kanonisch geprüft. Webabrufe binden sich an die bereits validierte öffentliche IP, wodurch DNS-Rebinding auf Loopback- oder Privatadressen blockiert wird. Destruktive Git- und Systembefehle werden blockiert oder in ein sichtbares interaktives Terminal ausgelagert. Die native Anwendungssandbox ist nicht identisch mit der proprietären Codex-Infrastruktur. Eine vollständige Modell- oder Cloud-Parität kann mit einem lokalen 14B-/20B-Modell nicht seriös garantiert werden.

### Lizenz

Apache License 2.0. Siehe `LICENSE`, `NOTICE` und `THIRD_PARTY_NOTICES.md`.

---

## English

### Quick start

1. Extract the ZIP completely into a new directory.
2. Double-click `START.bat` or `BUILD-AND-RUN.bat`.
3. If no supported Go version is available, the build script downloads the current stable Windows release directly from `go.dev`, verifies its official SHA-256 value, and uses it inside the project.
4. Before the main UI, LocalCode opens a compact startup window showing the current check, installation, and large model-download progress. On failure it offers retry, log-folder access, limited mode, and exit.
5. On first application startup, LocalCode verifies Ollama, the configured coding model, and the selected coding-agent engine. Missing supported components are installed automatically for the current user and verified afterwards.
6. On Windows, LocalCode preferably opens in Edge or Chrome application mode.

Default model for a fresh installation: `qwen2.5-coder:14b`. Existing Ollama installations and models are reused.

### Language

- `Automatic (Windows)` follows the Windows display language: German on German Windows, English otherwise.
- German and English can be selected manually under **Settings → General → Language**.
- The interface, dialogs, approvals, status messages, project templates, and central documentation are maintained in both languages.
- An automated test enforces identical keys in all language catalogs.


### Projects, tasks, and context menus

- Every expanded project permanently shows **Start new task**. The project row also provides a plus button, and the context menu contains **New task**.
- Right-clicking a project or using its ellipsis opens working actions for a new task, display-name editing, the default editor, Visual Studio, Visual Studio Code, the integrated terminal, File Explorer, pin/unpin, and removing the project from the sidebar. Removed projects can be restored under **Settings → Archived**.
- Right-clicking a task or using its ellipsis provides rename, open in a new LocalCode window, duplicate, archive, and delete actions.
- New task windows load their own task history. Prompts always carry the explicit project and task ID so a second window cannot accidentally write to the task most recently selected by another window. Tool execution intentionally remains limited to one globally active agent run.
- Context menus support mouse input, right-click, focus, arrow keys, Enter, and Escape. Submenus automatically flip near the edge of the window.
- Approvals provide **Allow once**, **Always allow for this project**, **Always allow globally**, and **Reject**. Persistent rules can be reviewed and deleted in Settings.

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

The native agent's startup context now follows a layered rule chain. LocalCode loads global LocalCode and compatible Codex instructions, project-local `AGENTS.override.md`/`AGENTS.md` files, fallback instructions such as `CLAUDE.md`, `README.md`, and `STATE.md`, relevant `.cursor/rules`, and local skills from `.codex/skills`, `.cursor/skills`, `.opencode/skills`, or `skills`. Skills are listed in an index; skills relevant to the task are embedded fully with limits.

The native LocalCode tool loop validates structured agent actions per action. `write_file` is rejected without a relative path and complete non-empty content; repeated incomplete write actions are repaired through a focused content-generation retry. For editing tasks, a completion guard blocks premature `finish` when requested files are missing or empty, obvious placeholders remain, requested feature markers are not visible in the changed files, or an explicitly requested check was not actually run after the last change. An additional completion review blocks implementation tasks that only change documentation and requires a suitable check after the last code, app, or tool change. The managed section of `STATE.md` is additionally protected from accidental overwrite.

For creation and modification tasks, LocalCode also adds internal, language- and domain-neutral quality requirements. They apply to Go, Python, JavaScript/TypeScript, HTML/CSS, Java/Kotlin, C/C++, C#, Rust, shell, configuration files, documents, file operations, and visual assets. The agent should derive acceptance criteria from the task itself, implement real logic instead of README claims, wire UI controls, verify file operations through existence/content checks, and produce concrete renderable artifacts for image or diagram requests. Verification is not JavaScript-specific; recognized checks include `go test`, `python -m py_compile`, `cargo check`, `javac`, `dotnet test`, `mvn test`, `gradle build`, `php -l`, `ruby -c`, `bash -n`, C/C++ syntax checks, and browser/DOM smokes where appropriate. Global installers such as `npm install -g`, `pip install --user`, `cargo install`, `go install`, `winget install`, `choco install`, or `scoop install` are not accepted as reflex repairs during ordinary tasks; the agent must prefer existing, project-local, managed, or non-invasive verification paths unless the user actually requested installation.

Image, drawing, diagram, and asset tasks must change a concrete artifact file such as SVG, HTML/canvas, CSS, or an image file. A description or SVG example only inside a README does not count as a finished image. For local icons, diagrams, and vector images, the native tool loop provides `create_svg_asset`: the model supplies complete SVG, and LocalCode validates XML structure, size metadata, and blocks scripts/event handlers before writing the file. Explicit image paths such as `assets/castle.svg` or `preview.webp` are recognized as required project files.

For local models, LocalCode budgets model context more aggressively than visible UI output. Large tool results remain visible in the output panel, but are shortened before being fed back to the next model call. After context compaction, LocalCode adaptively reduces unchanged recent messages, truncates very large newer messages, and can fall back to only a few latest steps plus the factual working state. This keeps enough context window available for the next repair or implementation step.

The native agent has local durable memories. Through `memory_remember`, `memory_list`, and `memory_forget`, it can store important project or user facts, receive them again in later agent context, and delete them by concrete ID. Project-scoped memories are the default; global memories are for explicitly cross-project preferences. Content that looks like passwords, tokens, private keys, or API keys is rejected.

Direct file operations return verifiable postconditions. `write_file`, `delete_file`, `copy_path`, and `move_path` report existence, type, size, and file SHA-256 hashes after execution; moves and deletes explicitly report whether the source or target is gone. This gives the local model concrete filesystem facts on the next step instead of only a success message.

Tool discovery covers Windows standard paths, per-user installations, and WinGet package paths for Node.js/npm/npx. Technology names such as `Node.js` are not mistaken for requested project files.

### Switchable coding-agent engines

Under **Settings → Configuration → Coding-agent engine**, LocalCode can switch between three fully integrated external editing engines:

- **Aider 0.86.2** – the default for local Ollama models. LocalCode installs Aider reproducibly through `uv tool` with Python 3.12 and uses `ollama_chat/<model>`.
- **Claude Code** – Anthropic's native CLI. On Windows, LocalCode uses the official per-user PowerShell installer, verifies the version and authentication state, and runs tasks non-interactively through `claude -p`. Stable, latest, or exact-version channels are configurable. A suitable Claude/Anthropic account or supported provider configuration is required.
- **OpenCode** – a provider-neutral CLI. LocalCode installs `opencode-ai` per user through managed Node.js/npm, supports `provider/model`, sign-in through `opencode auth login`, and local Ollama models. For Ollama, LocalCode supplies a process-scoped OpenCode provider configuration without overwriting the user's global OpenCode configuration.

**LocalCode native** remains available as the internal tool loop; it is not a fourth external engine.

Each external engine has integrated status checks, installation/repair, sign-in where required, repository-analysis testing, controlled cancellation, output capture, and restoration of the last change. The supervisor uses the selected engine for multi-file edits, repository analysis, linting, and tests. Legacy `aider_*` agent actions remain compatible aliases.

Before editing runs, LocalCode creates a project backup and detects changed files through fingerprints. **Restore last engine change** protects later manual edits through hash checks. LocalCode's approval boundary applies before the external process starts; the selected engine then applies its own tool and permission rules. Claude's dangerous `bypassPermissions` mode is intentionally not exposed. OpenCode auto-approval can be disabled in Settings.

Aider-specific details are in `docs/AIDER-INTEGRATION.md`; comparison, installation, models, authentication, and security boundaries for all engines are documented in `docs/CODING-ENGINES.md`.

### Tools

LocalCode searches for tools through:

- project wrappers and project-local binaries
- configured absolute tool paths
- `PATH` and relevant environment variables
- Android SDK directories
- Visual Studio installations and `vswhere.exe`
- known Windows installation locations
- the per-user LocalCode tools directory

The core runtime (Ollama, configured Ollama models, and the selected engine) is completed automatically at startup. **Allow downloads for automatic setup** is deliberately separate from the agent/web **Network access** switch, so disabling web research does not deadlock first-run setup. Aider provisions `uv` and Python 3.12; OpenCode provisions managed Node.js/npm when required; Claude Code uses the official native Windows installer. Additional project-specific tools are discovered on demand; for invasive installations LocalCode shows the searched paths, requests approval, installs from a documented source, verifies the result, and retries the original action. Supported installers include Git/MinGit, Android Platform-Tools, the .NET SDK, Visual Studio Build Tools, and several WinGet packages.


### Managed MCP suite

LocalCode 6.4.3 includes six managed MCP capability areas:

- **Filesystem MCP** – built-in, project-scoped file and directory tools.
- **PowerShell MCP** – built-in script execution, command discovery, and help without visible console windows.
- **Git MCP** – built-in Git tools with safe argument passing, approvals, and destructive-command blocking.
- **Fetch MCP** – official reference server through `uvx mcp-server-fetch`.
- **GitHub MCP** – official hosted Streamable HTTP server using a PAT or GitHub CLI sign-in.
- **Playwright MCP** – official Microsoft server with a persistent browser profile and persistent stdio session.

Under **Settings → Plugins**, installation, sign-in, enablement, connection testing, and session reset can be controlled per server. After explicit approval, LocalCode can install missing uv, portable Node.js LTS, and GitHub CLI for the current user. Custom stdio and Streamable HTTP servers remain available through the advanced JSON configuration. See `docs/MCP-SUITE.md` for details.

When initializing MCP servers, LocalCode carries server-wide `instructions` into the agent context. The built-in Filesystem, PowerShell, and Git MCP servers also provide short usage rules so the local model learns tool boundaries and result-checking behavior from more than tool names.

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

Project path boundaries and approvals for changes, commands, and network actions are enabled by default. Paths are checked after resolving symlinks and NTFS junctions. Web fetches dial the already validated public IP, blocking DNS rebinding to loopback or private addresses. Destructive Git and system commands are blocked or moved to a visible interactive terminal. The native application-level sandbox is not identical to proprietary Codex infrastructure. Complete model or cloud-service parity cannot be honestly guaranteed with a local 14B/20B model.

### License

Apache License 2.0. See `LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md`.
