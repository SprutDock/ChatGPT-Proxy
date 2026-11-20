package state

import (
	"bufio"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestEnsureLockSerializesProcesses(t *testing.T) {
	state := t.TempDir()
	process := exec.Command(os.Args[0], "-test.run=^TestEnsureLockHelperProcess$")
	process.Env = append(os.Environ(), "SPRU_ENSURE_LOCK_STATE="+state)
	output, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("lock helper output = %q, error = %v", line, err)
	}
	locked := make(chan error, 1)
	go func() {
		unlock, err := LockEnsure(Paths{State: state})
		if err == nil {
			unlock()
		}
		locked <- err
	}()
	select {
	case err := <-locked:
		t.Fatalf("second process acquired the lock early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := input.Write([]byte("release\n")); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-locked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second process did not acquire the released lock")
	}
}

func TestEnsureLockHelperProcess(t *testing.T) {
	state := os.Getenv("SPRU_ENSURE_LOCK_STATE")
	if state == "" {
		return
	}
	unlock, err := LockEnsure(Paths{State: state})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = os.Stdout.WriteString("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	unlock()
}
