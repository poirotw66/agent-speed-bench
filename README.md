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
| `codex` | `codex exec --json --ephemeral --yolo` | From `turn.completed` | Command/MCP/search event receipt intervals |
| `claude` | `claude --print --output-format stream-json --verbose --include-partial-messages --dangerously-skip-permissions` | Authoritative final `result.usage` | `tool_use` to corresponding `tool_result` |
| `cursor` | `agent --print --output-format stream-json --stream-partial-output --yolo --sandbox disabled` | Final `result.usage` camelCase counts when supplied | `tool_call` start/completion |
| `agy` | `agy -p PROMPT --output-format stream-json --dangerously-skip-permissions` | Authoritative `result.usage`, including thinking | Tool step ACTIVE/DONE receipt intervals |
| `generic` | Explicit executable and argv | Optional canonical JSONL | Optional canonical JSONL |
| `demo` | Same binary's offline fixture mode | Synthetic | Synthetic |

Built-in coding CLIs use the requested YOLO policy: Codex and Cursor use `--yolo`, and agy/Claude use `--dangerously-skip-permissions`. Cursor additionally sets `--sandbox disabled` so inherited settings do not select a different policy. Config isolation does not add conflicting sandbox/auto-review flags. Each run records its adapter-specific permission policy; old sandbox results remain historical and are not pooled with YOLO results. Cursor workspace trust remains an independent setting: `trust_workspace: true` adds `--trust`. Generic commands keep their explicit argv; demo and response-only API adapters have no coding CLI permission mode.

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

`usage_reported` adds per-turn counts; `usage_total` replaces them with authoritative totals. `agent_error` marks a failed attempt, including when the process exits 0. Structured nested errors retain the actionable message, category, code, HTTP status when present, retryability, and scope. Nonretryable model configuration or authentication errors stop future jobs for that agent; remaining jobs are persisted as `skipped`, while other agents continue. Jobs already running may finish. Transient service failures do not stop later repeats. `agent_unavailable`, `service_error`, `infrastructure_error`, `telemetry_error`, `canceled`, and `skipped` are ungraded; manifest counts distinguish planned, started, and skipped jobs. Generic adapters do not require `agent_completed`, since ordinary programs may only print text. Native adapters require their known terminal event. `assistant_output.timing_basis` may identify `text_delta_receipt` or `complete_message_receipt`; unspecified output uses `stdout_line_receipt`. Optional `agent_metadata` events report `model`, `reasoning_effort`, and `service_tier` observations. Supplied run IDs and timestamps are replaced with runner-owned values. Unknown event types remain available in `raw.jsonl` without being treated as actions.

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
    assistant.partial.txt # Available failed/timeout/canceled transcript
    verify.log       # When a command verifier runs
    run.json
