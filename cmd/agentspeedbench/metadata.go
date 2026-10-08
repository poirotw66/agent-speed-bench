package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/poirotw66/agent-speed-bench/internal/report"
	"github.com/poirotw66/agent-speed-bench/internal/runner"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func versionString() string {
	revision, modified, commitTime := "unknown", "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			case "vcs.time":
				commitTime = setting.Value
			}
		}
	}
	return fmt.Sprintf("AgentSpeedBench 0.1.0-dev\ncommit=%s dirty=%s commit_time=%s\ngo=%s platform=%s/%s", revision, modified, commitTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// Missing or invalid local manifests leave coverage unknown, never guessed.
func reportPlans(runs []telemetry.Run) []report.Plan {
	seen := map[string]bool{}
	plans := []report.Plan{}
	for _, r := range runs {
		if seen[r.ExperimentID] {
			continue
		}
		path := filepath.Join(filepath.Dir(r.ArtifactDir), "manifest.json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m runner.Manifest
		if json.Unmarshal(data, &m) != nil || m.ExperimentID != r.ExperimentID || m.Config.Validate() != nil {
			continue
		}
		seen[r.ExperimentID] = true
		plans = append(plans, manifestPlans(m)...)
	}
	return plans
}

func manifestPlans(m runner.Manifest) []report.Plan {
	plans := []report.Plan{}
	for _, agent := range m.Config.Agents {
		for _, task := range m.Config.Cases {
			plans = append(plans, report.Plan{Experiment: m.ExperimentID, Agent: agent.Name, Case: task.Name, Repeats: m.Config.Repeats})
		}
	}
	return plans
}

func explicitReportPlans(path, experiment string) ([]report.Plan, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
		return nil, fmt.Errorf("manifest must be a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m runner.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.ExperimentID == "" || m.Config.Validate() != nil || (experiment != "" && experiment != m.ExperimentID) {
		return nil, fmt.Errorf("manifest experiment or planned repeats do not match")
	}
	return manifestPlans(m), nil
}
