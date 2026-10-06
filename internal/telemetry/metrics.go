package telemetry

import (
	"math"
	"sort"
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
	m := Metrics{WallSeconds: wall, FirstStdoutSeconds: firstStdout}
	starts := map[string]int64{}
	finished := map[string]bool{}
	var latencies []float64
	for _, e := range events {
		if m.TTFASeconds == nil && (e.Type == "assistant_output" || e.Type == "tool_started") {
			s := float64(e.ElapsedNS) / 1e9
			m.TTFASeconds = &s
		}
		switch e.Type {
		case "usage_reported":
			if e.Usage != nil {
				add(&m.Usage.InputTokens, e.Usage.InputTokens)
				add(&m.Usage.OutputTokens, e.Usage.OutputTokens)
				add(&m.Usage.CachedTokens, e.Usage.CachedTokens)
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
		m.ToolLatencyMeanSeconds = &mean
		m.ToolLatencyP50Seconds = Percentile(latencies, .5)
		m.ToolLatencyP95Seconds = Percentile(latencies, .95)
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