```

SQLite stores one immutable record per run plus indexed scalar metrics, allowing historical queries. All raw events remain in JSONL for future analysis. Reports can be regenerated from SQLite without rerunning agents. Legacy tool-latency values are relabeled as receipt intervals in reports without altering historical records; legacy TTFA basis remains unspecified. Historical error grades cannot be recovered from absent structured evidence. Interruptions preserve completed/dispatched attempts; jobs not yet dispatched have no run records. Inspect the manifest config and observed record count for an interrupted matrix. An abrupt OS kill can leave a partial attempt or temporary workspace. Schema 3 experiments support explicit missing-job recovery as described below.

Artifacts are local and ignored by Git. New files use private permissions. Raw CLI output and config snapshots can contain private task contents; the tool does not redact or upload them. Prepared workspaces are removed after each attempt, and optional allowlisted source/patch capture preserves submissions for later inspection.

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

## Measurement profiles

The implementation plan and scope boundaries are in [measurement-remediation-plan.md](docs/measurement-remediation-plan.md).

- `benchmarks/output-lengths.yaml`: identical short (100), medium (500), and long (2,000) integer sequences across seven settings, one warmup per agent/case, ten measured repeats, serial execution. This plans 21 warmups and 210 measured model calls; review the matrix before running it.
- `benchmarks/go-engineering.yaml`: three **synthetic** Go debugging fixtures, three measured repeats across seven settings (63 calls). They test upper-bound clamping, stable deduplication including zero, and inclusive range endpoints. They are not upstream real-repository cases or HarnessBench parity.

```sh
make check-fixtures
./bin/agentspeedbench run -jobs 1 benchmarks/output-lengths.yaml
./bin/agentspeedbench run -jobs 1 benchmarks/go-engineering.yaml
```

Engineering fixtures require Python 3 and Go 1.26+ on PATH. `make check` includes their quality gates: every broken version must fail core scoring, and every fixed version must pass core and regression scoring. Trusted tests remain outside the agent workspace. Each scoring layer copies only `candidate.go` into a separate temporary module with trusted test/module files and network dependency lookup disabled. Candidate tests and go.mod cannot replace the grader. This is not a host security sandbox; submitted Go code executes locally. The runner retains explicitly allowlisted candidate source under `candidate/` with a `.txt` suffix (to avoid compiling artifacts as project code) and records each layer's result. `verify.core_tests` and `verify.regression_tests` are command/argv lists; all configured checks must pass. Each command uses the verifier timeout. Existing `verify.command` still works.

`warmup_repeats` defaults to zero (range 0–100). All warmup jobs finish before measured dispatch. Warmups use the same case and verification, are retained in raw artifacts and SQLite with `warmup: true`, and are excluded from aggregate statistics. Their planned/started/skipped counts are separate in the manifest. An unavailable agent discovered during warmup still stops that agent's future jobs. Warmup does not guarantee a warm server or equal cache state.

New receipt metrics separate the last complete assistant/final-result receipt (`answer_complete_seconds`), successful terminal event receipt (`terminal_receipt_seconds`), and terminal-to-process-exit gap. These are runner observations, not model execution timestamps; missing terminal evidence stays null. Effective Unicode code-point characters/s uses the authoritative transcript (including whitespace) divided by process wall time. It complements vendor tokens/s but does not establish pure generation speed. Samples with fewer than ten completed measured runs are marked descriptive in reports.

Codex reasoning and cache-write aliases are now parsed from native usage. Historical rows are not backfilled or rewritten. API streaming measurements and broader real-repository case curation remain follow-up work. A streaming receive interval would itself need to be labeled as a client observation; generation/model-active TPS remain unknown in these CLI profiles.


## Real Go case and Codex configuration isolation

`benchmarks/real-go.yaml` exercises [lazygit PR #5495](https://github.com/jesseduffield/lazygit/pull/5495), selected with reference to [HarnessBench](https://github.com/nyosegawa/harness-bench). The pinned base is `8f258a3650cef809b911df24881712bc6b5d96bd`; the upstream fix is `38dd035e289dd71ad16fb0caa34525ad03460d21`. Upstream lazygit is MIT licensed. Our independently written external tests check owner casing, exact branch matching, and different owners; regression also runs the existing upstream PR-map tests. This is one focused real bug, not a representative engineering suite.

```sh
# Requires Python 3.9+, Git, and Go 1.26+ on PATH.
python3 scripts/prepare-lazygit.py
./bin/agentspeedbench doctor benchmarks/real-go.yaml
./bin/agentspeedbench run benchmarks/real-go.yaml
```

Preparation downloads the pinned repository into ignored `runs/repos/lazygit` and requires the broken base to fail the named core test, base regression to pass, and the fixed source to pass both layers. Grading reconstructs the pinned `go.mod`, `go.sum`, package tree, and vendor tree in a temporary directory, then overlays **only** `pkg/commands/git_commands/github.go` and external tests. Candidate test/module/dependency changes are ignored. Each layer is bounded, uses vendored dependencies with module lookup disabled, and leaves the submitted source in the run artifacts. This is not an OS/network sandbox or a full repository test suite.

Optional `repo.fresh_history: true` removes upstream Git objects and remotes from the disposable clone and creates a single base commit. The run's `commit` remains the original upstream SHA and the manifest records the policy. The source repository remains intact. It does not remove repository instructions or prevent an agent from accessing other host files.

Optional `isolate_config: true` now supports Codex, Cursor, and agy. It creates a private temporary HOME and copies only the supported login data. Codex additionally ignores user config/rules and disables memories, plugins, apps, browser/computer use and project instruction discovery. Coding CLI calls use the YOLO policy above independently of config isolation; no sandbox permission settings are generated in the temporary home. Vendor permission systems are recorded separately and are not equivalent. Cursor may read its existing macOS Keychain login into a temporary private credential file; agy reuses its existing OAuth token. This does not repair or change system Keychain settings. Temporary credentials are removed on normal cleanup; abrupt termination may leave the private temporary directory.

Optional case-level `strip_instructions: true` removes known agent instruction files and configuration directories from disposable workspaces without traversing symlinks. This is a versioned allowlist, not complete host, network, managed-policy, environment or server-cache isolation. Both options default to false. Use compatible CLIs; older versions may reject the isolation flags.

## Controlled comparisons and retained submissions

`benchmarks/controlled-output.yaml` schedules all seven requested settings, three output lengths, one warmup per setting/case, and ten measured repeats, serially. `benchmarks/controlled-real-go.yaml` schedules three owner-casing attempts per setting. Run these profiles separately to avoid competing local work. A quota or authentication blocker stops future calls for that setting and leaves them ungraded; an incomplete matrix must not be advertised as a completed comparison.

`go_cache: cold` creates independent empty Go caches for each attempt. `go_cache: warm` requires trusted `cache_warmup` commands against the pinned baseline before starting the agent clock. Both policies use vendored dependencies, disable module lookup and automatic toolchain downloads, and record preparation time separately. They do not establish equal server cache state. Existing profiles retain their inherited cache behavior.

Two more authentic cases cover commit-message whitespace ([PR #5528](https://github.com/jesseduffield/lazygit/pull/5528)) and batched branch divergence ([PR #5536](https://github.com/jesseduffield/lazygit/pull/5536)). The whitespace submission allows six functional source files; external grading focuses on splitting/co-author behavior rather than the full interactive UI. Pinned commits, source allowlists and test packages are in `benchmarks/real-go/cases.json`.

```sh
python3 scripts/prepare-real-go.py
./bin/agentspeedbench run benchmarks/real-go-extended.yaml
# Replace ATTEMPT_DIR with the artifact directory of one whitespace attempt.
python3 scripts/verify-real-go.py whitespace core ATTEMPT_DIR/candidate --retained
python3 scripts/verify-real-go.py whitespace regression ATTEMPT_DIR/candidate --retained
```

`retain_patch: true` requires a repository and `retain_files`. It records `candidate.patch.txt` for the allowlisted submitted paths, including new files, plus `source_manifest.json` with SHA-256 hashes and byte counts. Retained regrading checks those hashes and reconstructs the trusted baseline/dependencies; it does not trust candidate tests. These artifacts contain submitted source, so inspect them before sharing.

Reports separate cache, instruction, isolation and permission policies and show preparation median, wall-time quartiles and range. Quartiles/ranges describe the sample, not confidence intervals. Explicit historical quota evidence is interpreted as unavailable/ungraded in the report without rewriting stored records. Quota and infrastructure failures are excluded from wall-speed samples. An explicit agy headless auto-denial is unavailable/ungraded even when the CLI exits zero; see the [official headless permission behavior](https://www.antigravity.google/docs/cli/headless/). Manifests are atomically checkpointed after each recorded attempt; interrupted experiments require explicit `resume`.

## Response-only API streaming

The `openai-responses` adapter relays HTTP SSE events and has no coding tools. It requires an explicit model, an environment-variable name for authentication, and an output-token limit. Example agent configuration (replace the model with one available to your API account):

```yaml
name: api-response
adapter: openai-responses
model: YOUR_API_MODEL
api:
  endpoint: https://api.openai.com/v1/responses
  key_env: OPENAI_API_KEY
  max_output_tokens: 2048
