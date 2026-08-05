# LocalCode 4.8.0 – bilingual UI, persistent approvals, and reviewed stability

## Deutsch

- Vollständige Deutsch-/Englisch-Pflege für Oberfläche, Einstellungsseite, Dialoge, Werkzeughinweise, Projektvorlagen, Git-Anleitung und Hauptdokumentation.
- Automatische Erkennung der Windows-Anzeigesprache: Deutsch bei deutschem Windows, sonst Englisch; zusätzlich manuelle Umschaltung.
- Dauerhafte Genehmigungsleiste unten mittig, auch wenn Einstellungen, Quellen oder andere Ansichten geöffnet sind.
- Verbesserte Projektordnerauswahl mit direkter Pfadeingabe und im Vordergrund gehaltenem Windows-Dialog.
- Reparierte Ganzzahlserialisierung für Splitter-, Kontext- und Timeoutwerte.
- Atomisches Speichern von Konfiguration und Chatverlauf mit Wiederherstellungsbackup.
- Zusammenfassender Hintergrundschreiber für Chatereignisse statt synchroner Dateizugriffe.
- Geordneter Persistenz-Shutdown mit Flush des neuesten Chatstands.
- Einstellungen werden erst nach erfolgreichem Speichern aktiviert; Projekt- und Dokumentationsfehler werden nicht mehr verschluckt.
- Bereinigter Genehmigungszustand nach Genehmigung, Ablehnung, Abbruch und Timeout.
- Startzeitlimit für Ollama-Modellabfrage.
- Zusätzlicher Schutz des lokalen Servers gegen fremde Hosts, Cross-Origin-/Cross-Site-Aufrufe und Einbettung.
- Bilinguale Build- und Hilfsskripte nach Windows-Anzeigesprache.
- Neue Race-, Reihenfolge-, Lokalisierungs-, Genehmigungs-, Projektwurzel-, Sicherheits- und Persistenztests.

## English

- Complete German/English maintenance for the interface, Settings, dialogs, tool guidance, project templates, Git instructions, and main documentation.
- Automatic Windows display-language detection: German on German Windows, English otherwise, with a manual override.
- Persistent bottom-center approval bar, including Settings, Sources, and other views.
- Improved project-folder selection with direct path entry and a foreground-owned Windows dialog.
- Fixed integer serialization for splitters, context, and timeout values.
- Atomic configuration and chat-history persistence with recovery backups.
- Coalesced background writer for chat events instead of synchronous file writes.
- Orderly persistence shutdown with a final flush of the newest chat state.
- Settings become active only after a successful durable write; project and documentation errors are no longer ignored.
- Pending approval state is cleared after approval, rejection, cancellation, and timeout.
- Startup timeout for Ollama model discovery.
- Additional local-server protection against foreign Hosts, cross-origin/cross-site requests, and embedding.
- Bilingual build and helper scripts following the Windows display language.
- New race, order-randomization, localization, approval, project-root, security, and persistence tests.
