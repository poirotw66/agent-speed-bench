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
| `codex` | `codex exec --json --ephemeral --sandbox workspace-write` | From `turn.completed` | Command/MCP/search event receipt intervals |
| `claude` | `claude --print --output-format stream-json --verbose --include-partial-messages` | Authoritative final `result.usage` | `tool_use` to corresponding `tool_result` |
| `cursor` | `agent --print --output-format stream-json --stream-partial-output` | Final `result.usage` camelCase counts when supplied | `tool_call` start/completion |
| `agy` | `agy -p PROMPT --output-format stream-json` | Authoritative `result.usage`, including thinking | Tool step ACTIVE/DONE receipt intervals |
| `generic` | Explicit executable and argv | Optional canonical JSONL | Optional canonical JSONL |
| `demo` | Same binary's offline fixture mode | Synthetic | Synthetic |

Built-in commands preserve each CLI's permission protections. Cursor workspace trust defaults to false; `trust_workspace: true` explicitly adds `--trust` for the isolated benchmark workspace. Force and permission-bypass flags are not added. Some coding tasks may need agent-specific permission configuration outside this harness; a recognized workspace trust blocker is unavailable and ungraded. These defaults are recorded through configuration and capabilities, not assumed to give equal permissions across vendors.

Antigravity has a native `agy` adapter. `benchmarks/antigravity.example.yaml` uses an explicit model and effort; installed model availability and authentication are still required. The adapter retains native raw events, streams response deltas, grades the authoritative final response, and requires terminal `SUCCESS`. Step usage is ignored in favor of final totals. Unknown structured events are retained in raw logs and ignored by the parser. Malformed structured telemetry fails the attempt rather than silently showing success.

Sources for the supported event shapes:

- [Codex non-interactive JSONL](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Claude Code programmatic streaming](https://code.claude.com/docs/en/headless)
- [Cursor output format and duplicate partial flushes](https://cursor.com/docs/cli/reference/output-format)
- [Antigravity headless stream-json](https://antigravity.google/docs/cli/headless/)

Parser fixtures cover these documented shapes. Codex has been exercised locally with ChatGPT authentication. Cursor Auto and agy Gemini 3.8 Flash have also been exercised locally; authenticated Claude remains unverified. CLI protocol changes may require adapter updates.

## What the metrics mean

| Metric | Definition and limits |
| --- | --- |
| First stdout line | Launch to the first non-empty complete stdout line received. This includes startup metadata and is **not** token TTFT. |
| TTFA | Launch to the first observed assistant output or tool start. Startup/session metadata and stderr do not qualify. The timing basis is recorded and separate first-text-delta, complete-message, and tool-action receipt times are retained. Codex completed messages may be buffered; these are not token TTFT. |
| Effective output tok/s | Agent-reported output tokens / agent process wall seconds. Includes reasoning, waiting, tools, and startup in the denominator. It is not inference speed. |
| Thinking tokens | Separately reported reasoning tokens when available. agy output tokens include these counts; do not add them to output again. |
| CLI-reported duration | Vendor CLI duration with its field source, separate from process wall time. It does not establish a decoding interval. |
| Generation tok/s | Unknown: CLI item timestamps do not establish decoding intervals. |
| Model-active tok/s | Unknown: none of these adapters establishes model-active intervals. |
| Tool receipt interval | Runner receipt gap for matching start/finish IDs, separate from execution duration. CLI buffering can compress the gap. Missing pairs are excluded and flagged. |
| Actual tool latency | Unknown without authoritative execution timestamps or duration. Existing adapters do not supply this evidence. |
| Wall p50 / p95 | Process launch-to-exit times, including failure and timeout samples; nearest-rank percentiles. Preparation and verification are excluded. |
| Pass / graded | Verification successes / attempts with known outcomes. Successful attempts without a verifier are `ungraded`; task failures count as failures. Agent configuration/authentication, transient service, infrastructure/telemetry failures and skipped attempts are ungraded. |
| Passed / wall hour | `3600 × passed / sum(process wall seconds)`, only when every attempt has a known grade and wall time. Sequential-equivalent, not measured concurrent throughput. |

Missing numeric fields remain JSON `null`, SQL `NULL`, and HTML `unknown`. Explicit zero usage stays zero. Usage counts retain vendor accounting and tokenizer differences; cache counts are separate fields. For Codex, per-turn usage is summed. For Claude, Cursor, and agy, final usage replaces intermediate totals. Thinking and cache-write counts remain separate nullable fields. agy output counts include thinking (`output_token_accounting: includes_thinking`); other accounting remains unknown. Do not treat cross-vendor effective rates as comparable visible-text decoding speeds. CLI-reported duration is stored with its source separately from wall time. Failed runs retain available partial evidence; throughput and TTFA aggregates use completed runs with observations. Inspect `summary.json` and per-run metrics for the underlying samples.

Reports aggregate by **experiment, agent, case, TTFA timing basis, and output token accounting**. History does not pool different experiments, which could contain different prompts, models, or agent settings. Pin model identifiers in YAML, pin repo commits, use `jobs: 1` for serial speed comparisons, and compare the manifests. Repeats rotate agent order. A p95 from three or five runs describes a small sample, not a reliable tail estimate. The first version does not attribute a slowdown to model inference, network, server queueing, or orchestration.

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
    reasoning_effort: high
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

`verify.output_equals` checks exact transcript bytes, including whitespace. `verify.integer_sequence` checks every line in an inclusive range followed by an exact end marker. It accepts LF or CRLF and one optional final newline, rejects missing/duplicate/reordered/extra lines, and is used by `benchmarks/quick.yaml`:

```yaml
verify:
  integer_sequence:
    start: 1
    end: 100
    end_marker: BENCH_DONE
```

`verify.command` executes an explicit argv in the attempt workspace after the agent exits; `output_contains` is an optional substring check on the assistant transcript. All configured checks must pass. Substring assertions are smoke checks, not semantic correctness tests. If no checks are configured, the grade is unknown. CLI `result` messages supply the final transcript for Claude, Cursor, and agy without being counted as new streaming output.

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

Arguments are passed directly without shell interpolation. `{prompt}`, `{model}`, and `{workdir}` are substituted within individual arguments, and the prompt is also available on stdin. To use a shell, configure `command: sh` explicitly; shell quoting is then the configuration author's responsibility. `args` are only allowed on generic adapters. Built-ins support overriding the executable via `command` and the model via `model`. Codex also accepts `reasoning_effort` (`minimal`, `low`, `medium`, `high`, `xhigh`, `max`), passed as an explicit `model_reasoning_effort` CLI config override. Valid efforts depend on the selected model. See [Codex developer settings](https://learn.chatgpt.com/docs/developer-settings). Each run records requested values, allowlisted user-config values, and observed values separately. Config snapshots read only `model`, `model_reasoning_effort`, `service_tier`, and `features.fast_mode` from `$CODEX_HOME/config.toml` or `~/.codex/config.toml`; they do not resolve project, managed, or profile layers. Secrets and other config fields are excluded. Observed settings remain unknown unless CLI events explicitly report them; a successful request alone does not establish the effective model, effort, or service tier. Generic version probing is opt-in through `version_args`.

An optional canonical stdout stream is newline-delimited JSON:

```jsonl
{"type":"assistant_output","text":"Working...\n"}
{"type":"tool_started","tool_id":"t1","tool_name":"shell"}
{"type":"tool_finished","tool_id":"t1","tool_name":"shell"}
{"type":"usage_reported","usage":{"input_tokens":120,"output_tokens":30}}
{"type":"agent_completed"}
```

`usage_reported` adds per-turn counts; `usage_total` replaces them with authoritative totals. `agent_error` marks a failed attempt, including when the process exits 0. Structured nested errors retain the actionable message, category, code, HTTP status when present, retryability, and scope. Nonretryable model configuration or authentication errors stop future jobs for that agent; remaining jobs are persisted as `skipped`, while other agents continue. Jobs already running may finish. Transient service failures do not stop later repeats. `agent_unavailable`, `service_error`, `infrastructure_error`, `telemetry_error`, and `skipped` are ungraded; manifest counts distinguish planned, started, and skipped jobs. Generic adapters do not require `agent_completed`, since ordinary programs may only print text. Native adapters require their known terminal event. `assistant_output.timing_basis` may identify `text_delta_receipt` or `complete_message_receipt`; unspecified output uses `stdout_line_receipt`. Optional `agent_metadata` events report `model`, `reasoning_effort`, and `service_tier` observations. Supplied run IDs and timestamps are replaced with runner-owned values. Unknown event types remain available in `raw.jsonl` without being treated as actions.

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

SQLite stores one immutable record per run plus indexed scalar metrics, allowing historical queries. All raw events remain in JSONL for future analysis. Reports can be regenerated from SQLite without rerunning agents. Legacy tool-latency values are relabeled as receipt intervals in reports without altering historical records; legacy TTFA basis remains unspecified. Historical error grades cannot be recovered from absent structured evidence. Interruptions preserve completed/dispatched attempts; jobs not yet dispatched have no run records. Inspect the manifest config and observed record count for an interrupted matrix. An abrupt OS kill can leave a partial experiment or temporary workspace; no crash-resume mechanism is provided.

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

Dependencies are pinned in `go.mod` / `go.sum`: YAML and TOML parsing and pure-Go SQLite. Process execution uses the standard library. Future work includes broader live tool-event coverage, trusted hidden-test grading, patch retention, crash recovery, model interval instrumentation, and a richer trend dashboard.

## Native comparison configuration

After authenticating Codex, Cursor, and agy, run the supplied comparison:

```sh
./bin/agentspeedbench doctor benchmarks/native-comparison.yaml
./bin/agentspeedbench run -jobs 1 benchmarks/native-comparison.yaml
```

| Agent | Model | Reasoning effort | Requested service tier |
| --- | --- | --- | --- |
| `sol-medium` | `gpt-6.1-sol` | `medium` | `default` |
| `luna-high` | `gpt-6-luna` | `high` | `default` |
| `luna-high-fast` | `gpt-6-luna` | `high` | `fast` |
| `cursor-auto` | `auto` | CLI-managed | Not configured |
| `agy-low` | `gemini-3.8-flash-low` | `low` | Not configured |
| `agy-medium` | `gemini-3.8-flash-medium` | `medium` | Not configured |
| `agy-high` | `gemini-3.8-flash-high` | `high` | Not configured |

`benchmarks/native-comparison.yaml` reproduces the seven model/settings combinations with serial execution and exact sequence verification. Cursor explicitly trusts the harness workspace in this example; review that setting before using repository or seeded-file cases.

Codex accepts `service_tier: default` or `fast`. Fast passes `--config 'service_tier="fast"' --enable fast_mode`; it records a request, not proof of the delivered tier or a guaranteed speed multiplier. See [Codex speed configuration](https://learn.chatgpt.com/docs/agent-configuration/speed?site_variant=chatgpt). agy accepts `reasoning_effort` low/medium/high/xhigh/max through `--effort`; support depends on the model. Observed effort remains unknown unless native metadata explicitly reports it.

A Cursor stderr workspace-trust blocker is used only when the process exits nonzero. It stops future jobs for that agent and retains the reason without counting a task failure. Ordinary stderr warnings do not change a successful result. Already running parallel jobs may finish. Historical run records are not rewritten; rebuild reports to apply display corrections, and rerun to capture newly supported fields.

## Local validation snapshot — 2026-10-07

The native adapters passed a serial regression with one attempt per setting above: **7/7** outputs matched the complete sequence 1–100 followed by `BENCH_DONE`. This validates the tested command and telemetry paths; one sample per setting does not establish a speed ranking. The committed comparison config uses five repeats per setting.

The run used macOS arm64, Go 1.26.8, Codex CLI 0.160.1, Cursor Agent 2026.10.01-e373342, and agy 1.3.1. Cursor reported 262 output tokens and zero cache-write tokens. All three agy settings retained thinking counts and CLI-reported durations separately from wall time. The Fast request succeeded, while the delivered service tier remained unknown because native events did not report it. Cursor Auto reported the Auto selection, not its underlying routed model.

A separate three-repeat run without Cursor workspace trust produced one `agent_unavailable` attempt followed by two `skipped` records, all ungraded. Tests also verify that ordinary stderr warnings do not fail a successful run and that an unavailable agent does not stop other agents.

`make check` passed all 45 top-level tests, including race checks, formatting, vet, and build. A Linux amd64 cross-build also passed. SQLite records matched the seven run artifacts. Live runs covered the response-only case; agy tool pairing was checked with fixtures. Authenticated Claude and hosted CI were not verified in this snapshot. Local reports and raw traces remain Git-ignored.
