# LocalCode Release Notes

Kanonische Release-Historie von LocalCode. Diese Datei ersetzt die früheren einzelnen `RELEASE-NOTES-<version>.md`-Dateien. Die neuesten Versionen stehen zuerst; historische Details bleiben erhalten.

## Inhalt

- [6.4.4](#644)
- [6.4.3](#643)
- [6.4.2](#642)
- [6.4.1](#641)
- [6.4.0](#640)
- [6.3.0](#630)
- [6.2.0](#620)
- [6.1.1](#611)
- [6.1.0](#610)
- [6.0.0](#600)
- [5.0.0](#500)
- [4.9.0](#490)
- [4.8.0](#480)
- [4.7.0](#470)
- [4.6.0](#460)
- [4.5.0](#450)
- [4.4.0](#440)
- [4.3.0](#430)
- [4.0.0](#400)

---

## 6.4.4

### Deutsch

- Fügt eine token-geschützte Handy-Remote-Web-App unter `/remote` hinzu.
- Pairing läuft über **Hilfe → Remote koppeln** mit kurzlebigem sechsstelligen Code.
- Remote-Geräte erhalten zufällige Tokens; LocalCode speichert nur Token-Hashes.
- Die Remote-App kann Projekte und Aufgaben auswählen, Chatverläufe verfolgen, neue Aufgaben starten, laufende Arbeit stoppen und Genehmigungen beantworten.
- Die Oberfläche orientiert sich am dunklen AHSMA-Handylayout.
- Grenze: native Android-Hülle, automatische LAN-Erkennung/QR und Benachrichtigungen bleiben Folgeaufgaben.

### English

- Adds a token-protected phone Remote web app at `/remote`.
- Pairing is started from **Help → Pair Remote** with a short-lived six-digit code.
- Remote devices receive random tokens; LocalCode stores only token hashes.
- The Remote app can select projects and tasks, follow chat history, start new tasks, stop active work, and answer approvals.
- The UI follows the dark AHSMA phone layout.
- Limit: a native Android shell, automatic LAN discovery/QR, and notifications remain follow-up work.

---

## 6.4.3

### Windows test isolation and portability

This release fixes the three failures reported by the native Windows build of 6.4.2.

- Coding-engine setup tests now disable the dedicated `SetupDownloadsEnabled` permission. They can no longer invoke a real Claude Code installer merely because agent/web networking is disabled.
- The project-folder picker is injected per `Server` instance. HTTP tests simulate picker failure, cancellation, and success and never open a real Windows Forms dialog.
- Cross-platform command tests use `echo` instead of the Unix-only `printf` command, so the same assertions run under PowerShell.

### Regression coverage

- The engine setup endpoint verifies the disabled-download error path without touching the network or user installation.
- `/api/browse-root` covers error, cancellation, and successful selection through an in-process fake picker.
- Terminal and agent command execution are exercised with commands understood by both POSIX shells and Windows PowerShell.

### Verification boundary

The complete Go suite, race detector, vet, randomized orders, coverage and UI simulation were executed in the release environment. Windows application and test executables were cross-compiled and inspected as PE32+ files. The release environment cannot natively execute Windows PE or batch files, so `dist/REBUILD-NATIVE.txt` remains and forces the full native Windows build before first launch.

---

## 6.4.2

### Windows native build and test isolation

- The native build no longer sets `LOCALCODE_CONFIG_HOME`, `LOCALCODE_CACHE_HOME`, or `LOCALCODE_USER_HOME` around the Go test process. Those variables intentionally have highest priority and previously shadowed `t.Setenv` in Windows tests.
- The build now isolates tests with temporary XDG, profile, AppData, and LocalAppData directories while leaving per-test overrides effective.
- Tests for project-root repair and legacy LocalCode data migration now explicitly replace the highest-priority directory overrides.
- Tests that require a missing Aider executable isolate `PATH`, so a real user installation cannot change the expected result.
- Coding-engine installation tests use the dedicated automatic-setup download permission rather than the unrelated web/agent network setting.

### MCP and coding-engine fixes

- MCP file resources now use canonical `file:///C:/...` URIs in tests and agent actions.
- The parser also accepts the common noncanonical Windows form `file://C:/...` without losing the drive letter and preserves UNC authorities.
- Claude Code and OpenCode version/authentication probes no longer use a possibly nonexistent LocalCode configuration directory as their working directory. This prevented false “engine missing” results and approval waits on fresh or isolated profiles.

### Verification

- All reported Windows failures have dedicated regression coverage.
- Normal tests, `go vet`, randomized-order tests, race-detector groups, coverage, Windows cross-compilation, UI checks, archive safety, and manifest verification are performed before packaging.

---

## 6.4.1

### Windows startup and Ollama installer fixes

#### Fixed

- Replaced the complex UTF-8 `BUILD.bat` implementation with a small ASCII/CRLF wrapper and a dedicated PowerShell build driver.
- Normalized every Windows batch launcher to ASCII with CRLF line endings so `cmd.exe` no longer interprets fragments such as `VERSION`, `LC_LANG`, or localized text as commands.
- Added a packaging regression test that rejects BOMs, non-ASCII bytes, and bare LF line endings in all `.bat` launchers.
- Increased the guarded Ollama installer download limit from 1 GiB to 4 GiB. This covers the observed official installer size of 1,563,278,432 bytes while retaining a finite safety ceiling.
- Added live Ollama-installer download progress to the startup splash, including transferred and total MiB/GiB.
- Removed stale `.part` files before retrying and kept atomic replacement of completed managed downloads.
- Added response-header timeout handling without imposing the former ten-minute total transfer timeout on a multi-gigabyte installer.
- Added tests for the installer size policy, progress reporting, partial-file cleanup, and human-readable byte formatting.
- Added dedicated `scripts/build.ps1` and `scripts/needs-build.ps1` drivers so batch launchers stay small, ASCII-only, and reliably parsed by `cmd.exe`.
- Added `scripts/build.ps1` to the startup freshness check so changes to the native build driver force a rebuild.
- Made clean start, diagnostics, and project-root reset delegate to the same current-build checks instead of running potentially stale binaries.
- Fixed project-root reset writing a UTF-8 BOM that Go's JSON decoder could reject. The reset script now writes UTF-8 without BOM, and LocalCode also accepts legacy BOM-prefixed configuration files.
- Deletes the multi-gigabyte Ollama installer after each installation attempt instead of leaving it in the LocalCode download cache.
- Clears inherited `GOOS`, `GOARCH`, and `CGO_ENABLED` before tests so a user's shell environment cannot accidentally cross-compile the test phase.

#### Windows packaging

All `.bat` files are intentionally ASCII and CRLF encoded. `scripts/install-go.ps1` is UTF-8 with BOM and CRLF for Windows PowerShell 5.1 compatibility.

#### Final verification

- 189 Go test functions; normal tests and `go vet` pass.
- Race detector passes across four complete partitions.
- Randomized test orders 11, 22, and 33 pass.
- Exact statement coverage: 6,576 / 8,201 = 80.185343%.
- Browser UI E2E: 37 mocked API requests pass.
- Final Windows GUI, debug, and test executables cross-compile as PE32+ x86-64 files.
- Native Windows batch/PowerShell execution remains intentionally required on first start through `dist\REBUILD-NATIVE.txt`.

---

## 6.4.0

### Startup setup and progress window

- Added a compact startup splash window before the main LocalCode UI.
- The splash reports Ollama discovery, installation, service start, installed-model checks, per-model download status and size progress, selected engine setup, and final readiness.
- Failure actions include retry, opening the log folder, starting in limited mode, and exiting.
- On Windows, Edge or Chrome app mode is opened at a compact splash size and expands before the main UI is shown.

### Fixed first-run network deadlock

- Automatic dependency downloads are now controlled by `setup_downloads_enabled`, independently from agent/web `network_enabled`.
- Configuration schema 10 migrates existing schema-9 installations with setup downloads enabled, while preserving the user's agent/web network preference.
- Disabling web search therefore no longer prevents LocalCode from installing Ollama, configured models, Aider, Claude Code, OpenCode, or explicitly enabled managed tools.
- Startup setup failures are localized and remain actionable in the splash instead of terminating behind a generic message box.

### Additional correctness and security fixes

- Ollama URLs preserve HTTPS, reject unsupported schemes, and correctly normalize IPv6 hosts.
- Only models required by the selected external engine are downloaded; inactive Aider/OpenCode models no longer trigger unnecessary multi-gigabyte pulls.
- Selecting an engine that is disabled now falls back explicitly to LocalCode native instead of leaving an unusable engine selected.
- The startup server validates loopback Host headers, uses bounded HTTP timeouts, and avoids double-escaping displayed paths and versions.
- MCP uvx setup now uses the same pinned, SHA-256-verified uv Windows archive instead of executing a downloaded PowerShell installer script.
- Project STATE output distinguishes agent network access from automatic setup downloads.

### Verification

The final package is rebuilt from this source revision and includes source tests, race-detector partitions, randomized test orders, vet, UI/API simulation, translation/DOM checks, statement coverage above 80%, and Windows-amd64 cross-build verification. Native execution of the Windows batch files and PE binaries remains a target-machine verification boundary; `REBUILD-NATIVE.txt` forces a complete first build on Windows.

---

## 6.3.0

### Three switchable coding-agent engines

LocalCode can now select **Aider**, **Claude Code**, or **OpenCode** under Settings. The selection is used consistently by repository analysis, multi-file editing, lint, test-repair, installation, status, login, cancellation, backup, and undo workflows. LocalCode native remains available as the internal tool loop.

- Aider retains the pinned, isolated `uv`/Python 3.12 installation and Ollama integration.
- Claude Code uses the official native Windows installer, supports stable/latest/exact channels, validates authentication, and runs non-interactively with bounded turns and a safe permission mode.
- OpenCode is installed through a user-local managed npm prefix, supports cloud providers and local Ollama, and receives a process-scoped Ollama provider configuration without modifying the user's OpenCode files.

### UI and configuration

- New engine selector and separate settings panels for all three external engines.
- Generic status, install/repair, sign-in, repository-analysis test, and undo controls.
- Engine version, executable, authentication state, and errors are visible in Settings and the status bar.
- Configuration schema updated to version 9 with migration-safe defaults.
- Complete German and English translations for the new controls and statuses.

### Execution and recovery

- Generic `engine_edit`, `engine_repo_map`, `engine_lint`, `engine_test`, and `engine_undo` actions route through the selected engine.
- Legacy `aider_*` actions remain compatible aliases.
- External `.cmd` and `.bat` launchers are executed correctly on Windows.
- All editing engines use pre-edit backups, changed-file fingerprints, controlled cancellation, output capture, timeout handling, and guarded restoration.
- Startup auto-setup installs only the selected external engine and pulls its required Ollama models when applicable.

### Security

- Claude Code `bypassPermissions` is rejected and not offered in the UI.
- OpenCode `--auto` can be disabled.
- LocalCode does not store Claude Code or OpenCode provider credentials.
- Documentation now distinguishes LocalCode's pre-launch approval/project checks from a true OS-level sandbox around external CLIs.

### Tests

The release adds deterministic tests for engine normalization, status, authentication, installation commands, command-line construction, Ollama provider injection, backup/undo, API endpoints, UI selection, and startup integration. The final release suite retains at least 80% statement coverage, race-detector verification, randomized test orders, UI/API simulation, translation parity, and Windows-amd64 cross-build validation.

---

## 6.2.0

### Deutsch

Diese Version erweitert die automatisierte Prüfung von LocalCode auf mehr als 80 Prozent Statement-Coverage und behebt dabei zusätzliche reale Fehler.

#### Testabdeckung

- 151 Go-Testfunktionen.
- Exakte Statement-Coverage: **5.873 von 7.316 Statements = 80,276107 %**.
- Neue Tests für Agentenaktionen und Fehlerpfade, Genehmigungen, Webzugriffe, Aider, MCP, Git, PowerShell, Tool-Installation, Ollama- und Modell-Bootstrap, Projektkatalog, Datei-Sandbox, HTTP-Endpunkte, Migrationen und Windows-spezifische Werkzeugauflösung.
- Erfolgs-, Abbruch-, Timeout-, ungültige Eingabe-, fehlende Werkzeug- und Prozessfreigabepfade werden kontrolliert geprüft.

#### Behobene Produktfehler

- **Race Condition beim Chatwechsel:** Ein Agent konnte nach dem Umschalten auf „nicht laufend“ noch sein abschließendes Statusereignis in einen gerade neu angelegten Chat schreiben. Die Finalisierung hält den Lauf nun bis zum letzten Statusereignis aktiv.
- **Ungeschützter Thread-Zugriff:** `NewChat` erzeugt seine Rückgabe jetzt noch unter dem State-Lock; parallele Ereignisaktualisierungen können die Threaddaten nicht mehr gleichzeitig verändern.
- **Aider-Backup-Pfad:** Erstellung und Wiederherstellung verwenden konsistent den konfigurierbaren LocalCode-Cachepfad.
- **Tool-Reparatur:** Ein typisierter `nil`-Fehler wird nicht mehr als scheinbar vorhandener Fehler weitergereicht.
- Die früheren Windows-Korrekturen für ADB-Tabulatorausgabe, MCP-Prozessfreigabe und isolierte Testprofile bleiben enthalten.

#### Automatische Einrichtung

LocalCode erkennt und installiert weiterhin fehlende Komponenten selbstständig:

- unterstützte Go-Werkzeugkette für den Build,
- Ollama unter Windows,
- konfigurierte Ollama-Modelle,
- `uv`, Python und Aider 0.86.2,
- unterstützte Projekt- und MCP-Werkzeuge nach Genehmigung.

### English

This release raises LocalCode's automated verification above 80 percent statement coverage and fixes additional real defects discovered by the expanded tests.

#### Test coverage

- 151 Go test functions.
- Exact statement coverage: **5,873 of 7,316 statements = 80.276107%**.
- New tests cover agent actions and failures, approvals, web access, Aider, MCP, Git, PowerShell, managed tool installation, Ollama/model bootstrap, the project catalog, file sandboxing, HTTP endpoints, migrations, and Windows-specific tool resolution.
- Success, cancellation, timeout, invalid-input, missing-tool, and process-release paths are exercised deterministically.

#### Product fixes

- **Race condition during chat switching:** an agent could mark itself not running and then write its final status event into a newly selected chat. Finalization now remains active until the final event has been recorded.
- **Unprotected thread access:** `NewChat` now constructs its return summary while holding the state lock, preventing concurrent event updates from mutating the same thread data.
- **Aider backup path:** backup creation and restoration now consistently use the configurable LocalCode cache root.
- **Tool repair:** a typed nil error is no longer returned as an apparently non-nil error.
- Previous Windows fixes for tab-delimited ADB output, MCP process cleanup, and isolated test profiles remain included.

#### Automatic setup

LocalCode continues to detect and install missing components automatically:

- a supported Go toolchain for builds,
- Ollama on Windows,
- configured Ollama models,
- `uv`, Python, and Aider 0.86.2,
- supported project and MCP tools after approval.

---

## 6.1.1

### Deutsch

Diese Wartungsversion behebt die unter Windows reproduzierten Build-Abbrüche aus 6.1.0.

#### Korrekturen

- Der Windows-ADB-Test erzeugt jetzt eine echte tabulatorgetrennte `adb devices -l`-Ausgabe. Das simulierte Gerät wird dadurch korrekt erkannt.
- Der persistente MCP-STDIO-Test beendet und reapet den Hilfsprozess, bevor Windows die temporären Arbeitsordner löscht.
- Eine zusätzliche Regressionprüfung löscht den MCP-Projektordner unmittelbar nach `Close()` und erkennt verbliebene Dateihandles direkt.
- Build-Tests verwenden eigene temporäre Konfigurations-, Cache- und Benutzerverzeichnisse und greifen nicht auf das reale LocalCode-Profil zu.
- Die simulierte Ollama-Modellinstallation schreibt keine irreführende Pull-Meldung mehr in den normalen Testlauf.
- Das Distributionsarchiv enthält wieder Windows-Binärdateien im Ordner `dist`. Eine Markierungsdatei erzwingt vor dem ersten Start einen vollständigen nativen Neuaufbau auf dem Zielsystem.

#### Automatische Laufzeiteinrichtung

Die in 6.1.0 eingeführte automatische Einrichtung bleibt enthalten:

- fehlendes Ollama unter Windows installieren und starten,
- konfigurierte Ollama-Modelle automatisch laden,
- `uv`, Python und die festgelegte Aider-Version automatisch einrichten,
- vorhandene Installationen erkennen und weiterverwenden.

### English

This maintenance release fixes the Windows build failures reproduced in 6.1.0.

#### Fixes

- The Windows ADB test now generates a real tab-delimited `adb devices -l` response, allowing the simulated device to be detected correctly.
- The persistent MCP STDIO test now terminates and reaps its helper process before Windows removes temporary working directories.
- An additional regression check removes the MCP project directory immediately after `Close()` and detects leaked file handles directly.
- Build tests use isolated temporary configuration, cache, and user directories instead of the real LocalCode profile.
- The simulated Ollama model installation no longer prints a misleading pull message during the normal test run.
- The distribution archive once again includes Windows executables in `dist`. A marker forces a complete native rebuild on the target system before first launch.

#### Automatic runtime setup

The automatic setup introduced in 6.1.0 remains included:

- install and start Ollama on Windows when missing,
- automatically download configured Ollama models,
- automatically provision `uv`, Python, and the pinned Aider version,
- detect and reuse existing installations.

---

## 6.1.0

### Deutsch

#### Behobene Windows-Fehler

| Gemeldeter Fehler | Ursache | Korrektur |
|---|---|---|
| Aider-Abbruch dauerte etwa zehn Sekunden; TempDir gesperrt | `exec.CommandContext` beendete den CMD-Wrapper, bevor der vollständige Kindprozessbaum zuverlässig beendet wurde | eigene Prozessgruppe, `taskkill /T /F`, kontrolliertes Warten auf `Cmd.Wait` |
| Legacy-Konfiguration wurde im Test nicht migriert | XDG-Testpfade wurden unter Windows ignoriert | plattformübergreifende `LOCALCODE_*_HOME`-/XDG-Unterstützung |
| MCP-Test hinterließ gesperrte Dateien | Session-Close wartete nicht auf die tatsächliche Prozessfreigabe | separates `processDone` und kontrolliertes Reaping |
| temporäre Projektwurzel sprang auf den echten Projektordner zurück | die Sicherheitsprüfung verwarf pauschal alles unter `LOCALAPPDATA` | nur LocalCodes konkrete verwaltete Datenordner werden abgelehnt |
| Projektkatalog-/Alias-/Kontexttests sahen reale Projekte | Folge der falschen Wurzelrücksetzung | vollständige Testisolation und korrekte temporäre Root-Anwendung |
| ADB-Version fehlte | Test schrieb Batch-Inhalt in eine Datei mit `.exe`-Endung | echter kleiner Windows-Testhelfer wird mit Go erzeugt |
| Missing-tool-Tests fanden vorhandenes ADB | Tests übernahmen reale `PATH`-/SDK-Variablen | isoliertes Profil, leeres Test-PATH und leere Android-Variablen |

#### Automatische Einrichtung

- Ollama wird beim Start gesucht und gestartet.
- Fehlt Ollama unter Windows, lädt LocalCode `OllamaSetup.exe` ausschließlich von `ollama.com`, verlangt eine gültige Authenticode-Signatur und installiert unbeaufsichtigt für den aktuellen Benutzer.
- Fehlende konfigurierte Modelle werden automatisch geladen. Auf einem frischen System wird standardmäßig `qwen2.5-coder:14b` verwendet; explizite Aider-Haupt-, Architect- und Editor-Modelle werden zusätzlich berücksichtigt.
- Aider wird auf Version 0.86.2 geprüft und bei Bedarf automatisch über ein SHA-256-geprüftes portables `uv` und isoliertes Python 3.12 installiert oder repariert.
- Vorhandene Installationen und Modelle werden nicht unnötig ersetzt.
- Die Optionen sind in den Einstellungen auf Deutsch und Englisch sichtbar.

#### Sicherheitsverbesserungen

- Sandboxprüfung löst Symlinks und NTFS-Junctions auf, bevor ein Pfad freigegeben wird.
- DNS-Rebinding kann die Webabrufprüfung nicht mehr durch eine zweite Namensauflösung umgehen.
- Externe Websuchanbieter verwenden ebenfalls den öffentlichen-IP-Dialer.
- Go-Builds verlangen eine noch unterstützte Go-1.25+-Linie; andernfalls installiert das Skript die aktuelle stabile Version aus der offiziellen Release-Liste mit SHA-256-Prüfung.

#### Auslieferung

Das ZIP enthält bewusst keine mit der im Container vorhandenen alten Go-1.23.2-Werkzeugkette erstellten EXE-Dateien. `START.bat` baut sie automatisch auf dem Windows-Zielrechner. Damit verwendet der gemeldete Rechner seine vorhandene Go-Version 1.26.5.

### English

#### Fixed Windows failures

| Reported failure | Root cause | Fix |
|---|---|---|
| Aider cancellation took about ten seconds and locked TempDir | `exec.CommandContext` terminated the CMD wrapper before the complete child tree was reliably stopped | dedicated process group, `taskkill /T /F`, controlled `Cmd.Wait` reaping |
| legacy configuration was not migrated in the test | Windows ignored XDG test paths | cross-platform `LOCALCODE_*_HOME`/XDG support |
| MCP test left locked files | session close did not wait for actual process release | separate `processDone` and controlled reaping |
| temporary project root reverted to the real project directory | safety logic rejected everything below `LOCALAPPDATA` | only LocalCode's concrete managed data directories are rejected |
| catalog/alias/context tests exposed real projects | consequence of the incorrect root reset | full test isolation and correct temporary-root application |
| ADB version was empty | the test wrote batch text into a file named `.exe` | a real small Windows helper executable is built with Go |
| missing-tool tests found installed ADB | tests inherited real `PATH` and SDK variables | isolated profile, empty test PATH, and cleared Android variables |

#### Automatic setup

- Ollama is discovered and started at application startup.
- If missing on Windows, LocalCode downloads `OllamaSetup.exe` only from `ollama.com`, requires a valid Authenticode signature, and installs it unattended for the current user.
- Missing configured models are pulled automatically. A fresh system defaults to `qwen2.5-coder:14b`; explicit Aider main, architect, and editor models are included.
- Aider is verified at version 0.86.2 and automatically installed or repaired through SHA-256-verified portable `uv` and isolated Python 3.12.
- Existing installations and models are reused.
- All options are visible in German and English settings.

#### Security improvements

- Sandbox validation resolves symlinks and NTFS junctions before granting access.
- DNS rebinding can no longer bypass web-fetch validation through a second hostname lookup.
- External web-search providers use the same public-IP dialer.
- Builds require a still-supported Go 1.25+ line; otherwise the script installs the current stable release from the official feed with SHA-256 verification.

#### Distribution

The ZIP intentionally excludes executables built with the container's obsolete Go 1.23.2 toolchain. `START.bat` builds them automatically on the Windows target machine. The reported target therefore uses its installed Go version 1.26.5.

---

## 6.0.0

### Deutsch

LocalCode 6.0.0 ersetzt für echte Quellcodeänderungen die bisherige primäre Eigenimplementierung durch eine kontrollierte Integration von Aider. Die LocalCode-Oberfläche und alle Sicherheits-, Genehmigungs-, MCP-, Werkzeug-, Backup- und Abbruchmechanismen bleiben erhalten.

#### Kernfunktionen

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

#### Sicherheit

LocalCode startet Aider nicht unkontrolliert. Vor schreibenden Läufen gilt weiterhin LocalCodes Genehmigungsmodell. Der Aider-Prozess erhält eine explizite verwaltete Konfiguration, eine leere `.env`, definierte Historienpfade, ein Timeout und einen kontrollierten Prozessbaum-Abbruch.

### English

LocalCode 6.0.0 replaces the previous primary in-house editing path for real source changes with a controlled Aider integration. The LocalCode UI and all approval, MCP, tool, backup, and cancellation mechanisms remain in place.

#### Core capabilities

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

#### Security

LocalCode does not start Aider without control. LocalCode's approval policy remains active before write-capable runs. The Aider subprocess receives an explicit managed configuration, an empty `.env`, defined history paths, a timeout, and controlled process-tree termination.

---

## 5.0.0

### Deutsch

LocalCode 5.0.0 ergänzt eine verwaltete MCP-Laufzeit, damit Dateioperationen, PowerShell, Git, Webabruf, GitHub und Browserautomation nicht nur konfiguriert, sondern dauerhaft verbunden, geprüft und kontrolliert ausgeführt werden können.

#### Enthaltene MCP-Server

- **Filesystem** – direkt in LocalCode implementiert, auf das aktive Projekt beschränkt, mit Lesen, Schreiben, Suchen, Kopieren, Verschieben, Metadaten und kontrolliertem Löschen.
- **PowerShell** – direkt in LocalCode implementiert, ohne sichtbare Konsolenfenster, mit Skriptausführung, `Get-Command` und `Get-Help`.
- **Git** – direkt in LocalCode implementiert, mit Status, Diff, Historie, Initialisierung, Staging, Commit, Branch, Checkout, Pull, Push und Show. Destruktive Git-Operationen bleiben blockiert.
- **Fetch** – offizieller MCP-Referenzserver über `uvx mcp-server-fetch`.
- **GitHub** – offizieller gehosteter GitHub MCP Server über Streamable HTTP. Die Authentifizierung verwendet `GITHUB_PAT_TOKEN` oder den Token einer bestehenden GitHub-CLI-Anmeldung.
- **Playwright Browser** – offizieller Microsoft Playwright MCP Server. Der stdio-Prozess und das Browserprofil bleiben über mehrere Werkzeugaufrufe erhalten.

#### Laufzeit und Zuverlässigkeit

- Persistente stdio-Sitzungen statt eines neuen MCP-Prozesses pro Werkzeugaufruf.
- Persistente Streamable-HTTP-Sitzungen mit `Mcp-Session-Id`.
- MCP-Protokollverhandlung mit aktuellen und kompatiblen älteren Versionen.
- Unterstützung für Serveranfragen wie `roots/list` und `ping`.
- Vollständige Unterstützung für `tools/list`, `tools/call`, Ressourcen und Prompts.
- Kontrollierter Timeout, Sitzungsreset und Prozessbaum-Abbruch.
- Projektplatzhalter `${PROJECT_ROOT}`, `${APP_DATA}` und `${USER_HOME}`.
- GitHub-Token werden nicht in der LocalCode-Konfiguration gespeichert.

#### Automatische Einrichtung

Unter **Einstellungen → Plugins** besitzt jeder verwaltete Server eine echte Statuskarte mit:

- Aktivieren/Deaktivieren
- Installieren
- Anmelden
- Verbindung testen
- Sitzung zurücksetzen
- Anzeige der tatsächlich erkannten Werkzeuge und Fehler

Fehlende Laufzeiten können nach ausdrücklicher Genehmigung benutzerlokal eingerichtet werden:

- `uv`/`uvx` für Fetch
- offizielles portables Node.js LTS für Playwright
- offizielle portable GitHub CLI für GitHub

#### Sicherheit

- Dateiwerkzeuge bleiben in der Projektwurzel beziehungsweise den ausdrücklich erlaubten Wurzeln.
- Schreibende MCP-Werkzeuge durchlaufen weiterhin LocalCodes Genehmigungsregeln.
- PowerShell wird ohne sichtbares Konsolenfenster und mit Timeout ausgeführt.
- Force-Push, `git reset --hard`, `git clean -fdx` und vergleichbar destruktive Git-Aktionen bleiben blockiert.
- Browser- und Netzwerkwerkzeuge sind weiterhin von Netzwerk- und Genehmigungseinstellungen abhängig.

### English

LocalCode 5.0.0 adds a managed MCP runtime so file operations, PowerShell, Git, web fetching, GitHub, and browser automation are not merely configurable: they stay connected, are verified, and run under explicit control.

#### Included MCP servers

- **Filesystem** – implemented directly in LocalCode, restricted to the active project, with read, write, search, copy, move, metadata, and controlled deletion tools.
- **PowerShell** – implemented directly in LocalCode, without visible console windows, with script execution, `Get-Command`, and `Get-Help`.
- **Git** – implemented directly in LocalCode, with status, diff, history, initialization, staging, commit, branch, checkout, pull, push, and show. Destructive Git operations remain blocked.
- **Fetch** – official MCP reference server through `uvx mcp-server-fetch`.
- **GitHub** – official hosted GitHub MCP Server with the complete toolset over Streamable HTTP. Authentication uses `GITHUB_PAT_TOKEN` or the token from an existing GitHub CLI sign-in.
- **Playwright Browser** – official Microsoft Playwright MCP Server. The stdio process and browser profile persist across tool calls.

#### Runtime and reliability

- Persistent stdio sessions instead of spawning a new MCP process per call.
- Persistent Streamable HTTP sessions using `Mcp-Session-Id`.
- MCP protocol negotiation with the current and compatible earlier versions.
- Support for server-to-client requests such as `roots/list` and `ping`.
- Full `tools/list`, `tools/call`, resources, and prompts support.
- Controlled timeout, session reset, and process-tree termination.
- `${PROJECT_ROOT}`, `${APP_DATA}`, and `${USER_HOME}` placeholders.
- GitHub tokens are never persisted in the LocalCode configuration.

#### Managed setup

Under **Settings → Plugins**, every managed server has a real status card with:

- Enable/disable
- Install
- Sign in
- Connection test
- Session reset
- Display of discovered tools and concrete errors

After explicit approval, missing runtimes can be installed for the current user:

- `uv`/`uvx` for Fetch
- official portable Node.js LTS for Playwright
- official portable GitHub CLI for GitHub

#### Security

- Filesystem tools remain inside the project root or explicitly allowed roots.
- Mutating MCP tools continue to use LocalCode approval rules.
- PowerShell runs without visible console windows and with a timeout.
- Force push, `git reset --hard`, `git clean -fdx`, and similarly destructive Git actions remain blocked.
- Browser and network tools remain governed by network and approval settings.

---

## 4.9.0

[Deutsch](#deutsch-490) · [English](#english-490)

### Deutsch

LocalCode 4.9.0 ergänzt die fehlenden Projekt- und Aufgabenaktionen aus der Seitenleiste und verbindet jedes sichtbare Menüelement mit einer realen Backend- oder Desktop-Aktion.

#### Projekte

- Neue Aufgabe direkt unter einem aufgeklappten Projekt starten.
- Neue Aufgabe über den Plus-Knopf oder das Kontextmenü starten.
- Anzeigenamen bearbeiten, ohne den Ordner auf der Festplatte umzubenennen.
- Im Standardeditor, Visual Studio, Visual Studio Code, Datei-Explorer oder integrierten Terminal öffnen.
- Projekte anheften oder lösen.
- Projekte aus der Seitenleiste entfernen und in den Einstellungen wiederherstellen.

#### Aufgaben

- Umbenennen, duplizieren, archivieren und löschen.
- In einem neuen LocalCode-Fenster öffnen.
- Aufgabenfenster laden gezielte Snapshots und senden Projekt- und Aufgaben-ID explizit mit jedem Prompt.
- Ereignisse werden anhand ihrer Aufgaben-ID pro Fenster gefiltert.

#### Bedienung und Qualität

- Rechtsklick und Drei-Punkte-Knöpfe öffnen dieselben Menüs.
- Kontextmenüs sind valide ARIA-Menüs ohne verschachtelte Schaltflächen.
- Tastatursteuerung mit Pfeiltasten, Enter und Escape.
- Untermenüs wechseln am rechten Fensterrand automatisch auf die linke Seite.
- Alle neuen Texte sind vollständig auf Deutsch und Englisch vorhanden.

### English

LocalCode 4.9.0 adds the missing project and task actions in the sidebar and connects every visible menu item to a real backend or desktop action.

#### Projects

- Start a new task directly below an expanded project.
- Start a task from the plus control or the context menu.
- Edit the display name without renaming the directory on disk.
- Open in the default editor, Visual Studio, Visual Studio Code, File Explorer, or the integrated terminal.
- Pin or unpin projects.
- Remove projects from the sidebar and restore them in Settings.

#### Tasks

- Rename, duplicate, archive, and delete.
- Open in a new LocalCode window.
- Task windows load targeted snapshots and include the explicit project and task ID with every prompt.
- Events are filtered per window by task ID.

#### Interaction and quality

- Right-click and ellipsis controls open the same menus.
- Context menus are valid ARIA menus without nested buttons.
- Keyboard operation with arrow keys, Enter, and Escape.
- Submenus automatically move to the left near the right edge of the window.
- All new text is maintained in German and English.

---

## 4.8.0

### Deutsch

- Vollständige Deutsch-/Englisch-Pflege für Oberfläche, Einstellungsseite, Dialoge, Werkzeughinweise, Projektvorlagen, Git-Anleitung und Hauptdokumentation.
- Automatische Erkennung der Windows-Anzeigesprache: Deutsch bei deutschem Windows, sonst Englisch; zusätzlich manuelle Umschaltung.
- Dauerhafte Genehmigungsleiste unten mittig, auch wenn Einstellungen, Quellen oder andere Ansichten geöffnet sind.
- Verbesserte Projektordnerauswahl mit direkter Pfadeingabe und im Vordergrund gehaltenem Windows-Dialog.
- Reparierte Ganzzahlserialisierung für Splitter-, Kontext- und Timeoutwerte.
- Atomisches Speichern von Konfiguration und Chatverlauf mit Wiederherstellungsbackup.
- Zusammenfassender Hintergrundschreiber für Chatereignisse statt synchroner Dateizugriffe.
- Geordneter Persistenz-Shutdown mit Flush des neuesten Chatstands.
- Einstellungen werden erst nach erfolgreichem Speichern aktiviert; Projekt- und Dokumentationsfehler werden nicht mehr verschluckt.
- Bereinigter Genehmigungszustand nach Genehmigung, Ablehnung, Abbruch und Timeout.
- Startzeitlimit für Ollama-Modellabfrage.
- Zusätzlicher Schutz des lokalen Servers gegen fremde Hosts, Cross-Origin-/Cross-Site-Aufrufe und Einbettung.
- Bilinguale Build- und Hilfsskripte nach Windows-Anzeigesprache.
- Neue Race-, Reihenfolge-, Lokalisierungs-, Genehmigungs-, Projektwurzel-, Sicherheits- und Persistenztests.

### English

- Complete German/English maintenance for the interface, Settings, dialogs, tool guidance, project templates, Git instructions, and main documentation.
- Automatic Windows display-language detection: German on German Windows, English otherwise, with a manual override.
- Persistent bottom-center approval bar, including Settings, Sources, and other views.
- Improved project-folder selection with direct path entry and a foreground-owned Windows dialog.
- Fixed integer serialization for splitters, context, and timeout values.
- Atomic configuration and chat-history persistence with recovery backups.
- Coalesced background writer for chat events instead of synchronous file writes.
- Orderly persistence shutdown with a final flush of the newest chat state.
- Settings become active only after a successful durable write; project and documentation errors are no longer ignored.
- Pending approval state is cleared after approval, rejection, cancellation, and timeout.
- Startup timeout for Ollama model discovery.
- Additional local-server protection against foreign Hosts, cross-origin/cross-site requests, and embedding.
- Bilingual build and helper scripts following the Windows display language.
- New race, order-randomization, localization, approval, project-root, security, and persistence tests.

---

## 4.7.0

### Agent Supervisor und kontrollierte Kontextkomprimierung

#### Behobene Ablaufprobleme

- Eine reine Projektanalyse verlangt kein Git-Repository mehr und darf weder `git init` noch Dateiänderungen ausführen.
- Bestätigt der Nutzer eine konkrete Git-Initialisierung mit „ja“, führt LocalCode `git init` direkt aus, verifiziert das Repository und setzt die ursprüngliche Aufgabe ohne Wiederholung der Rückfrage fort.
- Neue Aufgaben verdrängen veraltete Git-, Build- oder ADB-Rückfragen zuverlässig.
- Build-, Android-Deployment-, Web- und Git-Initialisierungsaufgaben beginnen mit deterministischen Werkzeugaktionen statt mit geratenen Shell-Befehlen.
- Wiederholte unpassende Aktionen oder unnötige Fragen werden vom Supervisor blockiert. Bei wiederholter Modelldrift endet eine Analyse kontrolliert mit einem überprüfbaren Bericht statt am Schrittlimit.

#### Werkzeuge und Internet

- Git-Initialisierung wird über den tatsächlich erkannten Git-Pfad ausgeführt und mit `git rev-parse --is-inside-work-tree` verifiziert.
- Fehlt `.gitignore`, wird eine Visual-Studio-, Build-, Cache-, Secret- und Ökosystem-taugliche Datei angelegt.
- Unterstützte fehlende Werkzeuge werden nach Genehmigung installiert, erneut entdeckt und der ursprüngliche Aufruf wird wiederholt.
- Websuchanfragen werden an der Werkzeuggrenze normalisiert und können nicht leer ausgeführt werden.
- DuckDuckGo-Suche besitzt einen Bing-RSS-Fallback.

#### Kontextkomprimierung

- LocalCode schätzt die Kontextbelegung fortlaufend.
- Standardmäßig wird bei 68 Prozent des konfigurierten Kontextfensters komprimiert.
- Erhalten bleiben ursprüngliche Aufgabe, Nutzerentscheidungen, Projektfakten, gelesene und geänderte Dateien, Befehle, Fehler, offene Punkte und die nächste Aktion.
- Die jüngsten Nachrichten bleiben unverändert erhalten.
- Scheitert die modellgestützte Verdichtung, wird eine deterministische lokale Zusammenfassung verwendet.
- Schwelle und Anzahl unverändert beibehaltener Nachrichten sind in den Einstellungen konfigurierbar.

#### Regressionstests

- Analyse ohne Git und ohne Mutation
- direkte Ausführung einer bestätigten Git-Initialisierung
- Erstellung einer geeigneten `.gitignore`
- kontrollierte Kontextkomprimierung und Fortsetzung
- UI-Bindung aller Komprimierungseinstellungen
- bestehende Build-, Tool-Reparatur-, Abbruch-, MCP-, Datei- und Einstellungsprüfungen

---

## 4.6.0

### Werkzeugreparatur, Build-Automation und zuverlässige Fortsetzungen

#### Behoben

- Git wird in Visual Studio, Standardpfaden, `PATH`, Projektpfaden und LocalCode-Werkzeugordnern gesucht.
- Fehlendes Git kann nach Genehmigung als offizielle portable MinGit-Ausgabe installiert und sofort verwendet werden.
- ADB und Fastboot werden in allen bekannten Android-SDK-Pfaden gesucht; fehlende Platform-Tools können nach Genehmigung aus der offiziellen Google-Quelle installiert werden.
- ADB prüft alternative Installationen und Windows Plug-and-Play, statt denselben erfolglosen Befehl zu wiederholen.
- Antworten auf Rückfragen setzen nur bei tatsächlichem semantischem Bezug fort. Neue Aufgaben beseitigen alte Fortsetzungen.
- Leere Websuchanfragen werden aus der aktuellen Aufgabe ergänzt.
- Unnötige Fragen nach Git-Initialisierung oder manueller Werkzeugsuche werden blockiert, wenn LocalCode die Diagnose selbst ausführen kann.

#### Neu

- Visual-Studio-Erkennung über `vswhere.exe` und installierte Instanzverzeichnisse.
- Genehmigte automatische Installation mit Verifikation und deterministischem Wiederholen der ursprünglichen Aktion.
- `project_info` für reproduzierbare Projekterkennung.
- `build_project` für Android/Gradle, Go, Rust, Node, .NET/MSBuild, CMake und Python.
- `deploy_android` für Build, APK-Auswahl, Geräteprüfung und `adb install -r`.
- Neue Tests für Fortsetzungslogik, Werkzeug-Metadaten, sichere ZIP-Extraktion, Projektplanerkennung und ADB-Geräteparser.

#### Sicherheit

- Installationen benötigen eine eigene Genehmigung.
- Downloads werden nur aus fest codierten offiziellen Quellen beziehungsweise über WinGet bezogen.
- ZIP-Archive werden gegen Pfadtraversierung geprüft.
- Installierte Programme werden vor dem erneuten Originalaufruf verifiziert.

---

## 4.5.0

### Vollständige Produktumbenennung und Lizenzierung

#### Umbenennung

- Der Produktname lautet überall **LocalCode**.
- Die Windows-Binärdateien heißen `LocalCode.exe` und `LocalCode-Debug.exe`.
- Konfiguration, Chats, Logs und Sicherungen werden unter dem Produktordner `LocalCode` gespeichert.
- UI, API-Kennung, MCP-Clientinfo, User-Agent, Build-Skripte, Dokumentation, Diagnose und Projektvorlagen wurden umbenannt.
- Das Go-Modul heißt `localcode`.

#### Verlustfreie Migration

Beim ersten Start kopiert LocalCode vorhandene Konfigurationen, Chats und Sicherungen der unmittelbar vorherigen Produktbezeichnung in die neuen LocalCode-Verzeichnisse, sofern dort noch keine entsprechenden Dateien existieren. Die alten Daten werden nicht gelöscht.

Bereits vorhandene verwaltete `STATE.md`-Abschnitte werden beim nächsten Update auf die Marker `LOCALCODE:STATE:BEGIN/END` migriert; manuelle Inhalte außerhalb des verwalteten Bereichs bleiben erhalten.

#### Lizenz

- Neuer vollständiger Lizenztext: `LICENSE` (Apache License 2.0)
- Produkthinweise: `NOTICE`
- Hinweise zu nicht gebündelten externen Komponenten: `THIRD_PARTY_NOTICES.md`

---

## 4.4.0

### Werkzeugauflösung und belastbare Diagnose

Diese Version behebt den gemeldeten Fehler, bei dem ein vorhandenes Android-Gerät nicht zuverlässig über ADB erkannt wurde und der Agent dieselbe Rückfrage wiederholt hat.

#### Werkzeugauflösung

- Neue strukturierte Agentenaktionen `discover_tool`, `tool_inventory` und `run_tool`.
- Bekannte Programme werden nicht mehr ausschließlich über den Prozess-PATH gesucht.
- Auflösung erfolgt in dieser Reihenfolge: feste Einstellung, projektlokale Wrapper/Binaries, projektspezifische Konfiguration, Umgebungsvariablen, PATH und bekannte Installationspfade.
- Gefundene Programme werden mit absolutem Pfad ausgeführt.
- Projektlokale Verzeichnisse `bin`, `tools`, `.tools`, `scripts` und `node_modules/.bin` werden berücksichtigt.
- Für unbekannte Werkzeugnamen funktioniert die Auflösung ebenfalls über Projektpfade und PATH.
- Einstellbare feste Werkzeugpfade erlauben eine eindeutige Zuordnung, ohne globale Systemänderungen.

#### Android und ADB

- Android-SDK-Erkennung über `local.properties`, `ANDROID_HOME`, `ANDROID_SDK_ROOT` und den üblichen Windows-SDK-Pfad.
- ADB wird bevorzugt direkt aus `platform-tools` ausgeführt.
- `adb devices -l` unterscheidet `device`, `unauthorized`, `offline` und keine gelisteten Geräte.
- Bei Server-/Verbindungsproblemen erfolgt genau ein kontrollierter Reparaturversuch mit `adb start-server`; bei leerer Geräteliste zusätzlich `adb reconnect` und eine erneute Abfrage.
- Vollständiger Pfad, Argumente, Arbeitsordner, Exitcode, Laufzeit, STDOUT und STDERR bleiben sichtbar.
- Ein vorhandenes, aber nicht autorisiertes oder offline gemeldetes Gerät wird nicht fälschlich als fehlende ADB-Installation behandelt.

#### Agentenkontrolle

- Identische unmittelbar aufeinanderfolgende Werkzeugaktionen werden blockiert.
- Bereits beantwortete Rückfragen dürfen nicht erneut gestellt werden.
- Nach einem Werkzeugfehler muss der Agent Pfad, Exitcode, STDOUT und STDERR auswerten und eine andere Diagnose wählen.
- Tool- und Shellprozesse übernehmen den Laufkontext; Abbruch und Timeout beenden unter Windows den gesamten Kindprozessbaum.

#### Offizielle Hilfe

- Wenn ein Werkzeug nicht gefunden wird oder fehlschlägt, kann LocalCode automatisch nach offizieller Herstellerdokumentation suchen.
- Netzwerkzugriff und Suchanbieter bleiben über die Einstellungen steuerbar.
- Direkte offizielle Dokumentationslinks sind in den Werkzeugprofilen hinterlegt.

#### Einstellungen und Diagnose

Unter **Einstellungen > Computernutzung** befinden sich:

- automatische Werkzeugerkennung
- automatische Recherche offizieller Werkzeughilfe
- feste Werkzeugpfade als JSON
- vollständige Werkzeugprüfung
- gezielte ADB-Diagnose

`LocalCode-Debug.exe --diagnose` listet zusätzlich alle erkannten Werkzeuge, deren absolute Pfade, Versionen und ADB-Gerätestatus auf.

---

## 4.3.0

Diese Version behebt die konkret gemeldeten Laufzeitprobleme.

- Keine kurz aufblinkenden Konsolenfenster mehr bei Hintergrundbefehlen unter Windows.
- Werkzeugausgaben, Befehlsausgaben, Diffs und Fehler erscheinen vollständig im Chat und im Ausgabenbereich.
- Rückfragen werden als Fortsetzung desselben Agentenlaufs behandelt; eine Antwort wie „ja“ startet nicht wieder von vorn.
- Dauerhaft sichtbare Abbruchsteuerung im Composer und in der Kopfzeile.
- Modellaufrufe besitzen ein konfigurierbares Zeitlimit.
- Kontrollierter Abbruch mit automatischer Notfallfreigabe der Oberfläche.
- SSE-Verbindung und Status werden regelmäßig mit dem Server abgeglichen.
- Einstellungen werden vorwärtskompatibel gespeichert und dürfen auch während eines Agentenlaufs geändert werden.
- Speicherfehler zeigen eine genaue Servermeldung statt nur „Bad Request“.

Die Standardwerte sind 240 Sekunden je Modellaufruf und 300 Sekunden je Befehl.

---

## 4.0.0

### UI- und Einstellungs-Neuaufbau

#### Oberfläche

- vollständige Codex-orientierte Desktopstruktur
- Windows-artige Menüleiste mit Datei, Bearbeiten, Ansicht und Hilfe
- Projekt- und Chatbaum links
- zentraler Arbeitsverlauf und Composer
- Ausgaben- und Quelleninspektor rechts
- verschiebbare linke und rechte Splitter
- verschiebbares Terminal
- Terminal wahlweise unten oder rechts
- gespeicherte Panelgrößen und Layoutoptionen
- Edge-/Chrome-App-Modus ohne normale Browser-Tabs

#### Einstellungen

Die Einstellungsseite wurde vollständig neu aufgebaut. Sichtbare Schalter und Eingabefelder sind mit realen Config-Feldern beziehungsweise API-Aktionen verbunden:

- Berechtigungen und Vollzugriff
- Standardeditor, Agentenumgebung und Terminal-Shell
- Sprache, Statusleiste, Terminal-Docking und Geschwindigkeit
- Profil, Avatar, Theme, Farben und Fonts
- Spracheingabe
- Ollama, Kontext, Agentenschritte und Timeouts
- Personalisierung und bevorzugte Antwortsprache
- Tastaturkürzel
- MCP-Server und Verbindungstest
- Websuche und Netzwerkzugriff
- Sandbox, erlaubte Pfade und blockierte Befehle
- Hooks, Umgebungsvariablen und Git
- SSH-Verbindungen
- Worktrees
- archivierte Chats
- Konfigurationsimport und -export

#### Agentenfunktionen

- Dateien aller unterstützten Typen über Auswahl, Drag-and-drop und Zwischenablage
- Bildanalyse mit lokalem Vision-Modell
- Git, Terminal, Webrecherche, MCP und Dateiwerkzeuge
- Projekt-README, AGENTS.md und verwaltete STATE.md
- Hook-Ausführung vor und nach Aufgaben
- Windows-native oder WSL-basierte Befehlsumgebung

#### Build

`BUILD-AND-RUN.bat` führt Formatierung, Tests, `go vet`, GUI-Build, Diagnose-Build und SHA-256-Erzeugung aus.
