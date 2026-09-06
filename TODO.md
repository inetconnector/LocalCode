# LocalCode — open work / offene Arbeit

**Verified:** 2026-09-06 Europe/Berlin
**Merged base:** `e511cc778e0e03ff1ba79ad4a18920844be33d8d`, master / latest published release `v6.9.1`, merged PR #87.
**Active branch:** `codex/localcode-ide-extension`; backend candidate 6.9.2, extension 0.1.0.
**Roadmap reference:** #32. Current implemented reality and detailed restart context: `STATE.md`.

The previous list contained completed Phase 7/8/9, ComputeMesh, browser/desktop, diagnostics, VM and documentation work. Those checked items have been removed; implemented details remain in STATE.md and Git history. The earlier reference to PR #88 was incorrect (no such PR at this workstream's start). No unchecked feature item from that list is being silently dropped.

Die vorherige Liste enthielt abgeschlossene Phase-7/8/9-, ComputeMesh-, Browser/Desktop-, Diagnose-, VM- und Dokumentationsarbeit. Abgehakte Punkte wurden entfernt; die implementierten Details stehen in STATE.md und Git. Der frühere Verweis auf PR #88 war falsch. Kein offener Funktionspunkt der bisherigen Liste wird stillschweigend entfernt.

## IDE extension release acceptance / Abnahme des IDE-Releases

- [ ] Complete final exact-worktree checks: Go format/vet/race, >=80.0% statement coverage, JS/PowerShell syntax, Desktop + extension browser menus, VS Code + Antigravity host integration, Windows amd64 GUI/debug/setup builds and VSIX contents. Record actual results in STATE.md. The CI threshold must match the documented 80.0%; never lower it to pass.
- [ ] Review final changes, push the branch, create PR, verify current master/head/reviews and wait for green exact-head Quality before merging. Refresh STATE.md/TODO.md in the same workstream.
- [ ] Publish backend 6.9.2 and extension 0.1.0 VSIX with checksums through the release pipeline; verify the downloaded artifact and install the extension in the user's Antigravity IDE.
- [ ] Publish Open VSX listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher namespace/account and `OVSX_PAT` are not configured in the current environment; user clarification is pending. Acceptance: listing resolves to the actual version and installable VSIX.
- [ ] Publish Visual Studio Marketplace listing under an authenticated, authorized `inetconnector` publisher. Dependency: publisher account and registry credential are not configured in the current environment. Acceptance: public listing resolves to the released extension.

## Permanent completion rules / Dauerhafte Abschlussregeln

Maintain README.md, STATE.md and this list after material scope/branch/PR/CI/release changes. Read architecture/security before code changes. Keep German/English catalogs synchronized. Preserve canonical workspace checks, approvals, bounded execution and >=80.0% coverage. `run_journal.go` remains the only durable active recovery authority; startup stays passive. Mobile authority remains narrower than Desktop. A published VSIX is not proof of Marketplace listing or Claude Code/Antigravity reasoning/service parity. Do not invent success evidence or leave completed tasks checked in this file.
