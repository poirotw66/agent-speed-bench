package telemetry

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// Percentile uses the nearest-rank convention, including for small samples.
func Percentile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	i := max(0, min(len(v)-1, int(math.Ceil(float64(len(v))*p))-1))
	return &v[i]
}

func Calculate(events []Event, wall float64, firstStdout *float64) Metrics {
	m := Metrics{SchemaVersion: 2, TTFABasis: "unobserved", WallSeconds: wall, FirstStdoutSeconds: firstStdout, ToolTimingBasis: "runner_receipt", ToolTimingConfidence: "unverified"}
	starts := map[string]int64{}
	finished := map[string]bool{}
	var latencies []float64
	var streamFirst, streamLast int64
	var streamChunks, streamCharacters int64
	for _, e := range events {
		if e.Type == "assistant_output" && e.StreamTransport == "responses_http_sse_relay" && e.Text != "" {
			if streamChunks == 0 {
				streamFirst = e.ElapsedNS
			} else {
				streamCharacters += int64(utf8.RuneCountInString(e.Text))
			}
			streamLast = e.ElapsedNS
			streamChunks++
		}
		if m.TTFASeconds == nil && (e.Type == "assistant_output" || e.Type == "tool_started" || e.Type == "assistant_message_receipt" || (e.Type == "final_output" && e.Text != "")) {
			s := float64(e.ElapsedNS) / 1e9
			m.TTFASeconds = &s
			m.TTFABasis = e.TimingBasis
			if m.TTFABasis == "" {
				m.TTFABasis = "stdout_line_receipt"
				if e.Type == "final_output" {
					m.TTFABasis = "complete_message_receipt"
				}
			}
		}
		seconds := float64(e.ElapsedNS) / 1e9
		if e.Type == "final_output" || e.Type == "assistant_message_receipt" || (e.Type == "assistant_output" && e.TimingBasis == "complete_message_receipt") {
			m.AnswerCompleteSeconds = &seconds
		}
		if e.Type == "agent_completed" {
			m.TerminalReceiptSeconds = &seconds
		}
		if e.Type == "assistant_output" || e.Type == "assistant_message_receipt" || (e.Type == "final_output" && e.Text != "") {
			if e.TimingBasis == "text_delta_receipt" && m.FirstTextDeltaSeconds == nil {
				m.FirstTextDeltaSeconds = &seconds
			}
			if (e.TimingBasis == "complete_message_receipt" || e.Type == "final_output") && m.FirstCompleteMessageSeconds == nil {
				m.FirstCompleteMessageSeconds = &seconds
			}
		}
		if e.Type == "tool_started" && m.FirstToolActionSeconds == nil {
			m.FirstToolActionSeconds = &seconds
		}
		if e.ReportedDurationSeconds != nil {
			m.ReportedDurationSeconds = e.ReportedDurationSeconds
			m.ReportedDurationSource = e.ReportedDurationSource
		}
		switch e.Type {
		case "usage_reported":
			if e.Usage != nil {
				add(&m.Usage.InputTokens, e.Usage.InputTokens)
				add(&m.Usage.OutputTokens, e.Usage.OutputTokens)
				add(&m.Usage.CachedTokens, e.Usage.CachedTokens)
				add(&m.Usage.ThinkingTokens, e.Usage.ThinkingTokens)
				add(&m.Usage.CacheWriteTokens, e.Usage.CacheWriteTokens)
			}
		case "usage_total":
			if e.Usage != nil {
				m.Usage = *e.Usage
			}
		case "tool_started":
			if e.ToolID == "" {
				m.Warnings = append(m.Warnings, "Tool start without an ID")
				continue
			}
			if _, ok := starts[e.ToolID]; !ok {
				starts[e.ToolID] = e.ElapsedNS
				m.ToolCalls++
			}
		case "tool_finished":
			if start, ok := starts[e.ToolID]; ok && !finished[e.ToolID] && e.ElapsedNS >= start {
				latencies = append(latencies, float64(e.ElapsedNS-start)/1e9)
				finished[e.ToolID] = true
			} else {
				m.Warnings = append(m.Warnings, "Unmatched or duplicate tool finish: "+e.ToolID)
			}
		}
	}
	if m.TerminalReceiptSeconds != nil && wall >= *m.TerminalReceiptSeconds {
		gap := wall - *m.TerminalReceiptSeconds
		m.TerminalToExitSeconds = &gap
	}
	if streamChunks > 1 && streamLast > streamFirst {
		interval := float64(streamLast-streamFirst) / 1e9
		rate := float64(streamCharacters) / interval
		m.StreamReceiveSeconds, m.StreamReceiveCharactersPerSecond = &interval, &rate
		m.StreamReceiveBasis = "first_to_last_sse_relay_delta_excluding_first_chunk"
	}
	m.MatchedToolCalls = len(latencies)
	if m.MatchedToolCalls < m.ToolCalls {
		m.Warnings = append(m.Warnings, "Some tool calls have no observed finish")
	}
	if len(latencies) > 0 {
		var total float64
		for _, v := range latencies {
			total += v
		}
		mean := total / float64(len(latencies))
		m.ToolReceiptIntervalMeanSeconds = &mean
		m.Warnings = append(m.Warnings, "Tool receipt intervals do not establish actual execution duration")
		m.ToolReceiptIntervalP50Seconds = Percentile(latencies, .5)
		m.ToolReceiptIntervalP95Seconds = Percentile(latencies, .95)
	}
	if m.Usage.OutputTokens != nil && wall > 0 {
		rate := float64(*m.Usage.OutputTokens) / wall
		m.EffectiveOutputTPS = &rate
	}
	// CLI item timestamps do not establish model decoding or active intervals.
	// GenerationTPS and ModelActiveTPS deliberately remain unknown.
	return m
}

func add(dst **int64, src *int64) {
	if src == nil {
		return
	}
	if *dst == nil {
		v := *src
		*dst = &v
	} else {
		**dst += *src
	}
}

// ReportRun reinterprets legacy receipt intervals without rewriting stored evidence.
func ReportRun(r Run) Run {
	if r.Failure != nil && IsUsageLimit(r.Failure.Code, r.Failure.Message) {
		failure := *r.Failure
		failure.Category, failure.Code, failure.Scope, failure.Retryable = "quota", "usage_limit_reached", "agent", false
		r.Failure = &failure
		r.Status, r.Success = "agent_unavailable", nil
	}
	if r.Metrics.SchemaVersion < 2 {
		r.Metrics.ToolReceiptIntervalMeanSeconds = r.Metrics.ToolLatencyMeanSeconds
		r.Metrics.ToolReceiptIntervalP50Seconds = r.Metrics.ToolLatencyP50Seconds
		r.Metrics.ToolReceiptIntervalP95Seconds = r.Metrics.ToolLatencyP95Seconds
		r.Metrics.ToolLatencyMeanSeconds = nil
		r.Metrics.ToolLatencyP50Seconds = nil
		r.Metrics.ToolLatencyP95Seconds = nil
		r.Metrics.TTFABasis = "legacy_unspecified"
		r.Metrics.ToolTimingBasis = "runner_receipt"
		r.Metrics.ToolTimingConfidence = "unverified"
	}
	return r
}

func IsUsageLimit(code, message string) bool {
	message = strings.ToLower(message)
	return code == "insufficient_quota" || code == "usage_limit_reached" || strings.Contains(message, "hit your usage limit") || strings.Contains(message, "usage limit reached")
}
