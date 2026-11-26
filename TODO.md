# LocalCode — open work / offene Arbeit

**Verified:** 2026-11-23 Europe/Berlin
**Merged base:** `96ad7f7d76e6b2f5b01308be52199cc9d895d9ea`, master / published release `v6.9.3`; lokale „Beyond Frontier“-Erweiterungen (Phasen 0–4: OS-Sandbox, `apply_patch`, ConPTY, E2EE-Relay `dist/localcode-relay.exe`, VS Code VSIX `localcode-0.1.0.vsix` inkl. Inline-Edit/FIM, Android Keystore-Hardening & `FrameLayout`-Lifecycle-Fixes) lokal gebaut & getestet, nicht gepusht/veröffentlicht.
**Active branch:** `master`.
**Roadmap reference:** #32. Current implemented reality and detailed restart context: `STATE.md`.

The 2026-11-20 project-directory guard is implemented and verified in the
canonical checkout and the AgentOS bundle; it adds no open TODO item. Existing
uncommitted documentation/toolchain changes remain preserved.

## Open work / Offene Punkte

- [ ] Publish Open VSX listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher namespace/account and `OVSX_PAT` are not configured in the current environment; user clarification is pending. Acceptance: listing resolves to the actual version and installable VSIX.
- [ ] Publish Visual Studio Marketplace listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher account and registry credential are not configured in the current environment. Acceptance: public listing resolves to the released extension.

## Offene Punkte (aus `STATE.md`)

| Bereich | Beschreibung | Status / Akzeptanzkriterium |
|--------|--------------|--------------------|
| Android Startup & Security | Schwarzbild-Fix (`FrameLayout`), TLS-Pinning ohne `isPrivateHost`-Bypass, DPAPI-Signatur | Lokal gebaut & verifiziert (`apksigner verify` / Keystore-Hardening); Geräte-Re-Deploy offen |
| Unified Studio & UI | Modulares `NomaMica Studio` (`ui/studio`) mit ConPTY, Side-by-Side Diff & Live-Proxy | Lokal gebaut & getestet (`12/12` + Live Cross-Runtime Smoke PASS) |
| ComputeMesh Integration | Auto‑Discovery & Hardcode-Bereinigung | Lokal implementiert & getestet (`go test -race`) |
| Tests & Coverage | Fokus-Suites mit `go test -race` + VS Code Extension Tests | `go test -race` Integration/Feature-Suites & `11/11` Extension-Tests PASS; Gesamt-Repo-Coverage-Lauf separat bei Release |
| VSIX Veröffentlichung | Open VSX & Marketplace‑Listing (`localcode-0.1.0.vsix` lokal gebaut) | **Offen:** Extension im Registry veröffentlichen (Publisher-Credentials erforderlich) |
| Installer & Binaries | `localcode-relay.exe` gebaut; `LocalCode-Setup.exe` Voll-Release | `localcode-relay.exe` gebaut; Voll-Installer bei Release-Build erzeugen |
| Documentation | `README.md`, `STATE.md`, `TODO.md` Synchronisation | `STATE.md` & `TODO.md` auf Stand 2026-11-23 |

## Permanent completion rules / Dauerhafte Abschlussregeln

Maintain README.md, STATE.md and this list after material scope/branch/PR/CI/release changes. Read architecture/security before code changes. Keep German/English catalogs synchronized. Preserve canonical workspace checks, approvals, bounded execution and >=80.0% coverage. `run_journal.go` remains the only durable active recovery authority; startup stays passive. Mobile authority remains narrower than Desktop. A published VSIX is not proof of Marketplace listing or Claude Code/Antigravity reasoning/service parity. Do not invent success evidence or leave completed tasks checked in this file.
