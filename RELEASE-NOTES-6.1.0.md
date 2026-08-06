# LocalCode 6.1.0 – Windows reliability and automatic runtime setup

[Deutsch](#deutsch) · [English](#english)

## Deutsch

### Behobene Windows-Fehler

| Gemeldeter Fehler | Ursache | Korrektur |
|---|---|---|
| Aider-Abbruch dauerte etwa zehn Sekunden; TempDir gesperrt | `exec.CommandContext` beendete den CMD-Wrapper, bevor der vollständige Kindprozessbaum zuverlässig beendet wurde | eigene Prozessgruppe, `taskkill /T /F`, kontrolliertes Warten auf `Cmd.Wait` |
| Legacy-Konfiguration wurde im Test nicht migriert | XDG-Testpfade wurden unter Windows ignoriert | plattformübergreifende `LOCALCODE_*_HOME`-/XDG-Unterstützung |
| MCP-Test hinterließ gesperrte Dateien | Session-Close wartete nicht auf die tatsächliche Prozessfreigabe | separates `processDone` und kontrolliertes Reaping |
| temporäre Projektwurzel sprang auf den echten Projektordner zurück | die Sicherheitsprüfung verwarf pauschal alles unter `LOCALAPPDATA` | nur LocalCodes konkrete verwaltete Datenordner werden abgelehnt |
| Projektkatalog-/Alias-/Kontexttests sahen reale Projekte | Folge der falschen Wurzelrücksetzung | vollständige Testisolation und korrekte temporäre Root-Anwendung |
| ADB-Version fehlte | Test schrieb Batch-Inhalt in eine Datei mit `.exe`-Endung | echter kleiner Windows-Testhelfer wird mit Go erzeugt |
| Missing-tool-Tests fanden vorhandenes ADB | Tests übernahmen reale `PATH`-/SDK-Variablen | isoliertes Profil, leeres Test-PATH und leere Android-Variablen |

### Automatische Einrichtung

- Ollama wird beim Start gesucht und gestartet.
- Fehlt Ollama unter Windows, lädt LocalCode `OllamaSetup.exe` ausschließlich von `ollama.com`, verlangt eine gültige Authenticode-Signatur und installiert unbeaufsichtigt für den aktuellen Benutzer.
- Fehlende konfigurierte Modelle werden automatisch geladen. Auf einem frischen System wird standardmäßig `qwen2.5-coder:14b` verwendet; explizite Aider-Haupt-, Architect- und Editor-Modelle werden zusätzlich berücksichtigt.
- Aider wird auf Version 0.86.2 geprüft und bei Bedarf automatisch über ein SHA-256-geprüftes portables `uv` und isoliertes Python 3.12 installiert oder repariert.
- Vorhandene Installationen und Modelle werden nicht unnötig ersetzt.
- Die Optionen sind in den Einstellungen auf Deutsch und Englisch sichtbar.

### Sicherheitsverbesserungen

- Sandboxprüfung löst Symlinks und NTFS-Junctions auf, bevor ein Pfad freigegeben wird.
- DNS-Rebinding kann die Webabrufprüfung nicht mehr durch eine zweite Namensauflösung umgehen.
- Externe Websuchanbieter verwenden ebenfalls den öffentlichen-IP-Dialer.
- Go-Builds verlangen eine noch unterstützte Go-1.25+-Linie; andernfalls installiert das Skript die aktuelle stabile Version aus der offiziellen Release-Liste mit SHA-256-Prüfung.

### Auslieferung

Das ZIP enthält bewusst keine mit der im Container vorhandenen alten Go-1.23.2-Werkzeugkette erstellten EXE-Dateien. `START.bat` baut sie automatisch auf dem Windows-Zielrechner. Damit verwendet der gemeldete Rechner seine vorhandene Go-Version 1.26.5.

## English

### Fixed Windows failures

| Reported failure | Root cause | Fix |
|---|---|---|
| Aider cancellation took about ten seconds and locked TempDir | `exec.CommandContext` terminated the CMD wrapper before the complete child tree was reliably stopped | dedicated process group, `taskkill /T /F`, controlled `Cmd.Wait` reaping |
| legacy configuration was not migrated in the test | Windows ignored XDG test paths | cross-platform `LOCALCODE_*_HOME`/XDG support |
| MCP test left locked files | session close did not wait for actual process release | separate `processDone` and controlled reaping |
| temporary project root reverted to the real project directory | safety logic rejected everything below `LOCALAPPDATA` | only LocalCode's concrete managed data directories are rejected |
| catalog/alias/context tests exposed real projects | consequence of the incorrect root reset | full test isolation and correct temporary-root application |
| ADB version was empty | the test wrote batch text into a file named `.exe` | a real small Windows helper executable is built with Go |
| missing-tool tests found installed ADB | tests inherited real `PATH` and SDK variables | isolated profile, empty test PATH, and cleared Android variables |

### Automatic setup

- Ollama is discovered and started at application startup.
- If missing on Windows, LocalCode downloads `OllamaSetup.exe` only from `ollama.com`, requires a valid Authenticode signature, and installs it unattended for the current user.
- Missing configured models are pulled automatically. A fresh system defaults to `qwen2.5-coder:14b`; explicit Aider main, architect, and editor models are included.
- Aider is verified at version 0.86.2 and automatically installed or repaired through SHA-256-verified portable `uv` and isolated Python 3.12.
- Existing installations and models are reused.
- All options are visible in German and English settings.

### Security improvements

- Sandbox validation resolves symlinks and NTFS junctions before granting access.
- DNS rebinding can no longer bypass web-fetch validation through a second hostname lookup.
- External web-search providers use the same public-IP dialer.
- Builds require a still-supported Go 1.25+ line; otherwise the script installs the current stable release from the official feed with SHA-256 verification.

### Distribution

The ZIP intentionally excludes executables built with the container's obsolete Go 1.23.2 toolchain. `START.bat` builds them automatically on the Windows target. The reported target therefore uses its installed Go 1.26.5.
