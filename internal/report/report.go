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
	Agent, Case, Experiment                    string
	TTFABasis                                  string
	Runs, Completed, Graded, Passed, Timeouts  int
	WallP50, WallP95, TTFAP50, EffectiveTPSP50 *float64
	SuccessRate, PassedPerWallHour             *float64
}
type Data struct {
	Groups []Group
	Runs   []telemetry.Run
}

func Aggregate(runs []telemetry.Run) []Group {
	type sample struct {
		group               Group
		walls, ttfas, rates []float64
		totalWall           float64
	}
	groups := map[string]*sample{}
	for _, r := range runs {
		r = telemetry.ReportRun(r)
		key := r.ExperimentID + "\x00" + r.Agent + "\x00" + r.Case + "\x00" + r.Metrics.TTFABasis
		s := groups[key]
		if s == nil {
			s = &sample{group: Group{Agent: r.Agent, Case: r.Case, Experiment: r.ExperimentID, TTFABasis: r.Metrics.TTFABasis}}
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
		if r.Metrics.WallSeconds > 0 {
			s.walls = append(s.walls, r.Metrics.WallSeconds)
			s.totalWall += r.Metrics.WallSeconds
		}
		// Failed runs stay in reliability and wall-time statistics; token and TTFA
		// aggregates use completed runs with observed metrics.
		if r.Status == "completed" {
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
		g.TTFAP50 = telemetry.Percentile(s.ttfas, .5)
		g.EffectiveTPSP50 = telemetry.Percentile(s.rates, .5)
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
	sort.Slice(result, func(i, j int) bool {
		if result[i].Agent == result[j].Agent {
			if result[i].Case == result[j].Case {
				if result[i].Experiment == result[j].Experiment {
					return result[i].TTFABasis < result[j].TTFABasis
				}
				return result[i].Experiment < result[j].Experiment
			}
			return result[i].Case < result[j].Case
		}
		return result[i].Agent < result[j].Agent
	})
	return result
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
		fmt.Fprintf(w, "%s / %s | %d | %d / %d | %s / %s | %s | %s | %s\n", g.Agent, g.Case, g.Runs, g.Passed, g.Graded, number(g.WallP50), number(g.WallP95), number(g.TTFAP50)+" ["+g.TTFABasis+"]", number(g.EffectiveTPSP50), number(g.PassedPerWallHour))
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
	t, err := template.New("report").Funcs(template.FuncMap{"number": number, "duration": duration, "setting": setting, "tokens": tokens, "success": success, "join": strings.Join}).Parse(page)
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
<p class="intro">Runner-observed CLI performance. Effective output tok/s is reported output tokens divided by agent process wall time. Generation and model-active tok/s are <strong>unknown</strong>: these CLI streams do not establish decoding intervals. Demo tokens are synthetic.</p>
<h2>Comparison by agent and case</h2><p class="intro">Wall times include failed attempts and timeouts. TTFA and effective throughput medians use completed runs with available values. Percentiles use nearest rank. Success is pass / graded; ungraded runs remain unknown. Experiments and TTFA timing bases are kept separate. Tool receipt intervals are not actual runtime; unsupported runtime remains unknown. Skipped and unavailable runs are ungraded. User config snapshots do not prove resolved or observed CLI settings.</p>
<div class="table-wrap"><table><thead><tr><th scope="col">Agent</th><th scope="col">Case / experiment</th><th scope="col">Runs / completed</th><th scope="col">Pass / graded</th><th scope="col">Timeouts</th><th scope="col">Wall p50 / p95, s</th><th scope="col">TTFA p50, s</th><th scope="col">Effective output tok/s p50</th><th scope="col">Passed / wall hour</th></tr></thead><tbody>
{{range .Groups}}<tr><td>{{.Agent}}</td><td>{{.Case}}<details><summary>Experiment</summary>{{.Experiment}}</details></td><td>{{.Runs}} / {{.Completed}}</td><td>{{.Passed}} / {{.Graded}}</td><td>{{.Timeouts}}</td><td>{{number .WallP50}} / {{number .WallP95}}</td><td>{{number .TTFAP50}}<br>{{.TTFABasis}}</td><td>{{number .EffectiveTPSP50}}</td><td>{{number .PassedPerWallHour}}</td></tr>{{else}}<tr><td colspan="9">No stored runs.</td></tr>{{end}}
</tbody></table></div><h2>Run history</h2><p class="intro">UTC timestamps allow comparisons across experiments. Tool latency is the observed start-to-finish interval, including harness overhead. Raw artifacts provide the evidence for each run.</p>
<div class="table-wrap"><table><thead><tr><th scope="col">Started (UTC)</th><th scope="col">Agent / case</th><th scope="col">State / grade</th><th scope="col">Wall, s</th><th scope="col">TTFA, s</th><th scope="col">Output tokens</th><th scope="col">Effective tok/s</th><th scope="col">Tool matched / started</th><th scope="col">Tool runtime mean / p95, s; receipt mean / p95</th><th scope="col">Evidence</th></tr></thead><tbody>
{{range .Runs}}<tr><td>{{.StartedAt.Format "2006-01-02 15:04:05"}}</td><td>{{.Agent}} / {{.Case}}<br>{{.Model}}</td><td class="status">{{.Status}} / {{success .Success}}</td><td>{{printf "%.3f" .Metrics.WallSeconds}}</td><td>{{number .Metrics.TTFASeconds}}<br>{{.Metrics.TTFABasis}}<br>Delta: {{number .Metrics.FirstTextDeltaSeconds}}; complete: {{number .Metrics.FirstCompleteMessageSeconds}}; tool: {{number .Metrics.FirstToolActionSeconds}}</td><td>{{tokens .Metrics.Usage.OutputTokens}}</td><td>{{number .Metrics.EffectiveOutputTPS}}</td><td>{{.Metrics.MatchedToolCalls}} / {{.Metrics.ToolCalls}}</td><td>{{number .Metrics.ToolLatencyMeanSeconds}} / {{number .Metrics.ToolLatencyP95Seconds}}<br>Receipt: {{duration .Metrics.ToolReceiptIntervalMeanSeconds}} / {{duration .Metrics.ToolReceiptIntervalP95Seconds}}<br>{{.Metrics.ToolTimingBasis}} / {{.Metrics.ToolTimingConfidence}}</td><td><details><summary>{{.ID}}</summary><p>Experiment: {{.ExperimentID}}</p><p>Commit: {{.Commit}}</p><p>Artifacts: <code>{{.ArtifactDir}}</code></p><p>{{.Error}}</p>{{with .Failure}}<p>Failure: {{.Category}} / {{.Code}}; scope={{.Scope}}; retryable={{.Retryable}}</p>{{end}}<p>Requested model: {{.Settings.RequestedModel}}; configured: {{setting .Settings.ConfiguredModel}}; observed: {{setting .Settings.ObservedModel}}</p><p>Requested effort: {{.Settings.RequestedReasoningEffort}}; configured: {{setting .Settings.ConfiguredReasoningEffort}}; observed: {{setting .Settings.ObservedReasoningEffort}}</p><p>Config snapshot: {{.Settings.ConfigStatus}}</p><p>{{join .Metrics.Warnings "; "}}</p></details></td></tr>{{end}}
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
