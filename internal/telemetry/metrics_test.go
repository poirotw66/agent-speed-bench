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
	if *m.TTFASeconds != 2 || m.ToolCalls != 3 || m.MatchedToolCalls != 2 || *m.ToolReceiptIntervalMeanSeconds != 2.5 || *m.ToolReceiptIntervalP95Seconds != 4 || *m.EffectiveOutputTPS != 10 || len(m.Warnings) != 2 {
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

func TestReceiptTimingIsNotToolRuntime(t *testing.T) {
	m := Calculate([]Event{
		{Type: "assistant_output", TimingBasis: "text_delta_receipt", ElapsedNS: 100},
		{Type: "assistant_message_receipt", TimingBasis: "complete_message_receipt", ElapsedNS: 200},
		{Type: "tool_started", ToolID: "t", ElapsedNS: 1000000},
		{Type: "tool_finished", ToolID: "t", ElapsedNS: 1053542},
	}, 1, nil)
	if m.ToolLatencyMeanSeconds != nil || m.ToolReceiptIntervalMeanSeconds == nil || *m.ToolReceiptIntervalMeanSeconds != .000053542 || m.TTFABasis != "text_delta_receipt" || m.FirstCompleteMessageSeconds == nil || m.FirstToolActionSeconds == nil {
		t.Fatal(m)
	}
	legacy := ReportRun(Run{Metrics: Metrics{ToolLatencyMeanSeconds: m.ToolReceiptIntervalMeanSeconds}})
	if legacy.Metrics.ToolLatencyMeanSeconds != nil || legacy.Metrics.ToolReceiptIntervalMeanSeconds == nil || legacy.Metrics.TTFABasis != "legacy_unspecified" {
		t.Fatal(legacy)
	}
}

func TestFinalOnlyResultEstablishesCompleteMessageReceipt(t *testing.T) {
	m := Calculate([]Event{{Type: "final_output", Text: "Done", ElapsedNS: 2000000000}}, 3, nil)
	if m.TTFASeconds == nil || *m.TTFASeconds != 2 || m.TTFABasis != "complete_message_receipt" || m.FirstTextDeltaSeconds != nil || m.FirstCompleteMessageSeconds == nil {
		t.Fatal(m)
	}
}

func TestCompletionReceiptAndExitGap(t *testing.T) {
	events := []Event{{Type: "assistant_output", Text: "first", TimingBasis: "complete_message_receipt", ElapsedNS: 2e9}, {Type: "assistant_output", Text: "last", TimingBasis: "complete_message_receipt", ElapsedNS: 4e9}, {Type: "agent_completed", ElapsedNS: 5e9}}
	m := Calculate(events, 8, nil)
	if *m.AnswerCompleteSeconds != 4 || *m.FirstCompleteMessageSeconds != 2 || *m.TerminalReceiptSeconds != 5 || *m.TerminalToExitSeconds != 3 || m.GenerationTPS != nil {
		t.Fatal(m)
	}
	m = Calculate(events[:2], 8, nil)
	if m.TerminalToExitSeconds != nil {
		t.Fatal(m)
	}
	m = Calculate(events, 4, nil)
	if m.TerminalToExitSeconds != nil {
		t.Fatal("negative exit interval", m)
	}
}

func TestSSEReceiveIntervalExcludesFirstChunk(t *testing.T) {
	events := []Event{{Type: "assistant_output", Text: "first chunk", ElapsedNS: 1000000000, StreamTransport: "responses_http_sse_relay"}, {Type: "assistant_output", Text: "\u4e2d\u6587\u5b57", ElapsedNS: 3000000000, StreamTransport: "responses_http_sse_relay"}}
	m := Calculate(events, 4, nil)
	if m.StreamReceiveSeconds == nil || *m.StreamReceiveSeconds != 2 || *m.StreamReceiveCharactersPerSecond != 1.5 || m.GenerationTPS != nil {
		t.Fatal(m)
	}
	if Calculate(events[:1], 4, nil).StreamReceiveSeconds != nil {
		t.Fatal("single chunk invented interval")
	}
}
