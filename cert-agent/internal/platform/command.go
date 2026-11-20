package platform

import (
	"os"
	"os/exec"
)

func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func available(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
