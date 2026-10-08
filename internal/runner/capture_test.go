//go:build darwin || linux

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func TestCaptureSurvivesFailureTimeoutCancellationAndAgentCommit(t *testing.T) {
	for _, status := range []string{"failed", "timeout", "canceled"} {
		t.Run(status, func(t *testing.T) {
			repo, artifacts := t.TempDir(), t.TempDir()
			os.WriteFile(filepath.Join(repo, "source.txt"), []byte("original\n"), 0600)
			for _, args := range [][]string{{"init", "--quiet", "--template="}, {"add", "source.txt"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "base"}} {
				if _, err := git(context.Background(), repo, args...); err != nil {
					t.Fatal(err)
				}
			}
			base, _ := git(context.Background(), repo, "rev-parse", "HEAD")
			task := fixtureTask()
			task.TimeoutSeconds = 1
			task.Repo = &benchmark.Repo{Path: repo, Commit: base}
			task.RetainFiles, task.RetainPatch = []string{"source.txt"}, true
			marker := filepath.Join(t.TempDir(), "ready")
			script := `printf 'changed\n' > source.txt; git add source.txt; git -c user.name=Fixture -c user.email=fixture@example.org -c commit.gpgsign=false commit --quiet -m candidate; printf partial; touch "$1"; `
			if status == "failed" {
				script += "exit 1"
			} else {
				script += "sleep 30"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if status == "canceled" {
				task.TimeoutSeconds = 10
				go func() {
					deadline := time.NewTimer(5 * time.Second)
					defer deadline.Stop()
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-deadline.C:
							cancel()
							return
						case <-ticker.C:
							if _, err := os.Stat(marker); err == nil {
								cancel()
								return
							}
						}
					}
				}()
			}
			r, err := executeOne(ctx, benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", script, "fixture", marker}}, task, 1, "capture", artifacts)
			if err != nil || r.Status != status || len(r.ArtifactErrors) != 0 {
				t.Fatal(r, err)
			}
			if status == "canceled" && r.Success != nil {
				t.Fatal("cancellation was graded")
			}
			patch, err := os.ReadFile(filepath.Join(r.ArtifactDir, "candidate.patch.txt"))
			if err != nil || !strings.Contains(string(patch), "+changed") || r.PatchBaseline != base {
				t.Fatal(string(patch), err, r.PatchBaseline)
			}
			data, err := os.ReadFile(filepath.Join(r.ArtifactDir, "candidate/source.txt.txt"))
			if err != nil || string(data) != "changed\n" {
				t.Fatal(string(data), err)
			}
			if _, err := os.Stat(filepath.Join(r.ArtifactDir, "assistant.partial.txt")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCapturedPatchReplaysDeletionIndexRemovalAndRecreation(t *testing.T) {
	for _, action := range []string{"delete", "untrack", "recreate"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			repo, artifacts := t.TempDir(), t.TempDir()
			source := filepath.Join(repo, "source.txt")
			if err := os.WriteFile(source, []byte("original\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"init", "--quiet", "--template="}, {"add", "source.txt"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "base"}} {
				if _, err := git(ctx, repo, args...); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := git(ctx, repo, "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			if action == "untrack" {
				_, err = git(ctx, repo, "rm", "--cached", "source.txt")
			} else {
				_, err = git(ctx, repo, "rm", "source.txt")
			}
			if err != nil {
				t.Fatal(err)
			}
			if action != "delete" {
				if err := os.WriteFile(source, []byte("replacement\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			indexBefore, err := git(ctx, repo, "ls-files", "--stage")
			if err != nil {
				t.Fatal(err)
			}
			if err := retainPatch(ctx, repo, artifacts, []string{"source.txt"}, baseline); err != nil {
				t.Fatal(err)
			}
			indexAfter, err := git(ctx, repo, "ls-files", "--stage")
			if err != nil || indexBefore != indexAfter {
				t.Fatal("capture changed agent index", err)
			}
			replay := filepath.Join(t.TempDir(), "replay")
			if _, err := git(ctx, "", "clone", "--quiet", repo, replay); err != nil {
				t.Fatal(err)
			}
			if _, err := git(ctx, replay, "apply", filepath.Join(artifacts, "candidate.patch.txt")); err != nil {
				t.Fatal("patch does not replay", err)
			}
			data, err := os.ReadFile(filepath.Join(replay, "source.txt"))
			if action == "delete" {
				if !os.IsNotExist(err) {
					t.Fatal("deleted file restored", err)
				}
			} else if err != nil || string(data) != "replacement\n" {
				t.Fatal(string(data), err)
			}
		})
	}
}
