package telemetry

import "testing"

func TestMetricsUnknownAndZeroAreDistinct(t *testing.T) {
	m := Calculate(nil, 5, nil)
	if m.EffectiveOutputTPS != nil || m.Usage.OutputTokens != nil || m.TTFASeconds != nil {
		t.Fatal(m)
	}
	zero := int64(0)
	m = Calculate([]Event{{Type: "usage_reported", Usage: &Usage{OutputTokens: &zero}}}, 5, nil)
	if m.EffectiveOutputTPS == nil || *m.EffectiveOutputTPS != 0 {
		t.Fatal(m)
	}
	if m.GenerationTPS != nil || m.ModelActiveTPS != nil {
		t.Fatal("missing decoding intervals must remain unknown")
	}
}

func TestMetricsIgnoreStartupAndPairToolsByID(t *testing.T) {
	tokens := int64(100)
	events := []Event{
		{Type: "agent_ready", ElapsedNS: 1e9},
		{Type: "tool_started", ToolID: "a", ElapsedNS: 2e9},
		{Type: "tool_started", ToolID: "b", ElapsedNS: 3e9},
		{Type: "tool_finished", ToolID: "b", ElapsedNS: 4e9},
		{Type: "tool_finished", ToolID: "a", ElapsedNS: 6e9},
		{Type: "tool_started", ToolID: "c", ElapsedNS: 7e9},
		{Type: "usage_reported", Usage: &Usage{OutputTokens: &tokens}},
	}
	m := Calculate(events, 10, nil)
	if *m.TTFASeconds != 2 || m.ToolCalls != 3 || m.MatchedToolCalls != 2 || *m.ToolLatencyMeanSeconds != 2.5 || *m.ToolLatencyP95Seconds != 4 || *m.EffectiveOutputTPS != 10 || len(m.Warnings) != 1 {
		t.Fatal(m)
	}
}

func TestUsageTotalsAndPercentiles(t *testing.T) {
	a, b, total := int64(4), int64(6), int64(20)
	m := Calculate([]Event{{Type: "usage_reported", Usage: &Usage{OutputTokens: &a}}, {Type: "usage_reported", Usage: &Usage{OutputTokens: &b}}, {Type: "usage_total", Usage: &Usage{OutputTokens: &total}}}, 2, nil)
	if *m.Usage.OutputTokens != 20 {
		t.Fatal(m.Usage)
	}
	values := []float64{5, 1, 3, 2, 4}
	if *Percentile(values, .5) != 3 || *Percentile(values, .95) != 5 || values[0] != 5 || Percentile(nil, .5) != nil {
		t.Fatal("percentile convention or mutation")
	}
}
