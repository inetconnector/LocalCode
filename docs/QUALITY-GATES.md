# LocalCode quality gates

LocalCode treats the Windows `Quality` workflow as a merge gate for production changes. The gate is intentionally strict and is not lowered to make a change pass.

## Required checks

Every pull request to `master` must pass, on Windows:

1. Go 1.25.13 toolchain setup.
2. `gofmt` cleanliness.
3. `go vet ./...`.
4. Frontend JavaScript syntax checks for the standalone script and inline scripts in both desktop and remote UIs.
5. `govulncheck` against the Go codebase.
6. Real loopback full-stack HTTP integration covering the desktop and remote server boundary.
7. The complete Go test suite.
8. The Go race detector over the complete test suite.
9. Statement coverage of at least **80.0%**. The threshold is a floor, not a target to be reduced when tests are missing.
10. Native Windows GUI and diagnostics builds.
11. `git diff --check`.

The workflow's third-party GitHub Actions are pinned to immutable commit SHAs. Release builds use the same Go toolchain version and add checksums, Go build metadata, executable attestations, and GitHub Release publication.

## Test policy

Coverage tests must exercise observable behavior, validation, policy boundaries, error handling, concurrency, or integration paths. Tests should not execute real installers or external services merely to increase coverage; those paths are covered through controlled local fixtures where practical. Security-sensitive regressions such as sandbox escapes, remote authentication, stream-ticket reuse, config races, subscriber races, MCP startup behavior, and approval classification require dedicated behavior tests.

A failing gate blocks merge. Flaky tests are treated as test-quality defects and should be fixed at the test boundary rather than by weakening production timeouts or security policy.

## Deutsch

Der Windows-Workflow `Quality` ist das verbindliche Merge-Gate für produktive Änderungen. Er prüft Formatierung, Vet, Frontend-Syntax, `govulncheck`, Full-Stack-HTTP, die vollständige Go-Testsuite, den Race Detector, mindestens **80,0 % Statement-Coverage**, native Windows-Builds und `git diff --check`.

Die Coverage-Grenze wird nicht abgesenkt, um einen Branch grün zu bekommen. Tests sollen echtes Verhalten, Validierung, Sicherheitsregeln, Fehlerfälle, Nebenläufigkeit oder Integrationsgrenzen prüfen. Echte Installer und externe Dienste werden nicht nur für Coverage ausgeführt; soweit möglich werden kontrollierte lokale Fixtures verwendet.
