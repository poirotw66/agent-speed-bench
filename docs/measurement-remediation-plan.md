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
