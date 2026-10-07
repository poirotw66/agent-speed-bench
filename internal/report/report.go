package report

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type Group struct {
	GoCachePolicy, IsolationPolicy, PermissionPolicy, InstructionsPolicy string
	WallP25, WallP75, WallMin, WallMax, PreparationP50                   *float64
	Agent, Case, Experiment                                              string
	TTFABasis                                                            string
	OutputTokenAccounting                                                string
	CharactersPerSecondP50, AnswerCompleteP50, ExitGapP50                *float64
	Runs, Completed, Graded, Passed, Timeouts                            int
	WallP50, WallP95, TTFAP50, EffectiveTPSP50                           *float64
	SuccessRate, PassedPerWallHour                                       *float64
}
type Data struct {
	Groups []Group
	Runs   []telemetry.Run
}

func Aggregate(runs []telemetry.Run) []Group {
	type sample struct {
		group                             Group
		walls, ttfas, rates               []float64
		characterRates, answers, exitGaps []float64
		totalWall                         float64
		preparations                      []float64
	}
	groups := map[string]*sample{}
	for _, r := range runs {
		if r.Warmup {
			continue
		}
		r = telemetry.ReportRun(r)
		key := r.ExperimentID + "\x00" + r.Agent + "\x00" + r.Case + "\x00" + r.Metrics.TTFABasis + "\x00" + r.Metrics.Usage.OutputTokenAccounting + "\x00" + r.Environment.GoCachePolicy + "\x00" + r.Environment.IsolationPolicy + "\x00" + r.Environment.PermissionPolicy + "\x00" + r.Environment.WorkspaceInstructions
		s := groups[key]
		if s == nil {
			s = &sample{group: Group{Agent: r.Agent, Case: r.Case, Experiment: r.ExperimentID, TTFABasis: r.Metrics.TTFABasis, OutputTokenAccounting: r.Metrics.Usage.OutputTokenAccounting, GoCachePolicy: r.Environment.GoCachePolicy, IsolationPolicy: r.Environment.IsolationPolicy, PermissionPolicy: r.Environment.PermissionPolicy, InstructionsPolicy: r.Environment.WorkspaceInstructions}}
			groups[key] = s
		}
		s.group.Runs++
		if r.Status == "completed" {
			s.group.Completed++
		}
		if r.Status == "timeout" {
			s.group.Timeouts++
		}
		if r.Success != nil {
			s.group.Graded++
			if *r.Success {
				s.group.Passed++
			}
		}
		if r.Environment.PreparationSeconds != nil {
			s.preparations = append(s.preparations, *r.Environment.PreparationSeconds)
		}
		if r.Metrics.WallSeconds > 0 && (r.Status == "completed" || r.Success != nil) {
			s.walls = append(s.walls, r.Metrics.WallSeconds)
			s.totalWall += r.Metrics.WallSeconds
		}
		// Failed runs stay in reliability and wall-time statistics; token and TTFA
		// aggregates use completed runs with observed metrics.
		if r.Status == "completed" {
			if r.Metrics.EffectiveCharactersPerSecond != nil {
				s.characterRates = append(s.characterRates, *r.Metrics.EffectiveCharactersPerSecond)
			}
			if r.Metrics.AnswerCompleteSeconds != nil {
				s.answers = append(s.answers, *r.Metrics.AnswerCompleteSeconds)
			}
			if r.Metrics.TerminalToExitSeconds != nil {
				s.exitGaps = append(s.exitGaps, *r.Metrics.TerminalToExitSeconds)
			}
			if r.Metrics.TTFASeconds != nil {
				s.ttfas = append(s.ttfas, *r.Metrics.TTFASeconds)
			}
			if r.Metrics.EffectiveOutputTPS != nil {
				s.rates = append(s.rates, *r.Metrics.EffectiveOutputTPS)
			}
		}
	}
	result := []Group{}
	for _, s := range groups {
		g := s.group
		g.WallP50 = telemetry.Percentile(s.walls, .5)
		g.WallP95 = telemetry.Percentile(s.walls, .95)
		g.WallP25 = telemetry.Percentile(s.walls, .25)
		g.WallP75 = telemetry.Percentile(s.walls, .75)
		g.WallMin = telemetry.Percentile(s.walls, 0)
		g.WallMax = telemetry.Percentile(s.walls, 1)
		g.PreparationP50 = telemetry.Percentile(s.preparations, .5)
		g.TTFAP50 = telemetry.Percentile(s.ttfas, .5)
		g.EffectiveTPSP50 = telemetry.Percentile(s.rates, .5)
		g.CharactersPerSecondP50 = telemetry.Percentile(s.characterRates, .5)
		g.AnswerCompleteP50 = telemetry.Percentile(s.answers, .5)
		g.ExitGapP50 = telemetry.Percentile(s.exitGaps, .5)
		if g.Graded > 0 {
			v := 100 * float64(g.Passed) / float64(g.Graded)
			g.SuccessRate = &v
		}
		if s.totalWall > 0 && g.Graded == g.Runs && len(s.walls) == g.Runs {
			v := 3600 * float64(g.Passed) / s.totalWall
			g.PassedPerWallHour = &v
		}
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool { return groupOrder(result[i]) < groupOrder(result[j]) })
	return result
}

