package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestStoreRoundTripPreservesUnknownAndFilters(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := telemetry.Run{ID: "run1", ExperimentID: "exp", Agent: "codex", Case: "task", Status: "completed", StartedAt: time.Now().UTC(), Metrics: telemetry.Metrics{WallSeconds: 1}}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Runs("exp")
	if err != nil || len(got) != 1 || got[0].Success != nil || got[0].Metrics.Usage.OutputTokens != nil {
		t.Fatal(got, err)
	}
	got, err = s.Runs("exp' OR 1=1 --")
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	if err := s.Save(r); err == nil {
		t.Fatal("duplicate run accepted")
	}
	var success, token any
	if err := s.db.QueryRow("SELECT success,output_tokens FROM runs WHERE id=?", "run1").Scan(&success, &token); err != nil {
		t.Fatal(err)
	}
	if success != nil || token != nil {
		t.Fatal("unknown fields must be SQL NULL", success, token)
	}
}

func TestReconcileDoesNotReplaceExistingEvidence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := telemetry.Run{ID: "immutable", ExperimentID: "e", Status: "canceled"}
	if err := s.Reconcile(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(r); err != nil {
		t.Fatal(err)
	}
	r.Status = "completed"
	if err := s.Reconcile(r); err == nil {
		t.Fatal("stored evidence replaced")
	}
	rows, _ := s.Runs("e")
	if len(rows) != 1 || rows[0].Status != "canceled" {
		t.Fatal(rows)
	}
}

func TestNewTelemetryRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	thinking, cache, duration, tier := int64(214), int64(0), 5.990406, "fast"
	r := telemetry.Run{ID: "new", ExperimentID: "exp", StartedAt: time.Now().UTC(), Settings: telemetry.Settings{RequestedServiceTier: "fast", ConfiguredServiceTier: &tier}, Metrics: telemetry.Metrics{Usage: telemetry.Usage{ThinkingTokens: &thinking, CacheWriteTokens: &cache, OutputTokenAccounting: "includes_thinking"}, ReportedDurationSeconds: &duration, ReportedDurationSource: "agy.result.duration_seconds"}}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Runs("exp")
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	v := got[0]
	if *v.Metrics.Usage.ThinkingTokens != 214 || *v.Metrics.Usage.CacheWriteTokens != 0 || *v.Metrics.ReportedDurationSeconds != duration || v.Settings.RequestedServiceTier != "fast" || v.Settings.ObservedServiceTier != nil {
		t.Fatal(v)
	}
}

func TestMeasurementPhaseAndScoringRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	receipt, gap, characters := 2.0, 1.0, int64(6)
	r := telemetry.Run{ID: "phase", ExperimentID: "exp", Warmup: true, Verification: []telemetry.VerificationResult{{Layer: "core", Passed: true}}, Metrics: telemetry.Metrics{AnswerCompleteSeconds: &receipt, TerminalReceiptSeconds: &receipt, TerminalToExitSeconds: &gap, OutputCharacters: &characters}}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Runs("exp")
	if err != nil || len(got) != 1 || !got[0].Warmup || *got[0].Metrics.TerminalToExitSeconds != 1 || *got[0].Metrics.OutputCharacters != 6 || !got[0].Verification[0].Passed {
		t.Fatal(got, err)
	}
}
