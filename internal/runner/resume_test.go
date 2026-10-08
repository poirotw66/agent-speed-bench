//go:build darwin || linux

package runner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
)

func TestResumeOnlyMissingJobsAndReconcilesArtifactOnlyCompletion(t *testing.T) {
	root := t.TempDir()
	initial, err := storage.Open(filepath.Join(root, "initial.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer initial.Close()
	task := fixtureTask()
	task.Verify.OutputContains = "Done"
	cfg := benchmark.Config{Name: "resume", Repeats: 3, WarmupRepeats: 1, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{task}, Agents: []benchmark.Agent{{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}}}
	first, err := Execute(context.Background(), cfg, root, initial, io.Discard)
	if err != nil || len(first.Runs) != 4 {
		t.Fatal(first, err)
	}
	retained := []string{}
	for _, r := range first.Runs {
		if !r.Warmup && r.Repeat == 3 {
			if err := os.RemoveAll(r.ArtifactDir); err != nil {
				t.Fatal(err)
			}
		} else {
			retained = append(retained, r.ID)
		}
	}
	recovered, err := storage.Open(filepath.Join(root, "recovered.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	result, err := Resume(context.Background(), first.Directory, recovered, io.Discard)
	if err != nil || len(result.Runs) != 4 || result.Manifest.Resumes != 1 || result.Manifest.StartedRuns != 3 || result.Manifest.WarmupStartedRuns != 1 || result.Manifest.State != "complete" {
		t.Fatal(result, err)
	}
	for _, id := range retained {
		found := false
		for _, r := range result.Runs {
			if r.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatal("recorded attempt repeated", id)
		}
	}
	rows, err := recovered.Runs(result.Manifest.ExperimentID)
	if err != nil || len(rows) != 4 {
		t.Fatal(rows, err)
	}
	// Simulate a crash after the final record, before the final checkpoint.
	result.Manifest.State = "running"
	result.Manifest.FinishedAt = time.Time{}
	if err := storage.WriteJSON(filepath.Join(first.Directory, "manifest.json"), result.Manifest); err != nil {
		t.Fatal(err)
	}
	again, err := Resume(context.Background(), first.Directory, recovered, io.Discard)
	if err != nil || len(again.Runs) != 4 || again.Manifest.Resumes != 1 || again.Manifest.State != "complete" || again.Manifest.FinishedAt.IsZero() {
		t.Fatal(again, err)
	}
}

func TestResumeRejectsChangedProvenanceLegacyAndConcurrentOwner(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := benchmark.Config{Name: "guard", Repeats: 1, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{fixtureTask()}, Agents: []benchmark.Agent{{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}}}
	first, err := Execute(context.Background(), cfg, root, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	release, err := claimExperiment(first.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Resume(context.Background(), first.Directory, store, io.Discard)
	release()
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatal(err)
	}
	first.Manifest.Provenance.BinarySHA256 = "changed"
	storage.WriteJSON(filepath.Join(first.Directory, "manifest.json"), first.Manifest)
	if _, err := Resume(context.Background(), first.Directory, store, io.Discard); err == nil {
		t.Fatal("mixed builds accepted")
	}
	first.Manifest.SchemaVersion = 2
	storage.WriteJSON(filepath.Join(first.Directory, "manifest.json"), first.Manifest)
	if _, err := Resume(context.Background(), first.Directory, store, io.Discard); err == nil || !strings.Contains(err.Error(), "legacy") {
		t.Fatal(err)
	}
}

func TestResumeRejectsChangedDirectVerifierBeforeScheduling(t *testing.T) {
	root := t.TempDir()
	verifier := filepath.Join(root, "verify.sh")
	if err := os.WriteFile(verifier, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(root, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := fixtureTask()
	task.Verify.Command = verifier
	cfg := benchmark.Config{Name: "verifier-guard", Repeats: 1, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{task}, Agents: []benchmark.Agent{{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}}}
	first, err := Execute(context.Background(), cfg, root, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verifier, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Resume(context.Background(), first.Directory, store, io.Discard); err == nil || !strings.Contains(err.Error(), "verifier changed") {
		t.Fatal("changed verifier accepted", err)
	}
	rows, err := store.Runs(first.Manifest.ExperimentID)
	if err != nil || len(rows) != 1 || rows[0].ID != first.Runs[0].ID {
		t.Fatal("evidence changed", rows, err)
	}
}
