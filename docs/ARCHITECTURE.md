# Architektur

## Prozessmodell

`LocalCodex.exe` startet einen ausschließlich an `127.0.0.1` gebundenen Go-HTTP-Server. Unter Windows wird die Oberfläche bevorzugt mit Edge oder Chrome im `--app`-Modus geöffnet. Dadurch entsteht ein eigenes App-Fenster ohne normale Browser-Tabs und Adressleiste. Ist kein Chromium-Browser verfügbar, wird der Standardbrowser verwendet.

## Komponenten

- `main.go`: Start, Ollama-Erkennung, Versionswechsel, HTTP-Server und App-Fenster.
- `server.go`: lokale API, SSE-Ereignisse, Projekte, Chats, Einstellungen, Git und Genehmigungen.
- `history.go`: persistente Chats pro Projekt.
- `agent.go`: strukturierte Agentenschleife und Werkzeugdispatch.
- `attachments.go`: allgemeine Dateianhänge, lokale Extraktion und temporäre Ablage.
- `ollama.go`: Ollama-API, strukturierte Ausgaben und Bildanalyse.
- `tools.go`, `git_tools.go`, `path_tools.go`, `web_tools.go`, `mcp.go`: reale lokale Werkzeuge.
- `state_doc.go`: README-/AGENTS-/STATE-Pflege.
- `static/index.html`: eingebettete dreigeteilte Desktop-Oberfläche.

## Dateianhänge

Der Browser überträgt Dateien als Base64-kodierte JSON-Anhänge an `/api/chat`. Der Server validiert Anzahl, Einzelgröße und Gesamtgröße, schreibt die Daten in einen zufälligen Unterordner des LocalCodex-Konfigurationsverzeichnisses und entfernt ihn nach dem Agentenlauf.

- Bilder gehen zusätzlich an ein lokales Vision-Modell.
- Textformate werden direkt extrahiert.
- Office-Open-XML-Dateien werden als ZIP gelesen und deren XML-Text extrahiert.
- Archive werden aufgelistet.
- PDFs verwenden `pdftotext`, wenn vorhanden, sonst einen konservativen Rohtextversuch.
- Andere Binärdateien bleiben als lokaler temporärer Pfad für genehmigte Werkzeuge verfügbar.

## Chatverlauf

Chats werden in `threads.json` im LocalCodex-Konfigurationsverzeichnis gespeichert. Binärdaten der Anhänge werden nicht dauerhaft in der Historie gespeichert; lediglich Name, MIME-Typ und Größe bleiben in den Ereignissen erhalten.

## Sicherheitsgrenzen

Der HTTP-Server lauscht nur lokal. Datei- und Befehlswerkzeuge unterliegen den Approval-, Sandbox- und Pfadregeln aus der Konfiguration. Diese Regeln sind anwendungseigen und kein identischer Nachbau einer proprietären Betriebssystem-Sandbox.


## Layout- und Einstellungsarchitektur

Die UI besitzt drei Hauptbereiche in einem CSS-Grid. Linke und rechte Spalte werden über Pointer-Events verschoben; der Terminalbereich besitzt einen horizontalen Splitter. Die Werte werden in der Go-Konfiguration gespeichert und bei jedem Start wiederhergestellt.

Der Terminal-Docking-Modus ist kein rein visueller Schalter: Das bestehende Terminal-DOM-Element wird wahlweise in den zentralen Arbeitsbereich oder in den rechten Inspektor verschoben. Ereignishandler und Prozessausführung bleiben dabei erhalten.

Die Einstellungsseite ist eine eigenständige Vollbildansicht. Jeder sichtbare Wert wird über `/api/settings` geladen und gespeichert oder löst eine konkrete API-Aktion aus.
