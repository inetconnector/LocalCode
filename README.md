# LocalCode 4.6.0

LocalCode ist ein eigenständiger lokaler Windows-Coding-Agent für Ollama. Die Anwendung bietet eine desktopartige Projekt- und Chatoberfläche, persistente Chats, Datei- und Bildanhänge, Git, Befehlsausführung, Webrecherche, MCP, Genehmigungen und eine zentrale Einstellungsseite.

## Schnellstart

1. ZIP vollständig entpacken.
2. `BUILD-AND-RUN.bat` doppelklicken.
3. Beim ersten Build lädt das Skript bei Bedarf eine portable offizielle Go-Version.
4. LocalCode öffnet sich unter Windows bevorzugt als Edge-/Chrome-App-Fenster ohne Browser-Tabs und Adressleiste.



## Umbenennung und Datenmigration

Die Anwendung heißt vollständig **LocalCode**. Programmdateien, UI, API-Kennung, Konfigurationsordner, Chats, Logs, Sicherungen, MCP-Clientinfo und Build-Artefakte verwenden diesen Namen.

Beim ersten Start übernimmt LocalCode vorhandene Konfigurationen, Chatverläufe und Sicherungen der vorherigen Produktbezeichnung automatisch, falls im neuen LocalCode-Datenordner noch keine entsprechenden Dateien liegen. Die alten Dateien werden aus Sicherheitsgründen nicht gelöscht. Verwaltete ältere `STATE.md`-Marker werden beim nächsten Statusupdate in die neuen `LOCALCODE:STATE:BEGIN/END`-Marker überführt, ohne manuelle Inhalte anzutasten.

## Werkzeugerkennung, Installation und Projekt-Automation in 4.6.0

- Externe Entwicklungswerkzeuge werden über Projekt-Wrapper, feste Pfade, `PATH`, Android-SDK-Verzeichnisse, Umgebungsvariablen, Visual-Studio-Installationen und `vswhere.exe` gesucht.
- Visual-Studio-Bestandteile wie MSBuild, das gebündelte Git, CMake, Ninja, NuGet und `devenv.exe` werden auch dann gefunden, wenn sie nicht im globalen `PATH` stehen.
- Für fehlendes Git installiert LocalCode nach ausdrücklicher Genehmigung eine offizielle portable MinGit-Ausgabe benutzerlokal. Für fehlendes ADB/Fastboot werden nach Genehmigung die offiziellen Android SDK Platform-Tools benutzerlokal installiert.
- Für Java nutzt LocalCode nach Genehmigung die offizielle Microsoft-OpenJDK-Paketierung, für .NET das offizielle benutzerlokale `dotnet-install.ps1` und für MSBuild den offiziellen Visual-Studio-Build-Tools-Bootstrapper mit dem MSBuild-Workload. Weitere bekannte Werkzeuge können über WinGet installiert werden. Jede Installation wird separat angezeigt, muss bestätigt und anschließend verifiziert werden.
- Nach einer erfolgreichen Installation wird exakt die ursprüngliche Aktion automatisch erneut ausgeführt. Bis zu vier nacheinander fehlende, unterstützte Werkzeuge können innerhalb eines Vorgangs repariert werden.
- `project_info`, `build_project` und `deploy_android` bieten deterministische Abläufe für Projekterkennung, Build und Android-Verteilung. Android-Deployment baut zuerst, sucht die aktuelle Debug-APK, prüft autorisierte Geräte und verwendet `adb install -r`.
- ADB prüft alle gefundenen SDK-Kopien, startet den Server kontrolliert neu, unterscheidet `device`, `unauthorized` und `offline` und ergänzt eine Windows-Plug-and-Play-Diagnose.
- Neue Aufgaben überschreiben alte Rückfragen. Unnötige Fragen nach `git init`, manueller Werkzeugeingabe oder einer bloßen Bestätigung, ob ADB installiert ist, werden blockiert, wenn LocalCode dies selbst prüfen kann.
- Leere Websuchanfragen werden deterministisch aus der aktuellen Aufgabe ergänzt; erwartbare Git- und ADB-Zustände lösen keine sinnlose Webrecherche aus.
- Details stehen in `docs/TOOLS.md`.

## Stabilität und Kontrolle in 4.6.0

- Hintergrundbefehle unter Windows laufen ohne aufblinkende Konsolenfenster.
- Werkzeug-, Git-, Diff-, Befehls- und Fehlerausgaben sind im Chat und im rechten Ausgabenbereich vollständig aufklappbar.
- Rückfragen werden innerhalb desselben Agentenkontexts fortgesetzt.
- Während der Ausführung bleiben Eingabe und Abbruch erreichbar. Nach einem normalen Abbruch steht zusätzlich ein kontrollierter Zwangsreset zur Verfügung.
- Modellaufrufe besitzen ein eigenes, konfigurierbares Zeitlimit.
- Die Oberfläche gleicht den Laufzustand regelmäßig mit dem Backend ab, auch wenn eine Ereignisverbindung kurz unterbrochen wurde.
- Einstellungen werden als vorwärtskompatibler Patch gespeichert; Fehler enthalten die konkrete Servermeldung.

## Oberfläche

- Linke Spalte: neuer Chat, Git-Übersicht, Terminal, Einstellungen, Projektsuche, Projekte und persistente Chats.
- Mitte: aktiver Chat und Arbeitsverlauf.
- Rechte Spalte: Genehmigungen, Befehle, Git-Ausgaben und Webquellen.
- Unterer Composer: Dateien, Genehmigungsmodus, Modell und Senden.

