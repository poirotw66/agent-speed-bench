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
