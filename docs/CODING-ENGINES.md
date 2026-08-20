# Coding-agent engines / Coding-Agent-Engines

LocalCode 6.4.4 can switch between four external coding-agent engines from **Settings → Configuration → Coding-agent engine** or directly in the composer next to the model selector. The selection affects multi-file editing, repository analysis, linting, and test-repair actions. Fresh desktop configurations default to **LocalCode native**, which remains available as the internal tool loop and is not a fifth external engine.

LocalCode 6.4.4 kann unter **Einstellungen → Konfiguration → Coding-Agent-Engine** oder direkt in der Eingabeleiste neben dem Modell zwischen vier externen Coding-Agent-Engines umschalten. Die Auswahl gilt für mehrdateilige Bearbeitung, Repository-Analyse, Linting und Testreparatur. Frische Desktop-Konfigurationen verwenden standardmäßig **LocalCode nativ**; diese interne Werkzeugschleife ist keine fünfte externe Engine.

## Comparison / Vergleich

| Engine | Best fit / Geeignet für | Installation by LocalCode | Model / provider |
|---|---|---|---|
| Aider | Local-first work with Ollama; explicit Git-oriented editing / lokale Ollama-Workflows und kontrollierte Git-orientierte Bearbeitung | Pinned `aider-chat==0.86.2` through a managed `uv tool` environment with Python 3.12 | `ollama_chat/<model>` by default; Aider-supported providers may be configured separately |
| Claude Code | Highly agentic cloud coding with Anthropic models / stark agentische Cloud-Bearbeitung mit Anthropic-Modellen | Official native Windows PowerShell installer; `stable`, `latest`, or an exact version/channel | Model alias such as `sonnet`; Anthropic sign-in or supported enterprise/provider setup required |
| OpenCode | Provider-neutral workflows, including local Ollama and cloud providers / provideroffene Workflows einschließlich lokalem Ollama und Cloud-Providern | `opencode-ai` through a LocalCode-managed Node.js/npm prefix | `provider/model`, for example `ollama/qwen2.5-coder:14b` or a configured cloud-provider model |
| Claw Code | Experimental Claw workflows under LocalCode supervision / experimentelle Claw-Workflows unter LocalCode-Aufsicht | LocalCode-managed Claw setup/download flow where enabled | Claw-supported model/provider configuration |

## Common integration / Gemeinsame Integration

For all four external engines LocalCode provides:

- selection in Settings and engine-specific settings panels;
- executable discovery, version check, installation/repair, and status display;
- authentication status and an interactive login terminal where applicable;
- non-interactive execution in the selected project directory;
- repository-map, edit, lint, and test modes;
- process output capture, timeout, cancellation, and exit-code reporting;
- a pre-edit backup and fingerprint-based changed-file detection;
- restoration of the last engine change with hash checks that protect later manual edits;
- supervisor routing through generic `engine_*` actions; legacy `aider_*` actions remain compatible aliases.
- explicitly configured Claude Code and OpenCode executable paths are authoritative; if such a path is missing, LocalCode reports that configuration error instead of falling back to a global executable.

Für alle vier externen Engines bietet LocalCode:

- Auswahl in den Einstellungen und enginespezifische Einstellungsbereiche;
- Erkennung der Programmdatei, Versionsprüfung, Installation/Reparatur und Statusanzeige;
- Anmeldestatus und bei Bedarf ein interaktives Anmeldeterminal;
- nichtinteraktive Ausführung im ausgewählten Projektordner;
- Repository-Analyse, Bearbeitung, Linting und Tests;
- Erfassung der Prozessausgabe, Zeitlimit, Abbruch und Exitcode;
- ein Backup vor bearbeitenden Läufen sowie Fingerprint-Erkennung geänderter Dateien;
- Wiederherstellung der letzten Engine-Änderung mit Hash-Prüfung zum Schutz späterer manueller Änderungen;
- Supervisor-Routing über generische `engine_*`-Aktionen; alte `aider_*`-Aktionen bleiben kompatible Aliase.
- ausdrücklich konfigurierte Claude-Code- und OpenCode-Programmpfade sind maßgeblich; fehlt ein solcher Pfad, meldet LocalCode diesen Konfigurationsfehler, statt auf eine globale Programmdatei auszuweichen.

