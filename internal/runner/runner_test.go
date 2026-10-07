//go:build darwin || linux

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestProcessTimeoutKillsDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r := RunProcess(ctx, adapters.Command{Path: "sh", Args: []string{"-c", `sleep 30 & printf '%s\n' "$!" > "$1"; wait`, "fixture", pidFile}}, "", io.Discard, io.Discard)
	if !errors.Is(r.Err, context.DeadlineExceeded) || r.Wall > 3*time.Second {
		t.Fatal(r)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		// A killed orphan may briefly remain a zombie until init reaps it.
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatal("descendant survived process group cancellation")
}

func TestProcessNonzeroAndMissingExecutable(t *testing.T) {
	r := RunProcess(context.Background(), adapters.Command{Path: "sh", Args: []string{"-c", "exit 7"}}, "", io.Discard, io.Discard)
	if r.Err == nil || r.ExitCode == nil || *r.ExitCode != 7 {
		t.Fatal(r)
	}
	r = RunProcess(context.Background(), adapters.Command{Path: "/nonexistent/agent"}, "", io.Discard, io.Discard)
	if r.Err == nil || r.ExitCode != nil {
		t.Fatal(r)
	}
}

func TestCollectorLineBoundariesAndTimestampProvenance(t *testing.T) {
	p, _ := adapters.New(benchmark.Agent{Adapter: "generic"})
	var raw, normalized bytes.Buffer
	c := &collector{start: time.Now(), runID: "real-run", agent: "real-agent", parser: p, raw: json.NewEncoder(&raw), normalized: json.NewEncoder(&normalized), cancel: func() {}}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &lineWriter{collector: c, stream: "stdout", file: f}
	line := `{"type":"assistant_output","text":"hello","elapsed_ns":-999,"run_id":"forged"}`
	if _, err := w.Write([]byte(line[:10])); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 0 {
		t.Fatal("partial JSON parsed")
	}
	if _, err := w.Write([]byte(line[10:] + "\nlast")); err != nil {
		t.Fatal(err)
	}
	if err := w.flush(); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 2 || c.events[0].RunID != "real-run" || c.events[0].ElapsedNS < 0 || c.firstStdout == nil {
		t.Fatal(c.events)
	}
	if bytes.Count(raw.Bytes(), []byte("\n")) != 2 {
		t.Fatal(raw.String())
	}
}

func TestCollectorOversizeLineCancels(t *testing.T) {
	p, _ := adapters.New(benchmark.Agent{Adapter: "generic"})
	var raw, events bytes.Buffer
	canceled := false
	c := &collector{start: time.Now(), parser: p, raw: json.NewEncoder(&raw), normalized: json.NewEncoder(&events), cancel: func() { canceled = true }}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &lineWriter{collector: c, stream: "stdout", file: f}
	if _, err := w.Write(bytes.Repeat([]byte("x"), maxLineBytes+1)); err == nil || !canceled {
		t.Fatal("oversize stream accepted")
	}
}

