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
	"unicode/utf8"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type Manifest struct {
	State             string                        `json:"state,omitempty"`
	Resumes           int                           `json:"resumes"`
	Provenance        Provenance                    `json:"provenance"`
	WarmupStartedRuns int                           `json:"warmup_started_runs"`
	WarmupSkippedRuns int                           `json:"warmup_skipped_runs"`
	SchemaVersion     int                           `json:"schema_version"`
	ExperimentID      string                        `json:"experiment_id"`
	StartedAt         time.Time                     `json:"started_at"`
	FinishedAt        time.Time                     `json:"finished_at"`
	ElapsedSeconds    float64                       `json:"elapsed_seconds"`
	Platform          string                        `json:"platform"`
	GoVersion         string                        `json:"go_version"`
	Config            benchmark.Config              `json:"config"`
	WarmupRuns        int                           `json:"warmup_runs"`
	PlannedRuns       int                           `json:"planned_runs"`
	StartedRuns       int                           `json:"started_runs"`
	SkippedRuns       int                           `json:"skipped_runs"`
	StoppedAgents     map[string]*telemetry.Failure `json:"stopped_agents,omitempty"`
	Agents            []AgentInfo                   `json:"agents"`
}
type AgentInfo struct {
	BinarySHA256 string                `json:"binary_sha256"`
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
		info.BinarySHA256, err = fileHash(path)
		if err != nil {
			return nil, err
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
	result.Manifest = Manifest{SchemaVersion: 3, State: "running", WarmupRuns: cfg.WarmupRepeats * len(cfg.Agents) * len(cfg.Cases), PlannedRuns: cfg.Repeats * len(cfg.Agents) * len(cfg.Cases), ExperimentID: experiment, StartedAt: start.UTC(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), Config: cfg, Agents: infos}
	result.Manifest.Provenance, err = provenance(cfg)
	if err != nil {
		return result, err
	}
	release, err := claimExperiment(dir)
	if err != nil {
		return result, err
	}
	defer release()
	if err := storage.WriteJSON(filepath.Join(dir, "manifest.json"), result.Manifest); err != nil {
		return result, err
	}
	return executeMatrix(ctx, cfg, result, store, progress)
}

func executeMatrix(ctx context.Context, cfg benchmark.Config, result Result, store *storage.Store, progress io.Writer) (Result, error) {
	start := time.Now()
	dir, experiment := result.Directory, result.Manifest.ExperimentID
	covered := map[string]bool{}
	for _, r := range result.Runs {
		covered[jobKey(r.Agent, r.Case, r.Repeat, r.Warmup)] = true
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type job struct {
		agent  benchmark.Agent
		task   benchmark.Case
		repeat int
		warmup bool
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var warmupWG sync.WaitGroup
	var firstErr error
	stopped := result.Manifest.StoppedAgents
	if stopped == nil {
		stopped = map[string]*telemetry.Failure{}
	}
	for range cfg.Jobs {
		wg.Go(func() {
			for j := range jobs {
				mu.Lock()
				reason := stopped[j.agent.Name]
				mu.Unlock()
				var r telemetry.Run
				var runErr error
				if reason != nil {
					r, runErr = skippedRun(j.agent, j.task, j.repeat, experiment, dir, reason)
				} else {
					r, runErr = executeAttempt(ctx, j.agent, j.task, j.repeat, experiment, dir, j.warmup)
				}
				r.Warmup = j.warmup
				if runErr == nil {
					runErr = storage.WriteJSON(filepath.Join(r.ArtifactDir, "run.json"), r)
				}
				mu.Lock()
				if r.Failure != nil && r.Failure.Scope == "agent" && !r.Failure.Retryable {
					stopped[r.Agent] = r.Failure
				}
				if runErr == nil {
					runErr = store.Save(r)
				}
				if runErr != nil && firstErr == nil {
					firstErr = runErr
					cancel()
				}
				result.Runs = append(result.Runs, r)
				if r.Warmup {
					if r.Status == "skipped" {
						result.Manifest.WarmupSkippedRuns++
					} else {
						result.Manifest.WarmupStartedRuns++
					}
				} else if r.Status == "skipped" {
					result.Manifest.SkippedRuns++
				} else {
					result.Manifest.StartedRuns++
				}
				result.Manifest.StoppedAgents = stopped
				if saveErr := storage.WriteJSON(filepath.Join(dir, "manifest.json"), result.Manifest); saveErr != nil && firstErr == nil {
					firstErr = saveErr
					cancel()
				}
				if progress != nil {
					phase := "measured"
					if r.Warmup {
						phase = "warmup"
					}
					fmt.Fprintf(progress, "%-14s %-22s phase=%s repeat=%d status=%s wall=%.3fs\n", r.Agent, r.Case, phase, r.Repeat, r.Status, r.Metrics.WallSeconds)
				}
				mu.Unlock()
				if j.warmup {
					warmupWG.Done()
				}
			}
		})
	}
	// Rotate agent order between repeats to reduce fixed ordering bias.
dispatch:
	for round := 1; round <= cfg.WarmupRepeats+cfg.Repeats; round++ {
		if round == cfg.WarmupRepeats+1 {
			warmupWG.Wait()
		}
		warmup := round <= cfg.WarmupRepeats
		repeat := round
		if !warmup {
			repeat -= cfg.WarmupRepeats
		}
		for _, task := range cfg.Cases {
			for i := range cfg.Agents {
				a := cfg.Agents[(i+repeat-1)%len(cfg.Agents)]
				if covered[jobKey(a.Name, task.Name, repeat, warmup)] {
					continue
				}
				if warmup {
					warmupWG.Add(1)
				}
				select {
				case jobs <- job{a, task, repeat, warmup}:
				case <-ctx.Done():
					if warmup {
						warmupWG.Done()
					}
					break dispatch
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(result.Runs, func(i, j int) bool { return result.Runs[i].ID < result.Runs[j].ID })
	result.Manifest.StoppedAgents = stopped
	result.Manifest.FinishedAt = time.Now().UTC()
	result.Manifest.ElapsedSeconds += time.Since(start).Seconds()
	result.Manifest.State = "complete"
	if ctx.Err() != nil || firstErr != nil {
		result.Manifest.State = "interrupted"
	}
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

func executeOne(parent context.Context, a benchmark.Agent, task benchmark.Case, repeat int, experiment, dir string) (telemetry.Run, error) {
	return executeAttempt(parent, a, task, repeat, experiment, dir, false)
}

func executeAttempt(parent context.Context, a benchmark.Agent, task benchmark.Case, repeat int, experiment, dir string, warmup bool) (r telemetry.Run, err error) {
	id, err := newID()
	if err != nil {
		return r, err
	}
	r = telemetry.Run{Warmup: warmup, ID: id, ExperimentID: experiment, Case: task.Name, Agent: a.Name, Adapter: a.Adapter, Model: a.Model, Settings: snapshotSettings(a), Repeat: repeat, StartedAt: time.Now().UTC(), Status: "error", Metrics: telemetry.Metrics{SchemaVersion: 2, TTFABasis: "unobserved", ToolTimingBasis: "runner_receipt", ToolTimingConfidence: "unverified"}, ArtifactDir: filepath.Join(dir, id)}
	r.Environment.PermissionPolicy = permissionPolicy(a.Adapter)
	if err := os.Mkdir(r.ArtifactDir, 0700); err != nil {
		return r, err
	}
	// Store a record even for workspace or executable startup failures.
	defer func() {
		if r.Status != "completed" && !ungradedFailure(r.Status) && r.Success == nil {
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
		if errors.Is(prepErr, context.Canceled) {
			r.Status = "canceled"
		} else {
			r.Status = "infrastructure_error"
			r.Failure = &telemetry.Failure{Category: "infrastructure", Code: "workspace_preparation", Scope: "attempt", Message: r.Error}
		}
		return r, nil
	}
	defer os.RemoveAll(workdir)
	workspace := filepath.Join(workdir, "workspace")
	prepStart := time.Now()
	r.Environment.GoCachePolicy = task.GoCache
	if task.StripInstructions {
		r.Environment.WorkspaceInstructions = "known_files_stripped_v1"
	}
	prepCtx, prepCancel := context.WithTimeout(parent, 60*time.Second)
	commit, prepErr := Prepare(prepCtx, task, workspace)
	prepCancel()
	if prepErr != nil {
		r.Error = prepErr.Error()
		if errors.Is(prepErr, context.Canceled) {
			r.Status = "canceled"
		} else {
			r.Status = "infrastructure_error"
			r.Failure = &telemetry.Failure{Category: "infrastructure", Code: "workspace_preparation", Scope: "attempt", Message: r.Error}
		}
		return r, nil
	}
	r.Commit = commit
	baseline := ""
	if task.RetainPatch {
		baseline, err = git(parent, workspace, "rev-parse", "HEAD")
		if err != nil {
			r.Status = "infrastructure_error"
			r.Error = "Could not record prepared patch baseline"
			return r, nil
		}
	}
	r.PatchBaseline = baseline
	defer func() {
		// Preserve submissions before removing the workspace, even after cancellation.
		captureCtx, captureCancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
		defer captureCancel()
		if captureErr := retainFiles(workspace, r.ArtifactDir, task.RetainFiles); captureErr != nil {
			r.ArtifactErrors = append(r.ArtifactErrors, captureErr.Error())
		}
		if task.RetainPatch {
			if captureErr := retainPatch(captureCtx, workspace, r.ArtifactDir, task.RetainFiles, baseline); captureErr != nil {
				r.ArtifactErrors = append(r.ArtifactErrors, captureErr.Error())
			}
		}
		if len(r.ArtifactErrors) > 0 && r.Status == "completed" {
			r.Status, r.Success = "infrastructure_error", nil
			r.Error = "Submitted artifact capture failed"
			r.Failure = &telemetry.Failure{Category: "infrastructure", Code: "artifact_capture", Scope: "attempt", Message: r.Error}
		}
	}()
	env, environmentErr := prepareGoEnvironment(parent, task, workspace, r.ArtifactDir)
	if environmentErr == nil {
		var homeEnv []string
		homeEnv, environmentErr = prepareAgentHome(a, workdir)
		env = append(env, homeEnv...)
	}
	if a.IsolateConfig {
		r.Environment.IsolationPolicy = "ephemeral_home_auth_only_v1"
	}
	preparationSeconds := time.Since(prepStart).Seconds()
	r.Environment.PreparationSeconds = &preparationSeconds
	if environmentErr != nil {
		r.Status = "infrastructure_error"
		r.Error = environmentErr.Error()
		r.Failure = &telemetry.Failure{Category: "infrastructure", Code: "environment_preparation", Scope: "attempt", Message: r.Error}
		return r, nil
	}
	adapter, err := adapters.New(a)
	if err != nil {
		return r, err
	}
	command, err := adapter.BuildCommand(task.Prompt, workspace)
	if err != nil {
		return r, err
	}
	command.Env = append(command.Env, env...)
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
		r.Status = "infrastructure_error"
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
	if c.failure == nil && c.diagnosticFailure != nil && ((p.ExitCode != nil && *p.ExitCode != 0) || c.diagnosticFailure.Code == "headless_tool_permission_denied") {
		c.failure = c.diagnosticFailure
	}
	r.Failure = c.failure
	if r.Failure != nil && r.Status != "canceled" {
		r.Error = r.Failure.Message
		if r.Failure.Scope == "agent" {
			r.Status = "agent_unavailable"
		} else if r.Failure.Category == "service" || r.Failure.Category == "network" {
			r.Status = "service_error"
		}
	}
	for _, event := range c.events {
		if event.ServiceTier != "" {
			value := event.ServiceTier
			r.Settings.ObservedServiceTier = &value
		}
		if event.Model != "" {
			value := event.Model
			r.Settings.ObservedModel = &value
		}
		if event.ReasoningEffort != "" {
			value := event.ReasoningEffort
			r.Settings.ObservedReasoningEffort = &value
		}
	}
	if c.parseErrors > 0 {
		r.Metrics.Warnings = append(r.Metrics.Warnings, fmt.Sprintf("%d malformed telemetry lines", c.parseErrors))
	}
	if c.failed && r.Error == "" {
		r.Error = "Agent reported a failure; inspect raw.jsonl"
	}
	if r.Failure == nil && (r.Status == "infrastructure_error" || r.Status == "telemetry_error") {
		r.Failure = &telemetry.Failure{Category: "infrastructure", Code: r.Status, Scope: "attempt", Retryable: true, Message: r.Error}
	}
	var transcript strings.Builder
	final := ""
	finalSeen := false
	for _, e := range c.events {
		if e.Type == "assistant_output" {
			transcript.WriteString(e.Text)
		}
		if e.Type == "final_output" {
			final = e.Text
			finalSeen = true
		}
	}
	if finalSeen {
		transcript.Reset()
		transcript.WriteString(final)
	}
	if r.Status == "completed" {
		characters := int64(utf8.RuneCountInString(transcript.String()))
		r.Metrics.OutputCharacters = &characters
		if r.Metrics.WallSeconds > 0 {
			rate := float64(characters) / r.Metrics.WallSeconds
			r.Metrics.EffectiveCharactersPerSecond = &rate
		}
	}
	transcriptName := "assistant.txt"
	if r.Status != "completed" {
		transcriptName = "assistant.partial.txt"
	}
	if err := os.WriteFile(filepath.Join(r.ArtifactDir, transcriptName), []byte(transcript.String()), 0600); err != nil {
		return r, err
	}
	if ungradedFailure(r.Status) {
		return r, nil
	}
	if r.Status != "completed" {
		failed := false
		r.Success = &failed
		return r, nil
	}
	if !task.Verify.HasChecks() {
		return r, nil
	}
	r.Error = task.Verify.CheckOutput(transcript.String())
	pass := r.Error == ""
	checks := append([]benchmark.Check(nil), task.Verify.CoreTests...)
	layers := make([]string, len(checks))
	for i := range layers {
		layers[i] = "core"
	}
	for _, check := range task.Verify.RegressionTests {
		checks = append(checks, check)
		layers = append(layers, "regression")
	}
	if task.Verify.Command != "" {
		checks = append(checks, benchmark.Check{Command: task.Verify.Command, Args: task.Verify.Args})
		layers = append(layers, "command")
	}
	if len(checks) > 0 {
		f, openErr := os.OpenFile(filepath.Join(r.ArtifactDir, "verify.log"), os.O_CREATE|os.O_WRONLY, 0600)
		if openErr != nil {
			return r, openErr
		}
		var locked lockedWriter
		locked.w = f
		for i, check := range checks {
			fmt.Fprintf(&locked, "Scoring layer: %s\n", layers[i])
			vctx, vcancel := context.WithTimeout(parent, time.Duration(task.Verify.TimeoutSeconds)*time.Second)
			graded := RunProcess(vctx, adapters.Command{Path: check.Command, Args: check.Args}, workspace, &locked, &locked)
			vcancel()
			vr := telemetry.VerificationResult{Layer: layers[i], Command: check.Command, ExitCode: graded.ExitCode, Passed: graded.Err == nil}
			if graded.Err != nil {
				pass = false
				vr.Error = graded.Err.Error()
				if r.Error == "" {
					r.Error = "Verifier failed (" + layers[i] + "): " + vr.Error
				}
			}
			r.Verification = append(r.Verification, vr)
		}
		if closeErr := f.Close(); closeErr != nil {
			return r, closeErr
		}
	}

	r.Success = &pass
	if !pass {
		if r.Error == "" {
			r.Error = "Output assertion failed"
		}
		r.Failure = &telemetry.Failure{Category: "verification", Code: "assertion_failed", Scope: "task", Message: r.Error}
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

func skippedRun(a benchmark.Agent, task benchmark.Case, repeat int, experiment, dir string, reason *telemetry.Failure) (telemetry.Run, error) {
	id, err := newID()
	if err != nil {
		return telemetry.Run{}, err
	}
	r := telemetry.Run{ID: id, ExperimentID: experiment, Agent: a.Name, Adapter: a.Adapter, Model: a.Model, Case: task.Name, Repeat: repeat, StartedAt: time.Now().UTC(), Status: "skipped", Failure: reason, Error: "Agent stopped: " + reason.Message, Settings: snapshotSettings(a), ArtifactDir: filepath.Join(dir, id), Metrics: telemetry.Metrics{SchemaVersion: 2, TTFABasis: "unobserved", ToolTimingBasis: "runner_receipt", ToolTimingConfidence: "unverified"}}
	r.Environment.PermissionPolicy = permissionPolicy(a.Adapter)
	if err := os.Mkdir(r.ArtifactDir, 0700); err != nil {
		return r, err
	}
	return r, storage.WriteJSON(filepath.Join(r.ArtifactDir, "run.json"), r)
}

func ungradedFailure(status string) bool {
	return status == "canceled" || status == "agent_unavailable" || status == "service_error" || status == "infrastructure_error" || status == "telemetry_error" || status == "skipped"
}
