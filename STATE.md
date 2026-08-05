# LocalCode project state / Projektstatus

**Version:** 4.8.0  
**License / Lizenz:** Apache-2.0  
**Status:** release candidate after code review and automated verification

## Deutsch

### Abgeschlossene Änderungen in 4.8.0

- Vollständige UI-Sprachumschaltung Deutsch/Englisch.
- Automatische Sprache nach Windows-Anzeigesprache: Deutsch bei deutschem Windows, sonst Englisch.
- Manuelle Sprachwahl und bevorzugte Antwortsprache.
- Identische Schlüssel in allen Sprachkatalogen werden automatisiert geprüft.
- Zweisprachige Projektvorlagen für `README.md` und `AGENTS.md`.
- Zweisprachige Hauptdokumentation und Git-Anleitung.
- Offene Genehmigungen erscheinen als dauerhafte Leiste unten mittig, unabhängig von aktiver Ansicht oder rechtem Tab.
- Genehmigungen werden nach Abbruch oder Timeout zuverlässig aus dem Backendzustand entfernt.
- Projektwurzel kann über ein modales Eingabefeld oder einen sichtbaren, im Vordergrund gehaltenen Windows-Ordnerdialog geändert werden.
- Splitterwerte und numerische Einstellungen werden vor dem Speichern auf Ganzzahlen normalisiert.
- Konfiguration und Chatverlauf werden mit temporärer Datei, Synchronisation, Backup und Wiederherstellung ersetzt.
- Chatereignisse werden nicht mehr synchron bei jedem Ereignis auf die Festplatte geschrieben; ein einzelner zusammenfassender Hintergrundschreiber verhindert UI-/Agentenblockaden und konkurrierende Schreibvorgänge.
- Ollama-Modellabfrage beim Start besitzt ein festes Zeitlimit.
- Vollständige Race-, Vet-, Syntax-, UI-Mock- und Cross-Build-Prüfung ergänzt.

### Bekannte Grenzen

- Ein lokales 14B-/20B-Modell erreicht nicht zuverlässig die Modellqualität eines gehosteten Frontier-Modells.
- Die native Schutzschicht ist anwendungsbasiert und keine identische OS-Sandbox der proprietären Codex-Infrastruktur.
- Externe Werkzeuge, Geräte, Zugangsdaten und Netzwerkdienste können nur auf dem Zielsystem endgültig verifiziert werden.

## English

### Completed changes in 4.8.0

- Complete German/English UI language switching.
- Automatic language based on the Windows display language: German on German Windows, English otherwise.
- Manual UI language and preferred response language selection.
- Automated enforcement of identical keys in every language catalog.
- Bilingual project templates for `README.md` and `AGENTS.md`.
- Bilingual main documentation and Git guide.
- Pending approvals are displayed in a persistent bottom-center bar regardless of the active view or right-side tab.
- Approvals are reliably removed from backend state after cancellation or timeout.
- The project root can be changed through a modal path field or a visible foreground-owned Windows folder dialog.
- Splitter values and numeric settings are normalized to integers before saving.
- Configuration and chat history use temporary files, synchronization, backup, and recovery during replacement.
- Chat events are no longer written synchronously to disk for every event; one coalescing background writer prevents UI/agent stalls and concurrent file writers.
- Startup model discovery has a fixed timeout.
- Race, vet, syntax, UI mock, and cross-build verification were expanded.

### Known limitations

- A local 14B/20B model cannot reliably match the model quality of a hosted frontier model.
- Native protection is application-level and not an identical OS sandbox to proprietary Codex infrastructure.
- External tools, devices, credentials, and network services can only be finally verified on the target machine.

<!-- LOCALCODE:STATE:BEGIN -->
Managed runtime state is written here when this repository itself is selected in LocalCode.
Verwalteter Laufzeitstatus wird hier geschrieben, wenn dieses Repository selbst in LocalCode ausgewählt ist.
<!-- LOCALCODE:STATE:END -->


## 4.8.0 final review additions

- Tool installation guidance is localized in German and English.
- Chat persistence has an explicit close/flush lifecycle.
- Settings and project selection no longer report success before durable persistence succeeds.
- The local HTTP server validates loopback Hosts and rejects cross-origin/cross-site mutations.
- Build and bootstrap scripts follow the Windows UI language.
