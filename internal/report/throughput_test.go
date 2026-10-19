package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestThroughputPanelsPreservePoliciesAndExperimentBoundaries(t *testing.T) {
	zero, rate, hundred, fiveHundred := 0.0, 12.345, 100, 500
	runs := []telemetry.Run{
		{ExperimentID: "one", Agent: "a", Case: "short", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, EffectiveOutputTPS: &zero}},
		{ExperimentID: "one", Agent: "a", Case: "short", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, EffectiveOutputTPS: &rate, Usage: telemetry.Usage{OutputTokenAccounting: "includes_thinking"}}},
		{ExperimentID: "one", Agent: "a", Case: "medium", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2}},
		{ExperimentID: "two", Agent: "a", Case: "short", Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, EffectiveOutputTPS: &rate}},
		{ExperimentID: "one", Agent: "a", Case: "short", Warmup: true, Status: "completed", Metrics: telemetry.Metrics{SchemaVersion: 2, EffectiveOutputTPS: &rate}},
	}
	plans := []Plan{{Experiment: "one", Agent: "a", Case: "short", OutputLines: &hundred}, {Experiment: "one", Agent: "a", Case: "medium", OutputLines: &fiveHundred}}
	groups := Aggregate(runs)
	panels := throughputExperiments(groups, plans)
	if len(panels) != 2 || len(panels[0].Cases) != 2 || panels[0].Cases[0].Label != "100 lines" || panels[0].Cases[1].Label != "500 lines" || len(panels[0].Cases[0].Groups) != 2 || panels[1].Cases[0].Label != "short" {
		t.Fatal(panels)
	}
	count := 0
	for _, p := range panels {
		for _, c := range p.Cases {
			for _, g := range c.Groups {
				count += g.Runs
			}
		}
	}
	if count != 4 {
		t.Fatal("warmups included or groups pooled", count)
	}
	var rendered bytes.Buffer
	if err := HTML(&rendered, runs, plans...); err != nil {
		t.Fatal(err)
	}
	s := rendered.String()
	start, end := strings.Index(s, `<section id="throughput"`), strings.Index(s, `<details class="metric-guide"`)
	if start < 0 || end < start || end > strings.Index(s, "Reliability across all metric groups") {
		t.Fatal("throughput must lead the report")
	}
	primary := s[start:end]
	for _, want := range []string{"100 lines", "500 lines", "0.00", "12.35", `class="unknown">unknown`, "n=0", "n=1", "includes_thinking", "unknown accounting", "small sample", "Passed / graded"} {
		if !strings.Contains(primary, want) {
			t.Fatal("missing throughput evidence", want)
		}
	}
}

func TestEmptyThroughputReportDoesNotInventSamples(t *testing.T) {
	var rendered bytes.Buffer
	if err := HTML(&rendered, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "No measured samples") || strings.Contains(rendered.String(), `class="throughput-value">0.00`) {
		t.Fatal(rendered.String())
	}
}
