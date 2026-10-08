package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestReportEscapesUntrustedContentAndDisplaysUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := HTML(&out, []telemetry.Run{{Agent: "<script>alert(1)</script>", Case: "task", Status: "completed", Error: "<img onerror=bad>"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<script>alert") || strings.Contains(out.String(), "<img onerror") || !strings.Contains(out.String(), "unknown") || !strings.Contains(out.String(), "ungraded") {
		t.Fatal("untrusted HTML was not escaped or unknown fields disappeared")
	}
}

func TestAggregateIncludesFailuresAndDoesNotInventSuccess(t *testing.T) {
	pass, fail := true, false
	ttfa, rate := 1.0, 10.0
	runs := []telemetry.Run{
		{Agent: "a", Case: "x", Status: "completed", Success: &pass, Metrics: telemetry.Metrics{WallSeconds: 10, TTFASeconds: &ttfa, EffectiveOutputTPS: &rate}},
		{Agent: "a", Case: "x", Status: "timeout", Success: &fail, Metrics: telemetry.Metrics{WallSeconds: 100}},
		{Agent: "a", Case: "y", Status: "completed", Metrics: telemetry.Metrics{WallSeconds: 1}},
	}
	g := Aggregate(runs)
	if len(g) != 2 || g[0].Runs != 2 || g[0].Timeouts != 1 || *g[0].WallP95 != 100 || *g[0].SuccessRate != 50 || *g[0].EffectiveTPSP50 != 10 {
		t.Fatal(g)
	}
	if g[1].SuccessRate != nil || g[1].PassedPerWallHour != nil {
		t.Fatal("ungraded throughput must remain unknown")
	}
}

func TestHistoryKeepsExperimentsSeparate(t *testing.T) {
	runs := []telemetry.Run{
		{ExperimentID: "old", Agent: "a", Case: "x", Metrics: telemetry.Metrics{WallSeconds: 1}},
		{ExperimentID: "new", Agent: "a", Case: "x", Metrics: telemetry.Metrics{WallSeconds: 100}},
	}
	groups := Aggregate(runs)
	if len(groups) != 2 || groups[0].Runs != 1 || groups[1].Runs != 1 {
		t.Fatal("incompatible experiments were pooled", groups)
	}
}

func TestTimingBasesStaySeparateAndLegacyLatencyIsRelabeled(t *testing.T) {
	delta, complete, interval := 1.0, 2.0, .000053542
	runs := []telemetry.Run{
		{Agent: "a", Case: "x", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, TTFASeconds: &delta, TTFABasis: "text_delta_receipt"}},
		{Agent: "a", Case: "x", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, TTFASeconds: &complete, TTFABasis: "complete_message_receipt"}},
	}
	if len(Aggregate(runs)) != 2 {
		t.Fatal("mixed TTFA bases were pooled")
	}
	var out bytes.Buffer
	if err := HTML(&out, []telemetry.Run{{Metrics: telemetry.Metrics{ToolLatencyMeanSeconds: &interval}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "53.542 us") || !strings.Contains(out.String(), "legacy_unspecified") {
		t.Fatal(out.String())
	}
}

func TestThinkingDurationAndTierRemainDistinct(t *testing.T) {
	thinking, duration, tier := int64(214), 5.99, "fast"
	r := telemetry.Run{ID: "native", Settings: telemetry.Settings{RequestedServiceTier: tier}, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 13, Usage: telemetry.Usage{ThinkingTokens: &thinking, OutputTokenAccounting: "includes_thinking"}, ReportedDurationSeconds: &duration, ReportedDurationSource: "agy.result.duration_seconds"}}
	var out bytes.Buffer
	if err := HTML(&out, []telemetry.Run{r}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Thinking: 214", "5.990 s", "agy.result.duration_seconds", "includes_thinking", "Service tier requested: fast; configured: unknown; observed: unknown"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("missing", want)
		}
	}
	out.Reset()
	Text(&out, []telemetry.Run{r})
	if !strings.Contains(out.String(), "thinking=214") || !strings.Contains(out.String(), "observed=unknown") {
		t.Fatal(out.String())
	}
	other := r
	other.Metrics.Usage.OutputTokenAccounting = ""
	if len(Aggregate([]telemetry.Run{r, other})) != 2 {
		t.Fatal("accounting bases pooled")
	}
}

