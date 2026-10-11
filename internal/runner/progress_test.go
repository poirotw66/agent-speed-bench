//go:build darwin || linux

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestWorkflowTimingSurvivesCaptureAndCleanup(t *testing.T) {
	task := fixtureTask()
	task.Files = map[string]string{"source.txt": "evidence"}
	task.RetainFiles = []string{"source.txt"}
	task.Verify = benchmark.Verify{Command: "sh", Args: []string{"-c", "sleep 0.1"}, TimeoutSeconds: 2}
	var output bytes.Buffer
	r, err := executeAttempt(context.Background(), benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf partial; sleep 0.1; printf Done"}}, task, 1, "test", t.TempDir(), false, attemptOptions{progress: &lockedWriter{w: &output}, interval: 10 * time.Millisecond})
	if err != nil || r.Timing == nil || r.Success == nil || !*r.Success {
		t.Fatal(r, err)
	}
	timing := r.Timing
	if timing.TotalSeconds < r.Metrics.WallSeconds+*timing.VerificationSeconds || timing.PreparationSeconds == nil || timing.CaptureSeconds == nil || timing.CleanupSeconds == nil || len(r.Verification) != 1 || r.Verification[0].WallSeconds == nil || *r.Verification[0].WallSeconds < 0.08 {
		t.Fatal(timing, r.Verification)
	}
	data, err := os.ReadFile(filepath.Join(r.ArtifactDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored telemetry.Run
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Timing == nil || stored.Timing.TotalSeconds != timing.TotalSeconds {
		t.Fatal("timing missing from persisted evidence", stored)
	}
	for _, phase := range []string{"preparation", "agent", "verification:command", "capture", "cleanup", "finished"} {
		if !strings.Contains(output.String(), "phase="+phase) {
			t.Fatal("missing progress phase", phase, output.String())
		}
	}
	if !strings.Contains(output.String(), "last_output_at=") || !strings.Contains(output.String(), "last_output_ago=") {
		t.Fatal(output.String())
	}
}

func TestEarlyPreparationFailureLeavesUnreachedStagesUnknown(t *testing.T) {
	task := fixtureTask()
	r, err := executeAttempt(context.Background(), benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh"}, task, 1, "test", t.TempDir(), false, attemptOptions{guard: func() error { return os.ErrNotExist }})
	if err != nil || r.Timing == nil || r.Timing.VerificationSeconds != nil || r.Timing.CaptureSeconds != nil || r.Timing.CleanupSeconds != nil || r.Timing.PreparationSeconds == nil {
		t.Fatal(r, err)
	}
}

type heartbeatSink struct{ lines chan string }

func (s *heartbeatSink) Write(data []byte) (int, error) {
	select {
	case s.lines <- string(data):
	default:
	}
	return len(data), nil
}

func TestHeartbeatContinuesWithoutOutputAndRemainsUnknown(t *testing.T) {
	sink := &heartbeatSink{lines: make(chan string, 32)}
	monitor := newAttemptMonitor("fixture", "task", "run", sink, 10*time.Millisecond)
	defer monitor.finish()
	monitor.setPhase("agent")
	agentUpdates := 0
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for agentUpdates < 2 {
		select {
		case line := <-sink.lines:
			if strings.Contains(line, "phase=agent") {
				agentUpdates++
				if !strings.Contains(line, "last_output_at=unknown") || strings.Contains(line, "stuck") {
					t.Fatal(line)
				}
			}
		case <-deadline.C:
			t.Fatal("no periodic progress while agent was quiet")
		}
	}
}

// A terminal that stops reading must not stall the process or receipt timestamps.
type blockedProgress struct{ entered, release chan struct{} }

func (b *blockedProgress) Write(data []byte) (int, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	return len(data), nil
}

func TestBlockedProgressDoesNotStallAttempt(t *testing.T) {
	sink := &blockedProgress{entered: make(chan struct{}, 1), release: make(chan struct{})}
	queue := newQueuedProgress(sink)
	defer func() { close(sink.release); queue.close() }()
	queue.Write([]byte("initial"))
	<-sink.entered
	// Saturate the bounded queue before running an attempt.
	for range 256 {
		queue.Write([]byte("queued"))
	}
	done := make(chan telemetry.Run, 1)
	go func() {
		r, _ := executeAttempt(context.Background(), benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}, fixtureTask(), 1, "test", t.TempDir(), false, attemptOptions{progress: queue})
		done <- r
	}()
	select {
	case r := <-done:
		if r.Status != "completed" || r.Metrics.TTFASeconds == nil || r.Timing == nil {
			t.Fatal(r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked progress stalled telemetry or completion")
	}
}

func TestCleanupFailurePreservesGradeAndEvidence(t *testing.T) {
	var residual string
	task := fixtureTask()
	task.Verify.OutputContains = "Done"
	r, err := executeAttempt(context.Background(), benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}, task, 1, "test", t.TempDir(), false, attemptOptions{removeWorkspace: func(path string) error { residual = path; return os.ErrPermission }})
	defer os.RemoveAll(residual)
	if err != nil || r.Status != "completed" || r.Success == nil || !*r.Success || r.Cleanup == nil || r.Cleanup.Path != residual {
		t.Fatal(r, err)
	}
	data, err := os.ReadFile(filepath.Join(r.ArtifactDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored telemetry.Run
	if err := json.Unmarshal(data, &stored); err != nil || stored.Cleanup == nil || stored.Cleanup.Path != residual {
		t.Fatal(stored, err)
	}
}

func TestProgressDrainDoesNotWaitForBlockedWriter(t *testing.T) {
	sink := &blockedProgress{entered: make(chan struct{}, 1), release: make(chan struct{})}
	queue := newQueuedProgress(sink)
	queue.Write([]byte("blocked"))
	<-sink.entered
	done := make(chan struct{})
	go func() { queue.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(sink.release)
		t.Fatal("final progress drain waited for a blocked writer")
	}
	close(sink.release)
	<-queue.done
}
