package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func TestCachePreparationAndProcessEnvironment(t *testing.T) {
	workspace, artifacts := t.TempDir(), t.TempDir()
	task := benchmark.Case{GoCache: "warm", CacheWarmup: []benchmark.Check{{Command: "sh", Args: []string{"-c", `test "$GOPROXY" = off && test "$GOENV" = off && printf prepared > "$GOCACHE/marker"`}}}}
	env, err := prepareGoEnvironment(context.Background(), task, workspace, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	result := RunProcess(context.Background(), adapters.Command{Path: "sh", Env: env, Args: []string{"-c", `test "$(cat "$GOCACHE/marker")" = prepared && test "$GOFLAGS" = -mod=vendor`}}, workspace, os.Stdout, os.Stderr)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	second := t.TempDir()
	_, err = prepareGoEnvironment(context.Background(), benchmark.Case{GoCache: "cold"}, second, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(second, ".benchmark-runtime/go-build/marker")); !os.IsNotExist(err) {
		t.Fatal("cache leaked between attempts")
	}
	task.CacheWarmup[0].Args = []string{"-c", "exit 7"}
	if _, err := prepareGoEnvironment(context.Background(), task, t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("failed warmup accepted")
	}
}

func TestEphemeralHomeCopiesOnlyAuthentication(t *testing.T) {
	original := t.TempDir()
	t.Setenv("HOME", original)
	t.Setenv("CODEX_HOME", filepath.Join(original, ".codex"))
	for path, data := range map[string]string{".cursor/auth.json": "PRIVATE_CANARY", ".config/cursor/auth.json": "PRIVATE_CANARY", ".codex/auth.json": "PRIVATE_CANARY", ".codex/AGENTS.md": "steering", ".cursor/cli-config.json": `{"authInfo":{"token":"PRIVATE_CANARY"},"steering":"secret instructions","permissions":{}}`, ".gemini/antigravity-cli/antigravity-oauth-token": "PRIVATE_CANARY"} {
		full := filepath.Join(original, path)
		os.MkdirAll(filepath.Dir(full), 0700)
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for adapter, path := range map[string]string{"codex": ".codex/auth.json", "cursor": ".cursor/cli-config.json", "agy": ".gemini/antigravity-cli/antigravity-oauth-token"} {
		work := t.TempDir()
		if adapter == "agy" && runtime.GOOS == "darwin" {
			_, err := prepareAgentHome(benchmark.Agent{Adapter: adapter, IsolateConfig: true}, work)
			if !errors.Is(err, errAGYKeychainIsolation) {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(work, "home")); !os.IsNotExist(err) {
				t.Fatal("blocked agy created a home", err)
			}
			continue
		}
		env, err := prepareAgentHome(benchmark.Agent{Adapter: adapter, IsolateConfig: true}, work)
		if err != nil || len(env) == 0 {
			t.Fatal(env, err)
		}
		data, err := os.ReadFile(filepath.Join(work, "home", path))
		if err != nil || !strings.Contains(string(data), "PRIVATE_CANARY") || strings.Contains(string(data), "steering") {
			t.Fatal("authentication isolation failed", err)
		}
		info, _ := os.Stat(filepath.Join(work, "home", path))
		if info.Mode().Perm() != 0600 {
			t.Fatal(info.Mode())
		}
		if _, err := os.Stat(filepath.Join(work, "home/.codex/AGENTS.md")); !os.IsNotExist(err) {
			t.Fatal("instructions copied")
		}
		if adapter == "agy" {
			if _, err := os.Stat(filepath.Join(work, "home/.gemini/antigravity-cli/settings.json")); !os.IsNotExist(err) {
				t.Fatal("host permission settings must not be copied", err)
			}
		}
	}
}

func TestStripInstructionsDoesNotFollowSymlinks(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(external, "AGENTS.md"), []byte("keep"), 0600)
	os.MkdirAll(filepath.Join(root, "nested/.cursor"), 0700)
	os.WriteFile(filepath.Join(root, "nested/AGENTS.md"), []byte("remove"), 0600)
	os.Symlink(external, filepath.Join(root, "linked"))
	if err := stripInstructions(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested/AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("instructions survived")
	}
	if _, err := os.Stat(filepath.Join(root, "nested/.cursor")); !os.IsNotExist(err) {
		t.Fatal("settings survived")
	}
	if _, err := os.Stat(filepath.Join(external, "AGENTS.md")); err != nil {
		t.Fatal("external source changed")
	}
}

func TestPatchIncludesTrackedAndNewSubmittedFiles(t *testing.T) {
	workspace, artifacts := t.TempDir(), t.TempDir()
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(workspace, "tracked.go"), []byte("package original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "tracked.go"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "base"}} {
		if _, err := git(ctx, workspace, args...); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(workspace, "candidate.go"), []byte("package candidate\n"), 0600)
	os.WriteFile(filepath.Join(workspace, "tracked.go"), []byte("package updated\n"), 0600)
	if err := retainFiles(workspace, artifacts, []string{"tracked.go", "candidate.go"}); err != nil {
		t.Fatal(err)
	}
	if err := retainPatch(ctx, workspace, artifacts, []string{"tracked.go", "candidate.go"}, "HEAD"); err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(filepath.Join(artifacts, "candidate.patch.txt"))
	if err != nil || !strings.Contains(string(patch), "+package candidate") || !strings.Contains(string(patch), "+package updated") || !strings.Contains(string(patch), "-package original") {
		t.Fatal(string(patch), err)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "source_manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestAGYKeychainIsolationPlatformPolicy(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		for _, isolated := range []bool{false, true} {
			err := checkAgentHomeIsolation(benchmark.Agent{Adapter: "agy", IsolateConfig: isolated}, platform)
			if errors.Is(err, errAGYKeychainIsolation) != (platform == "darwin" && isolated) {
				t.Fatal(platform, isolated, err)
			}
		}
		if err := checkAgentHomeIsolation(benchmark.Agent{Adapter: "codex", IsolateConfig: true}, platform); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBlockedAGYDoesNotInvokeExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Keychain guard")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "fake-agy")
	marker := filepath.Join(dir, "invoked")
	if err := os.WriteFile(command, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err := executeOne(context.Background(), benchmark.Agent{Name: "agy", Adapter: "agy", Command: command, IsolateConfig: true}, fixtureTask(), 1, "test", dir)
	if err != nil || r.Status != "agent_unavailable" || r.Success != nil || r.Failure == nil || r.Failure.Code != "macos_keychain_isolation_unsupported" || r.Failure.Scope != "agent" || r.Failure.Retryable {
		t.Fatal(r, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("blocked executable ran", err)
	}
}
