# Measurement results

Pinned harness commits, sample sizes (`n`), and stated limitations are kept here. Runtime artifacts under `runs/` are Git-ignored and are not publicly downloadable from this repository.

## Compact measurement profile

`benchmarks/native-output-100-500.yaml` contains seven agent configurations, only the 100/500 cases, three measured repeats, one warmup per agent/case, and `jobs: 1`: **14 warmups + 42 measured calls = 56 planned calls**. Codex and Cursor isolate settings; agy explicitly uses `isolate_config: false` and its native login HOME for macOS Keychain compatibility. agy inherits host settings; the profile does not claim uniform configuration isolation. The historical controlled-output profile remains available with 10 repeats and the 2000 case.

```sh
# Plan only: no CLI/version commands, repository resolution, database or artifacts.
./bin/agentspeedbench run --dry-run benchmarks/native-output-100-500.yaml
# Filter by exact configuration names, not adapter or model names.
./bin/agentspeedbench run --dry-run --agent luna-high,luna-high-fast --case short,medium --repeats 3 benchmarks/native-output-100-500.yaml
# Execute the selected plan (one warmup + three measured calls).
./bin/agentspeedbench run --agent luna-high --case short --repeats 3 benchmarks/native-output-100-500.yaml
```

`--agent` and `--case` accept comma-separated names and preserve configuration order; unknown names and empty list elements fail before execution. `--repeats` changes measured repeats (1-1000); warmup repeats remain configured. `--jobs` accepts 1-64. Explicit zero values are rejected. Filters and overrides are saved in the experiment's effective configuration and provenance. `resume` keeps the original immutable plan and does not accept these overrides.

Every new `run` prints the selected agents/cases, concurrency, warmup count, measured count, total planned calls, and profiles inheriting native HOME before creating a database or invoking an agent. `--dry-run` prints this plan as JSON and stops after static validation; it does not verify installed executables, authentication, repository commits, network access, or runtime correctness. Planned calls are attempt counts, not upstream model-request or billing estimates: an agent can make multiple model/tool calls. On macOS, unsupported isolated agy profiles fail static platform validation in `run`, `--dry-run`, and `doctor`, with an explicit native-HOME alternative; isolation is never silently changed. A blocked dry-run still prints its JSON plan and diagnostic notes, then exits nonzero.

## Parallel execution and latest native measurement

Use Go workers to run independent attempts concurrently:

```sh
./bin/agentspeedbench run -jobs 4 benchmarks/native-comparison.yaml
```

`-jobs` overrides the configuration's `jobs` value (1–64). All configured warmups finish before measured attempts begin. Use `jobs: 1` for isolated latency comparisons; concurrent attempts share machine resources and may share provider/account limits. Compare results at the same concurrency, and keep parallel measurements separate from sequential ones.

### 2026-10-11 sequential 100 / 500 output retest

The latest retest used clean harness commit `b2d2c674cd3754f1ae00a1e86145580cd8bae744`, `jobs: 1`, and the existing controlled integer-output cases for 100 and 500 lines. The 2000-line case was omitted. Codex and Cursor used isolated configuration; agy explicitly retained its native login HOME with `isolate_config: false`. Existing YOLO flags remained enabled, and the agy adapter disabled auto-update. This is a CLI output-task comparison; inherited agy settings can affect results.

Each requested configuration and output length has **3 completed measured samples**. All **42 measured samples and 14 warmups passed**. No Antigravity Keychain storage/reset dialogs appeared during this round. Long-running sessions and future OAuth token-refresh behavior remain unverified.

I reduced the original 10-repeat schedule to three measured repeats per cell: 13 completed measured samples were retained, 28 samples were added in a supplementary experiment, and one canceled agy-high / 500 call was replaced in a separate experiment. The canceled attempt remains in the raw evidence and is excluded from the medians below. Original manifests and run records were not rewritten; these are pooled descriptive results across three experiments, not a single uninterrupted matrix.

| Requested configuration | 100-line wall p50, s | 100-line effective tok/s p50 | 500-line wall p50, s | 500-line effective tok/s p50 |
| --- | ---: | ---: | ---: | ---: |
| GPT-6.1 Sol / medium | 9.76 | 21.20 | 19.39 | 51.92 |
| GPT-6 Luna / high | 7.69 | 29.64 | 12.41 | 83.51 |
| GPT-6 Luna / high / requested fast tier | 7.26 | 30.71 | 13.81 | 76.18 |
| Cursor Auto | 15.52 | 16.36 | 20.19 | 52.26 |
| AGY Gemini-3.8-Flash / low | 7.08 | 41.82 | 16.29 | 116.36 |
| AGY Gemini-3.8-Flash / medium | 6.21 | 47.66 | 13.73 | 138.13 |
| AGY Gemini-3.8-Flash / high | 8.48 | 51.30 | 15.19 | 145.93 |

