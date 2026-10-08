package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"syscall"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func jobKey(agent, task string, repeat int, warmup bool) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%t", agent, task, repeat, warmup)
}

// The stable lock inode is never removed; OS locks release after crashes.
func claimExperiment(dir string) (func(), error) {
	path := filepath.Join(dir, ".run.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("experiment is already active")
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintln(f, os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}

func readRecord(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxLineBytes {
		return errors.New("record must be a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, maxLineBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("record must contain one bounded JSON document")
	}
	return nil
}

// Resume only appends missing logical jobs; recorded failures/cancellations remain evidence.
func Resume(ctx context.Context, dir string, store *storage.Store, progress io.Writer) (Result, error) {
	var result Result
	abs, err := filepath.Abs(dir)
	if err != nil {
		return result, err
	}
	release, err := claimExperiment(abs)
	if err != nil {
		return result, err
	}
	defer release()
	if err := readRecord(filepath.Join(abs, "manifest.json"), &result.Manifest); err != nil {
		return result, err
	}
	m := &result.Manifest
	if m.SchemaVersion != 3 || m.Provenance.BinarySHA256 == "" {
		return Result{}, errors.New("legacy experiment lacks verified provenance; start a separate experiment instead")
	}
	cfg := m.Config
	infos, err := Preflight(ctx, cfg)
	if err != nil {
		return Result{}, err
	}
	if err := ResolveRepos(ctx, &cfg); err != nil {
		return Result{}, err
	}
	p, err := provenance(cfg)
	if err != nil {
		return Result{}, err
	}
	if !reflect.DeepEqual(p, m.Provenance) || !reflect.DeepEqual(infos, m.Agents) {
		return Result{}, errors.New("harness, config, CLI or direct verifier changed; start a separate experiment instead")
	}
	rows, err := store.Runs(m.ExperimentID)
	if err != nil {
		return Result{}, err
	}
	byID := map[string]telemetry.Run{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Result{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(abs, entry.Name(), "run.json")
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		var r telemetry.Run
		if err := readRecord(path, &r); err != nil {
			return Result{}, err
		}
		if r.ExperimentID != m.ExperimentID || r.ID != filepath.Base(filepath.Dir(path)) || r.ArtifactDir != filepath.Dir(path) {
			return Result{}, errors.New("artifact identity does not match experiment")
		}
		if old, exists := byID[r.ID]; exists && !reflect.DeepEqual(old, r) {
			return Result{}, errors.New("artifact and database evidence disagree")
		}
		byID[r.ID] = r
	}
	expected := map[string]bool{}
	for _, a := range cfg.Agents {
		for _, task := range cfg.Cases {
			for repeat := 1; repeat <= cfg.Repeats; repeat++ {
				expected[jobKey(a.Name, task.Name, repeat, false)] = true
			}
			for repeat := 1; repeat <= cfg.WarmupRepeats; repeat++ {
				expected[jobKey(a.Name, task.Name, repeat, true)] = true
			}
		}
	}
	covered := map[string]bool{}
	m.WarmupRuns = cfg.WarmupRepeats * len(cfg.Agents) * len(cfg.Cases)
	m.PlannedRuns = cfg.Repeats * len(cfg.Agents) * len(cfg.Cases)
	m.StartedRuns, m.SkippedRuns, m.WarmupStartedRuns, m.WarmupSkippedRuns = 0, 0, 0, 0
	m.StoppedAgents = map[string]*telemetry.Failure{}
	for _, r := range byID {
		key := jobKey(r.Agent, r.Case, r.Repeat, r.Warmup)
		if !expected[key] || covered[key] {
			return Result{}, errors.New("unexpected or duplicate logical job in experiment")
		}
		covered[key] = true
		if r.Warmup {
			if r.Status == "skipped" {
				m.WarmupSkippedRuns++
			} else {
				m.WarmupStartedRuns++
			}
		} else if r.Status == "skipped" {
			m.SkippedRuns++
		} else {
			m.StartedRuns++
		}
		if r.Failure != nil && r.Failure.Scope == "agent" && !r.Failure.Retryable {
			m.StoppedAgents[r.Agent] = r.Failure
		}
		result.Runs = append(result.Runs, r)
	}
	// Reconcile artifact-only completions after validating the entire evidence set.
	for _, r := range result.Runs {
		if err := store.Reconcile(r); err != nil {
			return Result{}, err
		}
	}
	result.Directory = abs
	if len(covered) == len(expected) {
		sort.Slice(result.Runs, func(i, j int) bool { return result.Runs[i].ID < result.Runs[j].ID })
		m.State = "complete"
		if m.FinishedAt.IsZero() {
			m.FinishedAt = time.Now().UTC()
		}
		if err := storage.WriteJSON(filepath.Join(abs, "manifest.json"), m); err != nil {
			return result, err
		}
		if err := storage.WriteJSON(filepath.Join(abs, "summary.json"), result); err != nil {
			return result, err
		}
		return result, nil
	}
	m.State, m.FinishedAt = "running", time.Time{}
	m.Resumes++
	if err := storage.WriteJSON(filepath.Join(abs, "manifest.json"), m); err != nil {
		return Result{}, err
	}
	return executeMatrix(ctx, cfg, result, store, progress)
}
