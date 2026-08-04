# LocalCodex 4.3.0

LocalCodex ist ein eigenständiger lokaler Windows-Coding-Agent für Ollama. Die Anwendung bietet eine desktopartige Projekt- und Chatoberfläche, persistente Chats, Datei- und Bildanhänge, Git, Befehlsausführung, Webrecherche, MCP, Genehmigungen und eine zentrale Einstellungsseite.

## Schnellstart

1. ZIP vollständig entpacken.
2. `BUILD-AND-RUN.bat` doppelklicken.
3. Beim ersten Build lädt das Skript bei Bedarf eine portable offizielle Go-Version.
4. LocalCodex öffnet sich unter Windows bevorzugt als Edge-/Chrome-App-Fenster ohne Browser-Tabs und Adressleiste.


## Stabilität und Kontrolle in 4.3.0

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

Chats werden lokal unter dem LocalCodex-Konfigurationsordner gespeichert und nach Projekt gruppiert. Der Verlauf enthält Benutzeraufgaben, Agentenschritte, Ergebnisse und Dateianhänge als Metadaten. Binärdaten der Anhänge werden nicht dauerhaft in den Chatverlauf eingebettet.

## Git, MCP, Web und Befehle

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

In ausgewählten Projekten kann LocalCodex fehlende `README.md` und `AGENTS.md` anlegen und einen klar markierten Bereich in `STATE.md` automatisch aktuell halten.

## Sicherheit

Standardmäßig arbeitet LocalCodex mit strikten Genehmigungen und auf das ausgewählte Projekt begrenzten Dateizugriffen. Der native Windows-Modus verwendet anwendungseigene Pfad-, Befehls- und Freigaberegeln. Er ist keine identische Kopie der proprietären Codex-Sandbox.

## Abgrenzung

LocalCodex ist nicht OpenAI Codex und verwendet keine OpenAI-Logos oder proprietären UI-Assets. Eine exakte Funktions- oder Modellparität mit einem gehosteten Frontier-Modell kann mit einem lokalen 14B-/20B-Modell nicht seriös zugesichert werden. Version 4.3.0 setzt jedoch die sichtbaren Arbeitsabläufe eigenständig und ohne absichtliche Dummy-Funktionen um.