```

The key is read from the environment and is not written to configuration or artifacts. Requests use `stream: true`, `store: false`, and no automatic retries. HTTPS is required except loopback HTTP for offline tests; redirects are rejected. Raw provider output is retained, so prompts and responses should be suitable for local storage. Provider errors are classified without retaining HTTP error bodies.

The first-to-last SSE delta receipt interval and Unicode characters/s exclude the first chunk from the numerator. They are client relay observations affected by buffering, not decoding or model-active TPS. Native total output tokens may include reasoning; accounting is labeled separately. Missing usage or a single delta leaves unsupported rates unknown. Live API testing still requires a chosen provider/model, authentication environment variable and spending limit; no live API calls have been made for this increment. See the [official streaming guide](https://developers.openai.com/api/docs/guides/streaming-responses).

Current local evidence: isolated short-response smoke succeeded for all seven settings after correcting Cursor credential reuse; agy completed 50 calls without Keychain/authentication diagnostics in the interrupted formal matrix. The cache-controlled owner-casing smoke passed both scoring layers. Both added real cases passed base-fails/fixed-passes gates. These checks validate the workflow, not a complete speed ranking or system Keychain repair.

## Reliability, failure evidence and recovery

The reliability table groups by experiment, agent and case, independently of TTFA and token-accounting groups. Attempts include every measured record, including skipped and canceled jobs; warmups are excluded. Completion rate is completed / attempts. Success rate is passed / graded. Speed tables retain their narrower metric/environment groups. Explicit connection diagnostics are service errors and ungraded; deadlines remain graded failures, while user cancellations are ungraded. Historical network and cancellation evidence is interpreted in reports without rewriting stored records. Missing diagnostics do not establish a failure cause.

Allowlisted source and patch capture runs after completed, failed, timed-out and canceled attempts, before workspace cleanup, with a separate bounded cleanup context. `patch_baseline` identifies the prepared pre-agent commit, so patches retain changes even when the agent commits them. Partial transcripts use `assistant.partial.txt`. Capture failures are recorded in `artifact_errors`; they do not erase the primary failure. Abrupt process kills cannot guarantee capture.

```sh
agentspeedbench resume -db agentspeedbench.db runs/<experiment>
```

Schema 3 manifests record the harness binary SHA-256, build revision/dirty flag when available, resolved configuration hash, CLI binary hashes/versions, and direct verifier/cache-preparation executable hashes. Resume requires matching provenance and an exclusive experiment lock; it reconciles artifact-only completed records into SQLite without replacing existing evidence. It schedules only missing logical jobs, including missing warmups before measured jobs. Already recorded failures, cancellations and skips are not retried. Use a new experiment to retry them or change binaries/settings. Older manifests cannot resume through this command. Preserve the original binary to recover an experiment after upgrading.

Manifest `state: complete` means all planned slots have records, not that every attempt passed. `elapsed_seconds` accumulates active invocation time across resumes rather than continuous calendar time. Hashes do not fingerprint the entire host environment, helper assets invoked by verifier arguments, wrapper dependencies, or service-side caches. Version matching prevents known local changes from silently mixing; it does not establish complete environment equivalence.

Responses HTTP failures retain only recognized machine codes (including quota, authentication, model and rate-limit codes) from a bounded error body. Unknown or malformed bodies become `http_error`; provider messages and bodies are not persisted by the relay. Transport failures are distinct from caller cancellation. No automatic retry or paid API validation is implied.
