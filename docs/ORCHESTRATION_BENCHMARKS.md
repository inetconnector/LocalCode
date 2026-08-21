# Orchestration parallelism benchmarks

LocalCode distinguishes **logical task parallelism**, **Scheduler resource capacity**, **executor concurrency**, and **local-model throughput under concurrent clients**. They are not interchangeable.

These benchmarks exist so performance claims are based on reproducible measurements rather than Scheduler configuration alone.

## 1. Deterministic dispatcher benchmark

`BenchmarkScheduledReadOnlyDispatcherParallelism` creates four independent, governed read-only Explorer tasks. All four are logically ready at the start of each run. The same workload is executed with model-inference limits of 1, 2 and 4.

The synthetic executor has a fixed short delay so overlapping execution is observable. The benchmark reports:

- `logical_ready/op` — tasks that are independently ready;
- `model_slots` — configured Scheduler capacity for model inference;
- `peak_executor_inflight` — maximum executor calls actually overlapping;
- `executor_to_ready_ratio` — observed executor overlap divided by logically ready tasks;
- `tasks/op` plus the normal Go benchmark timing/allocation metrics.

Current architecture intentionally calls the scheduled read-only executor synchronously inside the dispatch loop. Therefore higher model-slot limits do **not** by themselves establish concurrent child-model execution. If the dispatcher becomes concurrent later, this benchmark will expose the changed overlap instead of requiring a new measurement format.

From `src`:

```powershell
go test -run '^$' -bench '^BenchmarkScheduledReadOnlyDispatcherParallelism$' -benchmem -count 5
```

Run on the same machine, commit, Go version and power/performance profile when comparing results.

## 2. Opt-in local Ollama throughput benchmark

`TestOllamaConcurrencyBenchmarkOptIn` measures a fixed amount of real local-model work with client concurrency 1, 2 and 4. Each scenario uses the production `OllamaClient.Chat` path and the same exact installed model.

This is a fixed-work test rather than a `testing.B` benchmark because automatic benchmark calibration can accidentally generate many expensive LLM requests.

Safety and reproducibility rules:

- disabled unless `LOCALCODE_BENCH_OLLAMA=1`;
- `LOCALCODE_BENCH_MODEL` must contain the exact name of an already-installed model;
- only loopback Ollama endpoints are accepted;
- the benchmark never calls `EnsureRunning`, `Pull` or any installer;
- no LocalCode configuration or Scheduler limit is changed;
- default workload is four requests per concurrency level;
- default context length is 4096 and default per-request timeout is 180 seconds.

PowerShell example from `src`:

```powershell
$env:LOCALCODE_BENCH_OLLAMA='1'
$env:LOCALCODE_BENCH_MODEL='qwen2.5-coder:14b'
$env:LOCALCODE_BENCH_REQUESTS='4'
$env:LOCALCODE_BENCH_CONTEXT='4096'
go test -run '^TestOllamaConcurrencyBenchmarkOptIn$' -v -count 1
```

Optional bounds:

- `LOCALCODE_BENCH_REQUESTS`: 4..32;
- `LOCALCODE_BENCH_CONTEXT`: 4096..131072;
- `LOCALCODE_BENCH_TIMEOUT_SECONDS`: 10..1800.

Each scenario emits one `ORCHESTRATION_BENCH` JSON object containing wall time, mean and p95 request latency, requests/second, client overlap factor and speedup relative to the concurrency-1 scenario.

## 3. Interpretation boundary

The real benchmark measures **end-to-end throughput under concurrent client requests**. `client_overlap_factor > 1` means HTTP/model requests overlapped in wall-clock time from the client perspective. It does **not** prove simultaneous GPU kernel execution or simultaneous token generation inside Ollama. Ollama may queue, batch or otherwise schedule requests internally.

Accordingly:

- Scheduler `model_slots=4` is a capacity setting, not proof of four concurrent model executions;
- `peak_executor_inflight` measures LocalCode dispatcher overlap;
- Ollama requests/second and speedup measure practical local backend throughput for a fixed workload;
- hardware-level inference concurrency requires backend/GPU-specific profiling and is outside this benchmark contract.

Do not automatically raise Scheduler limits from benchmark output. Resource policy changes require a separate reviewed change with memory/VRAM pressure, cancellation, fairness and stability evidence.
