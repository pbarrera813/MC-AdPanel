//go:build linux

package minecraft

import (
	"os/exec"
	"syscall"
)

func prepareServerProcessCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func killServerProcessTree(pid int) error {
	// Negative PID targets the full process group created with Setpgid.
	return syscall.Kill(-pid, syscall.SIGKILL)
}