Every cell has **n=3**, a small sample; minor differences do not establish a stable ranking. Effective tok/s is reported output tokens divided by the whole agent process wall time, not pure generation throughput. AGY output accounting includes thinking; Codex and Cursor accounting remains unknown. Codex TTFA uses complete-message receipt, while Cursor and agy use text-delta receipt. These boundaries are not interchangeable. Generation and model-active tok/s remain unknown.

#### Fast evidence in new run records

New Codex attempts retain three separate layers in `settings`: the requested `requested_service_tier`, an optional `cli_launch` summary of explicit launch arguments, and the independently reported `observed_service_tier`. Existing `configured_*` fields remain a user-file snapshot; they are not overwritten with launch overrides. Isolation can therefore record `config_status: user_config_ignored` alongside CLI arguments requesting Fast.

`cli_launch` records only allowlisted tier/fast/isolation/YOLO flags, explicit tier and fast-feature values, and whether the process started. It excludes arbitrary arguments, prompts, environment variables, executable paths and credentials. The fast-feature value stays unknown when no explicit enable/disable argument exists; `service_tier=default` does not imply that the feature flag was disabled. Launch arguments do not prove resolved CLI settings, feature acceptance, or backend adoption.

Text and HTML reports display requested tier, CLI argument tier, fast-feature argument, user-config tier and server-reported tier separately. A terminal CLI receipt can supply the reported tier and `observed_service_tier_source`; startup/config echoes do not establish the served tier. A reported tier different from the request is retained as reported. Missing receipts remain `null`/`unknown`; historical records without launch evidence are not backfilled from requested settings. SQLite stores these additive fields in the existing run JSON without a schema migration. Offline fixture tests cover evidence boundaries; they do not prove live Fast adoption.

#### Fast-tier verification

