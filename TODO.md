# LocalCode — open work / offene Arbeit

**Verified:** 2026-09-22 Europe/Berlin
**Merged base:** `96ad7f7d76e6b2f5b01308be52199cc9d895d9ea`, master / published release `v6.9.3`; newer local UI build installed, not published.
**Active branch:** `master`.
**Roadmap reference:** #32. Current implemented reality and detailed restart context: `STATE.md`.

## Open work / Offene Punkte

- [ ] Publish Open VSX listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher namespace/account and `OVSX_PAT` are not configured in the current environment; user clarification is pending. Acceptance: listing resolves to the actual version and installable VSIX.
- [ ] Publish Visual Studio Marketplace listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher account and registry credential are not configured in the current environment. Acceptance: public listing resolves to the released extension.

## Permanent completion rules / Dauerhafte Abschlussregeln

Maintain README.md, STATE.md and this list after material scope/branch/PR/CI/release changes. Read architecture/security before code changes. Keep German/English catalogs synchronized. Preserve canonical workspace checks, approvals, bounded execution and >=80.0% coverage. `run_journal.go` remains the only durable active recovery authority; startup stays passive. Mobile authority remains narrower than Desktop. A published VSIX is not proof of Marketplace listing or Claude Code/Antigravity reasoning/service parity. Do not invent success evidence or leave completed tasks checked in this file.
