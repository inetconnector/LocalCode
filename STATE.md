# LocalCode Entwicklungsstand

**Version:** 4.7.0  
**Status:** Build-, test- und paketierfähig  
**Zielplattform:** Windows x64  
**Backend:** Go, lokaler HTTP-Server, Ollama  
**Frontend:** eingebettetes HTML/CSS/JavaScript im Edge-/Chrome-App-Modus

## Agent Supervisor und Kontextverwaltung in 4.7.0

- Aufgabenklassifikation für Analyse, Build, Android-Deployment, Webrecherche und Git-Initialisierung.
- Read-only-Policy für Analyseaufgaben; fehlendes Git ist kein Blocker.
- Direkt ausführbare Fortsetzungen für bestätigte Rückfragen, einschließlich verifiziertem `git init`.
- Wiederholungs- und Driftkontrolle mit deterministischem Abschlussbericht.
- Nicht leere Webquery an der Werkzeuggrenze und Bing-RSS-Fallback.
- Kontextkomprimierung standardmäßig bei 68 Prozent; aktuelle 12 Nachrichten bleiben unverändert.
- Modellgestützte strukturierte Verdichtung mit deterministischem Fallback.
- UI-Einstellungen und sichtbares Ereignis für jede Komprimierung.
- Regressionstests für Analyse ohne Mutation, Git-Fortsetzung und Langkontext-Fortsetzung.

## Werkzeugreparatur und deterministische Abläufe in 4.7.0

- Vollständige Werkzeugerkennung über Projekt-Wrapper, konfigurierte Pfade, `PATH`, Android SDK, Windows-Standardpfade sowie Visual Studio und `vswhere.exe`.
- Verifizierte, benutzerlokale Installation der offiziellen Android SDK Platform-Tools und der offiziellen portablen MinGit-Ausgabe nach separater Genehmigung.
- WinGet-Fallback für bekannte Pakete; Hintergrundprozesse bleiben unsichtbar und besitzen Timeout sowie kontrollierten Abbruch.
- Nach Installation wird die ursprüngliche Aktion ohne erneutes Modellraten wiederholt. Mehrere nacheinander fehlende unterstützte Werkzeuge werden begrenzt repariert.
- Neue Aktionen `project_info`, `build_project` und `deploy_android` erkennen Buildsysteme, führen den passenden Build aus und verteilen Android-Debug-APKs an genau ein autorisiertes Gerät.
- Visual-Studio-Werkzeuge MSBuild, Git, CMake, Ninja, NuGet und `devenv.exe` werden innerhalb installierter VS-Instanzen gesucht.
- ADB probiert alternative Installationen, startet/reconnectet den Server einmal, wertet Gerätezustände aus und ergänzt Windows-PnP-Diagnose.
- Alte Rückfragen werden bei neuen Aufgaben verworfen. Unnötige Git-Initialisierungs- und Werkzeug-Ausweichfragen werden serverseitig blockiert.
- Websuchen erhalten bei leerer Modellanfrage automatisch die aktuelle Nutzeraufgabe als Query.

## Produktidentität, Lizenz und Git-Paket

- Produktname und Binärdateien heißen **LocalCode**.
- Lizenz: Apache License 2.0 mit `LICENSE`, `NOTICE` und `THIRD_PARTY_NOTICES.md`.
- Git-fertiges Paket mit `.gitignore`, `.gitattributes`, `.editorconfig`, `GIT-SETUP.md` und `COMMIT_MESSAGE.txt`.

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

- LocalCode ist ein eigenständiger lokaler Client und nicht OpenAI Codex.
- Ein lokales 14B-/20B-Modell kann die Modellqualität eines gehosteten Frontier-Modells nicht garantieren.
- Die anwendungseigenen Pfad- und Freigaberegeln sind keine identische betriebssystemseitige Codex-Sandbox.
- Computersteuerung fremder GUI-Anwendungen ist nur über ausdrücklich konfigurierte Werkzeuge oder MCP-Server möglich.
