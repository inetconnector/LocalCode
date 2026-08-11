# LocalCode project state / Projektstatus

**Version:** 6.4.3  
**Status:** Startup setup fixed; progress splash, selectable coding engines, and native-agent completion guards

## Deutsch

### Start und automatische Einrichtung

- Ein kompaktes, token-geschütztes Loopback-Startfenster zeigt Ollama-Prüfung, Installation, Modelldownload und Engine-Einrichtung.
- Setup-Downloads sind vom Agenten-/Web-Netzwerkzugriff getrennt; Schema 9 wird sicher auf Schema 10 migriert.
- Bei Fehlern sind Wiederholen, Log-Ordner, eingeschränkter Start und Beenden verfügbar.
- Nur Modelle der aktiven Engine werden automatisch geladen.

### Coding-Agent-Engines

- Einstellungen können zwischen **Aider**, **Claude Code** und **OpenCode** umschalten; LocalCode nativ bleibt als interne Werkzeugschleife verfügbar.
- Installation/Reparatur, Status, Version, Anmeldung, Testlauf, Abbruch, Ausgabe, Backup und Undo sind über eine gemeinsame Engine-Schnittstelle angebunden.
- Aider verwendet weiterhin die reproduzierbare `uv`-/Python-3.12-Installation und `ollama_chat/<modell>`.
- Claude Code verwendet unter Windows den offiziellen nativen Installer und wird über `claude -p` mit begrenzter Schrittzahl und normalisiertem Berechtigungsmodus ausgeführt.
- OpenCode wird benutzerlokal über verwaltetes Node.js/npm installiert und unterstützt `provider/modell` sowie lokales Ollama über prozessbezogene Konfiguration.
- Alte `aider_*`-Agentenaktionen bleiben kompatible Aliase der neuen generischen `engine_*`-Aktionen.

### Native-Agent-Reparatur

