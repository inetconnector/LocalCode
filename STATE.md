# LocalCode project state / Projektstatus

**Version:** 6.3.0  
**Status:** Three-engine release with Aider, Claude Code, and OpenCode selection

## Deutsch

### Coding-Agent-Engines

- Einstellungen können zwischen **Aider**, **Claude Code** und **OpenCode** umschalten; LocalCode nativ bleibt als interne Werkzeugschleife verfügbar.
- Installation/Reparatur, Status, Version, Anmeldung, Testlauf, Abbruch, Ausgabe, Backup und Undo sind über eine gemeinsame Engine-Schnittstelle angebunden.
- Aider verwendet weiterhin die reproduzierbare `uv`-/Python-3.12-Installation und `ollama_chat/<modell>`.
- Claude Code verwendet unter Windows den offiziellen nativen Installer und wird über `claude -p` mit begrenzter Schrittzahl und normalisiertem Berechtigungsmodus ausgeführt.
- OpenCode wird benutzerlokal über verwaltetes Node.js/npm installiert und unterstützt `provider/modell` sowie lokales Ollama über prozessbezogene Konfiguration.
- Alte `aider_*`-Agentenaktionen bleiben kompatible Aliase der neuen generischen `engine_*`-Aktionen.

### Sicherheit und Wiederherstellung

- Vor bearbeitenden externen Läufen wird ein Projektbackup erstellt.
- Geänderte Dateien werden durch Fingerprints ermittelt; Undo schützt spätere manuelle Änderungen durch Hash-Prüfung.
- Claude `bypassPermissions` wird nicht angeboten; OpenCode `--auto` ist abschaltbar.
- Zugangsdaten bleiben bei der jeweiligen Engine und werden nicht von LocalCode gespeichert.
- Externe CLIs laufen mit ihren eigenen Berechtigungen; LocalCodes Projektgrenze ersetzt keine Betriebssystem-Sandbox.

### Verifikation

- Exakte finale Statement-Coverage: **6330/7896 = 80.167173 %**.
- Normale Tests, `go vet`, Race Detector, zufällige Testreihenfolgen, UI/API-Simulation, Übersetzungsparität und Windows-amd64-Cross-Build gehören zur Release-Prüfung.
- Der endgültige Messwert und die exakten Testzahlen stehen in `TEST-REPORT.txt` und `reports/COVERAGE-SUMMARY.txt`.

## English

### Coding-agent engines

- Settings can switch between **Aider**, **Claude Code**, and **OpenCode**; LocalCode native remains available as the internal tool loop.
- Installation/repair, status, version, authentication, test execution, cancellation, output, backup, and undo share one engine interface.
- Aider retains the reproducible `uv`/Python 3.12 setup and `ollama_chat/<model>`.
- Claude Code uses the official native Windows installer and runs through `claude -p` with bounded turns and a normalized permission mode.
- OpenCode is installed per user through managed Node.js/npm and supports `provider/model` plus local Ollama through process-scoped configuration.
- Legacy `aider_*` agent actions remain compatible aliases for the generic `engine_*` actions.

### Security and recovery

- A project backup is created before external editing runs.
- Changed files are detected through fingerprints; undo protects later manual edits with hash checks.
- Claude `bypassPermissions` is not exposed; OpenCode `--auto` can be disabled.
- Credentials remain managed by the selected engine and are not stored by LocalCode.
- External CLIs run with their own permissions; LocalCode's project boundary is not an operating-system sandbox.

### Verification

- Exact final statement coverage: **6330/7896 = 80.167173%**.
- Normal tests, `go vet`, the race detector, randomized test orders, UI/API simulation, translation parity, and Windows-amd64 cross-builds are part of release verification.
- The exact results are recorded in `TEST-REPORT.txt` and `reports/COVERAGE-SUMMARY.txt`.

## Verification boundary

The release environment can execute the complete source suite and race detector on Linux and can cross-compile Windows test and application binaries. It cannot execute `.bat` or PE files natively. `dist\REBUILD-NATIVE.txt` therefore forces a complete native Windows build before normal launch.

<!-- LOCALCODE:STATE:BEGIN -->
Managed runtime state is written here when this repository itself is selected in LocalCode.
Verwalteter Laufzeitstatus wird hier geschrieben, wenn dieses Repository selbst in LocalCode ausgewählt ist.
<!-- LOCALCODE:STATE:END -->
