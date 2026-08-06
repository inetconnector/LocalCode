# LocalCode 6.3.0 release notes

## Three switchable coding-agent engines

LocalCode can now select **Aider**, **Claude Code**, or **OpenCode** under Settings. The selection is used consistently by repository analysis, multi-file editing, lint, test-repair, installation, status, login, cancellation, backup, and undo workflows. LocalCode native remains available as the internal tool loop.

- Aider retains the pinned, isolated `uv`/Python 3.12 installation and Ollama integration.
- Claude Code uses the official native Windows installer, supports stable/latest/exact channels, validates authentication, and runs non-interactively with bounded turns and a safe permission mode.
- OpenCode is installed through a user-local managed npm prefix, supports cloud providers and local Ollama, and receives a process-scoped Ollama provider configuration without modifying the user's OpenCode files.

## UI and configuration

- New engine selector and separate settings panels for all three external engines.
- Generic status, install/repair, sign-in, repository-analysis test, and undo controls.
- Engine version, executable, authentication state, and errors are visible in Settings and the status bar.
- Configuration schema updated to version 9 with migration-safe defaults.
- Complete German and English translations for the new controls and statuses.

## Execution and recovery

- Generic `engine_edit`, `engine_repo_map`, `engine_lint`, `engine_test`, and `engine_undo` actions route through the selected engine.
- Legacy `aider_*` actions remain compatible aliases.
- External `.cmd` and `.bat` launchers are executed correctly on Windows.
- All editing engines use pre-edit backups, changed-file fingerprints, controlled cancellation, output capture, timeout handling, and guarded restoration.
- Startup auto-setup installs only the selected external engine and pulls its required Ollama models when applicable.

## Security

- Claude Code `bypassPermissions` is rejected and not offered in the UI.
- OpenCode `--auto` can be disabled.
- LocalCode does not store Claude Code or OpenCode provider credentials.
- Documentation now distinguishes LocalCode's pre-launch approval/project checks from a true OS-level sandbox around external CLIs.

## Tests

The release adds deterministic tests for engine normalization, status, authentication, installation commands, command-line construction, Ollama provider injection, backup/undo, API endpoints, UI selection, and startup integration. The final release suite retains at least 80% statement coverage, race-detector verification, randomized test orders, UI/API simulation, translation parity, and Windows-amd64 cross-build validation.
