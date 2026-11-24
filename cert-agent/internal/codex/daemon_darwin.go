package codex

import (
	"os"
	"os/exec"
)

func prepareDaemonCommand(*exec.Cmd) error {
	if os.Geteuid() == 0 {
		return errElevatedDaemon
	}
	return nil
}
