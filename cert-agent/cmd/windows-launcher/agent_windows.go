package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type agentEvent struct {
	Version int    `json:"v"`
	Event   string `json:"event"`
}

type agentRunError struct {
	stage string
}

func (err agentRunError) Error() string {
	return "certificate agent did not complete its setup"
}

func runCertificateAgent(root string, ui *setupUI) error {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return agentRunError{}
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return agentRunError{}
	}
	agent := supportFile(root, "cert-agent.exe")
	command := exec.Command(agent, "ensure", "--events")
	command.Dir = filepath.Dir(agent)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter
	input, err := command.StdinPipe()
	if err != nil {
		closeFiles(stdoutReader, stdoutWriter, stderrReader, stderrWriter)
		return agentRunError{}
	}
	if err := command.Start(); err != nil {
		_ = input.Close()
		closeFiles(stdoutReader, stdoutWriter, stderrReader, stderrWriter)
		return agentRunError{}
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	events := make(chan agentEvent, 16)
	go scanAgentEvents(stdoutReader, events)
	go func() {
		_, _ = io.Copy(io.Discard, stderrReader)
		_ = stderrReader.Close()
	}()
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	var eventStream <-chan agentEvent = events
	var waitErr error
	waitDone := false
	ready := false
	cancelled := false
	stage := "checking"
	for eventStream != nil || !waitDone {
		select {
		case event, ok := <-eventStream:
			if !ok {
				eventStream = nil
				continue
			}
			if event.Version != 1 || !knownAgentEvent(event.Event) {
				continue
			}
			stage = event.Event
			ui.update(event.Event)
			launcherLog("certificate event=" + event.Event)
			if event.Event == "ready" {
				ready = true
			}
			if event.Event == "invitation_required" && !cancelled {
				cancelled = waitForInvitation(input, command, ui)
			}
		case waitErr = <-wait:
			waitDone = true
		case action := <-ui.actions:
			if action.kind == "cancel" || action.kind == "close" || action.kind == "error" {
				cancelled = true
				_ = command.Process.Kill()
			}
		}
	}
	_ = input.Close()
	_ = stdoutReader.Close()
	if cancelled {
		return errOperationCancelled
	}
	if waitErr != nil || !ready {
		return agentRunError{stage: stage}
	}
	return nil
}

func waitForInvitation(input io.Writer, command *exec.Cmd, ui *setupUI) bool {
	for {
		action := <-ui.actions
		if action.kind == "invitation" {
			_, err := io.WriteString(input, action.value+"\n")
			action.value = ""
			if err != nil {
				_ = command.Process.Kill()
				return true
			}
			return false
		}
		if action.kind == "cancel" || action.kind == "close" || action.kind == "error" {
			_ = command.Process.Kill()
			return true
		}
	}
}

func scanAgentEvents(reader io.Reader, events chan<- agentEvent) {
	defer close(events)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 4096)
	for scanner.Scan() {
		var event agentEvent
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events <- event
		}
	}
}

func knownAgentEvent(event string) bool {
	switch event {
	case "checking", "setup_started", "invitation_required", "request_prepared", "enrolling", "renewing", "certificate_saved", "tunnel_starting", "verifying", "proxy_conflict", "ready":
		return true
	default:
		return false
	}
}

func closeFiles(files ...*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}