func groupOrder(g Group) string {
	return strings.Join([]string{g.Agent, g.Case, g.Experiment, g.TTFABasis, g.OutputTokenAccounting, g.GoCachePolicy, g.IsolationPolicy, g.PermissionPolicy, g.InstructionsPolicy}, "\x00")
}
func policy(value string) string {
	if value == "" {
		return "unspecified"
	}
	return value
}

func number(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.3f", *v)
}
func tokens(v *int64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprint(*v)
}
func success(v *bool) string {
	if v == nil {
		return "ungraded"
	}
	if *v {
		return "pass"
	}
	return "fail"
}

func Text(w io.Writer, runs []telemetry.Run) {
	fmt.Fprintln(w, "Agent / case | runs | pass / graded | wall p50 / p95 (s) | TTFA p50 (s) | effective output tok/s p50 | passed / wall hour")
	for _, g := range Aggregate(runs) {
		fmt.Fprintf(w, "Experiment: %s\n", g.Experiment)
		if g.Completed < 10 {
			fmt.Fprintln(w, "Small sample: fewer than 10 completed measured runs; descriptive only.")
		}
		fmt.Fprintf(w, "Environment: cache=%s isolation=%s permissions=%s instructions=%s; preparation p50=%s; wall IQR=%s–%s min/max=%s–%s\n", policy(g.GoCachePolicy), policy(g.IsolationPolicy), policy(g.PermissionPolicy), policy(g.InstructionsPolicy), duration(g.PreparationP50), number(g.WallP25), number(g.WallP75), number(g.WallMin), number(g.WallMax))
		fmt.Fprintf(w, "Receipt answer p50=%s; exit gap p50=%s; characters/s p50=%s\n", duration(g.AnswerCompleteP50), duration(g.ExitGapP50), number(g.CharactersPerSecondP50))
		fmt.Fprintf(w, "%s / %s | %d | %d / %d | %s / %s | %s | %s | %s\n", g.Agent, g.Case, g.Runs, g.Passed, g.Graded, number(g.WallP50), number(g.WallP95), number(g.TTFAP50)+" ["+g.TTFABasis+"]", number(g.EffectiveTPSP50)+" ["+accounting(g.OutputTokenAccounting)+"]", number(g.PassedPerWallHour))
	}
	for _, r := range runs {
		fmt.Fprintf(w, "Phase: %s; answer complete=%s; terminal=%s; terminal-to-exit=%s; characters=%s; effective characters/s=%s\n", phase(r), duration(r.Metrics.AnswerCompleteSeconds), duration(r.Metrics.TerminalReceiptSeconds), duration(r.Metrics.TerminalToExitSeconds), tokens(r.Metrics.OutputCharacters), number(r.Metrics.EffectiveCharactersPerSecond))
		fmt.Fprintf(w, "Run %s: thinking=%s; cache write=%s; CLI-reported duration=%s [%s]; tier requested=%s configured=%s observed=%s\n", r.ID, tokens(r.Metrics.Usage.ThinkingTokens), tokens(r.Metrics.Usage.CacheWriteTokens), duration(r.Metrics.ReportedDurationSeconds), r.Metrics.ReportedDurationSource, r.Settings.RequestedServiceTier, setting(r.Settings.ConfiguredServiceTier), setting(r.Settings.ObservedServiceTier))
	}
}

