# LocalCode 4.0.0 – UI- und Einstellungs-Neuaufbau

## Oberfläche

- vollständige Codex-orientierte Desktopstruktur
- Windows-artige Menüleiste mit Datei, Bearbeiten, Ansicht und Hilfe
- Projekt- und Chatbaum links
- zentraler Arbeitsverlauf und Composer
- Ausgaben- und Quelleninspektor rechts
- verschiebbare linke und rechte Splitter
- verschiebbares Terminal
- Terminal wahlweise unten oder rechts
- gespeicherte Panelgrößen und Layoutoptionen
- Edge-/Chrome-App-Modus ohne normale Browser-Tabs

## Einstellungen

Die Einstellungsseite wurde vollständig neu aufgebaut. Sichtbare Schalter und Eingabefelder sind mit realen Config-Feldern beziehungsweise API-Aktionen verbunden:

- Berechtigungen und Vollzugriff
- Standardeditor, Agentenumgebung und Terminal-Shell
- Sprache, Statusleiste, Terminal-Docking und Geschwindigkeit
- Profil, Avatar, Theme, Farben und Fonts
- Spracheingabe
- Ollama, Kontext, Agentenschritte und Timeouts
- Personalisierung und bevorzugte Antwortsprache
- Tastaturkürzel
- MCP-Server und Verbindungstest
- Websuche und Netzwerkzugriff
- Sandbox, erlaubte Pfade und blockierte Befehle
- Hooks, Umgebungsvariablen und Git
- SSH-Verbindungen
- Worktrees
- archivierte Chats
- Konfigurationsimport und -export

## Agentenfunktionen

- Dateien aller unterstützten Typen über Auswahl, Drag-and-drop und Zwischenablage
- Bildanalyse mit lokalem Vision-Modell
- Git, Terminal, Webrecherche, MCP und Dateiwerkzeuge
- Projekt-README, AGENTS.md und verwaltete STATE.md
- Hook-Ausführung vor und nach Aufgaben
- Windows-native oder WSL-basierte Befehlsumgebung

## Build

`BUILD-AND-RUN.bat` führt Formatierung, Tests, `go vet`, GUI-Build, Diagnose-Build und SHA-256-Erzeugung aus.
