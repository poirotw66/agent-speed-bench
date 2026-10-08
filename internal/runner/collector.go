package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

const maxLineBytes = 8 * 1024 * 1024

type collector struct {
	mu                sync.Mutex
	start             time.Time
	runID, agent      string
	parser            adapters.Adapter
	raw, normalized   *json.Encoder
	events            []telemetry.Event
	firstStdout       *float64
	parseErrors       int
	failed            bool
	diagnosticFailure *telemetry.Failure
	failure           *telemetry.Failure
	err               error
	cancel            func()
	receipt           func(time.Time)
}
type lineWriter struct {
	collector *collector
	stream    string
	file      *os.File
	pending   []byte
}

func (w *lineWriter) Write(data []byte) (int, error) {
	c := w.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return 0, c.err
	}
	if _, err := w.file.Write(data); err != nil {
		c.err = err
		c.cancel()
		return 0, err
	}
	if len(data) > 0 && c.receipt != nil {
		c.receipt(time.Now())
	}
	w.pending = append(w.pending, data...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		if i > maxLineBytes {
			c.err = fmt.Errorf("%s line exceeds %d bytes", w.stream, maxLineBytes)
			c.cancel()
			return 0, c.err
		}
		if err := c.line(w.stream, bytes.TrimSuffix(w.pending[:i], []byte("\r"))); err != nil {
			c.err = err
			c.cancel()
			return 0, err
		}
		w.pending = w.pending[i+1:]
	}
	if len(w.pending) > maxLineBytes {
		c.err = fmt.Errorf("%s line exceeds %d bytes", w.stream, maxLineBytes)
		c.cancel()
		return 0, c.err
	}
	if len(w.pending) == 0 {
		w.pending = nil
	}
	return len(data), nil
}
func (w *lineWriter) flush() error {
	c := w.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	if len(w.pending) > 0 {
		err := c.line(w.stream, w.pending)
		w.pending = nil
		if err != nil {
			c.err = err
		}
		return err
	}
	return nil
}

// Caller holds mu. All timestamps are runner receipt times from a monotonic clock.
func (c *collector) line(stream string, line []byte) error {
	now := time.Now()
	elapsed := now.Sub(c.start).Nanoseconds()
	if err := c.raw.Encode(telemetry.RawLine{TimestampNS: now.UnixNano(), ElapsedNS: elapsed, Stream: stream, Line: string(line)}); err != nil {
		return err
	}
	if stream == "stderr" {
		if f := adapters.DiagnosticFailure(c.parser, line); f != nil {
			c.diagnosticFailure = f
		}
		return nil
	}
	if stream != "stdout" || len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	if c.firstStdout == nil {
		s := float64(elapsed) / 1e9
		c.firstStdout = &s
	}
	events, err := c.parser.ParseEvent(line)
	if err != nil {
		c.parseErrors++
		return nil
	}
	for _, event := range events {
		event.RunID = c.runID
		event.Agent = c.agent
		event.TimestampNS = now.UnixNano()
		event.ElapsedNS = elapsed
		if event.Type == "tool_started" {
			event.TimingBasis = "tool_start_receipt"
		}
		if event.Type == "tool_finished" {
			event.TimingBasis = "tool_finish_receipt"
		}
		if event.Type == "agent_error" {
			c.failed = true
			if event.Failure != nil && (c.failure == nil || (c.failure.Scope != "agent" && event.Failure.Scope == "agent")) {
				c.failure = event.Failure
			}
		}
		if err := c.normalized.Encode(event); err != nil {
			return err
		}
		c.events = append(c.events, event)
	}
	return nil
}
func (c *collector) lifecycle(kind string) error {
	now := time.Now()
	e := telemetry.Event{RunID: c.runID, Agent: c.agent, Type: kind, TimestampNS: now.UnixNano(), ElapsedNS: now.Sub(c.start).Nanoseconds()}
	c.events = append(c.events, e)
	return c.normalized.Encode(e)
}