- Nutzerentscheidung für diese Wartung: keine Symptombehandlung; Ursachen müssen behoben und nachweisbar getestet werden.
- Ursache des Pac-Man-/HTML-Fehlers: Die native Werkzeugschleife ließ `write_file` ohne `content` zu, konnte Tool-Call-artige `arguments` nicht vollständig normalisieren, leitete bei aktivem LocalCode-nativ-Modus zu schwach von `engine_edit` weg und akzeptierte `finish`, obwohl Dateien leer, Platzhalter oder ungeprüft waren.
- Reparatur: `write_file` verlangt jetzt Pfad und vollständigen nicht-leeren Inhalt im Schema und in der Go-Validierung. Fehlende Felder aus `arguments` werden übernommen. Wiederholt unvollständige Schreibaktionen lösen eine fokussierte Inhaltserzeugung aus, statt leere Dateien zu schreiben.
- Reparatur: Der Supervisor-Hinweis unterscheidet externe Engines von **LocalCode nativ**. Native Läufe werden zu direkten Dateiwerkzeugen geführt; `engine_edit`-Fehler erhalten eine konkrete Wiederherstellungsanweisung.
- Reparatur: Editieraufgaben bekommen vor `finish` eine Abschlussprüfung. Sie blockiert fehlende oder leere erwähnte Dateien, offensichtliche Platzhalter, nicht erkennbare angeforderte Funktionsmarker und fehlende echte Prüfung nach der letzten Änderung, wenn die Aufgabe Tests, Syntax, Build, Lint oder Funktionsprüfung verlangt.
- Reparatur: `STATE.md` mit LocalCode-Markern darf über `write_file`/`replace_text` nicht mehr so überschrieben werden, dass der verwaltete Handoff-Abschnitt verloren geht.
- Reparatur nach dem echten Pac-Man-Lauf: Die Werkzeugerkennung fand vorhandenes WinGet-Node.js nicht, weil für `node` Standardpfade ohne `.exe` erzeugt wurden und WinGet-Paketpfade unter `LOCALAPPDATA\Microsoft\WinGet\Packages` fehlten. `node`, `npm` und `npx` werden jetzt auch aus benutzerlokalen Node.js-Installationen und OpenJS-WinGet-Paketen erkannt.
- Reparatur nach dem echten Pac-Man-Lauf: Der Abschlusswächter interpretierte `Node.js` aus „Syntaxprüfung mit Node.js“ fälschlich als verlangte Projektdatei. Bekannte Technologietokens wie `Node.js`, `Three.js`, `D3.js` und ähnliche werden nicht mehr als Projektdateien aufgenommen.
- Allgemeine Reparatur nach Nutzerfeedback: Die Lösung darf kein besserer Einmal-Prompt für Pac-Man sein. LocalCode erzeugt jetzt bei jeder Herstellungs-/Änderungsaufgabe interne, sprach- und domänenneutrale Qualitätsanforderungen: echte Code-/Artefaktlogik statt README-Behauptungen, verdrahtete UI-Kontrollen, konsistente Zustände, verifizierte Dateioperationen, konkrete darstellbare Bild-/Diagrammassets und passende Prüfungen je Sprache/Projekt. `copy`/`move`/`rename` sowie Bild-/Zeichenaufträge werden als umsetzende Aufgaben klassifiziert.
- Allgemeine Abschlussprüfung: `node -c` ist nur noch ein Spezialfall unter vielen. Erkannte Verifikationen umfassen unter anderem Go, Python, Rust/Cargo, Java/Javac, Maven, Gradle, .NET, TypeScript, PHP, Ruby, Shell, C/C++ und CMake/Make. Slash-getrennte Dateiaufzählungen wie `index.html/styles.css/README.md` werden als einzelne Dateien behandelt, echte Pfade wie `src/agent.go` bleiben erhalten.
- Allgemeine Werkzeugdisziplin: Globale Installer wie `npm install -g`, `pip install --user`, `cargo install`, `go install`, `winget install`, `choco install` und `scoop install` werden bei normalen Aufgaben als unpassende Aktionen blockiert, wenn die Nutzeraufgabe keine globale Installation verlangt. Nach fehlgeschlagenen Prüfungen soll der Agent vorhandene, projektlokale, verwaltete oder nicht-invasive Prüfungen wählen.
- Robustheit der Werkzeugschleife: Wenn das Modell nach einer Abschlussblockade erneut eine ungültige Aktion liefert, z. B. `write_file` ohne Inhalt, bricht LocalCode den Lauf nicht sofort ab. Bis zu drei Mal wird eine kompakte Reparaturanweisung in den Kontext gegeben; erst wiederholte ungültige Aktionen beenden den Lauf.
- Abschlussreparatur: Guard-Fehler werden jetzt mit einer allgemeinen Priorisierung zurückgespielt. Fehlende Laufzeitlogik muss zuerst in Quellcode-/Konfigurations-/Asset-Dateien behoben werden; UI-Kontrollen müssen sichtbares Element, Event-Handler und Zustandsänderung erhalten; Dokumentation und kosmetische Änderungen zählen nicht als Reparatur fehlender Funktion.
- Startrobustheit: Ein stale `last_model` wie `test-model` darf den Serverstart nicht mehr vor der UI blockieren. Wenn bereits Modelle vorhanden sind, wird beim Bootstrap das konfigurierte Standardmodell sichergestellt und ein fehlendes altes LastModel nicht automatisch gepullt; explizit konfigurierte Engine-Modelle bleiben weiterhin Pflichtmodelle.
- Kontext-Performance für lokale Modelle: Toolresultate werden im sichtbaren Ausgabenbereich weiterhin ausführlich gehalten, aber für den nächsten Modellaufruf anhand der Kontextgröße gekürzt. Nach Kontextkomprimierung wird die Zahl unveränderter Recent-Messages adaptiv reduziert; sehr große neue Nachrichten werden gekürzt und notfalls werden nur wenige jüngste Schritte plus faktischer Arbeitszustand behalten. Das verhindert, dass lokale Modelle nach großen Ausgaben oder mehreren Reparaturrunden ihr Kontextfenster überfüllen.
- Domänenprüfer: Interaktive Aufgaben blockieren jetzt fehlende Button-, Tastatur-, Canvas-, Status-, Zähler-, Lebens-/Health- und Reset-Verdrahtung. Visuelle Aufgaben blockieren fehlende konkrete Artefakte wie SVG/Canvas/Bilddateien. Pac-Man bleibt nur ein zusätzlicher Beispiel-Domänenprüfer mit Maze-, Pellet-, Gegner-, Zustands- und Restart-Regeln.
- Grenze: Diese Schutzschicht verbessert die Verlässlichkeit lokaler Ollama-Modelle, ersetzt aber keine nachgewiesene Codex-Parität und garantiert keine perfekte semantische Vollständigkeit jeder Aufgabe.
- Verifikation am 2026-08-11: `.tools\go\bin\go.exe fmt ./...`; fokussierte Go-Tests für Aktionsvalidierung, Schreibreparatur, allgemeine und Pac-Man-spezifische Abschlusswächter, Kontextkomprimierungs-Budgetierung, Toolresultat-Kürzung, sprachübergreifende Verifikationserkennung, Datei-/Bild-Aufgabenklassifikation, Slash-Dateiaufzählungen, `STATE.md`-Markersicherheit, Übersetzungsmeldungen, WinGet-Node-Erkennung und `Node.js`-Technologietoken; `scripts\build.ps1` vollständig erfolgreich mit isolierten Tests, `go vet`, zufälliger Testreihenfolge und Windows-amd64-GUI-/Debug-Builds; JavaScript-Syntaxprüfung für `src/static/i18n.js` und beide Inline-Skripte in `src/static/index.html`; isolierter UI-Smoke gegen `LocalCode-Debug.exe` auf `http://127.0.0.1:32145` mit `/api/ping`, `/api/status`, HTML-Menümarkern und Browser-Assertion.
- Nicht abgeschlossen am 2026-08-11: `go test -race -count=1 ./...` ist auf diesem Windows-System blockiert, weil Go für `-race` CGO verlangt und `gcc` nicht im `PATH` verfügbar ist. Ergebnis: erst `go: -race requires cgo`, danach mit `CGO_ENABLED=1` `cgo: C compiler "gcc" not found`.
- Lokale Benutzerumgebung am 2026-08-11: `C:\Users\frede\AppData\Roaming\LocalCode\config.json` enthielt `last_model: "test-model"` und blockierte den normalen Start durch einen fehlgeschlagenen Pull. Vor der Korrektur wurde `config.json.bak-20260811-212424` angelegt; `last_model` ist jetzt `qwen2.5-coder:14b`. Die dauerhafte Benutzer-Variable `OLLAMA_HOST` steht bereits auf `127.0.0.1:11434`; nur die laufende Codex-Prozessumgebung hatte noch `172.31.112.1:11434` geerbt. Die geprüften Desktop-Starter `local-codex.bat` und `open-llama-lan-8080.ps1` setzen keinen falschen Ollama-Host. Normaler Start-Smoke mit echter Benutzerconfig gelang danach am 2026-08-11: LocalCode `6.4.3-debug` startete auf `http://127.0.0.1:32145` mit Ollama `http://127.0.0.1:11434`.

