# LocalCode 4.7.0 – Agent Supervisor und kontrollierte Kontextkomprimierung

## Behobene Ablaufprobleme

- Eine reine Projektanalyse verlangt kein Git-Repository mehr und darf weder `git init` noch Dateiänderungen ausführen.
- Bestätigt der Nutzer eine konkrete Git-Initialisierung mit „ja“, führt LocalCode `git init` direkt aus, verifiziert das Repository und setzt die ursprüngliche Aufgabe ohne Wiederholung der Rückfrage fort.
- Neue Aufgaben verdrängen veraltete Git-, Build- oder ADB-Rückfragen zuverlässig.
- Build-, Android-Deployment-, Web- und Git-Initialisierungsaufgaben beginnen mit deterministischen Werkzeugaktionen statt mit geratenen Shell-Befehlen.
- Wiederholte unpassende Aktionen oder unnötige Fragen werden vom Supervisor blockiert. Bei wiederholter Modelldrift endet eine Analyse kontrolliert mit einem überprüfbaren Bericht statt am Schrittlimit.

## Werkzeuge und Internet

- Git-Initialisierung wird über den tatsächlich erkannten Git-Pfad ausgeführt und mit `git rev-parse --is-inside-work-tree` verifiziert.
- Fehlt `.gitignore`, wird eine Visual-Studio-, Build-, Cache-, Secret- und Ökosystem-taugliche Datei angelegt.
- Unterstützte fehlende Werkzeuge werden nach Genehmigung installiert, erneut entdeckt und der ursprüngliche Aufruf wird wiederholt.
- Websuchanfragen werden an der Werkzeuggrenze normalisiert und können nicht leer ausgeführt werden.
- DuckDuckGo-Suche besitzt einen Bing-RSS-Fallback.

## Kontextkomprimierung

- LocalCode schätzt die Kontextbelegung fortlaufend.
- Standardmäßig wird bei 68 Prozent des konfigurierten Kontextfensters komprimiert.
- Erhalten bleiben ursprüngliche Aufgabe, Nutzerentscheidungen, Projektfakten, gelesene und geänderte Dateien, Befehle, Fehler, offene Punkte und die nächste Aktion.
- Die jüngsten Nachrichten bleiben unverändert erhalten.
- Scheitert die modellgestützte Verdichtung, wird eine deterministische lokale Zusammenfassung verwendet.
- Schwelle und Anzahl unverändert beibehaltener Nachrichten sind in den Einstellungen konfigurierbar.

## Regressionstests

- Analyse ohne Git und ohne Mutation
- direkte Ausführung einer bestätigten Git-Initialisierung
- Erstellung einer geeigneten `.gitignore`
- kontrollierte Kontextkomprimierung und Fortsetzung
- UI-Bindung aller Komprimierungseinstellungen
- bestehende Build-, Tool-Reparatur-, Abbruch-, MCP-, Datei- und Einstellungsprüfungen
