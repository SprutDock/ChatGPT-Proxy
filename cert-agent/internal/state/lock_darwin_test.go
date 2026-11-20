package state

import (
	"testing"
	"time"
)

func TestEnsureLockSerializesCalls(t *testing.T) {
	p := Paths{State: t.TempDir()}
	unlockFirst, err := LockEnsure(p)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		unlockSecond, err := LockEnsure(p)
		if err == nil {
			unlockSecond()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		t.Fatalf("second lock was acquired before the first was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlockFirst()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second lock was not acquired after release")
	}
}