The Codex adapter passes `--config 'service_tier="fast"' --enable fast_mode`, matching the [official Fast mode configuration](https://learn.chatgpt.com/docs/agent-configuration/speed). The [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference) describes `fast` as mapping to request value `priority`. The local Codex CLI 0.160.1 model catalog advertised Fast for `gpt-6-luna`, with a `1.5x speed` description. A subsequent short diagnostic call completed successfully with these settings.

This verifies that Fast is enabled/requested at the CLI configuration level. The six measured fast calls and the diagnostic call did not provide an authoritative backend service-tier receipt, so actual service tier remains **unknown**. Relative to Luna high, requested-fast wall time was about 1.06x faster at 100 lines and 11.3% longer at 500 lines. Those observations neither prove a 1.5x acceleration nor establish that the backend ignored Fast.

Local evidence: `runs/native-output-100-500-20261011/summary-report.html`, `summary.json`, configuration files, database, and the three experiment directories. The complete report retains the cancellation and original schedule coverage. Fast verification is recorded separately in `runs/fast-verification-20261011/audit.json` and `diagnostic-summary.json`; the diagnostic call is not included in the speed table. These runtime artifacts are Git-ignored and are not publicly downloadable from this repository.

### 2026-10-11 parallel output measurement

This real CLI experiment used harness commit `9d9f2e8d7809579998d8e8485d970b5563055f02`, Go 1.26.9 on macOS arm64, Codex CLI 0.160.1, Cursor CLI `2026.10.01-e373342`, and agy 1.3.3. It used the seven configurations and integer-sequence case from `benchmarks/native-comparison.yaml`, with `jobs: 4`, `warmup_repeats: 1`, `repeats: 5`, a 180-second agent timeout, and `isolate_config: true` for every agent. Existing YOLO policies and Cursor workspace trust were retained. The command above enables parallelism but does not add the experiment's warmup or isolation overrides.

The task was to print integers 1–100, one per line, followed by `BENCH_DONE`. This measures output-task behavior, not repository repair correctness. All 35 measured attempts passed the integer-sequence verifier; seven additional warmups also passed. Matrix elapsed time, including warmups, was **137.84 seconds**. Each median below has **n=5**, a small sample. After the run, recurring Antigravity Keychain storage/reset dialogs appeared on macOS during this experiment. Passing verifier results do not establish unattended operation; timings may include interaction delays and must not be treated as a clean speed comparison.

| Requested configuration | Passed / measured | Agent wall p50, s | TTFA p50, s | Effective output tok/s p50 |
| --- | --- | ---: | ---: | ---: |
| GPT-6.1 Sol / medium | 5 / 5 | 9.34 | 8.44 | 22.16 |
| GPT-6 Luna / high | 5 / 5 | 8.34 | 7.32 | 26.61 |
| GPT-6 Luna / high / requested fast tier | 5 / 5 | 7.91 | 6.67 | 29.21 |
| Cursor Auto | 5 / 5 | 15.47 | 13.96 | 16.93 |
| AGY Gemini-3.8-Flash / low | 5 / 5 | 11.94 | 10.22 | 24.79 |
| AGY Gemini-3.8-Flash / medium | 5 / 5 | 13.74 | 11.19 | 21.55 |
| AGY Gemini-3.8-Flash / high | 5 / 5 | 13.13 | 11.04 | 36.77 |

Effective output tok/s divides reported output tokens by the entire agent process wall time; it is not decoding throughput. AGY output accounting includes thinking tokens, while Codex and Cursor accounting remains unknown. These values do not establish a cross-provider decoding-speed ranking. TTFA is runner-observed and subject to CLI buffering. Model and effort labels are requested settings; the fast tier was requested but not observed in CLI telemetry, so this run does not prove a 1.5× speed tier.

Local evidence is stored under `runs/native-parallel-round-20261011/`: `config.yaml`, `round-summary.json`, and experiment `native-parallel-output-100-20261011T033431Z-91bdcbfcd665aa64` containing the manifest, raw events, run records, and `report.html`. Runtime artifacts are Git-ignored and are not included in this repository; these paths describe the local evidence, not publicly downloadable results. Earlier reports remain unchanged.

## Local validation snapshot — 2026-10-07

The native adapters passed a serial regression with one attempt per setting above: **7/7** outputs matched the complete sequence 1–100 followed by `BENCH_DONE`. This validates the tested command and telemetry paths; one sample per setting does not establish a speed ranking. The committed comparison config uses five repeats per setting.

The run used macOS arm64, Go 1.26.8, Codex CLI 0.160.1, Cursor Agent 2026.10.01-e373342, and agy 1.3.1. Cursor reported 262 output tokens and zero cache-write tokens. All three agy settings retained thinking counts and CLI-reported durations separately from wall time. The Fast request succeeded, while the delivered service tier remained unknown because native events did not report it. Cursor Auto reported the Auto selection, not its underlying routed model.

A separate three-repeat run without Cursor workspace trust produced one `agent_unavailable` attempt followed by two `skipped` records, all ungraded. Tests also verify that ordinary stderr warnings do not fail a successful run and that an unavailable agent does not stop other agents.

`make check` passed all 45 top-level tests, including race checks, formatting, vet, and build. A Linux amd64 cross-build also passed. SQLite records matched the seven run artifacts. Live runs covered the response-only case; agy tool pairing was checked with fixtures. Authenticated Claude and hosted CI were not verified in this snapshot. Local reports and raw traces remain Git-ignored.

## macOS Antigravity Keychain limitation


`run` and `doctor` reject isolated `agy` on macOS before invoking any CLI; a new run does not create experiment artifacts for that incompatible plan. The attempt-level guard remains as defense in depth and can record ungraded `agent_unavailable` with code `macos_keychain_isolation_unsupported`. Copying an OAuth token into a temporary HOME does not prevent native Keychain access. This is a containment measure, not a system Keychain repair. macOS nonisolated agy execution still uses native authentication and may show dialogs; Linux isolation is unchanged. The runner does not reset or modify Keychain settings, silently disable isolation, or switch account-backed tests to a separately billed API-key provider. Earlier raw records remain unchanged.

The [official troubleshooting guide](https://www.antigravity.google/docs/cli/troubleshooting/) describes native keyring use. [Upstream issue #803](https://github.com/google-antigravity/antigravity-cli/issues/803) reports a matching storage dialog even when the default Keychain exists, including successful continuation after dismissal. The 2026-10-11 recovery smoke used the original login HOME with agy 1.3.3: a direct `AGY_OK` call succeeded, followed by low/medium/high (two attempts each), all six completed and passed. No storage-failure diagnostics were found in the direct-call log. No storage/reset dialogs appeared during this smoke. Long-running sessions and future token-refresh behavior remain unverified; this is a working native-home path, not an upstream Keychain repair.

### Run agy with its native login HOME

On macOS, use the explicit nonisolated authentication smoke:

```sh
make build
./bin/agentspeedbench run benchmarks/agy-native-auth-smoke.yaml
# Direct invocation from your normal login shell:
AGY_CLI_DISABLE_AUTO_UPDATE=true agy --dangerously-skip-permissions -p 'Reply with exactly AGY_OK. Do not use tools.'
```

This profile sets `isolate_config: false`, keeps the original HOME and Keychain context, and tests low/medium/high twice each. The agy adapter disables auto-update for benchmark calls while preserving `--dangerously-skip-permissions`. Host settings, rules, plugins and login state may affect these calls, so the smoke is not an isolated performance comparison. Other isolated profiles remain unchanged. Do not override HOME with a disposable directory for this path. If storage/reset dialogs recur in the normal HOME, stop and investigate the native credential store; successful output alone does not clear that failure.