## Aider

### Setup

When Aider is selected and automatic installation is enabled, LocalCode provisions `uv`, Python 3.12, and the pinned Aider version inside the LocalCode application-data tools directory. It verifies the executable and version before use. Existing explicitly configured Aider executables are respected.

Wenn Aider ausgewählt und die automatische Installation aktiviert ist, richtet LocalCode `uv`, Python 3.12 und die festgelegte Aider-Version im LocalCode-Werkzeugordner ein. Programmdatei und Version werden vor der Nutzung geprüft. Ein ausdrücklich konfigurierter Aider-Pfad hat Vorrang.

### Model

An empty Aider main-model field follows the model selected in LocalCode. Ollama models are passed as `ollama_chat/<model>`. Architect/editor models, edit formats, repository-map token budget, lint/test commands, Git use, and automatic commits remain configurable.

Ein leeres Aider-Hauptmodell verwendet das in LocalCode ausgewählte Modell. Ollama-Modelle werden als `ollama_chat/<modell>` übergeben. Architect-/Editor-Modelle, Edit-Formate, Repository-Map-Tokenbudget, Lint-/Testbefehle, Git-Nutzung und automatische Commits bleiben konfigurierbar.

## Claude Code

### Setup and authentication / Einrichtung und Anmeldung

On native Windows LocalCode uses Anthropic's official PowerShell installer. The configured channel can be `stable`, `latest`, or an exact supported version. After installation, LocalCode verifies `claude --version` and checks `claude auth status --text`. The **Sign in** button opens an interactive terminal for `claude auth login`.

Unter nativem Windows verwendet LocalCode Anthropics offiziellen PowerShell-Installer. Als Kanal können `stable`, `latest` oder eine konkret unterstützte Version gewählt werden. Anschließend prüft LocalCode `claude --version` und `claude auth status --text`. **Anmelden** öffnet ein interaktives Terminal für `claude auth login`.

### Execution / Ausführung

LocalCode runs Claude Code non-interactively through `claude -p`, supplies the selected model, maximum turn count, permission mode, a LocalCode session name, and an appended system instruction. The supported LocalCode permission choices are normalized to safe documented modes. `bypassPermissions` is deliberately rejected and is not exposed in the UI.

LocalCode startet Claude Code nichtinteraktiv über `claude -p` und übergibt Modell, maximale Schrittzahl, Berechtigungsmodus, einen LocalCode-Sitzungsnamen und eine ergänzende Systemanweisung. Die in LocalCode angebotenen Berechtigungsmodi werden auf dokumentierte sichere Werte normalisiert. `bypassPermissions` wird absichtlich abgelehnt und nicht in der Oberfläche angeboten.

### Limits / Grenzen

Claude Code uses Anthropic services unless the user has configured a supported alternative enterprise/provider route. Credentials are managed by Claude Code, not stored by LocalCode. Native Windows does not provide the same OS-level isolation as a dedicated VM or WSL sandbox.

Claude Code verwendet Anthropic-Dienste, sofern keine unterstützte alternative Enterprise-/Provider-Konfiguration eingerichtet wurde. Zugangsdaten verwaltet Claude Code; LocalCode speichert sie nicht. Natives Windows bietet nicht dieselbe Betriebssystemisolation wie eine dedizierte VM oder WSL-Sandbox.

## OpenCode

### Setup and authentication / Einrichtung und Anmeldung

