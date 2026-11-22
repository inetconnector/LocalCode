# LocalCode — open work / offene Arbeit

**Verified:** 2026-11-20 Europe/Berlin
**Merged base:** `96ad7f7d76e6b2f5b01308be52199cc9d895d9ea`, master / published release `v6.9.3`; newer local UI build installed, not published.
**Active branch:** `master`.
**Roadmap reference:** #32. Current implemented reality and detailed restart context: `STATE.md`.

The 2026-11-20 project-directory guard is implemented and verified in the
canonical checkout and the AgentOS bundle; it adds no open TODO item. Existing
uncommitted documentation/toolchain changes remain preserved.

## Open work / Offene Punkte

- [ ] Publish Open VSX listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher namespace/account and `OVSX_PAT` are not configured in the current environment; user clarification is pending. Acceptance: listing resolves to the actual version and installable VSIX.
- [ ] Publish Visual Studio Marketplace listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher account and registry credential are not configured in the current environment. Acceptance: public listing resolves to the released extension.

## Offene Punkte (aus `STATE.md`)

| Bereich | Beschreibung | Akzeptanzkriterium |
|--------|--------------|--------------------|
| Android Startup | Schwarzbild beim Neustart, persistente Projekt‑Wiederherstellung | App startet ohne schwarzes Bild, letztes Projekt wird automatisch wiederhergestellt |
| UI Redesign | Kombobox‑Overflow, Schriftgrößen, Glassmorphism‑Design | UI ist responsiv, keine Layout‑Überläufe, moderne Optik |
| ComputeMesh Integration | Auto‑Discovery, Zero‑Fee‑Compute, Provider‑Key‑Auto‑Discovery | ComputeMesh‑Knoten werden automatisch entdeckt und genutzt |
| Tests & Coverage | Fehlende Race‑Tests, Coverage < 80% | `go test -race` läuft ohne Fehler, Coverage ≥ 80% |
| VS IX Veröffentlichung | Open VSX & Marketplace‑Listing | Extension ist veröffentlicht und installierbar |
| Installer & Binaries | Build‑Fehler, Signatur | `LocalCode-Setup.exe` und portable Binaries bauen erfolgreich |
| Documentation | `README.md`, `STATE.md` Synchronisation | Dokumentation spiegelt aktuellen Stand wider |

## Permanent completion rules / Dauerhafte Abschlussregeln

Maintain README.md, STATE.md and this list after material scope/branch/PR/CI/release changes. Read architecture/security before code changes. Keep German/English catalogs synchronized. Preserve canonical workspace checks, approvals, bounded execution and >=80.0% coverage. `run_journal.go` remains the only durable active recovery authority; startup stays passive. Mobile authority remains narrower than Desktop. A published VSIX is not proof of Marketplace listing or Claude Code/Antigravity reasoning/service parity. Do not invent success evidence or leave completed tasks checked in this file.
