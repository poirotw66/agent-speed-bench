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
