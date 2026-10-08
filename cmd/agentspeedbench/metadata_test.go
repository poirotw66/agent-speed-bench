package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/runner"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestManifestCoverageRequiresMatchingValidEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest.json")
	m := runner.Manifest{ExperimentID: "e", Config: benchmark.Config{Name: "test", Repeats: 3, Jobs: 1, TimeoutSeconds: 1, Agents: []benchmark.Agent{{Name: "a", Adapter: "generic", Command: "sh"}}, Cases: []benchmark.Case{{Name: "c", Prompt: "test", TimeoutSeconds: 1, Verify: benchmark.Verify{TimeoutSeconds: 1}}}}}
	if err := storage.WriteJSON(path, m); err != nil {
		t.Fatal(err)
	}
	plans, err := explicitReportPlans(path, "e")
	if err != nil || len(plans) != 1 || plans[0].Repeats != 3 {
		t.Fatal(plans, err)
	}
	if _, err := explicitReportPlans(path, "other"); err == nil {
		t.Fatal("mismatched manifest accepted")
	}
	runs := []telemetry.Run{{ExperimentID: "e", ArtifactDir: filepath.Join(root, "attempt")}}
	if len(reportPlans(runs)) != 1 {
		t.Fatal("automatic manifest discovery failed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if len(reportPlans(runs)) != 0 {
		t.Fatal("missing manifest guessed")
	}
}
