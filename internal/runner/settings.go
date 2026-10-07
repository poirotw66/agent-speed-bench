package runner

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

// Snapshot only allowlisted fields; user config is not the resolved CLI configuration.
func snapshotSettings(a benchmark.Agent) telemetry.Settings {
	s := telemetry.Settings{RequestedServiceTier: a.ServiceTier, RequestedModel: a.Model, RequestedReasoningEffort: a.ReasoningEffort, ConfigStatus: "unknown"}
	if a.Adapter != "codex" {
		return s
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return s
		}
		home = filepath.Join(user, ".codex")
	}
	s.ConfigSource = filepath.Join(home, "config.toml")
	data, err := os.ReadFile(s.ConfigSource)
	if err != nil {
		s.ConfigStatus = "unavailable"
		return s
	}
	var config struct {
		Tier     *string `toml:"service_tier"`
		Features struct {
			FastMode *bool `toml:"fast_mode"`
		} `toml:"features"`
		Model  *string `toml:"model"`
		Effort *string `toml:"model_reasoning_effort"`
	}
	if toml.Unmarshal(data, &config) != nil {
		s.ConfigStatus = "invalid"
		return s
	}
	s.ConfiguredModel, s.ConfiguredReasoningEffort = config.Model, config.Effort
	s.ConfiguredServiceTier, s.ConfiguredFastMode = config.Tier, config.Features.FastMode
	s.ConfigStatus = "user_config_only"
	return s
}
