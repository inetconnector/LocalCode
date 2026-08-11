# Research notes / Recherchehinweise

## Deutsch

Die Architektur orientiert sich an öffentlich dokumentierten Agentenmustern: projektbezogene Arbeitsbereiche, fortsetzbare Chats, sichtbare Genehmigungen, kontrollierte Werkzeuge, Worktrees, MCP und Kontextkomprimierung. Die Oberfläche verwendet keine proprietären Assets und behauptet keine vollständige Dienst- oder Modellparität.

Für neue Integrationen sind zuerst Primärquellen zu verwenden: offizielle Herstellerdokumentation, Spezifikationen und Release Notes. Erkenntnisse, Annahmen und technische Grenzen müssen dokumentiert und durch Tests abgesichert werden.

Recherche am 2026-08-11 für OpenCode-/Codex-Parität: Offizielle OpenAI-Dokumentation beschreibt Codex als lokalen CLI-Loop mit Projektanalyse, Dateiedits, lokalen Werkzeugen, Skills/Plugins, Cloud-Aufgaben, MCP, Sandboxing, Approval-Review und Review/Test-Schleifen. Offizielle OpenCode-Dokumentation beschreibt Agenten/Subagenten, Skills, Custom Tools, MCP und fein granulierte Berechtigungen. LocalCode bildet davon lokale Projektwerkzeuge, MCP, Genehmigungen, externe Engine-Integration und Kontextkomprimierung ab; nicht behauptet werden Codex-Cloud-Parität, proprietäre Modellqualität, echte Subagent-Orchestrierung oder ChatGPT-/GitHub-/Slack-/Linear-Cloud-Integrationen. Der aktuelle Paritätsschritt stärkt die lokale Abschluss-Review-Schicht: keine README-only-Implementierungen und Pflichtprüfung nach Code-/App-/Tool-Änderungen.

## English

The architecture follows publicly documented agent patterns: project workspaces, resumable chats, visible approvals, controlled tools, worktrees, MCP, and context compaction. The interface does not use proprietary assets and does not claim full service or model parity.

New integrations must start with primary sources: official vendor documentation, specifications, and release notes. Findings, assumptions, and technical limitations must be documented and covered by tests.

Research on 2026-08-11 for OpenCode/Codex parity: official OpenAI documentation describes Codex as a local CLI loop with project analysis, file edits, local tools, skills/plugins, cloud tasks, MCP, sandboxing, approval review, and review/test loops. Official OpenCode documentation describes agents/subagents, skills, custom tools, MCP, and fine-grained permissions. LocalCode covers local project tools, MCP, approvals, external engine integration, and context compaction; it does not claim Codex cloud parity, proprietary model quality, real subagent orchestration, or ChatGPT/GitHub/Slack/Linear cloud integrations. The current parity step strengthens the local completion-review layer: no README-only implementations and required checks after code/app/tool changes.