LocalCode installs `opencode-ai` into a user-local npm prefix. If no suitable Node.js/npm exists, LocalCode can provision its managed Node.js LTS runtime first. `opencode --version` verifies installation. `opencode auth list` reports configured provider credentials; **Sign in** opens `opencode auth login` interactively.

LocalCode installiert `opencode-ai` in einen benutzerlokalen npm-Präfix. Fehlt Node.js/npm, kann LocalCode zuerst seine verwaltete Node.js-LTS-Laufzeit einrichten. `opencode --version` prüft die Installation. `opencode auth list` zeigt konfigurierte Provider-Zugänge; **Anmelden** öffnet interaktiv `opencode auth login`.

### Models and Ollama / Modelle und Ollama

The model field accepts `provider/model`. If it is empty, LocalCode derives `ollama/<selected LocalCode model>`. For `ollama/...`, LocalCode injects an official process-scoped `OPENCODE_CONFIG_CONTENT` provider definition using `@ai-sdk/openai-compatible` and the configured Ollama `/v1` endpoint. The user's global or project OpenCode configuration is not overwritten.

Das Modellfeld akzeptiert `provider/modell`. Bleibt es leer, verwendet LocalCode `ollama/<in LocalCode ausgewähltes Modell>`. Für `ollama/...` übergibt LocalCode über die offizielle prozessbezogene Variable `OPENCODE_CONFIG_CONTENT` eine Providerdefinition mit `@ai-sdk/openai-compatible` und dem konfigurierten Ollama-`/v1`-Endpunkt. Globale oder projektbezogene OpenCode-Konfigurationen werden nicht überschrieben.

### Execution and permissions / Ausführung und Berechtigungen

LocalCode uses `opencode run`, the configured agent (default `build`), model, output format, and optionally `--auto`. OpenCode's own permission configuration remains authoritative inside the external process. Disabling **Apply changes automatically** omits `--auto`.

LocalCode verwendet `opencode run`, den konfigurierten Agenten (Standard `build`), das Modell, das Ausgabeformat und optional `--auto`. Innerhalb des externen Prozesses bleibt OpenCodes eigene Berechtigungskonfiguration maßgeblich. Wird **Änderungen automatisch anwenden** abgeschaltet, entfällt `--auto`.

## Security boundary / Sicherheitsgrenze

LocalCode requests approval before launching an external editing engine when required by the LocalCode approval policy, creates a backup, chooses the working directory, and controls process lifetime. After the process starts, the external engine can use the permissions and tools granted by its own configuration. LocalCode's project path checks do not constitute an operating-system sandbox around an independently executed CLI.

LocalCode fordert entsprechend seiner Genehmigungsregeln vor dem Start einer externen Bearbeitungs-Engine eine Freigabe an, erstellt ein Backup, legt den Arbeitsordner fest und kontrolliert die Prozesslaufzeit. Nach dem Start kann die externe Engine die durch ihre eigene Konfiguration erlaubten Werkzeuge und Rechte verwenden. LocalCodes Projektpfadprüfungen sind keine Betriebssystem-Sandbox um eine separat ausgeführte CLI.

Recommended precautions / Empfohlene Vorsichtsmaßnahmen:

- keep projects under version control and review diffs;
- do not expose unrelated secrets through project files or inherited environment variables;
- use conservative permission modes for unfamiliar repositories;
- prefer WSL/VM/container isolation for highly sensitive projects;
- test restore behavior before relying on backups as the only recovery mechanism.

## Upstream documentation / Upstream-Dokumentation

- Aider installation: https://aider.chat/docs/install.html
- Aider with Ollama: https://aider.chat/docs/llms/ollama.html
- Claude Code setup: https://docs.anthropic.com/en/docs/claude-code/getting-started
- Claude Code CLI: https://docs.anthropic.com/en/docs/claude-code/cli-usage
- OpenCode introduction/install: https://opencode.ai/docs
- OpenCode CLI: https://opencode.ai/docs/cli
- OpenCode providers/Ollama: https://opencode.ai/docs/providers