### Sicherheit und Wiederherstellung

- Vor bearbeitenden externen Läufen wird ein Projektbackup erstellt.
- Geänderte Dateien werden durch Fingerprints ermittelt; Undo schützt spätere manuelle Änderungen durch Hash-Prüfung.
- Claude `bypassPermissions` wird nicht angeboten; OpenCode `--auto` ist abschaltbar.
- Zugangsdaten bleiben bei der jeweiligen Engine und werden nicht von LocalCode gespeichert.
- Externe CLIs laufen mit ihren eigenen Berechtigungen; LocalCodes Projektgrenze ersetzt keine Betriebssystem-Sandbox.

### Verifikation

- Exakte finale Statement-Coverage: **6576/8201 = 80.185343 %**.
- Normale Tests, `go vet`, Race Detector, zufällige Testreihenfolgen, UI/API-Simulation, Übersetzungsparität und Windows-amd64-Cross-Build gehören zur Release-Prüfung.
- Der endgültige Messwert und die exakten Testzahlen stehen in `TEST-REPORT.txt` und `reports/COVERAGE-SUMMARY.txt`.

## English

### Startup and automatic setup

- A compact, token-protected loopback startup window shows Ollama checks, installation, model downloads, and engine setup.
- Setup downloads are separate from agent/web network access; schema 9 migrates safely to schema 10.
- Failure actions include retry, log-folder access, limited mode, and exit.
- Only models required by the active engine are pulled automatically.

### Coding-agent engines

- Settings can switch between **Aider**, **Claude Code**, and **OpenCode**; LocalCode native remains available as the internal tool loop.
- Installation/repair, status, version, authentication, test execution, cancellation, output, backup, and undo share one engine interface.
- Aider retains the reproducible `uv`/Python 3.12 setup and `ollama_chat/<model>`.
- Claude Code uses the official native Windows installer and runs through `claude -p` with bounded turns and a normalized permission mode.
- OpenCode is installed per user through managed Node.js/npm and supports `provider/model` plus local Ollama through process-scoped configuration.
- Legacy `aider_*` agent actions remain compatible aliases for the generic `engine_*` actions.

