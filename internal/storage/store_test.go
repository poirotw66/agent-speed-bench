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
