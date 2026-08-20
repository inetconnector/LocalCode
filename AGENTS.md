# LocalCode repository instructions / Repository-Regeln

## Deutsch

- Lies vor Änderungen `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md` und `docs/SECURITY.md`.
- Halte `STATE.md` und `TODO.md` nach jeder materiellen Änderung vollständig und widerspruchsfrei aktuell. `STATE.md` beschreibt die gegenwärtig implementierte Realität; `TODO.md` enthält ausschließlich noch offene Arbeit, Abhängigkeiten und Abnahmekriterien. Nach Branch-/PR-/CI-/Merge-/Roadmap-/Scope-Änderungen müssen beide Dateien im selben Workstream oder unmittelbar danach aktualisiert werden.
- **`STATE.md` ist der vollständige KI-Bootstrap:** Eine neu gestartete KI ohne Chat-Historie, Memory oder Vorwissen muss allein aus `STATE.md` das Projekt vollständig genug verstehen, um sicher und korrekt weiterprogrammieren zu können. `STATE.md` muss deshalb mindestens aktuellen Master/Branch/PR/Head/CI-/Review-Stand, Produktziel, Architektur und zentrale Komponenten, Sicherheits-/Quality-Invarianten, wichtige Dateien/Entrypoints, relevante bereits implementierte Fähigkeiten, aktive Änderungen, bekannte Probleme/Fehlversuche, offene Entscheidungen, exakte nächste Schritte sowie den kanonischen Verweis auf `TODO.md` enthalten. Wenn dafür Details aus anderen Dokumenten wesentlich sind, müssen sie in `STATE.md` ausreichend zusammengefasst und die Quelldateien benannt werden; ein bloßer Verweis ohne arbeitsfähigen Kontext reicht nicht.
- **Sprachpflege ist verpflichtend:** Jede neue oder geänderte sichtbare Zeichenfolge muss gleichzeitig auf Deutsch und Englisch gepflegt werden. Alle Sprachkataloge müssen identische Schlüssel besitzen. Neue Sprachen müssen in Tests, Dokumentation, Systemerkennung und manueller Auswahl vollständig ergänzt werden.
- Standardverhalten: Windows-Anzeigesprache verwenden; Deutsch bei deutschem Windows, sonst Englisch. Die manuelle Auswahl muss diese Automatik überschreiben können.
- Bezeichne externe Programme erst nach vollständiger Werkzeugerkennung als fehlend. Dokumentiere Pfad, Exitcode, STDOUT, STDERR und durchsuchte Orte.
- Recherchiere bei unbekannter Werkzeugbedienung zuerst offizielle Herstellerdokumentation.
- Echte mehrdateilige Quellcodeänderungen müssen standardmäßig über `aider_edit` laufen. Die native `replace_text`-/`write_file`-Schleife ist nur für kleine deterministische Verwaltungsänderungen oder als ausdrücklich gewählter Fallback vorgesehen.
- Bei Aider-Änderungen müssen Vorher-Backup, Hash-Manifest, Timeout, Prozessbaum-Abbruch, vollständige Ausgabe und sichere Wiederherstellung erhalten bleiben.
- Aider-Version, CLI-Optionen und Modellpräfixe dürfen nur nach Prüfung gegen offizielle Aider-Dokumentation geändert werden.
- Führe vor Abschluss `go fmt ./...`, `go test -race -count=1 ./...`, `go vet ./...`, JavaScript-Syntaxprüfungen, den Browser-UI-Smoke-Test für sichtbare Menüs und Windows-amd64-Builds aus.
- Änderungen an Datei-, Shell-, Git-, Netzwerk- oder MCP-Zugriff müssen Genehmigungen, Sandboxgrenzen, Timeouts, Abbruch und Protokollierung berücksichtigen.
- MCP-stdio-Prozesse müssen persistent, unsichtbar und kontrolliert beendbar sein; Serveranfragen wie `roots/list` dürfen nicht ignoriert werden. Externe Standardserver dürfen nur aus offiziellen Quellen konfiguriert werden.
- Destruktive Git- und Systembefehle bleiben blockiert oder werden in ein sichtbares interaktives Terminal ausgelagert.
- Behaupte keine vollständige Codex-Parität ohne nachprüfbare Tests; dokumentiere Grenzen offen.

## English

- Read `README.md`, `STATE.md`, `TODO.md`, `docs/ARCHITECTURE.md`, and `docs/SECURITY.md` before changing code.
- Keep `STATE.md` and `TODO.md` fully current and mutually consistent after every material change. `STATE.md` describes current implemented reality; `TODO.md` contains only unfinished work, dependencies, and acceptance criteria. Branch/PR/CI/merge/roadmap/scope changes require both files to be refreshed in the same workstream or immediately afterward.
- **`STATE.md` is the complete AI bootstrap:** A newly started AI with no chat history, memory, or prior context must be able to understand the project from `STATE.md` well enough to resume implementation safely and correctly. `STATE.md` must therefore contain at least the current master/branch/PR/head/CI/review state, product objective, architecture and core components, safety/Quality invariants, important files/entrypoints, relevant implemented capabilities, active changes, known problems/failed attempts, open decisions, exact next steps, and the canonical pointer to `TODO.md`. When details from other documents are material, `STATE.md` must summarize them sufficiently and name the source files; a bare link without working context is not enough.
- **Localization maintenance is mandatory:** every new or changed user-visible string must be maintained in German and English at the same time. All language catalogs must contain identical keys. New languages must be added completely to tests, documentation, system detection, and manual selection.
- Default behavior: follow the Windows display language; use German on German Windows and English otherwise. Manual selection must override automatic detection.
- Do not declare an external tool missing before full tool discovery. Record path, exit code, STDOUT, STDERR, and searched locations.
- When tool usage is unknown, research official vendor documentation first.
- Real multi-file source changes must use `aider_edit` by default. The native `replace_text`/`write_file` loop is reserved for small deterministic administrative changes or an explicitly selected fallback.
- Aider changes must preserve pre-run backup, hash manifest, timeout, process-tree cancellation, complete output, and guarded restore.
- Aider version, CLI options, and model prefixes may only be changed after verification against official Aider documentation.
- Before completion, run `go fmt ./...`, `go test -race -count=1 ./...`, `go vet ./...`, JavaScript syntax checks, the browser UI smoke test for visible menus, and Windows-amd64 builds.
- Changes to file, shell, Git, network, or MCP access must preserve approvals, sandbox boundaries, timeouts, cancellation, and logging.
- MCP stdio processes must remain persistent, hidden, and controllably terminable; server requests such as `roots/list` must not be ignored. External default servers may only be configured from official sources.
- Destructive Git and system commands remain blocked or are delegated to a visible interactive terminal.
- Do not claim full Codex parity without verifiable tests; document limitations honestly.