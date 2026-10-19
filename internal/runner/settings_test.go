package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestLaunchEvidenceExcludesSecretsAndUsesActualArguments(t *testing.T) {
	a := benchmark.Agent{Adapter: "codex", ServiceTier: "fast"}
	command := adapters.Command{Path: "SECRET_CANARY", Stdin: "SECRET_CANARY", Env: []string{"TOKEN=SECRET_CANARY"}, Args: []string{
		"exec", "--config", "secret=\"SECRET_CANARY\"", "--config", "service_tier=\"default\" # SECRET_CANARY", "--disable", "fast_mode", "--model", "SECRET_CANARY", "SECRET_CANARY",
	}}
	s := snapshotCLILaunch(a, command)
	data, _ := json.Marshal(s)
	if s.ServiceTier == nil || *s.ServiceTier != "default" || s.FastMode == nil || *s.FastMode || strings.Contains(string(data), "SECRET_CANARY") {
		t.Fatal(string(data))
	}
	if got := snapshotCLILaunch(a, adapters.Command{}); got.ServiceTier != nil || got.FastMode != nil {
		t.Fatal("absence of explicit flags must stay unknown", got)
	}
	if snapshotCLILaunch(benchmark.Agent{Adapter: "generic"}, command) != nil {
		t.Fatal("custom commands must not be interpreted as Codex settings")
	}
}

func TestIsolatedFastLaunchEvidenceRoundTripWithoutServerInference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("service_tier='default'\nsecret='SECRET_CANARY'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, serverTier := range []string{"", "default", "priority"} {
		t.Run("receipt-"+serverTier, func(t *testing.T) {
			root := t.TempDir()
			command := filepath.Join(root, "fake-codex")
			terminal := `{"type":"turn.completed","usage":{"output_tokens":1}}`
			if serverTier != "" {
				terminal = `{"type":"turn.completed","service_tier":"` + serverTier + `","usage":{"output_tokens":1}}`
			}
			script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Done\"}}' '" + terminal + "'\n"
			if err := os.WriteFile(command, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			r, err := executeOne(context.Background(), benchmark.Agent{Name: "fast", Adapter: "codex", Command: command, ServiceTier: "fast", IsolateConfig: true}, fixtureTask(), 1, "test", root)
			if err != nil || r.Status != "completed" {
				t.Fatal(r, err)
			}
			s := r.Settings
			if s.ConfigStatus != "user_config_ignored" || s.ConfiguredServiceTier != nil || s.CLILaunch == nil || !s.CLILaunch.Started || s.CLILaunch.ServiceTier == nil || *s.CLILaunch.ServiceTier != "fast" || s.CLILaunch.FastMode == nil || !*s.CLILaunch.FastMode {
				t.Fatal(s)
			}
			if serverTier == "" {
				if s.ObservedServiceTier != nil || s.ObservedServiceTierSource != nil {
					t.Fatal("launch flags were promoted into server evidence", s)
				}
			} else if s.ObservedServiceTier == nil || *s.ObservedServiceTier != serverTier || s.ObservedServiceTierSource == nil || *s.ObservedServiceTierSource != "codex.turn.completed.service_tier" {
				t.Fatal(s)
			}
			store, err := storage.Open(filepath.Join(root, "db.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Save(r); err != nil {
				t.Fatal(err)
			}
			rows, err := store.Runs("test")
			if err != nil || len(rows) != 1 {
				t.Fatal(rows, err)
			}
			want, _ := json.Marshal(s)
			got, _ := json.Marshal(rows[0].Settings)
			if string(want) != string(got) || strings.Contains(string(got), "SECRET_CANARY") {
				t.Fatal(string(got))
			}
		})
	}
}

func TestFailedLaunchAndLegacyEvidenceRemainDistinct(t *testing.T) {
	r, err := executeOne(context.Background(), benchmark.Agent{Name: "fast", Adapter: "codex", Command: "/nonexistent-codex", ServiceTier: "fast", IsolateConfig: true}, fixtureTask(), 1, "test", t.TempDir())
	if err != nil || r.Settings.CLILaunch == nil || r.Settings.CLILaunch.Started || r.Settings.ObservedServiceTier != nil {
		t.Fatal(r, err)
	}
	var old telemetry.Settings
	if err := json.Unmarshal([]byte(`{"requested_service_tier":"fast","config_status":"user_config_ignored"}`), &old); err != nil || old.CLILaunch != nil || old.ObservedServiceTier != nil {
		t.Fatal(old, err)
	}
}
