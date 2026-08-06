# LocalCode 6.4.3

## Windows test isolation and portability

This release fixes the three failures reported by the native Windows build of 6.4.2.

- Coding-engine setup tests now disable the dedicated `SetupDownloadsEnabled` permission. They can no longer invoke a real Claude Code installer merely because agent/web networking is disabled.
- The project-folder picker is injected per `Server` instance. HTTP tests simulate picker failure, cancellation, and success and never open a real Windows Forms dialog.
- Cross-platform command tests use `echo` instead of the Unix-only `printf` command, so the same assertions run under PowerShell.

## Regression coverage

- The engine setup endpoint verifies the disabled-download error path without touching the network or user installation.
- `/api/browse-root` covers error, cancellation, and successful selection through an in-process fake picker.
- Terminal and agent command execution are exercised with commands understood by both POSIX shells and Windows PowerShell.

## Verification boundary

The complete Go suite, race detector, vet, randomized orders, coverage and UI simulation were executed in the release environment. Windows application and test executables were cross-compiled and inspected as PE32+ files. The release environment cannot natively execute Windows PE or batch files, so `dist/REBUILD-NATIVE.txt` remains and forces the full native Windows build before first launch.
