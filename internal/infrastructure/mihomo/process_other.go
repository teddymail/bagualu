//go:build !linux

package mihomo

import "os/exec"

func configureCommand(*exec.Cmd) {}

func killCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
