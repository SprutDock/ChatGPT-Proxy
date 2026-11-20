package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sprutdock/cert-agent/internal/identity"
	"sprutdock/cert-agent/internal/platform"
	"sprutdock/cert-agent/internal/state"
	"sprutdock/cert-agent/internal/tunnel"
)

func Ensure(p state.Paths, interactive bool) error {
	return EnsureWithEvents(p, interactive, nil)
}

func EnsureWithEvents(p state.Paths, interactive bool, events *EventStream) error {
	if err := events.Emit("checking"); err != nil {
		return err
	}
	if err := state.EnsurePrivateDir(p.State); err != nil {
		return err
	}
	unlock, err := state.LockEnsure(p)
	if err != nil {
		return err
	}
	defer unlock()
	currentState, err := state.LoadState(p)
	if err != nil {
		return err
	}
	if err := platform.InstallBackgroundCheck(p, interactive); err != nil {
		return err
	}
	configChanged, err := tunnel.WriteStunnelConfig(p)
	if err != nil {
		return err
	}
	pending, err := state.LoadPending(p)
	if err != nil {
		return err
	}
	proxyChecked := false
	if pending.Enrollment != nil {
		if err := events.Emit("setup_started"); err != nil {
			return err
		}
		err = completeEnrollment(p, &currentState, &pending, events)
		proxyChecked = err == nil
	} else if !identity.ActiveExists(p) {
		if err := events.Emit("setup_started"); err != nil {
			return err
		}
		err = beginEnrollment(p, &currentState, &pending, interactive, events)
		proxyChecked = err == nil
	} else {
		proxyChecked, err = renewIfDue(p, &currentState, &pending, events)
	}
	if err != nil {
		if eventErr := emitProxyConflict(events, err); eventErr != nil {
			return eventErr
		}
		currentState.LastError = safeError(err)
		_ = state.SaveState(p, currentState)
		return err
	}
	if !identity.ActiveExists(p) {
		return fmt.Errorf("device enrollment has not completed")
	}
	if proxyChecked {
		err = platform.EnsureStunnelRunning(p)
	} else if configChanged {
		if err := events.Emit("tunnel_starting"); err != nil {
			return err
		}
		err = platform.RestartStunnel(p)
	} else {
		if err := events.Emit("tunnel_starting"); err != nil {
			return err
		}
		err = platform.EnsureStunnelRunning(p)
	}
	if err != nil {
		if eventErr := emitProxyConflict(events, err); eventErr != nil {
			return eventErr
		}
		currentState.LastError = safeError(err)
		_ = state.SaveState(p, currentState)
		return err
	}
	if !proxyChecked {
		if err := events.Emit("verifying"); err != nil {
			return err
		}
		verify := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return tunnel.WaitForProxy(ctx, p)
		}
		err = verifyProxyWithRestart(verify, func() error {
			if err := events.Emit("tunnel_starting"); err != nil {
				return err
			}
			if err := platform.RestartStunnel(p); err != nil {
				return err
			}
			return events.Emit("verifying")
		})
		if err != nil {
			if eventErr := emitProxyConflict(events, err); eventErr != nil {
				return eventErr
			}
			currentState.LastError = safeError(err)
			_ = state.SaveState(p, currentState)
			return fmt.Errorf("local proxy tunnel did not pass its mTLS and HTTPS check")
		}
	}
	if pending.Renewal == nil {
		currentState.LastError = ""
	}
	if err := state.SaveState(p, currentState); err != nil {
		return err
	}
	return events.Emit("ready")
}

func verifyProxyWithRestart(verify, restart func() error) error {
	if err := verify(); err == nil {
		return nil
	} else if errors.Is(err, platform.ErrProxyOwnerConflict) {
		return err
	}
	if err := restart(); err != nil {
		return err
	}
	return verify()
}

func emitProxyConflict(events *EventStream, err error) error {
	if errors.Is(err, platform.ErrProxyOwnerConflict) {
		return events.Emit("proxy_conflict")
	}
	return nil
}

func safeError(err error) string {
	return strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, err.Error())
}
