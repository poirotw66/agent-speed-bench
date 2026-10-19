package report

import (
	"fmt"
	"sort"
)

type ThroughputExperiment struct {
	Experiment string
	Cases      []ThroughputCase
}

type ThroughputCase struct {
	Name        string
	OutputLines *int
	Label       string
	Groups      []Group
}

// Reuse existing aggregate groups without pooling experiments or metric policies.
func throughputExperiments(groups []Group, plans []Plan) []ThroughputExperiment {
	labels := map[string]*int{}
	for _, p := range plans {
		labels[p.Experiment+"\x00"+p.Case] = p.OutputLines
	}
	byExperiment := map[string]map[string][]Group{}
	for _, g := range groups {
		if byExperiment[g.Experiment] == nil {
			byExperiment[g.Experiment] = map[string][]Group{}
		}
		byExperiment[g.Experiment][g.Case] = append(byExperiment[g.Experiment][g.Case], g)
	}
	result := []ThroughputExperiment{}
	for experiment, cases := range byExperiment {
		section := ThroughputExperiment{Experiment: experiment}
		for name, rows := range cases {
			panel := ThroughputCase{Name: name, Label: name, Groups: rows, OutputLines: labels[experiment+"\x00"+name]}
			if panel.OutputLines != nil {
				panel.Label = fmt.Sprintf("%d lines", *panel.OutputLines)
			}
			section.Cases = append(section.Cases, panel)
		}
		sort.Slice(section.Cases, func(i, j int) bool {
			a, b := section.Cases[i], section.Cases[j]
			if a.OutputLines != nil && b.OutputLines != nil && *a.OutputLines != *b.OutputLines {
				return *a.OutputLines < *b.OutputLines
			}
			if (a.OutputLines == nil) != (b.OutputLines == nil) {
				return a.OutputLines != nil
			}
			return a.Name < b.Name
		})
		result = append(result, section)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Experiment < result[j].Experiment })
	return result
}
