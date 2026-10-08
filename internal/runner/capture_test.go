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
