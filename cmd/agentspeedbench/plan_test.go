package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/runner"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
)

func planFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	cfg := benchmark.Config{Name: "selection", Jobs: 1, Repeats: 10, WarmupRepeats: 1, TimeoutSeconds: 2,
		Agents: []benchmark.Agent{
			{Name: "unused", Adapter: "generic", Command: "/nonexistent-agent"},
			{Name: "chosen", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}},
		},
		Cases: []benchmark.Case{
			{Name: "short", Prompt: "test", Verify: benchmark.Verify{OutputContains: "Done"}},
			{Name: "medium", Prompt: "test", Verify: benchmark.Verify{OutputContains: "Done"}},
			{Name: "long", Prompt: "test", Repo: &benchmark.Repo{Path: "/nonexistent-repo", Commit: "HEAD"}},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func capturePlan(t *testing.T, args []string) (runPlan, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() { os.Stdout = old }()
	runErr := run(context.Background(), args)
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	var p runPlan
	if runErr == nil {
		if err := json.Unmarshal(data, &p); err != nil {
			t.Fatal(string(data), err)
		}
	}
	return p, runErr
}

func TestDryRunCountsWithoutExecutablesReposOrWrites(t *testing.T) {
	root, path := planFixture(t)
	db, out := filepath.Join(root, "missing", "db"), filepath.Join(root, "artifacts")
	p, err := capturePlan(t, []string{"run", "--dry-run", "--repeats", "3", "--case", "medium,short", "--jobs", "2", "--db", db, "--out", out, path})
	if err != nil || p.Jobs != 2 || p.Repeats != 3 || p.WarmupCalls != 4 || p.MeasuredCalls != 12 || p.TotalCalls != 16 || strings.Join(p.Cases, ",") != "short,medium" {
		t.Fatal(p, err)
	}
	for _, path := range []string{filepath.Dir(db), out} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("dry-run created output", path, err)
		}
	}
}

func TestSelectionAndRepeatValidationBeforeExecution(t *testing.T) {
	_, path := planFixture(t)
	for _, flags := range [][]string{
		{"--repeats", "0"}, {"--repeats", "-1"}, {"--repeats", "1001"},
		{"--jobs", "0"}, {"--agent", "typo"}, {"--case", "typo"}, {"--case", "short,"},
		{"--agent", ""}, {"--case", ""},
	} {
		args := append([]string{"run", "--dry-run"}, flags...)
		if _, err := capturePlan(t, append(args, path)); err == nil {
			t.Fatal("invalid flags accepted", flags)
		}
	}
}

func TestSelectedRunPersistsEffectivePlan(t *testing.T) {
	root, path := planFixture(t)
	db := filepath.Join(root, "runs.db")
	if err := run(context.Background(), []string{"run", "--agent", "chosen", "--case", "short", "--repeats", "2", "--db", db, "--out", root, path}); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	runs, err := s.Runs("")
	if err != nil || len(runs) != 3 {
		t.Fatal(runs, err)
	}
	for _, r := range runs {
		if r.Agent != "chosen" || r.Case != "short" || r.Status != "completed" || r.Success == nil || !*r.Success {
			t.Fatal(r)
		}
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(runs[0].ArtifactDir), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m runner.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.PlannedRuns != 2 || m.WarmupRuns != 1 || m.Config.Repeats != 2 || len(m.Config.Agents) != 1 || len(m.Config.Cases) != 1 {
		t.Fatal(m)
	}
}

func TestCompactNativePlan(t *testing.T) {
	cfg, err := benchmark.Load("../../benchmarks/native-output-100-500.yaml")
	if err != nil || cfg.Repeats != 3 || cfg.Jobs != 1 || cfg.WarmupRepeats != 1 || len(cfg.Agents) != 7 || len(cfg.Cases) != 2 {
		t.Fatal(cfg, err)
	}
	for _, a := range cfg.Agents {
		if (a.Adapter == "agy") == a.IsolateConfig {
			t.Fatal("unexpected isolation policy", a)
		}
	}
	if cfg.Cases[0].Verify.IntegerSequence.End != 100 || cfg.Cases[1].Verify.IntegerSequence.End != 500 {
		t.Fatal("unexpected output lengths")
	}
	p, err := capturePlan(t, []string{"run", "--dry-run", "../../benchmarks/native-output-100-500.yaml"})
	if err != nil || p.WarmupCalls != 14 || p.MeasuredCalls != 42 || p.TotalCalls != 56 || len(p.NativeHomeAgents) != 3 {
		t.Fatal(p, err)
	}
}
