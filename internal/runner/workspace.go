package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func Prepare(ctx context.Context, task benchmark.Case, dir string) (string, error) {
	if task.Repo != nil {
		// Resolve once in preflight, then clone an independent local object store.
		if task.Repo.FreshHistory {
			// Fetch only the pinned tree, also supporting shallow/filtered caches
			// without requesting unrelated historical objects from their server.
			if _, err := git(ctx, "", "init", "--quiet", "--template=", dir); err != nil {
				return "", err
			}
			if _, err := git(ctx, dir, "fetch", "--quiet", "--depth=1", "--", task.Repo.Path, task.Repo.Commit); err != nil {
				return "", err
			}
		} else {
			if _, err := git(ctx, "", "clone", "--quiet", "--no-local", "--no-checkout", "--", task.Repo.Path, dir); err != nil {
				return "", err
			}
		}
		if _, err := git(ctx, dir, "checkout", "--quiet", "--detach", task.Repo.Commit); err != nil {
			return "", err
		}
		commit, err := git(ctx, dir, "rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		if task.StripInstructions {
			if err := stripInstructions(dir); err != nil {
				return "", err
			}
		}
		if task.Repo.FreshHistory {
			// Remove upstream objects and remotes only from this disposable clone.
			if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
				return "", err
			}
			for _, args := range [][]string{
				{"init", "--quiet", "--template="},
				{"add", "--force", "--all"},
				{"-c", "user.name=AgentSpeedBench", "-c", "user.email=benchmark@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--quiet", "-m", "Benchmark base"},
			} {
				if _, err := git(ctx, dir, args...); err != nil {
					return "", err
				}
			}
		}
		return commit, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for path, content := range task.Files {
		if !benchmark.SafePath(path) {
			return "", fmt.Errorf("unsafe seed path %q", path)
		}
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			return "", err
		}
	}
	if task.StripInstructions {
		return "", stripInstructions(dir)
	}
	return "", nil
}

func ResolveRepos(ctx context.Context, cfg *benchmark.Config) error {
	for i := range cfg.Cases {
		r := cfg.Cases[i].Repo
		if r == nil {
			continue
		}
		commit, err := git(ctx, r.Path, "rev-parse", "--verify", r.Commit+"^{commit}")
		if err != nil {
			return err
		}
		r.Commit = commit
	}
	return nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	var out limitedBuffer
	result := RunProcess(ctx, adapters.Command{Path: "git", Args: args}, dir, &out, &out)
	if result.Err != nil {
		return "", fmt.Errorf("git: %w: %s", result.Err, strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
