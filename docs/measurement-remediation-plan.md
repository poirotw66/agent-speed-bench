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
