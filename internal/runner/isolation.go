package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

// Only disposable workspaces are sanitized; symlinks are never traversed.
func stripInstructions(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if name == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		directory := name == ".codex" || name == ".cursor" || name == ".gemini" || name == ".agents" || name == ".claude" || name == ".antigravitycli"
		instruction := name == "agents.md" || name == "agents.override.md" || name == "gemini.md" || name == "claude.md" || name == ".cursorrules"
		if directory || instruction {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

var errAGYKeychainIsolation = errors.New("isolated agy OAuth execution on macOS is unavailable: temporary HOME does not isolate native Keychain access and may trigger storage/reset dialogs; no keychain settings were changed")

func checkAgentHomeIsolation(a benchmark.Agent, platform string) error {
	if a.IsolateConfig && a.Adapter == "agy" && platform == "darwin" {
		return errAGYKeychainIsolation
	}
	return nil
}

// Credentials remain private in an ephemeral home and are never stored in artifacts.
func prepareAgentHome(a benchmark.Agent, workdir string) ([]string, error) {
	if err := checkAgentHomeIsolation(a, runtime.GOOS); err != nil {
		return nil, err
	}
	if !a.IsolateConfig {
		return nil, nil
	}
	original, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	home := filepath.Join(workdir, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, err
	}
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "AGY_CLI_DISABLE_AUTO_UPDATE=true"}
	copyPrivate := func(source, dest string) error {
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
			return fmt.Errorf("invalid authentication file: %s", source)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0600)
	}
	switch a.Adapter {
	case "codex":
		source := os.Getenv("CODEX_HOME")
		if source == "" {
			source = filepath.Join(original, ".codex")
		}
		target := filepath.Join(home, ".codex")
		if err := os.MkdirAll(target, 0700); err != nil {
			return nil, err
		}
		if err := copyPrivate(filepath.Join(source, "auth.json"), filepath.Join(target, "auth.json")); err != nil {
			return nil, err
		}
		env = append(env, "CODEX_HOME="+target)
	case "cursor":
		source := filepath.Join(original, ".cursor", "cli-config.json")
		data, err := os.ReadFile(source)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		var config map[string]json.RawMessage
		if len(data) > 0 {
			if err := json.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("invalid Cursor authentication configuration")
			}
		}
		clean := map[string]json.RawMessage{}
		for _, key := range []string{"authInfo", "version"} {
			if value, ok := config[key]; ok {
				clean[key] = value
			}
		}
		data, err = json.Marshal(clean)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(home, ".cursor")
		if err := os.MkdirAll(target, 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(target, "cli-config.json"), data, 0600); err != nil {
			return nil, err
		}
		authSource := filepath.Join(original, ".cursor", "auth.json")
		if runtime.GOOS == "linux" {
			authSource = filepath.Join(original, ".config", "cursor", "auth.json")
		}
		if err := copyPrivate(authSource, filepath.Join(target, "auth.json")); err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(target, "auth.json")); os.IsNotExist(err) && runtime.GOOS == "darwin" && os.Getenv("CURSOR_API_KEY") == "" && len(clean["authInfo"]) > 0 {
			credentials := map[string]string{}
			for _, field := range []struct{ key, service string }{{"accessToken", "cursor-access-token"}, {"refreshToken", "cursor-refresh-token"}, {"apiKey", "cursor-api-key"}} {
				// Read only this CLI's existing login. Never include private stdout in diagnostics.
				bounded, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				cmd := exec.CommandContext(bounded, "security", "find-generic-password", "-a", "cursor-user", "-s", field.service, "-w")
				if value, err := cmd.Output(); err == nil {
					credentials[field.key] = strings.TrimSpace(string(value))
				}
				cancel()
			}
			if len(credentials) > 0 {
				private, err := json.Marshal(credentials)
				if err != nil {
					return nil, err
				}
				if err := os.WriteFile(filepath.Join(target, "auth.json"), private, 0600); err != nil {
					return nil, err
				}
			}
		}
		if runtime.GOOS == "linux" {
			configTarget := filepath.Join(home, ".config", "cursor")
			if err := os.MkdirAll(configTarget, 0700); err != nil {
				return nil, err
			}
			for _, name := range []string{"cli-config.json", "auth.json"} {
				if err := copyPrivate(filepath.Join(target, name), filepath.Join(configTarget, name)); err != nil {
					return nil, err
				}
			}
		}
		env = append(env, "AGENT_CLI_CREDENTIAL_STORE=file")
	case "agy":
		target := filepath.Join(home, ".gemini", "antigravity-cli")
		if err := os.MkdirAll(target, 0700); err != nil {
			return nil, err
		}
		if err := copyPrivate(filepath.Join(original, ".gemini", "antigravity-cli", "antigravity-oauth-token"), filepath.Join(target, "antigravity-oauth-token")); err != nil {
			return nil, err
		}
	}
	return env, nil
}
