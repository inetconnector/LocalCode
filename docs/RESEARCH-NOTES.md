# Research Notes

## Referenzumfang

Version 4.0.0 orientiert sich an öffentlich dokumentierten Codex-Arbeitsmustern und an den vom Nutzer bereitgestellten Referenz-Screenshots. Relevant waren insbesondere:

- Projekte und fortsetzbare Chats
- lokale Arbeitsordner und Datei-Kontext
- Review-/Ausgabenbereich
- Git-Worktrees für parallele Arbeit
- lokale Umgebungen, Setup-Schritte und Aktionen
- anpassbare Themes, Farben und Fonts
- Einstellungen für Berechtigungen, Terminal, Browser, Git, Hooks und Verbindungen
- MCP über stdio und Streamable HTTP

## Offizielle Referenzen

- https://openai.com/index/introducing-the-codex-app/
- https://developers.openai.com/codex/features
- https://developers.openai.com/codex/environments/git-worktrees
- https://developers.openai.com/codex/environments/local-environment
- https://developers.openai.com/codex/changelog
- https://developers.openai.com/codex/learn/best-practices
- https://help.openai.com/en/articles/20001275-chatgpt-work-and-codex
- https://help.openai.com/en/articles/20001277-using-the-built-in-browser-in-the-chatgpt-desktop-app

## Umsetzungsregeln

- Keine sichtbaren Navigationspunkte ohne reale Funktion.
- Keine kopierten OpenAI-Logos oder proprietären UI-Assets.
- Eigenständige Icons, Farben und Implementierung.
- Persistente lokale Chats und Einstellungen.
- Anhänge werden lokal verarbeitet.
- App-Modus über einen vorhandenen Chromium-Browser statt Electron.
- Sicherheitsrelevante Grenzen werden offen dokumentiert.


## Werkzeug- und Android-Referenzen

- https://developer.android.com/tools/releases/platform-tools
- https://developer.android.com/tools/adb
- https://developer.android.com/studio/command-line/variables
- https://developer.android.com/studio/run/device
- https://developer.android.com/studio/run/oem-usb
- https://developer.android.com/tools/sdkmanager

Die Implementierung verwendet diese Quellen für die SDK-/ADB-Suchpfade, Gerätezustände und Installationshinweise.