Alle sichtbaren Navigationspunkte sind an echte Funktionen angebunden. Es gibt keine absichtlich eingebauten Platzhalterseiten.


## Layout und Einstellungen

Die drei Hauptbereiche können mit der Maus verschoben werden. Das integrierte Terminal besitzt einen eigenen horizontalen Splitter und kann in **Einstellungen > Allgemein** unten oder rechts angedockt werden. Die Größen werden dauerhaft gespeichert.

Die Einstellungsseite entspricht dem Aufbau einer modernen Desktop-Entwicklungsanwendung und enthält nur verdrahtete Funktionen. Dazu gehören Berechtigungen, Ausführungsumgebung, Terminal, Appearance, Personalisierung, Tastaturkürzel, MCP, Webrecherche, Pfad-/Befehlsregeln, Hooks, Verbindungen, Git, Worktrees und archivierte Chats.

## Dateianhänge

Pro Anfrage können bis zu 20 Dateien mit insgesamt höchstens 96 MiB aus der Oberfläche übertragen werden. Der Server akzeptiert bis zu 128 MiB dekodierte Dateidaten.

Unterstützte Verarbeitung:

- Bilder: PNG, JPEG, WebP, GIF über ein lokales Vision-Modell.
- Text, Quellcode, JSON, XML, CSV, Markdown und Konfigurationsdateien: direkte UTF-8-Extraktion.
- DOCX, PPTX, XLSX/XLSM: lokale XML-Extraktion aus dem Office-Container.
- ZIP, JAR, APK, AAB: Inhaltsliste.
- PDF: `pdftotext`, wenn installiert; andernfalls konservativer Rohtextversuch.
- Andere Binärformate: lokales Zwischenspeichern mit Pfad und Metadaten für genehmigte lokale Werkzeuge.

Dateien können über den Plus-Button, Drag-and-drop oder die Zwischenablage eingefügt werden. Enter sendet; Umschalt+Enter erzeugt eine neue Zeile. Nach Abschluss springt der Fokus wieder in ein leeres Eingabefeld.

## Projekt- und Chatverlauf

Chats werden lokal unter dem LocalCode-Konfigurationsordner gespeichert und nach Projekt gruppiert. Der Verlauf enthält Benutzeraufgaben, Agentenschritte, Ergebnisse und Dateianhänge als Metadaten. Binärdaten der Anhänge werden nicht dauerhaft in den Chatverlauf eingebettet.

## Git, MCP, Web und Befehle

- Strukturierte Werkzeugerkennung und direkte Ausführung über `discover_tool`, `tool_inventory` und `run_tool`.
- Git-Status, Diff, Log, Branches, Commits und weitere freigegebene Git-Operationen.
- Nicht-interaktive Befehle sowie sichtbare interaktive Terminals für Logins.
- Websuche und Seitenabruf nach den konfigurierten Netzwerk- und Freigaberegeln.
- MCP über stdio und Streamable HTTP.
- Kopieren und Verschieben innerhalb der konfigurierten Pfadregeln.

## Projektdokumentation

Der Quellcode enthält:

- `README.md`
- `AGENTS.md`
- `STATE.md`
- `docs/ARCHITECTURE.md`
- `docs/SECURITY.md`
- `docs/MCP-EXAMPLES.md`
- `docs/RESEARCH-NOTES.md`

In ausgewählten Projekten kann LocalCode fehlende `README.md` und `AGENTS.md` anlegen und einen klar markierten Bereich in `STATE.md` automatisch aktuell halten.

## Sicherheit

Standardmäßig arbeitet LocalCode mit strikten Genehmigungen und auf das ausgewählte Projekt begrenzten Dateizugriffen. Der native Windows-Modus verwendet anwendungseigene Pfad-, Befehls- und Freigaberegeln. Er ist keine identische Kopie der proprietären Codex-Sandbox.

## Abgrenzung

LocalCode ist nicht OpenAI Codex und verwendet keine OpenAI-Logos oder proprietären UI-Assets. Eine exakte Funktions- oder Modellparität mit einem gehosteten Frontier-Modell kann mit einem lokalen 14B-/20B-Modell nicht seriös zugesichert werden. Version 4.6.0 erweitert diese Arbeitsabläufe um verifizierte Werkzeugreparatur und deterministische Build-/Android-Deployment-Aktionen; eine Modellparität mit einem gehosteten Frontier-Modell wird weiterhin nicht behauptet.

## Lizenz

LocalCode steht unter der **Apache License 2.0**. Der vollständige Lizenztext befindet sich in [`LICENSE`](LICENSE); produktspezifische Hinweise stehen in [`NOTICE`](NOTICE), Hinweise zu externen Komponenten in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

Die Apache-2.0-Lizenz erlaubt Nutzung, Veränderung und Verteilung, verlangt dabei aber die Beibehaltung der Lizenz- und Hinweistexte. Für externe Programme, Modelle, MCP-Server und Webinhalte gelten jeweils deren eigene Lizenzen und Nutzungsbedingungen.

## Git and Visual Studio

The source package includes `.gitignore`, `.gitattributes`, `.editorconfig`,
`GIT-SETUP.md` and `COMMIT_MESSAGE.txt`. Open the folder in Visual Studio and use
the included commit message for the first Git commit.

