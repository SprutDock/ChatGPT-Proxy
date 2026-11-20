package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"sprutdock/cert-agent/internal/state"
)

func EnsureStunnelRunning(p state.Paths) error {
	pid, err := readStunnelPID(p)
	if err == nil {
		if stunnelProcessRunning(pid, filepath.Join(p.Resources, "stunnel.exe")) || stunnelProcessOwnsProxyPort(pid) {
			return nil
		}
		if err := stopStunnelProcess(pid); err != nil {
			return err
		}
	}
	_ = os.Remove(StunnelPIDFile(p))
	return startStunnel(p)
}

func RestartStunnel(p state.Paths) error {
	if err := StopStunnel(p); err != nil {
		return err
	}
	return startStunnel(p)
}

func StopStunnel(p state.Paths) error {
	pid, err := readStunnelPID(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		_ = os.Remove(StunnelPIDFile(p))
		return nil
	}
	if err := stopStunnelProcess(pid); err != nil {
		return err
	}
	return os.Remove(StunnelPIDFile(p))
}

func stopStunnelProcess(pid uint32) error {
	image := stunnelProcessPath(pid)
	if image == "" || !strings.EqualFold(filepath.Base(image), "stunnel.exe") {
		return nil
	}
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && strings.EqualFold(stunnelProcessPath(pid), image) {
		time.Sleep(100 * time.Millisecond)
	}
	if strings.EqualFold(stunnelProcessPath(pid), image) {
		return fmt.Errorf("could not stop the local proxy tunnel")
	}
	return nil
}

func startStunnel(p state.Paths) error {
	logFile, err := os.OpenFile(p.StunnelLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	stunnel := Command(filepath.Join(p.Resources, "stunnel.exe"), p.StunnelConf)
	stunnel.Env = stunnelEnvironment(p.Resources)
	stunnel.Stdout = logFile
	stunnel.Stderr = logFile
	stunnel.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := stunnel.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("could not start the local proxy tunnel: %w", err)
	}
	pid := strconv.Itoa(stunnel.Process.Pid)
	if err := state.WriteAtomic(StunnelPIDFile(p), []byte(pid), 0o600); err != nil {
		_ = stunnel.Process.Kill()
		_ = stunnel.Process.Release()
		_ = logFile.Close()
		return err
	}
	_ = stunnel.Process.Release()
	_ = logFile.Close()
	return nil
}

func stunnelEnvironment(resources string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if strings.EqualFold(name, "OPENSSL_CONF") || strings.EqualFold(name, "OPENSSL_MODULES") {
			continue
		}
		environment = append(environment, variable)
	}
	return append(environment,
		"OPENSSL_CONF="+filepath.Join(resources, "config", "openssl.cnf"),
		"OPENSSL_MODULES="+filepath.Join(resources, "ossl-modules"),
	)
}

func readStunnelPID(p state.Paths) (uint32, error) {
	data, err := os.ReadFile(StunnelPIDFile(p))
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("stunnel process id is invalid")
	}
	return uint32(value), nil
}

func stunnelProcessRunning(pid uint32, executable string) bool {
	actual := stunnelProcessPath(pid)
	return actual != "" && strings.EqualFold(actual, filepath.Clean(executable))
}

func stunnelProcessPath(pid uint32) string {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(handle)
	path := make([]uint16, 32768)
	pathLength := uint32(len(path))
	queryImage := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
	result, _, _ := queryImage.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&pathLength)))
	if result == 0 {
		return ""
	}
	return filepath.Clean(syscall.UTF16ToString(path[:pathLength]))
}
