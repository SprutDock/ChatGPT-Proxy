package state

import (
	"os"
	"path/filepath"
	"syscall"
)

func LockEnsure(p Paths) (func(), error) {
	file, err := os.OpenFile(filepath.Join(p.State, "ensure.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