func HTML(w io.Writer, runs []telemetry.Run) error {
	normalized := make([]telemetry.Run, len(runs))
	for i, r := range runs {
		normalized[i] = telemetry.ReportRun(r)
	}
	runs = normalized
	ordered := append([]telemetry.Run(nil), runs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartedAt.Before(ordered[j].StartedAt) })
	t, err := template.New("report").Funcs(template.FuncMap{"phase": phase, "policy": policy, "accounting": accounting, "number": number, "duration": duration, "setting": setting, "tokens": tokens, "success": success, "join": strings.Join}).Parse(page)
	if err != nil {
		return err
	}
	return t.Execute(w, Data{Groups: Aggregate(runs), Runs: ordered})
}

const page = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>AgentSpeedBench report</title>
<style>
:root{color-scheme:light;--ink:#202a31;--muted:#56656c;--line:#d8dfdf;--accent:#146651}*{box-sizing:border-box}body{margin:0;background:#f4f6f3;color:var(--ink);font:15px/1.6 system-ui,sans-serif}main{max-width:1440px;margin:auto;padding:48px 28px}h1{font-size:clamp(30px,4vw,52px);line-height:1.1;margin:10px 0 18px;letter-spacing:-.04em}h2{font-size:23px;margin-top:42px}.eyebrow{color:var(--accent);font-size:12px;letter-spacing:.12em;text-transform:uppercase;font-weight:700}.intro{max-width:850px;color:var(--muted)}.table-wrap{overflow:auto;border:1px solid var(--line);background:white}table{border-collapse:collapse;width:100%;white-space:nowrap}th{text-align:left;background:#e9eeea;font-size:12px}td,th{padding:12px 14px;border-bottom:1px solid var(--line);vertical-align:top}td{font-variant-numeric:tabular-nums;font-size:13px}details{max-width:380px;white-space:normal}summary{cursor:pointer;color:var(--accent)}code{font-size:12px}footer{color:var(--muted);margin-top:32px}.status{font-weight:600}a{color:var(--accent)}
</style></head><body><main><div class="eyebrow">Latency · throughput · correctness</div><h1>AgentSpeedBench</h1>
<p class="intro">Runner-observed CLI and response-only API performance. Effective output tok/s is reported output tokens divided by agent process wall time. Generation and model-active tok/s are <strong>unknown</strong>: these receive streams do not establish decoding intervals. API SSE receive rates exclude the first chunk and include client/network buffering. Demo tokens are synthetic. Vendor token accounting differs: agy output includes thinking tokens; other accounting is unknown unless reported. CLI-reported duration is separate from process wall time and does not establish decoding time.</p>
<h2>Comparison by agent and case</h2><p class="intro">Wall times include graded failures and timeouts; unavailable/quota/preparation failures do not contribute speed samples. TTFA and effective throughput medians use completed runs with available values. Percentiles use nearest rank. Success is pass / graded; ungraded runs remain unknown. Warmups are retained in history and excluded from aggregates. Experiments, TTFA timing bases, cache, isolation and permission policies are kept separate. IQR and min/max describe sample spread, not confidence intervals. Missing historical policies remain unspecified. Tool receipt intervals are not actual runtime; unsupported runtime remains unknown. Skipped and unavailable runs are ungraded. User config snapshots do not prove resolved or observed CLI settings.</p>
<div class="table-wrap"><table><thead><tr><th scope="col">Agent</th><th scope="col">Case / experiment</th><th scope="col">Runs / completed</th><th scope="col">Pass / graded</th><th scope="col">Timeouts</th><th scope="col">Wall p50 / p95, s</th><th scope="col">TTFA p50, s</th><th scope="col">Effective output tok/s p50</th><th scope="col">Passed / wall hour</th></tr></thead><tbody>
{{range .Groups}}<tr><td>{{.Agent}}</td><td>{{.Case}}<details><summary>Experiment</summary>{{.Experiment}}</details></td><td>{{.Runs}} / {{.Completed}}{{if lt .Completed 10}}<br>Small sample; descriptive only{{end}}</td><td>{{.Passed}} / {{.Graded}}</td><td>{{.Timeouts}}</td><td>{{number .WallP50}} / {{number .WallP95}}<br>IQR: {{number .WallP25}}–{{number .WallP75}}; min/max: {{number .WallMin}}–{{number .WallMax}}<br>Preparation p50: {{duration .PreparationP50}}<br>Cache: {{policy .GoCachePolicy}}; isolation: {{policy .IsolationPolicy}}<br>Permissions: {{policy .PermissionPolicy}}<br>Instructions: {{policy .InstructionsPolicy}}<br>Answer p50: {{number .AnswerCompleteP50}}; exit gap p50: {{number .ExitGapP50}}</td><td>{{number .TTFAP50}}<br>{{.TTFABasis}}</td><td>{{number .EffectiveTPSP50}}<br>{{accounting .OutputTokenAccounting}}<br>Characters/s p50: {{number .CharactersPerSecondP50}}</td><td>{{number .PassedPerWallHour}}</td></tr>{{else}}<tr><td colspan="9">No stored runs.</td></tr>{{end}}
</tbody></table></div><h2>Run history</h2><p class="intro">UTC timestamps allow comparisons across experiments. Tool receipt intervals include harness buffering and do not establish execution duration. Raw artifacts provide the evidence for each run.</p>
<div class="table-wrap"><table><thead><tr><th scope="col">Started (UTC)</th><th scope="col">Agent / case</th><th scope="col">State / grade</th><th scope="col">Wall, s</th><th scope="col">TTFA, s</th><th scope="col">Output tokens</th><th scope="col">Effective tok/s</th><th scope="col">Tool matched / started</th><th scope="col">Tool runtime mean / p95, s; receipt mean / p95</th><th scope="col">Evidence</th></tr></thead><tbody>
{{range .Runs}}<tr><td>{{.StartedAt.Format "2006-01-02 15:04:05"}}</td><td>{{.Agent}} / {{.Case}}<br>{{.Model}}</td><td class="status">{{.Status}} / {{success .Success}}<br>{{phase .}}</td><td>{{printf "%.3f" .Metrics.WallSeconds}}<br>Answer complete: {{duration .Metrics.AnswerCompleteSeconds}}<br>Terminal: {{duration .Metrics.TerminalReceiptSeconds}}; exit gap: {{duration .Metrics.TerminalToExitSeconds}}<br>CLI-reported: {{duration .Metrics.ReportedDurationSeconds}}<br>{{.Metrics.ReportedDurationSource}}</td><td>{{number .Metrics.TTFASeconds}}<br>{{.Metrics.TTFABasis}}<br>Delta: {{number .Metrics.FirstTextDeltaSeconds}}; complete: {{number .Metrics.FirstCompleteMessageSeconds}}; tool: {{number .Metrics.FirstToolActionSeconds}}</td><td>{{tokens .Metrics.Usage.OutputTokens}}<br>{{accounting .Metrics.Usage.OutputTokenAccounting}}<br>Thinking: {{tokens .Metrics.Usage.ThinkingTokens}}; cache write: {{tokens .Metrics.Usage.CacheWriteTokens}}</td><td>{{number .Metrics.EffectiveOutputTPS}}<br>Characters: {{tokens .Metrics.OutputCharacters}}; characters/s: {{number .Metrics.EffectiveCharactersPerSecond}}</td><td>{{.Metrics.MatchedToolCalls}} / {{.Metrics.ToolCalls}}</td><td>{{number .Metrics.ToolLatencyMeanSeconds}} / {{number .Metrics.ToolLatencyP95Seconds}}<br>Receipt: {{duration .Metrics.ToolReceiptIntervalMeanSeconds}} / {{duration .Metrics.ToolReceiptIntervalP95Seconds}}<br>{{.Metrics.ToolTimingBasis}} / {{.Metrics.ToolTimingConfidence}}</td><td><details><summary>{{.ID}}</summary><p>Experiment: {{.ExperimentID}}</p><p>Commit: {{.Commit}}</p><p>Artifacts: <code>{{.ArtifactDir}}</code></p><p>{{.Error}}</p>{{with .Failure}}<p>Failure: {{.Category}} / {{.Code}}; scope={{.Scope}}; retryable={{.Retryable}}</p>{{end}}<p>Requested model: {{.Settings.RequestedModel}}; configured: {{setting .Settings.ConfiguredModel}}; observed: {{setting .Settings.ObservedModel}}</p><p>Requested effort: {{.Settings.RequestedReasoningEffort}}; configured: {{setting .Settings.ConfiguredReasoningEffort}}; observed: {{setting .Settings.ObservedReasoningEffort}}</p><p>Service tier requested: {{.Settings.RequestedServiceTier}}; configured: {{setting .Settings.ConfiguredServiceTier}}; observed: {{setting .Settings.ObservedServiceTier}}</p>{{range .Verification}}<p>Scoring {{.Layer}}: {{.Passed}}; {{.Error}}</p>{{end}}<p>Preparation: {{duration .Environment.PreparationSeconds}}; Go cache: {{policy .Environment.GoCachePolicy}}</p><p>Isolation: {{policy .Environment.IsolationPolicy}}; permissions: {{policy .Environment.PermissionPolicy}}; instructions: {{policy .Environment.WorkspaceInstructions}}</p><p>SSE receive interval: {{duration .Metrics.StreamReceiveSeconds}}; characters/s: {{number .Metrics.StreamReceiveCharactersPerSecond}}; {{.Metrics.StreamReceiveBasis}}</p><p>Config snapshot: {{.Settings.ConfigStatus}}</p><p>{{join .Metrics.Warnings "; "}}</p></details></td></tr>{{end}}
</tbody></table></div><footer>Passed / wall hour is a sequential-equivalent measure: 3600 × passed tasks / sum of agent process wall seconds. It is unknown if any attempt is ungraded. It excludes workspace preparation and verification and is not measured concurrent experiment throughput. Compare the same cases, model settings, CLI versions, machine, permissions, and concurrency.</footer></main></body></html>`

func duration(v *float64) string {
	if v == nil {
		return "unknown"
	}
	if *v > 0 && *v < .001 {
		return fmt.Sprintf("%.3f us", *v*1e6)
	}
	if *v > 0 && *v < 1 {
		return fmt.Sprintf("%.3f ms", *v*1e3)
	}
	return fmt.Sprintf("%.3f s", *v)
}

func setting(v *string) string {
	if v == nil {
		return "unknown"
	}
	return *v
}

func accounting(v string) string {
	if v == "" {
		return "unknown accounting"
	}
	return v
}

func phase(r telemetry.Run) string {
	if r.Warmup {
		return "warmup"
	}
	return "measured"
}