func TestWarmupsExcludedButRetainedInHistory(t *testing.T) {
	r := telemetry.Run{Agent: "a", Case: "c", Warmup: true, Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 100}}
	measured := r
	measured.Warmup = false
	measured.Metrics.WallSeconds = 2
	g := Aggregate([]telemetry.Run{r, measured})
	if len(g) != 1 || g[0].Runs != 1 || *g[0].WallP50 != 2 {
		t.Fatal(g)
	}
	var out bytes.Buffer
	if err := HTML(&out, []telemetry.Run{r, measured}); err != nil || !strings.Contains(out.String(), "warmup") || !strings.Contains(out.String(), "measured") {
		t.Fatal(err)
	}
}

func TestCharacterAndReceiptStatisticsAreRendered(t *testing.T) {
	rate, answer, gap, count := 10.0, 2.0, 1.0, int64(30)
	r := telemetry.Run{Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 3, EffectiveCharactersPerSecond: &rate, OutputCharacters: &count, AnswerCompleteSeconds: &answer, TerminalToExitSeconds: &gap}}
	g := Aggregate([]telemetry.Run{r})
	if len(g) != 1 || *g[0].CharactersPerSecondP50 != 10 || *g[0].AnswerCompleteP50 != 2 || *g[0].ExitGapP50 != 1 {
		t.Fatal(g)
	}
	var out bytes.Buffer
	if err := HTML(&out, []telemetry.Run{r}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Characters/s p50: 10.000", "Characters: 30", "Answer p50: 2.000", "Small sample"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(want)
		}
	}
}

func TestQuotaDoesNotBecomeSpeedOrFailureSample(t *testing.T) {
	fail := false
	runs := []telemetry.Run{{Agent: "a", Case: "c", Status: "failed", Success: &fail, Failure: &telemetry.Failure{Message: "You have hit your usage limit"}, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1}}}
	groups := Aggregate(runs)
	if groups[0].Graded != 0 || groups[0].WallP50 != nil || groups[0].PassedPerWallHour != nil {
		t.Fatal(groups)
	}
	if runs[0].Success == nil || runs[0].Status != "failed" || runs[0].Failure.Category != "" {
		t.Fatal("historical record mutated")
	}
}

func TestPoliciesAreSeparated(t *testing.T) {
	runs := []telemetry.Run{{Agent: "a", Case: "c", Status: "completed", Environment: telemetry.Environment{GoCachePolicy: "cold"}, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1}}, {Agent: "a", Case: "c", Status: "completed", Environment: telemetry.Environment{GoCachePolicy: "warm"}, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 10}}}
	groups := Aggregate(runs)
	if len(groups) != 2 || groups[0].GoCachePolicy != "cold" || *groups[0].WallMax != 1 || *groups[1].WallMin != 10 {
		t.Fatal(groups)
	}
}

func TestReliabilityKeepsTheDenominatorAcrossMetricGroups(t *testing.T) {
	pass, fail := true, false
	runs := []telemetry.Run{
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Success: &pass, Metrics: telemetry.Metrics{SchemaVersion: 2, TTFABasis: "text_delta_receipt", Usage: telemetry.Usage{OutputTokenAccounting: "includes_thinking"}}},
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "failed", Success: &fail, Failure: &telemetry.Failure{Message: "connection reset by peer"}},
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "canceled", Success: &fail},
		{ExperimentID: "e", Agent: "a", Case: "c", Warmup: true, Status: "completed", Success: &pass},
	}
	g := ReliabilityTotals(runs)
	if len(g) != 1 || g[0].Attempts != 3 || g[0].Graded != 1 || g[0].Passed != 1 || g[0].Unavailable != 1 || g[0].Canceled != 1 || *g[0].CompletionRate < 33 || *g[0].CompletionRate > 34 {
		t.Fatal(g)
	}
	if runs[1].Status != "failed" || runs[1].Success == nil || runs[1].Failure.Code != "" {
		t.Fatal("stored evidence was modified")
	}
	var out bytes.Buffer
	if err := HTML(&out, runs); err != nil || !strings.Contains(out.String(), "Reliability across all metric groups") || !strings.Contains(out.String(), "33.333") {
		t.Fatal(err, out.String())
	}
}