### Native-agent repair

- User decision for this maintenance pass: no symptom treatment; root causes must be fixed and verified with tests.
- Root cause of the Pac-Man/HTML failure: the native tool loop allowed `write_file` without `content`, did not fully normalize tool-call-like `arguments`, did not steer strongly enough away from `engine_edit` in LocalCode-native mode, and accepted `finish` even when files were empty, placeholders, or unverified.
- Fix: `write_file` now requires a path and complete non-empty content in both the schema and Go validation. Missing fields are recovered from `arguments`. Repeated incomplete write actions trigger focused content generation instead of writing empty files.
- Fix: the supervisor hint distinguishes external engines from **LocalCode native**. Native runs are directed to direct file tools; `engine_edit` failures receive a concrete recovery directive.
- Fix: editing tasks pass through a completion guard before `finish`. It blocks missing or empty mentioned files, obvious placeholders, missing requested feature markers, and a missing real check after the last change when the task asks for tests, syntax, build, lint, or functional verification.
- Fix: `STATE.md` files with LocalCode markers can no longer be overwritten through `write_file`/`replace_text` in a way that loses the managed handoff section.
- Fix after the real Pac-Man run: tool discovery did not find existing WinGet Node.js because `node` standard paths were generated without `.exe` and WinGet package paths under `LOCALAPPDATA\Microsoft\WinGet\Packages` were missing. `node`, `npm`, and `npx` are now discovered from per-user Node.js installations and OpenJS WinGet packages.
- Fix after the real Pac-Man run: the completion guard treated `Node.js` from “syntax check with Node.js” as a requested project file. Known technology tokens such as `Node.js`, `Three.js`, `D3.js`, and similar names are no longer added as project files.
- General fix after user feedback: the solution must not be a better one-off Pac-Man prompt. For every creation/modification task, LocalCode now injects internal, language- and domain-neutral quality requirements: real code/artifact logic instead of README claims, wired UI controls, consistent states, verified file operations, concrete renderable image/diagram assets, and checks appropriate to the language/project. `copy`/`move`/`rename` and image/drawing requests are classified as implementation tasks.
- General completion checking: `node -c` is now only one special case among many. Recognized verification commands include Go, Python, Rust/Cargo, Java/Javac, Maven, Gradle, .NET, TypeScript, PHP, Ruby, shell, C/C++, and CMake/Make. Slash-separated file lists such as `index.html/styles.css/README.md` are split into individual files while real paths such as `src/agent.go` are preserved.
- General tool discipline: global installers such as `npm install -g`, `pip install --user`, `cargo install`, `go install`, `winget install`, `choco install`, and `scoop install` are blocked as unsuitable actions during ordinary tasks unless the user explicitly requested a global install. After failed checks, the agent should choose existing, project-local, managed, or non-invasive verification paths.
- Tool-loop robustness: if the model returns another invalid action after a completion block, for example `write_file` without content, LocalCode no longer aborts immediately. It injects a compact repair hint up to three times; only repeated invalid actions end the run.
- Completion repair: guard failures are now fed back with general prioritization. Missing runtime logic must be fixed first in source/config/asset files; UI controls must include visible elements, event handlers, and state changes; documentation and cosmetic edits do not count as fixing missing functionality.
- Startup robustness: a stale `last_model` such as `test-model` can no longer block server startup before the UI is reachable. When models already exist, bootstrap ensures the configured default model and does not automatically pull a missing stale LastModel; explicitly configured engine models remain required.
- Context performance for local models: tool results remain detailed in the visible output panel, but are shortened for the next model call according to the configured context size. After context compaction, unchanged recent messages are reduced adaptively; very large newer messages are truncated and, if needed, only a few latest steps plus the factual working state are kept. This prevents local models from overfilling their context window after large outputs or repeated repair rounds.
- Domain checkers: interactive tasks now block missing button, keyboard, canvas, status, counter, lives/health, and reset wiring. Visual tasks block missing concrete artifacts such as SVG/canvas/image files. Pac-Man remains only an additional example-domain checker with maze, pellet, enemy, state, and restart rules.
- Boundary: this guardrail improves reliability for local Ollama models, but it is not proof of Codex parity and does not guarantee perfect semantic completeness for every task.
- Verification on 2026-08-11: `.tools\go\bin\go.exe fmt ./...`; focused Go tests for action validation, write repair, general and Pac-Man-specific completion guards, context-compaction budgeting, tool-result truncation, language-agnostic verification detection, file/image task classification, slash-separated file lists, `STATE.md` marker safety, translated UI messages, WinGet Node discovery, and `Node.js` technology-token handling; full `scripts\build.ps1` succeeded with isolated tests, `go vet`, randomized test order, and Windows-amd64 GUI/debug builds; JavaScript syntax checks for `src/static/i18n.js` and both inline scripts in `src/static/index.html`; isolated UI smoke against `LocalCode-Debug.exe` at `http://127.0.0.1:32145` with `/api/ping`, `/api/status`, HTML menu markers, and browser assertion.
- Not completed on 2026-08-11: `go test -race -count=1 ./...` is blocked on this Windows system because Go requires CGO for `-race` and `gcc` is not available in `PATH`. Result: first `go: -race requires cgo`, then with `CGO_ENABLED=1` `cgo: C compiler "gcc" not found`.
- Local user environment on 2026-08-11: `C:\Users\frede\AppData\Roaming\LocalCode\config.json` contained `last_model: "test-model"` and blocked normal startup through a failed pull. Backup `config.json.bak-20260811-212424` was created before correction; `last_model` is now `qwen2.5-coder:14b`. The persistent user-level `OLLAMA_HOST` already points to `127.0.0.1:11434`; only the current Codex process environment had inherited stale `172.31.112.1:11434`. Checked desktop starters `local-codex.bat` and `open-llama-lan-8080.ps1` do not set a wrong Ollama host. Normal startup smoke with the real user config then passed on 2026-08-11: LocalCode `6.4.3-debug` started at `http://127.0.0.1:32145` with Ollama `http://127.0.0.1:11434`.

