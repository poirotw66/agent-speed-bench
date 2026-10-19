package runner

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

// Capture only allowlisted arguments from the actual command; never serialize
// arbitrary argv, executable paths, stdin, environment, or configuration files.
func snapshotCLILaunch(a benchmark.Agent, command adapters.Command) *telemetry.CLILaunch {
	if a.Adapter != "codex" {
		return nil
	}
	s := &telemetry.CLILaunch{Arguments: []string{}}
	for i := 0; i < len(command.Args); i++ {
		arg := command.Args[i]
		switch arg {
		case "--ignore-user-config", "--ignore-rules", "--yolo":
			s.Arguments = append(s.Arguments, arg)
		case "--config":
			if i+1 >= len(command.Args) {
				continue
			}
			i++
			key, _, ok := strings.Cut(command.Args[i], "=")
			if !ok || key != "service_tier" {
				continue
			}
			var config struct {
				Tier string `toml:"service_tier"`
			}
			if toml.Unmarshal([]byte(command.Args[i]), &config) != nil || (config.Tier != "fast" && config.Tier != "default") {
				continue
			}
			s.ServiceTier = &config.Tier
			s.Arguments = append(s.Arguments, "--config", key+"="+strconv.Quote(config.Tier))
		case "--enable", "--disable":
			if i+1 < len(command.Args) {
				i++
				if command.Args[i] == "fast_mode" {
					enabled := arg == "--enable"
					s.FastMode = &enabled
					s.Arguments = append(s.Arguments, arg, "fast_mode")
				}
			}
		}
	}
	return s
}

// Snapshot only allowlisted fields; user config is not the resolved CLI configuration.
func snapshotSettings(a benchmark.Agent) telemetry.Settings {
	s := telemetry.Settings{RequestedServiceTier: a.ServiceTier, RequestedModel: a.Model, RequestedReasoningEffort: a.ReasoningEffort, ConfigStatus: "unknown"}
	if a.Adapter != "codex" {
		return s
	}
	if a.IsolateConfig {
		s.ConfigStatus = "user_config_ignored"
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
