# AgentSpeedBench

Benchmark coding-agent latency, reported token throughput, tool intervals, and end-to-end correctness with a Go CLI.

[繁體中文](README-tw.md)

This is a runnable first version, inspired by [HarnessBench](https://github.com/nyosegawa/harness-bench). It is an independent implementation, not a compatible rewrite of HarnessBench's case corpus or hidden-test protocol. The first version uses YAML, local repository snapshots, JSONL, SQLite, and a standalone HTML report. No web service or JavaScript runtime is required.

## Start offline

Requires Go 1.26+ to build; benchmark execution supports macOS and Linux.

```sh
go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench
./bin/agentspeedbench run benchmarks/demo.yaml
```

The demo runs six attempts, writes a fixture file, verifies the output and file contents, and produces `runs/<experiment>/report.html`. Its timings and tokens are **synthetic**, useful for testing the harness, not measuring any AI service. A compiled binary includes SQLite and does not require an installed SQLite command.

```sh
./bin/agentspeedbench doctor benchmarks/quick.yaml
./bin/agentspeedbench run -jobs 1 benchmarks/quick.yaml
./bin/agentspeedbench report -out runs/history.html
./bin/agentspeedbench report -experiment EXPERIMENT_ID -out runs/experiment.html
```

`doctor` validates configuration, resolves pinned repository commits, checks executables, and reads CLI versions. It does not test authentication or run model prompts. Real `run` commands use the installed agents' account configuration and can consume quota. Authenticate each CLI separately before running.

Flags must come before the YAML filename. `run` accepts `-out`, `-db`, and `-jobs`; `report` accepts `-out`, `-db`, and `-experiment`. Defaults are `runs/` and `agentspeedbench.db`. Exit code 1 indicates an infrastructure error, an unsuccessful agent attempt, or failed verification. Successful but ungraded attempts do not assert correctness.

## Supported agents

| Adapter | Execution | Token usage | Tool intervals |
| --- | --- | --- | --- |
| `codex` | `codex exec --json --ephemeral --sandbox workspace-write` | From `turn.completed` | Observed command/MCP/search item lifetimes |
| `claude` | `claude --print --output-format stream-json --verbose --include-partial-messages` | Authoritative final `result.usage` | `tool_use` to corresponding `tool_result` |
| `cursor` | `agent --print --output-format stream-json --stream-partial-output` | Unknown | `tool_call` start/completion |
| `generic` | Explicit executable and argv | Optional canonical JSONL | Optional canonical JSONL |
| `demo` | Same binary's offline fixture mode | Synthetic | Synthetic |

Built-in commands preserve each CLI's permission protections. They do not enable permission bypasses, auto-trust, or force execution. Some coding tasks may need agent-specific permission configuration outside this harness; denied work is an observed failed attempt. These defaults are recorded through configuration and capabilities, not assumed to give equal permissions across vendors.

Antigravity is a **custom-command integration**, not a verified built-in adapter. Edit `benchmarks/antigravity.example.yaml` with your installed headless CLI's actual executable and arguments. Plain stdout supports response timing and output assertions; token and tool metrics require normalized events. Unknown structured events are retained in raw logs and ignored by the parser. Malformed structured telemetry fails the attempt rather than silently showing success.

Sources for the supported event shapes:

- [Codex non-interactive JSONL](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Claude Code programmatic streaming](https://code.claude.com/docs/en/headless)
- [Cursor output format and duplicate partial flushes](https://cursor.com/docs/cli/reference/output-format)

Parser fixtures cover these documented shapes. Real authenticated benchmark runs against all three vendors have not been verified as part of initial local development. CLI protocol changes may require adapter updates.

## What the metrics mean

| Metric | Definition and limits |
| --- | --- |
| First stdout line | Launch to the first non-empty complete stdout line received. This includes startup metadata and is **not** token TTFT. |
| TTFA | Launch to the first observed assistant output or tool start. Startup/session metadata and stderr do not qualify. Codex completed messages may be buffered. |
| Effective output tok/s | Agent-reported output tokens / agent process wall seconds. Includes reasoning, waiting, tools, and startup in the denominator. It is not inference speed. |
| Generation tok/s | Unknown: CLI item timestamps do not establish decoding intervals. |
| Model-active tok/s | Unknown: none of these adapters establishes model-active intervals. |
| Tool latency | Runner-observed start/finish interval for matching IDs. Includes harness overhead; missing pairs are excluded and flagged. |
| Wall p50 / p95 | Process launch-to-exit times, including failure and timeout samples; nearest-rank percentiles. Preparation and verification are excluded. |
| Pass / graded | Verification successes / attempts with known outcomes. Successful attempts without a verifier are `ungraded`; failed agent attempts count as failures. |
| Passed / wall hour | `3600 × passed / sum(process wall seconds)`, only when every attempt has a known grade and wall time. Sequential-equivalent, not measured concurrent throughput. |

Missing numeric fields remain JSON `null`, SQL `NULL`, and HTML `unknown`. Explicit zero usage stays zero. Usage counts retain vendor accounting and tokenizer differences; cache counts are separate fields. For Codex, per-turn usage is summed. For Claude, final usage replaces intermediate totals. Failed runs retain available partial evidence; throughput and TTFA aggregates use completed runs with observations. Inspect `summary.json` and per-run metrics for the underlying samples.

Reports aggregate by **experiment, agent, and case**. History does not pool different experiments, which could contain different prompts, models, or agent settings. Pin model identifiers in YAML, pin repo commits, use `jobs: 1` for serial speed comparisons, and compare the manifests. Repeats rotate agent order. A p95 from three or five runs describes a small sample, not a reliable tail estimate. The first version does not attribute a slowdown to model inference, network, server queueing, or orchestration.

## Define a benchmark

```yaml
name: project-repair
repeats: 3
jobs: 1
timeout_seconds: 600
agents:
  - name: codex
    adapter: codex
    model: YOUR_EXPLICIT_MODEL_ID
cases:
  - name: repair-bug
    repo:
      path: /absolute/path/to/local/repository
      commit: YOUR_PINNED_BUGGY_COMMIT
    prompt: |
      Repair the specific bug described here and preserve existing behavior.
    verify:
      command: go
      args: [test, ./...]
      timeout_seconds: 120
```

Every attempt gets a temporary independent clone of the same resolved commit. Dirty and untracked source files are excluded, and the original working tree is preserved. Local repository paths are supported; fetch a remote repository separately. Alternatively, omit `repo` and define `files: {path: content}` for small seeded fixtures, or omit both for an empty workspace. Seed paths cannot escape the workspace or write `.git` metadata. Seeded files are ordinary non-executable files. Repositories and seed files cannot be combined in one case.

`verify.command` executes an explicit argv in the attempt workspace after the agent exits; `output_contains` is an optional substring check on the assistant transcript. Both must pass when configured together. Substring assertions are smoke checks, not semantic correctness tests. If no checks are configured, the grade is unknown. CLI `result` messages supply the final transcript for Claude and Cursor without being counted as new streaming output.

Verifiers, configurations, and repository code must come from trusted sources: they execute local commands. Workspace isolation protects the source checkout from ordinary edits; it is **not** a host security sandbox. Repository tests inside the workspace can be changed by the agent. Use a separate trusted verifier for strong behavioral grading. This version does not provide tamper-proof hidden tests, Docker isolation, or HarnessBench parity.

Config uses a single YAML document, rejects unknown fields, duplicate names and invalid limits, and defaults to one repeat, one job and a 300-second timeout. Case-level timeouts override the global timeout. Verification defaults to 60 seconds. Limits: repeats 1–1000, jobs 1–64, timeouts 1–86400 seconds. Each stdout/stderr line is limited to 8 MiB; exceeding it cancels execution and reports an infrastructure error.

Repository paths and executable paths containing `/` resolve relative to the YAML file. Bare executable names resolve through `PATH`. Verifier arguments resolve in the temporary workspace. Environment values are inherited for CLI authentication but are not copied into manifests. Agent processes are terminated as process groups on timeout or cancellation; a descendant that deliberately leaves its process group is outside this mechanism.

## Add a custom CLI

```yaml
agents:
  - name: custom
    adapter: generic
    command: my-cli
    args: [--prompt, "{prompt}", --model, "{model}"]
    model: explicit-model-id
    version_args: [--version]
```

Arguments are passed directly without shell interpolation. `{prompt}`, `{model}`, and `{workdir}` are substituted within individual arguments, and the prompt is also available on stdin. To use a shell, configure `command: sh` explicitly; shell quoting is then the configuration author's responsibility. `args` are only allowed on generic adapters. Built-ins support overriding the executable via `command` and the model via `model`. Generic version probing is opt-in through `version_args`.

An optional canonical stdout stream is newline-delimited JSON:

```jsonl
{"type":"assistant_output","text":"Working...\n"}
{"type":"tool_started","tool_id":"t1","tool_name":"shell"}
{"type":"tool_finished","tool_id":"t1","tool_name":"shell"}
{"type":"usage_reported","usage":{"input_tokens":120,"output_tokens":30}}
{"type":"agent_completed"}
```

`usage_reported` adds per-turn counts; `usage_total` replaces them with authoritative totals. `agent_error` marks a failed attempt, including when the process exits 0. Generic adapters do not require `agent_completed`, since ordinary programs may only print text. Native adapters require their known terminal event. Supplied run IDs and timestamps are replaced with runner-owned values. Unknown event types remain available in `raw.jsonl` without being treated as actions.

## Artifacts and storage

```text
agentspeedbench.db
runs/<experiment>/
  manifest.json      # Config, CLI paths/versions/capabilities, platform, timing
  summary.json       # All run records and experiment metadata
  report.html        # Offline HTML with escaped untrusted content
  <run-id>/
    raw.jsonl        # Timestamped original stdout/stderr lines
    events.jsonl     # Normalized runner-owned events
    stdout.log
    stderr.log
    assistant.txt    # Completed attempt transcript
    verify.log       # When a command verifier runs
    run.json
```

SQLite stores one immutable record per run plus indexed scalar metrics, allowing historical queries. All raw events remain in JSONL for future analysis. Reports can be regenerated from SQLite without rerunning agents. Interruptions preserve completed/dispatched attempts; jobs not yet dispatched have no run records. Inspect the manifest config and observed record count for an interrupted matrix. An abrupt OS kill can leave a partial experiment or temporary workspace; no crash-resume mechanism is provided.

Artifacts are local and ignored by Git. New files use private permissions. Raw CLI output and config snapshots can contain private task contents; the tool does not redact or upload them. Prepared workspaces are removed after each attempt, so this version retains telemetry and verifier output, not generated patches.

## Development

```sh
make check
make demo
```

`make check` runs formatting checks, `go vet`, race-enabled tests, and a build. Tests cover documented parser traces, Cursor flush deduplication, unknown/zero usage, strict configuration, timestamp provenance, process-tree timeout, verifier failure and timeout, isolated matrices, dirty source preservation, HTML escaping, and SQLite null/filter semantics. CI is configured for macOS and Linux; local checks do not establish hosted CI or live vendor behavior.

```text
cmd/agentspeedbench    CLI and offline demo
internal/benchmark    Strict config and task contracts
internal/adapters     Vendor command/event differences
internal/runner       Scheduling, workspace, processes, collection, grading
internal/telemetry    Normalized events and metrics
internal/storage      SQLite run records and JSON artifacts
internal/report       Statistics and standalone HTML
```

Dependencies are pinned in `go.mod` / `go.sum`: YAML parsing and pure-Go SQLite. Process execution uses the standard library. Future work includes verified Antigravity telemetry, trusted hidden-test grading, patch retention, crash recovery, model interval instrumentation, and a richer trend dashboard.
