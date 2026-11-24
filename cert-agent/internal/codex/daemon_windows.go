package codex

import (
	"os/exec"
	"syscall"
	"unsafe"
)

func prepareDaemonCommand(command *exec.Cmd) error {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	var elevated, size uint32
	if err := syscall.GetTokenInformation(token, syscall.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &size); err != nil {
		return err
	}
	if elevated != 0 {
		return errElevatedDaemon
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return nil
}
