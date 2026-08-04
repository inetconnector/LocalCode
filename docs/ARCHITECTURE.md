# Architektur

## Prozessmodell

`LocalCode.exe` startet einen ausschließlich an `127.0.0.1` gebundenen Go-HTTP-Server. Unter Windows wird die Oberfläche bevorzugt mit Edge oder Chrome im `--app`-Modus geöffnet. Dadurch entsteht ein eigenes App-Fenster ohne normale Browser-Tabs und Adressleiste. Ist kein Chromium-Browser verfügbar, wird der Standardbrowser verwendet.

## Komponenten

- `main.go`: Start, Ollama-Erkennung, Versionswechsel, HTTP-Server und App-Fenster.
- `server.go`: lokale API, SSE-Ereignisse, Projekte, Chats, Einstellungen, Git und Genehmigungen.
- `history.go`: persistente Chats pro Projekt.
- `agent.go`: strukturierte Agentenschleife und Werkzeugdispatch.
- `attachments.go`: allgemeine Dateianhänge, lokale Extraktion und temporäre Ablage.
- `ollama.go`: Ollama-API, strukturierte Ausgaben und Bildanalyse.
- `tool_registry.go`: strukturierte Werkzeugauflösung, absolute Ausführung, ADB-Diagnose und offizielle Hilfe.
- `tools.go`, `git_tools.go`, `path_tools.go`, `web_tools.go`, `mcp.go`: reale lokale Werkzeuge.
- `state_doc.go`: README-/AGENTS-/STATE-Pflege.
- `static/index.html`: eingebettete dreigeteilte Desktop-Oberfläche.

## Dateianhänge

Der Browser überträgt Dateien als Base64-kodierte JSON-Anhänge an `/api/chat`. Der Server validiert Anzahl, Einzelgröße und Gesamtgröße, schreibt die Daten in einen zufälligen Unterordner des LocalCode-Konfigurationsverzeichnisses und entfernt ihn nach dem Agentenlauf.

- Bilder gehen zusätzlich an ein lokales Vision-Modell.
- Textformate werden direkt extrahiert.
- Office-Open-XML-Dateien werden als ZIP gelesen und deren XML-Text extrahiert.
- Archive werden aufgelistet.
- PDFs verwenden `pdftotext`, wenn vorhanden, sonst einen konservativen Rohtextversuch.
- Andere Binärdateien bleiben als lokaler temporärer Pfad für genehmigte Werkzeuge verfügbar.

## Chatverlauf

Chats werden in `threads.json` im LocalCode-Konfigurationsverzeichnis gespeichert. Binärdaten der Anhänge werden nicht dauerhaft in der Historie gespeichert; lediglich Name, MIME-Typ und Größe bleiben in den Ereignissen erhalten.

## Sicherheitsgrenzen

Der HTTP-Server lauscht nur lokal. Datei- und Befehlswerkzeuge unterliegen den Approval-, Sandbox- und Pfadregeln aus der Konfiguration. Diese Regeln sind anwendungseigen und kein identischer Nachbau einer proprietären Betriebssystem-Sandbox.


## Layout- und Einstellungsarchitektur

Die UI besitzt drei Hauptbereiche in einem CSS-Grid. Linke und rechte Spalte werden über Pointer-Events verschoben; der Terminalbereich besitzt einen horizontalen Splitter. Die Werte werden in der Go-Konfiguration gespeichert und bei jedem Start wiederhergestellt.

Der Terminal-Docking-Modus ist kein rein visueller Schalter: Das bestehende Terminal-DOM-Element wird wahlweise in den zentralen Arbeitsbereich oder in den rechten Inspektor verschoben. Ereignishandler und Prozessausführung bleiben dabei erhalten.

Die Einstellungsseite ist eine eigenständige Vollbildansicht. Jeder sichtbare Wert wird über `/api/settings` geladen und gespeichert oder löst eine konkrete API-Aktion aus.


## Werkzeug-Resolver

Der Agent verwendet für einzelne Programme bevorzugt `run_tool` statt frei formulierter Shell-Befehle. Der Resolver sucht in festen Einstellungen, Projektverzeichnissen, Projektkonfiguration, Umgebungsvariablen, PATH und bekannten Installationspfaden. Ausgeführt wird anschließend der absolute Pfad.

Jeder Werkzeuglauf ist an den Agentenkontext gebunden. Ein Abbruch oder Timeout beendet unter Windows den Prozessbaum. Ausgaben werden getrennt als STDOUT und STDERR erfasst und zusammen mit Exitcode und Laufzeit an Agent und Oberfläche zurückgegeben.

ADB besitzt eine eigene Zustandsdiagnose. Ein automatischer Reparaturversuch ist begrenzt; identische Aktionen und identische Rückfragen werden von der Agentenschleife blockiert.
## Agent Supervisor und Kontextkomprimierung (4.7)

Vor jedem Modellschritt klassifiziert der Supervisor die ursprüngliche Aufgabe. Für häufige Workflows erzwingt er zunächst eine deterministische Aktion (`project_info`, `build_project`, `deploy_android`, `web_search` oder `git init`). Eine Intent-Policy blockiert mutierende Aktionen bei reiner Analyse und Nicht-Web-Aktionen bei einer Internetrecherche. Wiederholte unpassende Modellaktionen führen zu einem kontrollierten Supervisor-Abschluss statt zu einer Endlosschleife.

Die Kontextverwaltung schätzt die Belegung fortlaufend. Beim konfigurierten Schwellenwert wird der ältere Verlauf in einen strukturierten Arbeitszustand überführt. Dieser enthält keine Gedankenkette, sondern ausschließlich Aufgabe, Entscheidungen, Projektfakten, gelesene/geänderte Dateien, Befehle, Fehler, offene Punkte und nächste Aktion. Die jüngsten Nachrichten bleiben unverändert.

