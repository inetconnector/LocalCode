# EDID Sentinel — public coding challenge

Experimental, isolated branch of LocalCode (not merged into master). This independent EDID/DisplayID fuzzing project also serves as a possible end-to-end coding-agent benchmark; it is NOT a feature request for LocalCode itself.

## Aim
Build a standalone, reproducible, open-source fuzz suite for EDID 1.x, CTA-861, DisplayID v1/v2 and downstream display parsers. Test only isolated processes and instrumented parsers: never inject EDID into live hardware, change system EDID overrides or crash active desktops.

An offline Python MVP has already been prepared: synthetic EDID corpus, deterministic boundary-aware mutation, subprocess execution, signal/sanitizer/timeout detection, JSON evidence and 9 passing unit tests. This branch is the public collaboration point, not yet a standalone repo.

## Technical acceptance criteria
1. Pin libdisplay-info version and toolchain; provide reproducible sanitizer build and Linux container run.
2. Valid EDID base, CTA and DisplayID v1/v2 seeds, with structured field and block checksums.
3. Mutations: lengths versus actual buffer, DTD 12-bit dimensions, pixel clocks, extension counts, truncation, boundary values, field contradictions and compound cases.
4. AFL++ and libFuzzer coverage feedback; benchmark against naive random mutation, report reproducible branch coverage.
5. Crash minimization, replay command, stacktrace-based deduplication; never confuse normal validation rejects with crashes.
6. CI smoke tests and isolated, bounded fuzz campaigns; cross-platform support where feasible.
7. Confidential upstream vulnerability disclosure and regression tests. No CVE claims without verified new findings.

## Starter C harness
See native/fuzz_libdisplay_info.c. Example, assuming suitably instrumented libdisplay-info itself:

    clang -g -O1 -fsanitize=fuzzer,address,undefined native/fuzz_libdisplay_info.c $(pkg-config --cflags --libs libdisplay-info) -o fuzz_libdisplay_info
    ./fuzz_libdisplay_info corpus -max_len=4096 -timeout=2

Instrumenting only the harness DOES NOT instrument a prebuilt libdisplay-info shared library. Use an isolated build of the target with compatible sanitizer/coverage flags.

## Research context
Upstream libdisplay-info already documents AFL fuzzing; NCC Group published a prior EDIDFuzzer. CVE-2026-112271 and CVE-2026-112272 were publicly disclosed on 2026-10-19; these are existing vulnerabilities, not Sentinel discoveries. At publication, no real parser fuzz campaign or new exploit has been verified.

## First contributor tasks
- Build reproducible full instrumented target and CI smoke workflow.
- Add valid DisplayID v1/v2 sample builders and structural mutators.
- Add AFL++ dictionaries and benchmark input coverage.
- Add reproducible minimized testcase and crash clustering.
- Prepare upstream patch workflow and confidential vendor contact.

## Ethical testing
Run only on software and computers you are authorized to test, in a VM/container with limits. Keep new findings private until vendor coordination.

## References
- https://gitlab.freedesktop.org/emersion/libdisplay-info
- https://emersion.pages.freedesktop.org/libdisplay-info/libdisplay-info/info.h.html
- https://github.com/nccgroup/EDIDFuzzer
- https://www.cve.org/CVERecord?id=CVE-2026-112271
- https://www.cve.org/CVERecord?id=CVE-2026-112272

Project initiated by inetconnector, 22 October 2026.
