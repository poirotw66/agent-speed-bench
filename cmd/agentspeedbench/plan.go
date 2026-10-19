package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

type runOptions struct {
	Jobs, Repeats *int
	Agents, Cases string
}

func applyRunOptions(cfg benchmark.Config, opts runOptions) (benchmark.Config, error) {
	if opts.Jobs != nil {
		cfg.Jobs = *opts.Jobs
	}
	if opts.Repeats != nil {
		cfg.Repeats = *opts.Repeats
	}
	agents, cases := []string{}, []string{}
	for _, a := range cfg.Agents {
		agents = append(agents, a.Name)
	}
	for _, c := range cfg.Cases {
		cases = append(cases, c.Name)
	}
	selectedAgents, err := selectNames(opts.Agents, agents)
	if err != nil {
		return cfg, fmt.Errorf("--agent: %w", err)
	}
	selectedCases, err := selectNames(opts.Cases, cases)
	if err != nil {
		return cfg, fmt.Errorf("--case: %w", err)
	}
	// Preserve configuration order and avoid mutating the source slices.
	filteredAgents := []benchmark.Agent{}
	for _, a := range cfg.Agents {
		if selectedAgents[a.Name] {
			filteredAgents = append(filteredAgents, a)
		}
	}
	filteredCases := []benchmark.Case{}
	for _, c := range cfg.Cases {
		if selectedCases[c.Name] {
			filteredCases = append(filteredCases, c)
		}
	}
	cfg.Agents, cfg.Cases = filteredAgents, filteredCases
	return cfg, cfg.Validate()
}

func selectNames(value string, available []string) (map[string]bool, error) {
	known := map[string]bool{}
	for _, name := range available {
		known[name] = true
	}
	if value == "" {
		return known, nil
	}
	selected := map[string]bool{}
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		if !known[name] {
			return nil, fmt.Errorf("unknown name %q; available: %s", name, strings.Join(available, ", "))
		}
		selected[name] = true
	}
	return selected, nil
}

type runPlan struct {
	Name             string   `json:"name"`
	Agents           []string `json:"agents"`
	Cases            []string `json:"cases"`
	Jobs             int      `json:"jobs"`
	WarmupRepeats    int      `json:"warmup_repeats"`
	Repeats          int      `json:"repeats"`
	WarmupCalls      int      `json:"warmup_calls"`
	MeasuredCalls    int      `json:"measured_calls"`
	TotalCalls       int      `json:"total_calls"`
	NativeHomeAgents []string `json:"native_home_agents"`
	Notes            []string `json:"notes"`
}

func printRunPlan(w io.Writer, cfg benchmark.Config, jsonOutput bool) error {
	p := runPlan{Name: cfg.Name, Agents: []string{}, Cases: []string{}, Jobs: cfg.Jobs, WarmupRepeats: cfg.WarmupRepeats, Repeats: cfg.Repeats, NativeHomeAgents: []string{}, Notes: []string{}}
	for _, a := range cfg.Agents {
		p.Agents = append(p.Agents, a.Name)
		if !a.IsolateConfig && (a.Adapter == "codex" || a.Adapter == "cursor" || a.Adapter == "agy" || a.Adapter == "claude") {
			p.NativeHomeAgents = append(p.NativeHomeAgents, a.Name)
		}
		if runtime.GOOS == "darwin" && a.Adapter == "agy" && a.IsolateConfig {
			p.Notes = append(p.Notes, "agent "+a.Name+": isolated agy is unsupported on macOS; explicitly select native HOME")
		}
	}
	for _, c := range cfg.Cases {
		p.Cases = append(p.Cases, c.Name)
	}
	p.WarmupCalls = len(p.Agents) * len(p.Cases) * cfg.WarmupRepeats
	p.MeasuredCalls = len(p.Agents) * len(p.Cases) * cfg.Repeats
	p.TotalCalls = p.WarmupCalls + p.MeasuredCalls
	if jsonOutput {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		return e.Encode(p)
	}
	if _, err := fmt.Fprintf(w, "Plan: agents=[%s] cases=[%s] jobs=%d warmups=%d measured=%d total=%d (measured repeats=%d)\n", strings.Join(p.Agents, ", "), strings.Join(p.Cases, ", "), p.Jobs, p.WarmupCalls, p.MeasuredCalls, p.TotalCalls, p.Repeats); err != nil {
		return err
	}
	if len(p.NativeHomeAgents) > 0 {
		if _, err := fmt.Fprintf(w, "Native HOME: %s; inherited user settings may affect results.\n", strings.Join(p.NativeHomeAgents, ", ")); err != nil {
			return err
		}
	}
	return nil
}
