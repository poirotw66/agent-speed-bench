package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func permissionPolicy(adapter string) string {
	switch adapter {
	case "codex":
		return "codex_yolo_v1"
	case "cursor":
		return "cursor_yolo_sandbox_disabled_v1"
	case "agy", "claude":
		return adapter + "_dangerously_skip_permissions_v1"
	default:
		return ""
	}
}

// Each attempt starts with its own cache; warmup never shares agent-mutated data.
func prepareGoEnvironment(ctx context.Context, task benchmark.Case, workspace, artifacts string) ([]string, error) {
	if task.GoCache == "" {
		return nil, nil
	}
	runtimeDir := filepath.Join(workspace, ".benchmark-runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		return nil, err
	}
	for _, name := range []string{"go-build", "go-mod"} {
		if err := os.MkdirAll(filepath.Join(runtimeDir, name), 0700); err != nil {
			return nil, err
		}
	}
	env := []string{"GOCACHE=" + filepath.Join(runtimeDir, "go-build"), "GOMODCACHE=" + filepath.Join(runtimeDir, "go-mod"), "GOWORK=off", "GOENV=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=vendor", "CGO_ENABLED=0"}
	if len(task.CacheWarmup) == 0 {
		return env, nil
	}
	log, err := os.OpenFile(filepath.Join(artifacts, "preparation.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	for _, check := range task.CacheWarmup {
		bounded, cancel := context.WithTimeout(ctx, 180*time.Second)
		result := RunProcess(bounded, adapters.Command{Path: check.Command, Args: check.Args, Env: env}, workspace, log, log)
		cancel()
		if result.Err != nil {
			return nil, fmt.Errorf("cache warmup %s: %w", check.Command, result.Err)
		}
	}
	return env, nil
}
