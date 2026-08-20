# LocalCode engine benchmarks

This directory documents the reproducible benchmark harness exposed by `src/cmd/localcode-bench`.

## Fair-run contract

Each benchmark run:

1. resolves `base_ref` to an immutable Git commit;
2. creates a fresh detached Git worktree from that commit;
3. injects the same task, model, base commit and worktree through explicit environment variables;
4. runs the configured engine command without a shell;
5. executes declared syntax/lint/build/test/hidden checks;
6. measures changed files/lines and changes outside the task allow-list;
7. optionally imports adapter metrics for turns, tool calls, tokens, failed patches, retries, compactions and human intervention;
8. removes the worktree unless `keep_worktree` is enabled.

The source repository is never used as the engine's working directory. This prevents one engine run from contaminating the next.

## Running

From `src`:

```text
go run ./cmd/localcode-bench -manifest ../benchmarks/my-task.json -out ../benchmarks/results/native.json
```

Run the same manifest once per engine with only `engine`, `engine_command` and engine-specific adapter environment changed. Keep `repository`, `base_ref`, `task`, `model`, setup and checks identical for LocalCode native, Aider, OpenCode and Claw Code.

## Claw Code adapter

`src/cmd/localcode-bench-claw` is the LocalCode adapter for reproducible Claw Code runs. Build it before benchmarking; the adapter never downloads, installs or upgrades Claw during a measured run.

From `src` on Windows, for example:

```text
go build -o ../benchmarks/bin/localcode-bench-claw.exe ./cmd/localcode-bench-claw
```

A Claw manifest points `engine_command` at that adapter and supplies the exact Claw executable under test:

```json
{
  "engine": "claw-code",
  "model": "qwen2.5-coder:14b",
  "engine_command": ["C:\\bench-tools\\localcode-bench-claw.exe"],
  "environment": {
    "LOCALCODE_BENCH_CLAW": "C:\\LocalCode\\tools\\claw-code\\bin\\claw.exe",
    "LOCALCODE_BENCH_OLLAMA_HOST": "http://127.0.0.1:11434"
  }
}
```

The harness itself supplies `LOCALCODE_BENCH_TASK`, `LOCALCODE_BENCH_MODEL`, `LOCALCODE_BENCH_WORKTREE`, `LOCALCODE_BENCH_ENGINE` and `LOCALCODE_BENCH_BASE`. The Claw adapter refuses a missing or relative Claw executable path, refuses to run outside the isolated benchmark worktree, and always invokes Claw with JSON output and `workspace-write`. It never requests `danger-full-access`.

For local-first fairness, the adapter sets `OLLAMA_HOST` only in the Claw subprocess and removes ambient OpenAI, Anthropic, xAI and DashScope credentials/base URLs plus Claw model/permission overrides. This keeps an ostensibly local benchmark from silently drifting to a cloud provider. Use the same Ollama endpoint, model, quantization and context settings for every engine being compared.

The adapter currently does not manufacture optional self-reported token/tool counters from unstable output fields. Build/test/hidden-test success, runtime and Git diff metrics remain measured independently by the harness. Add adapter metrics only when the engine exposes a stable machine-readable contract for them.

## Manifest example

```json
{
  "version": 1,
  "name": "go-api-callsite-update",
  "repository": "../fixture-repo",
  "base_ref": "benchmark-base",
  "task": "Change the Parser API and update all call sites. Add or update tests.",
  "engine": "localcode-native",
  "model": "qwen2.5-coder:14b",
  "engine_command": [
    "path-to-engine-adapter",
    "--worktree", "${WORKTREE}",
    "--task", "${TASK}",
    "--model", "${MODEL}"
  ],
  "allowed_paths": ["parser", "cmd", "tests"],
  "checks": [
    {
      "name": "go-test",
      "kind": "test",
      "command": ["go", "test", "./..."],
      "required": true,
      "timeout_seconds": 300
    },
    {
      "name": "hidden-contract",
      "kind": "hidden",
      "command": ["path-to-hidden-checker", "${WORKTREE}"],
      "required": true,
      "timeout_seconds": 120
    }
  ],
  "metrics_file": ".localcode-benchmark-metrics.json",
  "timeout_seconds": 1200
}
```

`${WORKTREE}`, `${TASK}`, `${MODEL}`, `${ENGINE}` and `${BASE}` are expanded as individual process arguments, not concatenated into a shell command. Shell metacharacters in a task therefore do not become executable syntax.

## Adapter metrics

An engine adapter may write `metrics_file` inside the isolated worktree:

```json
{
  "agent_turns": 7,
  "tool_calls": 18,
  "input_tokens": 18400,
  "output_tokens": 3200,
  "failed_patches": 1,
  "retries": 2,
  "compactions": 1,
  "human_intervention": 0
}
```

These counters are optional. Build/test success and Git diff metrics are measured independently by the harness and cannot be self-reported by the engine.

## Recommended benchmark set

Use multiple immutable fixtures, including:

- single Go bug fix;
- new feature plus tests;
- multi-file refactor;
- public API change with all call sites;
- failing-test repair;
- race-condition repair;
- build-error repair;
- frontend/backend coordinated change;
- new file plus tests;
- large-repository navigation task.

Do not tune prompts or hidden checks separately per engine. The purpose is to measure the native agent architecture, not to hand-optimize individual runs.
