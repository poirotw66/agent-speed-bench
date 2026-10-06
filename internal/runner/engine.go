package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type Manifest struct {
	SchemaVersion  int              `json:"schema_version"`
	ExperimentID   string           `json:"experiment_id"`
	StartedAt      time.Time        `json:"started_at"`
	FinishedAt     time.Time        `json:"finished_at"`
	ElapsedSeconds float64          `json:"elapsed_seconds"`
	Platform       string           `json:"platform"`
	GoVersion      string           `json:"go_version"`
	Config         benchmark.Config `json:"config"`
	Agents         []AgentInfo      `json:"agents"`
}
type AgentInfo struct {
	Name         string                `json:"name"`
	Executable   string                `json:"executable"`
	Version      string                `json:"version"`
	Capabilities adapters.Capabilities `json:"capabilities"`
}
type Result struct {
	Manifest  Manifest        `json:"manifest"`
	Runs      []telemetry.Run `json:"runs"`
	Directory string          `json:"directory"`
}

func Preflight(ctx context.Context, cfg benchmark.Config) ([]AgentInfo, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("benchmark execution currently supports macOS and Linux process groups")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	infos := []AgentInfo{}
	for _, a := range cfg.Agents {
		adapter, err := adapters.New(a)
		if err != nil {
			return nil, err
		}
		c, err := adapter.BuildCommand("preflight", "")
		if err != nil {
			return nil, err
		}
		path, err := exec.LookPath(c.Path)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", a.Name, err)
		}
		info := AgentInfo{Name: a.Name, Executable: path, Version: "unknown", Capabilities: adapter.Capabilities()}
		args := a.VersionArgs
		if len(args) == 0 && a.Adapter != "generic" && a.Adapter != "demo" {
			args = []string{"--version"}
		}
		if a.Adapter == "demo" {
			info.Version = "synthetic-v1"
		} else if len(args) > 0 {
			vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			var out limitedBuffer
			r := RunProcess(vctx, adapters.Command{Path: path, Args: args}, "", &out, &out)
			cancel()
			if r.Err == nil {
				info.Version = strings.TrimSpace(out.String())
			}
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func Execute(ctx context.Context, cfg benchmark.Config, root string, store *storage.Store, progress io.Writer) (Result, error) {
	var result Result
	infos, err := Preflight(ctx, cfg)
	if err != nil {
		return result, err
	}
	if err := ResolveRepos(ctx, &cfg); err != nil {
		return result, err
	}
	for i := range cfg.Agents {
		cfg.Agents[i].Command = infos[i].Executable
	}
	id, err := newID()
	if err != nil {
		return result, err
	}
	experiment := cfg.Name + "-" + time.Now().UTC().Format("20060102T150405Z") + "-" + id
	dir, err := filepath.Abs(filepath.Join(root, experiment))
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return result, err
	}
	start := time.Now()
	result.Directory = dir
	result.Manifest = Manifest{SchemaVersion: 1, ExperimentID: experiment, StartedAt: start.UTC(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), Config: cfg, Agents: infos}
	if err := storage.WriteJSON(filepath.Join(dir, "manifest.json"), result.Manifest); err != nil {
		return result, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type job struct {
		agent  benchmark.Agent
		task   benchmark.Case
		repeat int
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for range cfg.Jobs {
		wg.Go(func() {
			for j := range jobs {
				r, runErr := executeOne(ctx, j.agent, j.task, j.repeat, experiment, dir)
				mu.Lock()
				if runErr == nil {
					runErr = store.Save(r)
				}
				if runErr != nil && firstErr == nil {
					firstErr = runErr
					cancel()
				}
				result.Runs = append(result.Runs, r)
				if progress != nil {
					fmt.Fprintf(progress, "%-14s %-22s repeat=%d status=%s wall=%.3fs\n", r.Agent, r.Case, r.Repeat, r.Status, r.Metrics.WallSeconds)
				}
				mu.Unlock()
			}
		})
	}
	// Rotate agent order between repeats to reduce fixed ordering bias.
dispatch:
	for repeat := 1; repeat <= cfg.Repeats; repeat++ {
		for _, task := range cfg.Cases {
			for i := range cfg.Agents {
				a := cfg.Agents[(i+repeat-1)%len(cfg.Agents)]
				select {
				case jobs <- job{a, task, repeat}:
				case <-ctx.Done():
					break dispatch
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(result.Runs, func(i, j int) bool { return result.Runs[i].ID < result.Runs[j].ID })
	result.Manifest.FinishedAt = time.Now().UTC()
	result.Manifest.ElapsedSeconds = time.Since(start).Seconds()
	if err := storage.WriteJSON(filepath.Join(dir, "manifest.json"), result.Manifest); firstErr == nil {
		firstErr = err
	}
	if err := storage.WriteJSON(filepath.Join(dir, "summary.json"), result); firstErr == nil {
		firstErr = err
	}
	if firstErr != nil {
		return result, firstErr
	}
	return result, ctx.Err()
}

func executeOne(parent context.Context, a benchmark.Agent, task benchmark.Case, repeat int, experiment, dir string) (r telemetry.Run, err error) {
	id, err := newID()
	if err != nil {
		return r, err
	}
	r = telemetry.Run{ID: id, ExperimentID: experiment, Case: task.Name, Agent: a.Name, Adapter: a.Adapter, Model: a.Model, Repeat: repeat, StartedAt: time.Now().UTC(), Status: "error", ArtifactDir: filepath.Join(dir, id)}
	if err := os.Mkdir(r.ArtifactDir, 0700); err != nil {
		return r, err
	}
	// Store a record even for workspace or executable startup failures.
	defer func() {
		if r.Status != "completed" && r.Success == nil {
			failed := false
			r.Success = &failed
		}
		if saveErr := storage.WriteJSON(filepath.Join(r.ArtifactDir, "run.json"), r); err == nil {
			err = saveErr
		}
	}()
	workdir, prepErr := os.MkdirTemp("", "agent-speed-bench-work-")
	if prepErr != nil {
		r.Error = prepErr.Error()
		return r, nil
	}
	defer os.RemoveAll(workdir)
	workspace := filepath.Join(workdir, "workspace")
	prepCtx, prepCancel := context.WithTimeout(parent, 60*time.Second)
	commit, prepErr := Prepare(prepCtx, task, workspace)
	prepCancel()
	if prepErr != nil {
		r.Error = prepErr.Error()
		return r, nil
	}
	r.Commit = commit
	adapter, err := adapters.New(a)
	if err != nil {
		return r, err
	}
	command, err := adapter.BuildCommand(task.Prompt, workspace)
	if err != nil {
		return r, err
	}
	files := make([]*os.File, 0, 4)
	defer func() {
		for _, f := range files {
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
	}()
	for _, name := range []string{"stdout.log", "stderr.log", "raw.jsonl", "events.jsonl"} {
		f, openErr := os.OpenFile(filepath.Join(r.ArtifactDir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if openErr != nil {
			return r, openErr
		}
		files = append(files, f)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(task.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	r.StartedAt = start.UTC()
	c := &collector{start: start, runID: r.ID, agent: a.Name, parser: adapter, raw: json.NewEncoder(files[2]), normalized: json.NewEncoder(files[3]), cancel: cancel}
	if err := c.lifecycle("run_started"); err != nil {
		return r, err
	}
	out := &lineWriter{collector: c, stream: "stdout", file: files[0]}
	stderr := &lineWriter{collector: c, stream: "stderr", file: files[1]}
	p := RunProcess(ctx, command, workspace, out, stderr)
	if flushErr := out.flush(); flushErr != nil {
		return r, flushErr
	}
	if flushErr := stderr.flush(); flushErr != nil {
		return r, flushErr
	}
	if err := c.lifecycle("run_finished"); err != nil {
		return r, err
	}
	r.ExitCode = p.ExitCode
	r.Metrics = telemetry.Calculate(c.events, p.Wall.Seconds(), c.firstStdout)
	r.Status = "completed"
	switch {
	case errors.Is(p.Err, context.DeadlineExceeded):
		r.Status = "timeout"
	case errors.Is(p.Err, context.Canceled):
		r.Status = "canceled"
	case p.ExitCode == nil:
		r.Status = "error"
	case p.Err != nil || c.failed:
		r.Status = "failed"
	case adapter.RequiresTerminal() && !adapter.TerminalSeen():
		r.Status = "telemetry_error"
		r.Error = "Required terminal event was not observed"
	case c.parseErrors > 0:
		r.Status = "telemetry_error"
		r.Error = fmt.Sprintf("%d malformed telemetry lines", c.parseErrors)
	}
	if p.Err != nil {
		r.Error = p.Err.Error()
	}
	if c.parseErrors > 0 {
		r.Metrics.Warnings = append(r.Metrics.Warnings, fmt.Sprintf("%d malformed telemetry lines", c.parseErrors))
	}
	if c.failed && r.Error == "" {
		r.Error = "Agent reported a failure; inspect raw.jsonl"
	}
	if r.Status != "completed" {
		failed := false
		r.Success = &failed
		return r, nil
	}
	var transcript strings.Builder
	final := ""
	for _, e := range c.events {
		if e.Type == "assistant_output" {
			transcript.WriteString(e.Text)
		}
		if e.Type == "final_output" {
			final = e.Text
		}
	}
	if final != "" {
		transcript.Reset()
		transcript.WriteString(final)
	}
	if err := os.WriteFile(filepath.Join(r.ArtifactDir, "assistant.txt"), []byte(transcript.String()), 0600); err != nil {
		return r, err
	}
	if task.Verify.Command == "" && task.Verify.OutputContains == "" {
		return r, nil
	}
	pass := true
	if task.Verify.OutputContains != "" {
		pass = strings.Contains(transcript.String(), task.Verify.OutputContains)
	}
	if task.Verify.Command != "" {
		f, err := os.OpenFile(filepath.Join(r.ArtifactDir, "verify.log"), os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return r, err
		}
		vctx, vcancel := context.WithTimeout(parent, time.Duration(task.Verify.TimeoutSeconds)*time.Second)
		var locked lockedWriter
		locked.w = f
		graded := RunProcess(vctx, adapters.Command{Path: task.Verify.Command, Args: task.Verify.Args}, workspace, &locked, &locked)
		vcancel()
		if err := f.Close(); err != nil {
			return r, err
		}
		if graded.Err != nil {
			pass = false
			r.Error = "Verifier failed: " + graded.Err.Error()
		}
	}
	r.Success = &pass
	if !pass && r.Error == "" {
		r.Error = "Output assertion failed"
	}
	return r, nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

type limitedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if b.b.Len() < 4096 {
		b.b.Write(p[:min(n, 4096-b.b.Len())])
	}
	return n, nil
}
func (b *limitedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }
