package agent

import (
	"errors"
	"testing"

	"sprutdock/cert-agent/internal/platform"
)

func TestVerifyProxyWithRestartRetriesOnce(t *testing.T) {
	attempts := 0
	restarts := 0
	err := verifyProxyWithRestart(func() error {
		attempts++
		if attempts == 1 {
			return errors.New("stale address")
		}
		return nil
	}, func() error {
		restarts++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || restarts != 1 {
		t.Fatalf("got %d checks and %d restarts, want 2 checks and 1 restart", attempts, restarts)
	}
}

func TestVerifyProxyWithRestartSkipsRestartWhenHealthy(t *testing.T) {
	restarts := 0
	err := verifyProxyWithRestart(func() error { return nil }, func() error {
		restarts++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 0 {
		t.Fatalf("got %d restarts, want none", restarts)
	}
}

func TestVerifyProxyWithRestartSkipsPortOwnerConflict(t *testing.T) {
	restarts := 0
	err := verifyProxyWithRestart(func() error { return platform.ErrProxyOwnerConflict }, func() error {
		restarts++
		return nil
	})
	if !errors.Is(err, platform.ErrProxyOwnerConflict) {
		t.Fatalf("got %v, want proxy owner conflict", err)
	}
	if restarts != 0 {
		t.Fatalf("got %d restarts, want none", restarts)
	}
}
