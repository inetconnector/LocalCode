# LocalCodex Entwicklungsstand

**Version:** 4.3.0  
**Status:** Build-, test- und paketierfähig  
**Zielplattform:** Windows x64  
**Backend:** Go, lokaler HTTP-Server, Ollama  
**Frontend:** eingebettetes HTML/CSS/JavaScript im Edge-/Chrome-App-Modus

## In 4.3.0 behobene Laufzeitfehler

- Windows-Hintergrundprozesse werden mit `CREATE_NO_WINDOW` und verborgenem Fenster gestartet. Sichtbar bleiben nur vom Nutzer ausdrücklich geöffnete interaktive Terminals.
- Werkzeugergebnisse, Befehlsausgaben, Git-Ausgaben, Diffs und Fehler werden als aufklappbare Karten im Chat und gesammelt im rechten Ausgabenbereich angezeigt.
- Eine Antwort auf `ask_user`, beispielsweise „ja“, setzt denselben Agentenlauf mit der ursprünglichen Aufgabe fort, statt die Analyse neu zu starten und dieselbe Frage erneut zu stellen.
- Während eines Laufs bleiben Eingabefeld und Abbruchsteuerung erreichbar. Ein normaler Abbruch beendet den Kontext; nach zwölf Sekunden kann die Oberfläche den Lauf kontrolliert zwangsweise zurücksetzen.
- Jeder Modellschritt besitzt ein konfigurierbares Zeitlimit. Der Status zeigt Phase und Laufzeit und wird zusätzlich zur Ereignisverbindung regelmäßig abgeglichen.
- Das Speichern der Einstellungen akzeptiert vorwärtskompatible Felder, erhält nicht mitgesendete Werte und liefert konkrete deutsche Fehlermeldungen statt eines pauschalen `Bad Request`.

## Verbindliche UI-Funktionen

- Codex-orientierte Desktopstruktur mit Menüleiste, Projekt-/Chatnavigation, Arbeitsbereich und Ausgaben-/Quellenbereich
- verschiebbare linke und rechte vertikale Splitter
- verschiebbarer Terminal-Splitter
- Terminal wahlweise unten oder rechts angedockt
- persistierte Panelgrößen, Sichtbarkeit, Farben und Fonts
- Vollbild-Einstellungsseite mit Kategorien für Allgemein, Import, Profil, Aussehen, Stimme, Konfiguration, Personalisierung, Tastaturkürzel, Plugins/MCP, Browser/Web, Computernutzung, Hooks, Verbindungen, Git, Umgebungen, Worktrees und archivierte Chats
- keine absichtlich funktionslosen Navigationspunkte

## Agenten- und Projektfunktionen

- Projektwurzel und Projektauswahl
- persistente Chats pro Projekt, Archivieren und Wiederherstellen
- Enter zum Senden, Umschalt+Enter für neue Zeile und Fokus-Rückkehr
- allgemeine Datei-, Bild-, Office-, PDF- und Archivanhänge
- Zwischenablage und Drag-and-drop
- lokale Bildanalyse über ein Vision-Modell
- Git-Werkzeuge, Git-Übersicht und Worktree-Befehle
- Dateiänderungen, Diffs, Genehmigungen und Befehle
- Internetrecherche mit Quellenansicht
- MCP über stdio und Streamable HTTP
- integriertes und externes Terminal, interaktive Logins, Kopieren und Verschieben
- automatische Projekt-README, AGENTS.md und vollständig aktualisierter verwalteter STATE.md-Bereich
- Hooks, Umgebungsvariablen und personalisierte Agentenanweisungen

## Automatisch geprüfte Fehlerpfade

- kontrollierter Abbruch eines blockierten Modellaufrufs
- Modellzeitüberschreitung mit anschließender Freigabe der Oberfläche
- erzwungener Reset eines festhängenden Laufs
- Fortsetzung nach einer Rückfrage ohne Wiederholung der ursprünglichen Analyse
- sichtbare Ergebnisse von Nur-Lese-Werkzeugen
- vorwärtskompatibles Speichern der Einstellungen auch während eines Agentenlaufs
- keine sichtbaren Windows-Konsolenfenster für Hintergrundbefehle
- Einbettung der Laufsteuerung und der Ausgabekarten im Frontend

## Prüfungen vor Veröffentlichung

- `go fmt ./...`
- `go test -count=1 ./...`
- `go vet ./...`
- JavaScript-Syntaxprüfung mit `node --check`
- Windows-amd64-GUI-Build
- Windows-amd64-Diagnose-Build
- PE-Struktur- und SHA-256-Prüfung
- ZIP-Integritätsprüfung

## Technische Grenzen

- LocalCodex ist ein eigenständiger lokaler Client und nicht OpenAI Codex.
- Ein lokales 14B-/20B-Modell kann die Modellqualität eines gehosteten Frontier-Modells nicht garantieren.
- Die anwendungseigenen Pfad- und Freigaberegeln sind keine identische betriebssystemseitige Codex-Sandbox.
- Computersteuerung fremder GUI-Anwendungen ist nur über ausdrücklich konfigurierte Werkzeuge oder MCP-Server möglich.
