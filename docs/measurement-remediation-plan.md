# Measurement remediation plan

## Objective

Separate CLI task latency, effective output throughput, and engineering correctness. Do not infer decoding speed from buffered CLI messages. Preserve existing YAML defaults and immutable historical records.

## Implementation

1. Parse Codex reasoning_output_tokens and cache_write_input_tokens; preserve absent versus zero and reject conflicting aliases.
2. Record last authoritative answer receipt, successful terminal receipt, terminal-to-exit time, and final transcript Unicode character count/rate. Keep output/wall throughput unchanged. Label all timings as runner receipts.
3. Add optional warmup_repeats (default zero), persist warmups with an explicit phase, finish all warmups before measured jobs, and exclude them from aggregates. Rotate agent order; recommend serial comparisons and ten measured repeats.
4. Provide short/medium/long exact sequence cases and three synthetic Go debugging cases. Use trusted external core and regression checks, both required to pass. Reconstruct candidate source in a separate temporary module so candidate tests/module changes do not replace scoring tests. Preserve the submitted source artifact.
5. Verify old defaults, warmup barriers/exclusion, partial/missing terminal timing, Unicode counting, grading failures, fixture base-fails/fixed-passes gates, SQLite round trips, reports, and CLI runs. Update English and Traditional Chinese documentation.

## Boundaries

Go cases are synthetic behavioral fixtures, not upstream real-repository bugs or HarnessBench parity. External graders are not a host security sandbox. Existing repository cases are not sanitized automatically. API streaming is a separate future adapter requiring explicitly chosen authentication/model/usage settings; no API key or billing changes are part of this increment. No generation or model-active TPS is synthesized. No expensive seven-agent ten-repeat matrix is needed for implementation acceptance.

## Acceptance and rollback

make check and a Linux cross-build pass. Offline fixture gates reject every broken source and accept every fixed source. A small native smoke run validates new fields and warmup phase. Historical metrics remain missing when unsupported. Roll back this increment's source/configuration changes to restore defaults; retained raw evidence is never rewritten. Commit/push only when requested for this increment.

## Validation record

Implemented the five CLI/fixture steps above. A native Luna/high smoke matrix completed two warmups and two measured jobs (short response and Go clamp repair); all outputs/scoring passed. Warmups were excluded from report groups, source was retained, and native reasoning/cache-write counts were populated. The short measured run separated terminal receipt at 9.239 seconds from process exit at 12.484 seconds. This smoke run is descriptive, not a comparative speed result. Full seven-agent output/engineering matrices and API measurements have not been run for this increment.


## Next increment: implemented scope

Add opt-in Codex user-config/rules and feature isolation without changing authentication or other adapters. Preserve the exact narrower policy in manifest configuration and configuration snapshots; do not claim AGENTS.md/skills/managed-policy isolation. Add opt-in fresh Git history to disposable repo clones while retaining the upstream commit identity. Add one authentic pinned lazygit owner-casing case with independently written external tests, trusted vendored dependencies, source retention, and a base-fails/fixed-passes preparation gate. API streaming and cross-vendor host isolation remain deferred. Validate with make check, Linux cross-build, case gates, and one native smoke attempt before making performance claims.


Validation: make check and Linux amd64 cross-build passed. The real-case gate rejected the original source's three owner-casing combinations, accepted base regression, and accepted both fixed layers. Grader invalid-layer, missing-file, and symlink inputs were rejected. One native gpt-6-luna/high attempt with configuration isolation passed both external layers in 54.778 seconds; terminal-to-exit was 1.941 seconds. Its manifest, retained source, and SQLite record matched. The agent recovered from a default Go build-cache sandbox permission error by using /tmp, so this single attempt is process validation, not a comparative speed estimate. An earlier filtered-cache clone preparation failure was retained as ungraded infrastructure_error; fresh-history preparation now fetches only the pinned commit before resetting its history. Full real-case repeats have not been run.

## Current increment: controlled environments and API relay

1. Implement independent cold/warm vendored Go caches and preparation timing outside agent wall time.
2. Implement opt-in ephemeral authentication-only homes for Codex/Cursor/agy, known workspace instruction removal and explicit permission policies. Preserve host credentials/settings.
3. Provide serial seven-setting, three-length, ten-repeat and three-repeat real-case profiles. Classify explicit usage limits as unavailable/ungraded and stop that setting. Checkpoint recorded counts atomically; no automatic interrupted-run resume.
4. Add pinned whitespace (six submitted source files) and batched divergence cases with external core/regression gates. Their focused tests do not prove full interactive UI correctness.
5. Implement response-only OpenAI Responses HTTP SSE with offline transport/parser tests. Live calls remain pending provider/model/authentication variable/spending limit. Receipt intervals are not decoding intervals.
6. Record environment policies, preparation median and sample quartiles/range; capture allowlisted patches and SHA-256 manifests and support retained-source regrading. Preserve immutable old evidence.

Validation so far: make check passed, both added real cases passed base-fails/fixed-passes gates, isolated native smoke succeeded for all seven settings after Cursor credential reuse was fixed, and controlled Go-cache owner smoke passed both layers. agy completed 50 calls without Keychain/authentication diagnostics in a partial formal matrix. That matrix was interrupted and included explicit Codex quota errors; it is not a completed comparison. System Keychain repair is not claimed. Full matrices, extended-case native smoke, retained regrading and API end-to-end local validation must be recorded separately when completed.

Additional verification: make check and Linux amd64 cross-build passed; the updated binary was installed locally. A synthetic loopback SSE server passed the full CLI/runner/parser/storage/report workflow, and authentication did not appear in artifacts. Retained whitespace regression regrading passed; a modified retained source was rejected by SHA-256 validation before grading. Native extended-case smoke exposed agy headless command soft-denials with exit zero. The runner now classifies the explicit diagnostic as unavailable/ungraded, and generated ephemeral settings use proceed-in-sandbox without changing host settings. The zero-exit regression test passed. The corrected native smoke and serial formal matrices are still pending completion; local runtime progress is retained under ignored runs/. No new commit or push has been made.

## User-requested YOLO policy

All built-in coding CLI adapters now use the explicitly requested YOLO mode: codex exec --yolo, agent --yolo --sandbox disabled, and agy/claude --dangerously-skip-permissions. Remove conflicting sandbox/accept-edits/auto-review settings. Keep authentication-only config isolation independent. Record the specific permission policy for attempts, preparation errors and skipped runs. Generic argv and response-only API behavior remain unchanged. Stop and retain the earlier sandbox smoke/queue, then start new labeled experiments after validation; do not overwrite or pool historical results.

YOLO validation: make check and Linux amd64 cross-build passed; the local installed binary was refreshed. Codex/sol-medium, Cursor/auto and agy/high each completed a native shell-write smoke with passing file verification and recorded YOLO policies. Claude's bypass flag and absence of conflicting flags were verified through command construction tests; no native Claude call was made. The earlier sandbox process and matrix queue were stopped with raw evidence preserved. New serial YOLO output/real-Go/extended profiles use separate experiment names and runs/controlled-yolo.db; the output matrix has started and complete comparative results are pending.
