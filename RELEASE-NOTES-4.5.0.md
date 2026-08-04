# LocalCode 4.5.0 – vollständige Produktumbenennung und Lizenzierung

## Umbenennung

- Der Produktname lautet überall **LocalCode**.
- Die Windows-Binärdateien heißen `LocalCode.exe` und `LocalCode-Debug.exe`.
- Konfiguration, Chats, Logs und Sicherungen werden unter dem Produktordner `LocalCode` gespeichert.
- UI, API-Kennung, MCP-Clientinfo, User-Agent, Build-Skripte, Dokumentation, Diagnose und Projektvorlagen wurden umbenannt.
- Das Go-Modul heißt `localcode`.

## Verlustfreie Migration

Beim ersten Start kopiert LocalCode vorhandene Konfigurationen, Chats und Sicherungen der unmittelbar vorherigen Produktbezeichnung in die neuen LocalCode-Verzeichnisse, sofern dort noch keine entsprechenden Dateien existieren. Die alten Daten werden nicht gelöscht.

Bereits vorhandene verwaltete `STATE.md`-Abschnitte werden beim nächsten Update auf die Marker `LOCALCODE:STATE:BEGIN/END` migriert; manuelle Inhalte außerhalb des verwalteten Bereichs bleiben erhalten.

## Lizenz

- Neuer vollständiger Lizenztext: `LICENSE` (Apache License 2.0)
- Produkthinweise: `NOTICE`
- Hinweise zu nicht gebündelten externen Komponenten: `THIRD_PARTY_NOTICES.md`
