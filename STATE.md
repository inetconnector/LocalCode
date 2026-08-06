# LocalCode project state / Projektstatus

**Version:** 6.1.0  
**Status:** source release candidate; Windows-native verification required after extraction

## Deutsch

### Abgeschlossene Änderungen in 6.1.0

- Alle vom Windows-Build gemeldeten Testfehler auf konkrete Ursachen zurückgeführt und behoben.
- Windows-Tests von realem Benutzerprofil, echtem `PATH`, installierten Android-Werkzeugen und `C:\Users\...\Projekte` isoliert.
- Portable Verzeichnis-Overrides (`LOCALCODE_*_HOME` und XDG) plattformübergreifend umgesetzt.
- Aider-Abbruch beendet unter Windows zuerst den gesamten Prozessbaum und wartet kontrolliert auf die Freigabe.
- Persistente MCP-stdio-Sitzungen werden beim Schließen vollständig beendet und geerntet; temporäre Verzeichnisse bleiben nicht gesperrt.
- Projektwurzelprüfung akzeptiert legitime temporäre Windows-Projekte und sperrt weiterhin LocalCodes eigene Datenverzeichnisse.
- Datei-Sandbox gegen Ausbruch über Symlinks und NTFS-Junctions gehärtet, auch bei noch nicht vorhandenen Zieldateien.
- Webabruf gegen DNS-Rebinding gehärtet: geprüft und verbunden wird dieselbe öffentliche IP.
- Automatischer Start-Bootstrap für Ollama, konfigurierte Modelle und Aider ergänzt.
- Fehlendes Ollama wird über den offiziellen Windows-Installer installiert; die Authenticode-Signatur wird vor Ausführung geprüft.
- Fehlende Modelle werden automatisch über die lokale Ollama-API geladen; auf einem frischen System standardmäßig nur `qwen2.5-coder:14b` sowie explizit konfigurierte Aider-Modelle.
- Fehlendes oder abweichendes Aider wird über das SHA-256-geprüfte portable `uv`, isoliertes Python 3.12 und `aider-chat==0.86.2` installiert oder repariert.
- Einstellungen für Ollama-Auto-Installation, Modell-Auto-Pull und Standardmodell in DE/EN ergänzt.
- Build-Skript akzeptiert nur noch unterstützte Go-1.25+-Werkzeugketten und lädt andernfalls die aktuelle stabile Go-Version mit offizieller SHA-256-Prüfung.
- `START.bat` baut automatisch, wenn die EXE fehlt oder der Quellcode neuer ist.
- Neue Regressionsprüfungen für Modell-Pull, Schema-Migration, portable Verzeichnisse, Sandbox und öffentliche Netzwerkziele.

### Verifikation in dieser Umgebung

- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go test -shuffle=on -count=3 ./...`
- `go vet ./...`
- 122 Go-Testfunktionen, 46,1 % Statement-Coverage
- Windows-amd64-Testkompilierung sowie GUI-/Diagnose-Cross-Build
- JavaScript-Syntax, 198 eindeutige HTML-IDs, 493/493 DE/EN-Schlüssel
- Browser-UI-E2E mit 33 simulierten API-Aufrufen

### Offene externe Verifikationsgrenze

Der Build-Container kann Windows-Programme nicht nativ ausführen und konnte die aktuelle Linux-Go-1.26.5-Binärdistribution nicht importieren. Deshalb werden keine mit Go 1.23.2 erzeugten EXE-Dateien ausgeliefert. `START.bat` erstellt sie auf dem Zielrechner mit dessen unterstützter Go-Version; beim gemeldeten Zielsystem ist Go 1.26.5 vorhanden. Ein echter Ollama-/Modell-/Aider-Download bleibt vom Netzwerk und Windows-Zielrechner abhängig.

## English

### Completed changes in 6.1.0

- Traced every reported Windows build-test failure to a concrete cause and fixed it.
- Isolated Windows tests from the real user profile, real `PATH`, installed Android tools, and `C:\Users\...\Projects`.
- Made portable directory overrides (`LOCALCODE_*_HOME` and XDG) cross-platform.
- Windows Aider cancellation now terminates the complete process tree first and waits for release.
- Persistent MCP stdio sessions fully terminate and reap their subprocesses so temporary directories are not left locked.
- Project-root validation accepts legitimate Windows temporary projects while continuing to reject LocalCode's own data directories.
- Hardened the file sandbox against symlink and NTFS-junction escapes, including destinations that do not exist yet.
- Hardened web fetches against DNS rebinding by validating and dialing the same public IP.
- Added automatic startup bootstrap for Ollama, configured models, and Aider.
- Missing Ollama is installed with the official Windows installer after Authenticode validation.
- Missing models are pulled through the local Ollama API; a fresh system defaults to only `qwen2.5-coder:14b` plus explicitly configured Aider models.
- Missing or mismatched Aider is installed or repaired through SHA-256-verified portable `uv`, isolated Python 3.12, and `aider-chat==0.86.2`.
- Added German and English settings for Ollama auto-install, model auto-pull, and the fresh-install default model.
- The build script now accepts only supported Go 1.25+ toolchains and otherwise downloads the current stable Go release with official SHA-256 verification.
- `START.bat` rebuilds automatically when the executable is missing or source files are newer.
- Added regression checks for model pulling, schema migration, portable directories, sandboxing, and public network targets.

### Verification in this environment

- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go test -shuffle=on -count=3 ./...`
- `go vet ./...`
- 122 Go test functions, 46.1% statement coverage
- Windows-amd64 test compilation and GUI/diagnostic cross-builds
- JavaScript syntax, 198 unique HTML IDs, 493/493 DE/EN keys
- Browser UI E2E with 33 mocked API requests

### Remaining external verification boundary

The build container cannot execute Windows programs natively and could not import the current Linux Go 1.26.5 binary distribution. Therefore no executable compiled with Go 1.23.2 is shipped. `START.bat` builds the executables on the target computer using its supported Go toolchain; the reported target already has Go 1.26.5. Real Ollama/model/Aider downloads still depend on the target Windows computer and network.

<!-- LOCALCODE:STATE:BEGIN -->
Managed runtime state is written here when this repository itself is selected in LocalCode.
Verwalteter Laufzeitstatus wird hier geschrieben, wenn dieses Repository selbst in LocalCode ausgewählt ist.
<!-- LOCALCODE:STATE:END -->
