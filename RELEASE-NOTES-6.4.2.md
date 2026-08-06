# LocalCode 6.4.2

## Windows native build and test isolation

- The native build no longer sets `LOCALCODE_CONFIG_HOME`, `LOCALCODE_CACHE_HOME`, or `LOCALCODE_USER_HOME` around the Go test process. Those variables intentionally have highest priority and previously shadowed `t.Setenv` in Windows tests.
- The build now isolates tests with temporary XDG, profile, AppData, and LocalAppData directories while leaving per-test overrides effective.
- Tests for project-root repair and legacy LocalCode data migration now explicitly replace the highest-priority directory overrides.
- Tests that require a missing Aider executable isolate `PATH`, so a real user installation cannot change the expected result.
- Coding-engine installation tests use the dedicated automatic-setup download permission rather than the unrelated web/agent network setting.

## MCP and coding-engine fixes

- MCP file resources now use canonical `file:///C:/...` URIs in tests and agent actions.
- The parser also accepts the common noncanonical Windows form `file://C:/...` without losing the drive letter and preserves UNC authorities.
- Claude Code and OpenCode version/authentication probes no longer use a possibly nonexistent LocalCode configuration directory as their working directory. This prevented false “engine missing” results and approval waits on fresh or isolated profiles.

## Verification

- All reported Windows failures have dedicated regression coverage.
- Normal tests, `go vet`, randomized-order tests, race-detector groups, coverage, Windows cross-compilation, UI checks, archive safety, and manifest verification are performed before packaging.