func TestPassedOnlySpeedExcludesWrongAndUngradedAnswers(t *testing.T) {
	passed, failed := true, false
	a, b, c := 10.0, 100.0, 200.0
	runs := []telemetry.Run{
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Success: &passed, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 10, EffectiveOutputTPS: &a}},
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Success: &failed, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1, EffectiveOutputTPS: &b}},
		{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1, EffectiveOutputTPS: &c}},
	}
	groups := Aggregate(runs)
	if len(groups) != 1 || groups[0].PassedSamples != 1 || *groups[0].PassedTPSP50 != 10 || *groups[0].EffectiveTPSP50 != 100 {
		t.Fatal(groups)
	}
}

func TestCoverageIncludesUndispatchedJobsAndMissingManifests(t *testing.T) {
	runs := []telemetry.Run{{ExperimentID: "e", Agent: "a", Case: "c", Status: "canceled"}}
	groups := ReliabilityTotals(runs, Plan{Experiment: "e", Agent: "a", Case: "c", Repeats: 3}, Plan{Experiment: "e", Agent: "b", Case: "c", Repeats: 3})
	if len(groups) != 2 || *groups[0].Missing != 2 || *groups[1].Missing != 3 || *groups[1].CoverageRate != 0 || groups[1].CompletionRate != nil {
		t.Fatal(groups)
	}
	if ReliabilityTotals(runs)[0].Planned != nil {
		t.Fatal("missing manifest guessed")
	}
	var html bytes.Buffer
	if err := HTML(&html, runs, Plan{Experiment: "e", Agent: "a", Case: "c", Repeats: 3}); err != nil || !strings.Contains(html.String(), "3 / 1 / 2") {
		t.Fatal(err, html.String())
	}
}

func TestMetricCountsUseObservedValuesAndWarnForPassedSubset(t *testing.T) {
	passed := true
	zero := 0.0
	value := 2.0
	runs := []telemetry.Run{}
	for i := 0; i < 10; i++ {
		r := telemetry.Run{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Success: &passed, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1}}
		if i < 3 {
			r.Metrics.TTFASeconds = &value
		}
		if i < 2 {
			r.Metrics.EffectiveOutputTPS = &zero
		}
		if i < 4 {
			r.Metrics.AnswerCompleteSeconds = &value
		}
		if i < 5 {
			r.Metrics.TerminalToExitSeconds = &value
		}
		if i < 7 {
			r.Environment.PreparationSeconds = &zero
		}
		runs = append(runs, r)
	}
	group := Aggregate(runs)[0]
	if group.Counts.Wall != 10 || group.Counts.TTFA != 3 || group.Counts.OutputTPS != 2 || group.Counts.Characters != 0 || group.Counts.AnswerComplete != 4 || group.Counts.ExitGap != 5 || group.Counts.Preparation != 7 || group.PassedCounts.OutputTPS != 2 || group.PassedSamples != 10 {
		t.Fatal(group)
	}
	var html, text bytes.Buffer
	if err := HTML(&html, runs); err != nil {
		t.Fatal(err)
	}
	Text(&text, runs)
	for _, want := range []string{"0.000 (n=2; small sample)", "2.000 (n=3; small sample)", "unknown (n=0)"} {
		if !strings.Contains(html.String(), want) || !strings.Contains(text.String(), want) {
			t.Fatal("missing metric-specific sample evidence", want)
		}
	}
}

func TestWorkflowTimingsAreSeparateFromAgentSpeedAndUnknownHistorically(t *testing.T) {
	value := 0.5
	passed := true
	runs := []telemetry.Run{{ExperimentID: "e", Agent: "a", Case: "c", Status: "completed", Success: &passed, Metrics: telemetry.Metrics{SchemaVersion: 2, WallSeconds: 1}, Timing: &telemetry.AttemptTiming{TotalSeconds: 3, VerificationSeconds: &value, CaptureSeconds: &value, CleanupSeconds: &value, PreparationSeconds: &value}}}
	group := Aggregate(runs)[0]
	if *group.WallP50 != 1 || *group.TotalP50 != 3 || *group.VerificationP50 != 0.5 || group.Counts.Total != 1 {
		t.Fatal(group)
	}
	var html bytes.Buffer
	if err := HTML(&html, runs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "Workflow p50, s") || !strings.Contains(html.String(), "Workflow total: 3.000 s") {
		t.Fatal(html.String())
	}
	runs[0].Timing = nil
	group = Aggregate(runs)[0]
	if group.TotalP50 != nil || group.Counts.Total != 0 {
		t.Fatal("historical timing invented", group)
	}
}
