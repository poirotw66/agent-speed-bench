package report

import (
	"sort"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

// Reliability totals do not depend on which metrics an attempt supplied.
type Reliability struct {
	Experiment, Agent, Case                                    string
	Attempts, Completed, Graded, Passed, Unavailable, Canceled int
	Skipped, Timeouts                                          int
	SuccessRate, CompletionRate                                *float64
}

func ReliabilityTotals(runs []telemetry.Run) []Reliability {
	groups := map[string]*Reliability{}
	for _, original := range runs {
		if original.Warmup {
			continue
		}
		r := telemetry.ReportRun(original)
		key := r.ExperimentID + "\x00" + r.Agent + "\x00" + r.Case
		g := groups[key]
		if g == nil {
			g = &Reliability{Experiment: r.ExperimentID, Agent: r.Agent, Case: r.Case}
			groups[key] = g
		}
		g.Attempts++
		switch r.Status {
		case "completed":
			g.Completed++
		case "canceled":
			g.Canceled++
		case "skipped":
			g.Skipped++
		case "timeout":
			g.Timeouts++
		case "agent_unavailable", "service_error", "infrastructure_error", "telemetry_error":
			g.Unavailable++
		}
		if r.Success != nil {
			g.Graded++
			if *r.Success {
				g.Passed++
			}
		}
	}
	result := []Reliability{}
	for _, g := range groups {
		if g.Graded > 0 {
			rate := 100 * float64(g.Passed) / float64(g.Graded)
			g.SuccessRate = &rate
		}
		rate := 100 * float64(g.Completed) / float64(g.Attempts)
		g.CompletionRate = &rate
		result = append(result, *g)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		return left.Experiment+"\x00"+left.Agent+"\x00"+left.Case < right.Experiment+"\x00"+right.Agent+"\x00"+right.Case
	})
	return result
}
