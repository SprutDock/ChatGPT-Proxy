package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	stunnelLabel = "com.sprutdock.proxy.stunnel"
	agentLabel   = "com.sprutdock.cert-agent.scheduled"
	apiBaseURL   = "https://enroll.sprutdock.com"
	proxyHost    = "sprutdock.com"
	proxyAddress = "127.0.0.1:3128"
)

type Paths struct {
	Resources      string
	State          string
	Releases       string
	ActiveLink     string
	StunnelConf    string
	Log            string
	StunnelLog     string
	StunnelAgent   string
	SchedulerAgent string
	StunnelPIDFile string
	StateFile      string
	PendingFile    string
}

type State struct {
	DeviceID     string `json:"device_id,omitempty"`
	ActiveSerial string `json:"active_serial,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type PendingEnrollment struct {
	RequestID   string `json:"request_id"`
	Credential  string `json:"credential"`
	Invitation  string `json:"invitation"`
	CSRPath     string `json:"csr_path"`
	PrivatePath string `json:"private_path"`
}

type PendingRenewal struct {
	RequestID    string `json:"request_id"`
	CSRPath      string `json:"csr_path"`
	PrivatePath  string `json:"private_path"`
	Previous     string `json:"previous_serial"`
	IssuedSerial string `json:"issued_serial,omitempty"`
}

type PendingState struct {
	Enrollment *PendingEnrollment `json:"enrollment,omitempty"`
	Renewal    *PendingRenewal    `json:"renewal,omitempty"`
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func writeAtomic(path string, value []byte, mode os.FileMode) error {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sprutdock-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replaceFile(temporaryPath, path)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func saveJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0o600)
}

func loadState(p Paths) (State, error) {
	var state State
	err := readJSON(p.StateFile, &state)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	return state, err
}

func saveState(p Paths, state State) error {
	return saveJSON(p.StateFile, state)
}

func loadPending(p Paths) (PendingState, error) {
	var pending PendingState
	err := readJSON(p.PendingFile, &pending)
	if errors.Is(err, os.ErrNotExist) {
		return PendingState{}, nil
	}
	return pending, err
}

func savePending(p Paths, state PendingState) error {
	return saveJSON(p.PendingFile, state)
}

func randomHex(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