### Security and recovery

- A project backup is created before external editing runs.
- Changed files are detected through fingerprints; undo protects later manual edits with hash checks.
- Claude `bypassPermissions` is not exposed; OpenCode `--auto` can be disabled.
- Credentials remain managed by the selected engine and are not stored by LocalCode.
- External CLIs run with their own permissions; LocalCode's project boundary is not an operating-system sandbox.

### Verification

- Exact final statement coverage: **6576/8201 = 80.185343%**.
- Normal tests, `go vet`, the race detector, randomized test orders, UI/API simulation, translation parity, and Windows-amd64 cross-builds are part of release verification.
- The exact results are recorded in `TEST-REPORT.txt` and `reports/COVERAGE-SUMMARY.txt`.

## Verification boundary

The release environment can execute the complete source suite and race detector on Linux and can cross-compile Windows test and application binaries. It cannot execute `.bat` or PE files natively. `dist\REBUILD-NATIVE.txt` therefore forces a complete native Windows build before normal launch.

<!-- LOCALCODE:STATE:BEGIN -->
Managed runtime state is written here when this repository itself is selected in LocalCode.
Verwalteter Laufzeitstatus wird hier geschrieben, wenn dieses Repository selbst in LocalCode ausgewählt ist.
<!-- LOCALCODE:STATE:END -->


## Version 6.4.2 Windows-Testfixes

- Native Buildtests are fully isolated without shadowing test-local path overrides.
- Canonical and tolerant Windows file-URI handling for MCP resources.
- Engine probes work before the LocalCode app-data directory exists.
- 191 tests; statement coverage 80.038996 %.

## Version 6.4.3 Windows-Testisolation / Windows test isolation

- Engine-Setup-Tests deaktivieren den dedizierten Setup-Download-Schalter und können dadurch niemals eine reale Claude-Code-Installation auslösen.
- Der Ordnerauswahldialog ist pro Serverinstanz injizierbar; HTTP-Tests öffnen unter Windows keine echte GUI mehr und prüfen Fehler-, Abbruch- und Erfolgsfall deterministisch.
- Shell-Befehle in plattformübergreifenden Tests verwenden `echo` statt des unter PowerShell nicht vorhandenen `printf`.
- 191 Testfunktionen; exakte Statement-Coverage: 6569/8206 = 80.051182 %.
