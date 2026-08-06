# LocalCode 6.4.1

## Windows startup and Ollama installer fixes

### Fixed

- Replaced the complex UTF-8 `BUILD.bat` implementation with a small ASCII/CRLF wrapper and a dedicated PowerShell build driver.
- Normalized every Windows batch launcher to ASCII with CRLF line endings so `cmd.exe` no longer interprets fragments such as `VERSION`, `LC_LANG`, or localized text as commands.
- Added a packaging regression test that rejects BOMs, non-ASCII bytes, and bare LF line endings in all `.bat` launchers.
- Increased the guarded Ollama installer download limit from 1 GiB to 4 GiB. This covers the observed official installer size of 1,563,278,432 bytes while retaining a finite safety ceiling.
- Added live Ollama-installer download progress to the startup splash, including transferred and total MiB/GiB.
- Removed stale `.part` files before retrying and kept atomic replacement of completed managed downloads.
- Added response-header timeout handling without imposing the former ten-minute total transfer timeout on a multi-gigabyte installer.
- Added tests for the installer size policy, progress reporting, partial-file cleanup, and human-readable byte formatting.
- Added dedicated `scripts/build.ps1` and `scripts/needs-build.ps1` drivers so batch launchers stay small, ASCII-only, and reliably parsed by `cmd.exe`.
- Added `scripts/build.ps1` to the startup freshness check so changes to the native build driver force a rebuild.
- Made clean start, diagnostics, and project-root reset delegate to the same current-build checks instead of running potentially stale binaries.
- Fixed project-root reset writing a UTF-8 BOM that Go's JSON decoder could reject. The reset script now writes UTF-8 without BOM, and LocalCode also accepts legacy BOM-prefixed configuration files.
- Deletes the multi-gigabyte Ollama installer after each installation attempt instead of leaving it in the LocalCode download cache.
- Clears inherited `GOOS`, `GOARCH`, and `CGO_ENABLED` before tests so a user's shell environment cannot accidentally cross-compile the test phase.

### Windows packaging

All `.bat` files are intentionally ASCII and CRLF encoded. `scripts/install-go.ps1` is UTF-8 with BOM and CRLF for Windows PowerShell 5.1 compatibility.


## Final verification

- 189 Go test functions; normal tests and `go vet` pass.
- Race detector passes across four complete partitions.
- Randomized test orders 11, 22, and 33 pass.
- Exact statement coverage: 6,576 / 8,201 = 80.185343%.
- Browser UI E2E: 37 mocked API requests pass.
- Final Windows GUI, debug, and test executables cross-compile as PE32+ x86-64 files.
- Native Windows batch/PowerShell execution remains intentionally required on first start through `dist\REBUILD-NATIVE.txt`.
