//go:build !darwin && !linux

package runner

import (
	"os"
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {}
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
