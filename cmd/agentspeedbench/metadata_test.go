package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestRunReportsCaptureFailureWithoutErasingStoredGrade(t *testing.T) {
	root := t.TempDir()
	config := benchmark.Config{Name: "capture-grade", Repeats: 1, Jobs: 1, TimeoutSeconds: 2, Agents: []benchmark.Agent{{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}}, Cases: []benchmark.Case{{Name: "task", Prompt: "offline fixture", RetainFiles: []string{"missing.txt"}, Verify: benchmark.Verify{OutputContains: "Done"}}}}
	path := filepath.Join(root, "config.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "runs.db")
	if err := run(context.Background(), []string{"run", "-db", db, "-out", root, path}); err == nil || !strings.Contains(err.Error(), "grading results are preserved") {
		t.Fatal(err)
	}
	store, err := storage.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := store.Runs("")
	if err != nil || len(rows) != 1 || rows[0].Status != "completed" || rows[0].Success == nil || !*rows[0].Success || len(rows[0].ArtifactErrors) != 1 {
		t.Fatal(rows, err)
	}
}