func fixtureTask() benchmark.Case {
	return benchmark.Case{Name: "task", Prompt: "fixture", TimeoutSeconds: 2, Verify: benchmark.Verify{TimeoutSeconds: 1}}
}
func TestRunGradingAndTelemetryFailures(t *testing.T) {
	for _, tc := range []struct {
		name, adapter, script, status string
		verify                        benchmark.Verify
		wantSuccess                   *bool
	}{
		{"ungraded", "generic", "printf hello", "completed", benchmark.Verify{TimeoutSeconds: 1}, nil},
		{"passed", "generic", "printf Done", "completed", benchmark.Verify{OutputContains: "Done", TimeoutSeconds: 1}, boolPtr(true)},
		{"grader-failed", "generic", "printf Done", "completed", benchmark.Verify{Command: "sh", Args: []string{"-c", "exit 1"}, TimeoutSeconds: 1}, boolPtr(false)},
		{"agent-failed", "generic", "printf Done; exit 3", "failed", benchmark.Verify{OutputContains: "Done", TimeoutSeconds: 1}, boolPtr(false)},
		{"malformed", "generic", `printf '{"type":invalid}\n'`, "telemetry_error", benchmark.Verify{TimeoutSeconds: 1}, nil},
		{"incomplete", "codex", `printf '{"type":"item.completed","item":{"type":"agent_message","text":"Done"}}\n'`, "telemetry_error", benchmark.Verify{TimeoutSeconds: 1}, nil},
		{"reported-error", "generic", `printf '{"type":"agent_error","text":"failed"}\n'`, "failed", benchmark.Verify{TimeoutSeconds: 1}, boolPtr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			task := fixtureTask()
			task.Verify = tc.verify
			// Custom scripts use generic command arguments. For the Codex parser,
			// a wrapper ignores built-in args and emits a documented event fixture.
			a := benchmark.Agent{Name: tc.name, Adapter: tc.adapter, Command: "sh", Args: []string{"-c", tc.script}}
			if tc.adapter == "codex" {
				path := filepath.Join(t.TempDir(), "agent")
				if err := os.WriteFile(path, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
				a.Command = path
				a.Args = nil
			}
			r, err := executeOne(context.Background(), a, task, 1, "test", dir)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != tc.status || (r.Success == nil) != (tc.wantSuccess == nil) || (r.Success != nil && *r.Success != *tc.wantSuccess) {
				t.Fatalf("status=%s success=%v error=%s", r.Status, r.Success, r.Error)
			}
			if r.Metrics.GenerationTPS != nil {
				t.Fatal("invented generation rate")
			}
			if _, err := os.Stat(filepath.Join(r.ArtifactDir, "run.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func boolPtr(v bool) *bool { return &v }

func TestRunTimeoutAndVerifierTimeout(t *testing.T) {
	task := fixtureTask()
	task.TimeoutSeconds = 1
	r, err := executeOne(context.Background(), benchmark.Agent{Name: "slow", Adapter: "generic", Command: "sh", Args: []string{"-c", "sleep 30"}}, task, 1, "test", t.TempDir())
	if err != nil || r.Status != "timeout" || r.Success == nil || *r.Success {
		t.Fatal(r, err)
	}
	task.Verify = benchmark.Verify{Command: "sh", Args: []string{"-c", "sleep 30"}, TimeoutSeconds: 1}
	r, err = executeOne(context.Background(), benchmark.Agent{Name: "fast", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}, task, 1, "test", t.TempDir())
	if err != nil || r.Status != "completed" || r.Success == nil || *r.Success || !strings.Contains(r.Error, "Verifier failed") {
		t.Fatal(r, err)
	}
}

func TestMatrixKeepsWorkspacesIndependentAndPersistsEveryRun(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task := fixtureTask()
	task.Files = map[string]string{"state.txt": "initial"}
	task.Verify.OutputContains = "Done"
	cfg := benchmark.Config{Name: "matrix", Repeats: 3, Jobs: 3, TimeoutSeconds: 2, Cases: []benchmark.Case{task}, Agents: []benchmark.Agent{
		{Name: "a", Adapter: "generic", Command: "sh", Args: []string{"-c", `test "$(cat state.txt)" = initial || exit 3; printf changed > state.txt; printf Done`}},
		{Name: "b", Adapter: "generic", Command: "sh", Args: []string{"-c", `test "$(cat state.txt)" = initial || exit 3; printf changed > state.txt; printf Done`}},
	}}
	result, err := Execute(context.Background(), cfg, dir, s, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 6 {
		t.Fatal(len(result.Runs))
	}
	for _, r := range result.Runs {
		if r.Success == nil || !*r.Success {
			t.Fatal(r)
		}
	}
	stored, err := s.Runs(result.Manifest.ExperimentID)
	if err != nil || len(stored) != 6 {
		t.Fatal(stored, err)
	}
	if result.Manifest.ElapsedSeconds <= 0 {
		t.Fatal(result.Manifest)
	}
}

func TestRepoSnapshotIgnoresDirtyWorkingTree(t *testing.T) {
	source := t.TempDir()
	gitTest := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = source
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitTest("init", "--quiet")
	file := filepath.Join(source, "value.txt")
	if err := os.WriteFile(file, []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest("add", "value.txt")
	gitTest("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture")
	commit := gitTest("rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("dirty"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "workspace")
	got, err := Prepare(context.Background(), benchmark.Case{Repo: &benchmark.Repo{Path: source, Commit: commit}}, dest)
	if err != nil || got != commit {
		t.Fatal(got, err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "value.txt"))
	if err != nil || string(data) != "committed" {
		t.Fatal(string(data), err)
	}
	data, err = os.ReadFile(file)
	if err != nil || string(data) != "dirty" {
		t.Fatal("source working tree changed")
	}
}

func TestCanceledRunRemainsARecordedFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := executeOne(ctx, benchmark.Agent{Name: "a", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}, fixtureTask(), 1, "test", t.TempDir())
	if err != nil || r.Status == "completed" || r.Success == nil || *r.Success || r.StartedAt.IsZero() {
		t.Fatal(r, err)
	}
	var saved telemetry.Run
	data, err := os.ReadFile(filepath.Join(r.ArtifactDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Success == nil || *saved.Success {
		t.Fatal(saved)
	}
}

func TestFatalAgentFailureStopsOnlyThatAgent(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := fixtureTask()
	task.Verify.OutputContains = "Done"
	cfg := benchmark.Config{Name: "stop", Repeats: 5, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{task}, Agents: []benchmark.Agent{
		{Name: "unavailable", Adapter: "generic", Command: "sh", Args: []string{"-c", `printf '{"type":"agent_error","text":"The model is not supported with this account"}\n'; exit 1`}},
		{Name: "healthy", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}},
	}}
	result, err := Execute(context.Background(), cfg, dir, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	skipped, unavailable, passed := 0, 0, 0
	for _, r := range result.Runs {
		switch r.Status {
		case "skipped":
			skipped++
			if r.Success != nil || r.ExitCode != nil {
				t.Fatal(r)
			}
		case "agent_unavailable":
			unavailable++
			if r.Success != nil || r.Error != "The model is not supported with this account" {
				t.Fatal(r)
			}
		case "completed":
			if r.Success != nil && *r.Success {
				passed++
			}
		default:
			t.Fatal(r)
		}
	}
	if skipped != 4 || unavailable != 1 || passed != 5 || result.Manifest.StartedRuns != 6 || result.Manifest.SkippedRuns != 4 {
		t.Fatal(result.Manifest, skipped, unavailable, passed)
	}
	stored, err := store.Runs(result.Manifest.ExperimentID)
	if err != nil || len(stored) != 10 {
		t.Fatal(stored, err)
	}
}

func TestSettingsSnapshotExcludesSecretsAndDistinguishesRequests(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("model = 'configured'\nmodel_reasoning_effort = 'high'\nsecret = 'SECRET_CANARY'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := snapshotSettings(benchmark.Agent{Adapter: "codex", Model: "requested", ReasoningEffort: "low"})
	data, err := json.Marshal(s)
	if err != nil || strings.Contains(string(data), "SECRET_CANARY") || s.RequestedModel != "requested" || s.ConfiguredModel == nil || *s.ConfiguredModel != "configured" || s.ObservedModel != nil || s.ConfigStatus != "user_config_only" {
		t.Fatal(s, err)
	}
	if err := os.WriteFile(path, []byte("not TOML = ["), 0600); err != nil {
		t.Fatal(err)
	}
	s = snapshotSettings(benchmark.Agent{Adapter: "codex"})
	if s.ConfigStatus != "invalid" || s.ConfiguredModel != nil {
		t.Fatal(s)
	}
}

func TestTransientFailuresDoNotStopRemainingRepeats(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	task := fixtureTask()
	cfg := benchmark.Config{Name: "transient", Repeats: 3, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{task}, Agents: []benchmark.Agent{
		{Name: "busy", Adapter: "generic", Command: "sh", Args: []string{"-c", `printf '%s\n' '{"type":"agent_error","text":"{\"status\":429,\"error\":{\"message\":\"Busy\"}}"}'; exit 1`}},
	}}
	result, err := Execute(context.Background(), cfg, dir, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.SkippedRuns != 0 || len(result.Runs) != 3 {
		t.Fatal(result)
	}
	for _, r := range result.Runs {
		if r.Status != "service_error" || r.Success != nil || r.Failure == nil || r.Failure.Category != "service" || !r.Failure.Retryable {
			t.Fatal(r)
		}
	}
}

func TestCursorTrustDiagnosticStopsOnlyBlockedAgent(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join(dir, "cursor-fixture")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo fixture; exit 0; fi\nprintf '⚠ Workspace Trust Required\\n' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := benchmark.Config{Name: "trust", Repeats: 3, Jobs: 1, TimeoutSeconds: 2, Cases: []benchmark.Case{fixtureTask()}, Agents: []benchmark.Agent{{Name: "blocked", Adapter: "cursor", Command: path}, {Name: "healthy", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}}}
	result, err := Execute(context.Background(), cfg, dir, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	unavailable, skipped, healthy := 0, 0, 0
	for _, r := range result.Runs {
		switch r.Status {
		case "agent_unavailable":
			unavailable++
			if r.Success != nil || r.Failure.Code != "workspace_trust_required" || r.Failure.Retryable {
				t.Fatal(r)
			}
		case "skipped":
			skipped++
		case "completed":
			healthy++
		default:
			t.Fatal(r)
		}
	}
	if unavailable != 1 || skipped != 2 || healthy != 3 {
		t.Fatal(result)
	}
	// A diagnostic-looking warning cannot turn a successful exit into a failure.
	script := "#!/bin/sh\nprintf '⚠ Workspace Trust Required\\n' >&2\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"Done\"}'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := executeOne(context.Background(), cfg.Agents[0], fixtureTask(), 1, "warning", dir)
	if err != nil || r.Status != "completed" || r.Failure != nil {
		t.Fatal(r, err)
	}
}
func TestServiceTierSnapshotExcludesSecretsAndDoesNotInferObserved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("service_tier='default'\nsecret='SECRET_CANARY'\n[features]\nfast_mode=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := snapshotSettings(benchmark.Agent{Adapter: "codex", ServiceTier: "fast"})
	data, _ := json.Marshal(s)
	if s.RequestedServiceTier != "fast" || s.ConfiguredServiceTier == nil || *s.ConfiguredServiceTier != "default" || s.ConfiguredFastMode == nil || *s.ConfiguredFastMode || s.ObservedServiceTier != nil || strings.Contains(string(data), "SECRET_CANARY") {
		t.Fatal(s)
	}
}
func TestAgyEmptyAuthoritativeTranscriptOverridesDeltas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agy-fixture")
	script := `#!/bin/sh
printf '%s\n' '{"event":"step_update","step_update":{"step_type":"agent_response","state":"ACTIVE","text_delta":"Done"}}' '{"event":"result","result":{"status":"SUCCESS","response":""}}'
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	task := fixtureTask()
	task.Verify.OutputContains = "Done"
	r, err := executeOne(context.Background(), benchmark.Agent{Name: "agy", Adapter: "agy", Command: path}, task, 1, "empty", t.TempDir())
	if err != nil || r.Status != "completed" || r.Success == nil || *r.Success {
		t.Fatal(r, err)
	}
}
