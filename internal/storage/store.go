package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Precreate private files; SQLite sidecars inherit the database permissions.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS runs (
 id TEXT PRIMARY KEY, experiment_id TEXT NOT NULL, agent TEXT NOT NULL,
 case_name TEXT NOT NULL, started_at TEXT NOT NULL, status TEXT NOT NULL,
 success INTEGER, wall_seconds REAL NOT NULL, output_tokens INTEGER,
 effective_output_tps REAL, ttfa_seconds REAL, record_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_experiment ON runs(experiment_id);
CREATE INDEX IF NOT EXISTS runs_agent_time ON runs(agent, started_at);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Save(r telemetry.Run) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO runs(id,experiment_id,agent,case_name,started_at,status,success,wall_seconds,output_tokens,effective_output_tps,ttfa_seconds,record_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.ExperimentID, r.Agent, r.Case, r.StartedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), r.Status, r.Success, r.Metrics.WallSeconds, r.Metrics.Usage.OutputTokens, r.Metrics.EffectiveOutputTPS, r.Metrics.TTFASeconds, string(b))
	return err
}

// Reconcile imports a missing artifact record without replacing stored evidence.
func (s *Store) Reconcile(r telemetry.Run) error {
	var raw string
	err := s.db.QueryRow("SELECT record_json FROM runs WHERE id=?", r.ID).Scan(&raw)
	if err == sql.ErrNoRows {
		return s.Save(r)
	}
	if err != nil {
		return err
	}
	var stored telemetry.Run
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return err
	}
	if !reflect.DeepEqual(stored, r) {
		return fmt.Errorf("record %s disagrees with immutable stored evidence", r.ID)
	}
	return nil
}
func (s *Store) Runs(experiment string) ([]telemetry.Run, error) {
	query := "SELECT record_json FROM runs"
	var args []any
	if experiment != "" {
		query += " WHERE experiment_id=?"
		args = append(args, experiment)
	}
	query += " ORDER BY started_at,id"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []telemetry.Run{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r telemetry.Run
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, fmt.Errorf("invalid stored run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".json-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
