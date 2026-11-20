package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	lockUpdateFile      = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	unlockUpdateFile    = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
	waitForSingleObject = syscall.NewLazyDLL("kernel32.dll").NewProc("WaitForSingleObject")
)

func runUpdateHelper(arguments []string) error {
	if len(arguments) != 3 {
		return fmt.Errorf("update helper arguments are invalid")
	}
	parentPID, err := parseProcessID(arguments[2])
	if err != nil {
		return err
	}
	return applyStagedUpdate(arguments[0], arguments[1], parentPID)
}

func applyStagedUpdate(stage, destination string, parentPID uint32) error {
	stage, err := filepath.Abs(stage)
	if err != nil {
		return fmt.Errorf("update staging path is invalid")
	}
	writeUpdateCleanupMarker(filepath.Dir(stage))
	destination, err = filepath.Abs(destination)
	if err != nil || !filepath.IsAbs(destination) {
		return fmt.Errorf("application path is invalid")
	}
	if err := waitForProcessExit(parentPID, 30*time.Second); err != nil {
		return fmt.Errorf("launcher did not exit before the update")
	}
	if err := checkPackageFiles(stage); err != nil {
		return fmt.Errorf("staged update package is incomplete")
	}
	unlock, err := lockUpdateState()
	if err != nil {
		return fmt.Errorf("could not lock certificate state for update")
	}
	defer unlock()
	parent := filepath.Dir(destination)
	stagedCopy, err := os.MkdirTemp(parent, ".chatgpt-proxy-update-")
	if err != nil {
		return fmt.Errorf("could not prepare the new application files")
	}
	defer os.RemoveAll(stagedCopy)
	if err := copyPackage(stage, stagedCopy); err != nil {
		return fmt.Errorf("could not copy the update into the application folder")
	}
	oldAgent := supportFile(destination, "cert-agent.exe")
	if err := runAgentCommand(oldAgent, "stop"); err != nil {
		return fmt.Errorf("could not stop the certificate tunnel for update")
	}
	backup, err := newSiblingPath(parent, ".chatgpt-proxy-backup-")
	if err != nil {
		_ = runAgentCommand(oldAgent, "ensure", "--background")
		return fmt.Errorf("could not prepare the update rollback")
	}
	if err := os.Rename(destination, backup); err != nil {
		_ = os.Remove(backup)
		_ = runAgentCommand(oldAgent, "ensure", "--background")
		return fmt.Errorf("application files are busy; close ChatGPT Proxy and retry the update")
	}
	if err := os.Rename(stagedCopy, destination); err != nil {
		if rollbackErr := os.Rename(backup, destination); rollbackErr != nil {
			return fmt.Errorf("update failed and the previous application could not be restored")
		}
		_ = runAgentCommand(supportFile(destination, "cert-agent.exe"), "ensure", "--background")
		return fmt.Errorf("update could not be activated; the previous version was restored")
	}
	if err := startUpdatedApplication(destination); err != nil {
		failed, pathErr := newSiblingPath(parent, ".chatgpt-proxy-failed-")
		if pathErr != nil {
			return fmt.Errorf("new application did not start and rollback could not be prepared")
		}
		if moveErr := os.Rename(destination, failed); moveErr != nil {
			if removeErr := os.RemoveAll(destination); removeErr != nil {
				return fmt.Errorf("new application did not start and rollback could not replace it")
			}
		}
		if os.Rename(backup, destination) != nil {
			return fmt.Errorf("new application did not start and rollback failed")
		}
		_ = runAgentCommand(supportFile(destination, "cert-agent.exe"), "ensure", "--background")
		_ = os.RemoveAll(failed)
		return fmt.Errorf("new application did not start; the previous version was restored")
	}
	_ = os.RemoveAll(backup)
	launcherLog("update applied")
	return nil
}

func newSiblingPath(parent, pattern string) (string, error) {
	path, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func copyPackage(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("staged file path is invalid")
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("staged package contains an unsupported file type")
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return inputErr
	})
}

func runAgentCommand(agent string, arguments ...string) error {
	if _, err := os.Stat(agent); err != nil {
		if len(arguments) > 0 && arguments[0] == "stop" {
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, agent, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := command.Run(); err != nil {
		return err
	}
	return nil
}

func startUpdatedApplication(destination string) error {
	command := exec.Command(filepath.Join(destination, "ChatGPT Proxy.exe"))
	command.Dir = destination
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := command.Start(); err != nil {
		return err
	}
	_ = command.Process.Release()
	return nil
}

func lockUpdateState() (func(), error) {
	stateDirectory := userStateDirectory()
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(stateDirectory, "ensure.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock := syscall.Overlapped{}
	deadline := time.Now().Add(20 * time.Minute)
	for {
		result, _, callErr := lockUpdateFile.Call(file.Fd(), 0x3, 0, 1, 0, uintptr(unsafe.Pointer(&lock)))
		if result != 0 {
			return func() {
				_, _, _ = unlockUpdateFile.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&lock)))
				_ = file.Close()
			}, nil
		}
		if callErr != syscall.Errno(33) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, callErr
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitForProcessExit(processID uint32, timeout time.Duration) error {
	const synchronize = 0x00100000
	handle, err := syscall.OpenProcess(synchronize, false, processID)
	if err != nil {
		if err == syscall.Errno(87) {
			return nil
		}
		return err
	}
	defer syscall.CloseHandle(handle)
	result, _, callErr := waitForSingleObject.Call(uintptr(handle), uintptr(timeout.Milliseconds()))
	switch result {
	case 0:
		return nil
	case 0x102:
		return fmt.Errorf("process did not exit before timeout")
	default:
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("could not wait for process exit")
	}
}

func writeUpdateCleanupMarker(path string) {
	if !validUpdateTempPath(path) {
		return
	}
	stateDirectory := userStateDirectory()
	if os.MkdirAll(stateDirectory, 0o700) != nil {
		return
	}
	data, err := json.Marshal(struct {
		Path string `json:"path"`
		PID  uint32 `json:"pid"`
	}{Path: path, PID: uint32(os.Getpid())})
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(stateDirectory, "update-cleanup.json"), data, 0o600)
}
