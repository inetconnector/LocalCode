# EDID Sentinel local quickstart

Use Python 3.10+ in a disposable VM/container. No Python third-party dependencies are required for the initial Python runner.

From the **experiments/edid-sentinel** directory:

```sh
python -m unittest discover -s tests -v
python -m edidsentinel seeds --output corpus
python -m edidsentinel fuzz --command 'edid-decode {input}' --iterations 1000 --corpus corpus --output findings
```

The tool requires a separately installed target parser, e.g. edid-decode, in PATH. On Windows use command quoting suitable for PowerShell and the executable path. This is a **software parser** harness, not a way to program physical monitors.

Evidence: findings/summary.json and for each candidate crash/timeout findings/crashes or findings/timeouts with a .bin and .json sharing the SHA-256 filename. Safe nonzero parse errors are classified as rejections.

C libFuzzer template: native/fuzz_libdisplay_info.c. Instrument the libdisplay-info target library itself, not only the harness. Pin target version/commit for reproducible reports.

**Limitations:** The initial run used unit tests and a harmless dummy subprocess; actual native parser coverage / newly discovered issues have not been demonstrated. The CTA seed is minimal, and the DisplayID carrier is only a robustness seed, **not** a validated fully conformant DisplayID v1/v2 block.

**Responsible disclosure:** Do not file unpatched reproducer inputs or confidential crash traces on a public issue. First contact the relevant vendor privately.
