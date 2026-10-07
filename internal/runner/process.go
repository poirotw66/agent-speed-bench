package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
)

type ProcessResult struct {
	ExitCode *int
	Err      error
	Wall     time.Duration
}

// RunProcess owns a process group. Writers must be concurrency safe if shared.
func RunProcess(ctx context.Context, c adapters.Command, dir string, stdout, stderr io.Writer) ProcessResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	cmd.Stdin = strings.NewReader(c.Stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return ProcessResult{Err: err, Wall: time.Since(start)}
	}
	err := cmd.Wait()
	// Also reap descendants when the parent exits without waiting for them.
	_ = killProcessGroup(cmd)
	code := cmd.ProcessState.ExitCode()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	return ProcessResult{ExitCode: &code, Err: err, Wall: time.Since(start)}
}
