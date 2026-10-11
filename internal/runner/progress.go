//go:build darwin || linux

package runner

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

const progressInterval = 30 * time.Second

type attemptOptions struct {
	removeWorkspace func(string) error
	guard           func() error
	progress        io.Writer
	interval        time.Duration
}

// Receipt times describe observable output, never model liveness or execution.
type attemptMonitor struct {
	mu                             sync.Mutex
	start, phaseStart, lastReceipt time.Time
	phase, agent, task, run        string
	output                         io.Writer
	durations                      map[string]float64
	stop, done                     chan struct{}
}

func newAttemptMonitor(agent, task, run string, output io.Writer, interval time.Duration) *attemptMonitor {
	now := time.Now()
	m := &attemptMonitor{start: now, phaseStart: now, phase: "preparation", agent: agent, task: task, run: run, output: output, durations: map[string]float64{}, stop: make(chan struct{}), done: make(chan struct{})}
	if interval <= 0 {
		interval = progressInterval
	}
	if output == nil {
		close(m.done)
		return m
	}
	m.emit(now)
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.mu.Lock()
				m.emit(time.Now())
				m.mu.Unlock()
			case <-m.stop:
				return
			}
		}
	}()
	return m
}

func (m *attemptMonitor) emit(now time.Time) {
	if m.output == nil {
		return
	}
	last, stamp := "unknown", "unknown"
	if !m.lastReceipt.IsZero() {
		stamp = m.lastReceipt.UTC().Format(time.RFC3339Nano)
		last = fmt.Sprintf("%.1fs", now.Sub(m.lastReceipt).Seconds())
	}
	fmt.Fprintf(m.output, "Progress: run=%s agent=%s case=%s phase=%s elapsed=%.1fs last_output_ago=%s last_output_at=%s\n", m.run, m.agent, m.task, m.phase, now.Sub(m.start).Seconds(), last, stamp)
}

func (m *attemptMonitor) setPhase(phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase == phase {
		return
	}
	now := time.Now()
	m.durations[m.phase] += now.Sub(m.phaseStart).Seconds()
	m.phase, m.phaseStart = phase, now
	m.emit(now)
}

func (m *attemptMonitor) receipt(now time.Time) { m.mu.Lock(); m.lastReceipt = now; m.mu.Unlock() }

func (m *attemptMonitor) finish() *telemetry.AttemptTiming {
	close(m.stop)
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.durations[m.phase] += now.Sub(m.phaseStart).Seconds()
	m.phase = "finished"
	m.emit(now)
	value := func(phase string) *float64 {
		var seconds float64
		found := false
		for name, duration := range m.durations {
			if name == phase || strings.HasPrefix(name, phase+":") {
				seconds += duration
				found = true
			}
		}
		if !found {
			return nil
		}
		return &seconds
	}
	return &telemetry.AttemptTiming{StartedAt: m.start.UTC(), FinishedAt: now.UTC(), TotalSeconds: now.Sub(m.start).Seconds(), PostprocessSeconds: value("postprocess"), PreparationSeconds: value("preparation"), VerificationSeconds: value("verification"), CaptureSeconds: value("capture"), CleanupSeconds: value("cleanup")}
}

type receiptWriter struct {
	writer  io.Writer
	receipt func(time.Time)
}

func (w *receiptWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	if n > 0 {
		w.receipt(time.Now())
	}
	return n, err
}

// queuedProgress keeps terminal backpressure outside telemetry and timing locks.
// Only one output goroutine is created per matrix. Progress is best effort.
type queuedProgress struct {
	lines chan []byte
	done  chan struct{}
}

func newQueuedProgress(output io.Writer) *queuedProgress {
	p := &queuedProgress{lines: make(chan []byte, 128), done: make(chan struct{})}
	go func() {
		defer close(p.done)
		for line := range p.lines {
			if _, err := output.Write(line); err != nil {
				return
			}
		}
	}()
	return p
}

func (p *queuedProgress) Write(data []byte) (int, error) {
	// No output lock or wait is allowed on the collection path.
	select {
	case p.lines <- append([]byte(nil), data...):
	default:
	}
	return len(data), nil
}

func (p *queuedProgress) close() {
	close(p.lines)
	// An arbitrary io.Writer cannot be canceled. Bound the final drain wait.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
}
