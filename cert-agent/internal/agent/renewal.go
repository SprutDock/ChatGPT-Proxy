package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sprutdock/cert-agent/internal/api"
	"sprutdock/cert-agent/internal/identity"
	"sprutdock/cert-agent/internal/platform"
	"sprutdock/cert-agent/internal/state"
	"sprutdock/cert-agent/internal/tunnel"
)

func renewIfDue(p state.Paths, currentState *state.State, pending *state.PendingState, events *EventStream) (bool, error) {
	current, err := identity.LoadActive(p)
	if err != nil {
		return false, err
	}
	leaf := current.Leaf
	if leaf == nil && len(current.Certificate) > 0 {
		leaf, err = identity.ParseLeaf(current.Certificate[0])
	}
	if err != nil {
		return false, err
	}
	if pending.Renewal == nil && time.Until(leaf.NotAfter) > 10*24*time.Hour {
		return false, nil
	}
	if pending.Renewal == nil {
		requestID, err := newUUID()
		if err != nil {
			return false, err
		}
		requestDir := filepath.Join(p.State, "identity", "pending", requestID)
		csrPath, keyPath, err := identity.CreateCSR(requestDir)
		if err != nil {
			return false, err
		}
		pending.Renewal = &state.PendingRenewal{RequestID: requestID, CSRPath: csrPath, PrivatePath: keyPath, Previous: currentState.ActiveSerial}
		if err := state.SavePending(p, *pending); err != nil {
			return false, err
		}
	}
	request := pending.Renewal
	if request.IssuedSerial != "" {
		if err := activateRenewal(p, currentState, pending, current, events); err != nil {
			return false, err
		}
		return true, nil
	}
	csr, err := identity.ReadCSR(request.CSRPath)
	if err != nil {
		return false, err
	}
	if err := events.Emit("renewing"); err != nil {
		return false, err
	}
	result, err := api.NewClient().Renew(request.RequestID, csr, current)
	if err != nil {
		return false, err
	}
	serial, err := identity.SaveIdentity(p, result.CertificateChain, request.PrivatePath)
	if err != nil {
		return false, err
	}
	request.IssuedSerial = serial
	if err := state.SavePending(p, *pending); err != nil {
		return false, err
	}
	if err := activateRenewal(p, currentState, pending, current, events); err != nil {
		return false, err
	}
	return true, nil
}

func activateRenewal(p state.Paths, currentState *state.State, pending *state.PendingState, current *tls.Certificate, events *EventStream) error {
	request := pending.Renewal
	currentSerial, err := identity.CertificateSerial(current)
	if err != nil {
		return err
	}
	rollbackTarget := ""
	if request.Previous != "" {
		rollbackTarget = filepath.Join("releases", request.Previous)
	}
	if currentSerial != request.IssuedSerial {
		rollbackTarget, err = identity.SetActive(p, request.IssuedSerial)
		if err != nil {
			return err
		}
	}
	if err := events.Emit("certificate_saved"); err != nil {
		_ = identity.RestoreActive(p, rollbackTarget)
		return err
	}
	if err := events.Emit("tunnel_starting"); err != nil {
		_ = identity.RestoreActive(p, rollbackTarget)
		return err
	}
	if err := platform.RestartStunnel(p); err != nil {
		_ = identity.RestoreActive(p, rollbackTarget)
		return err
	}
	if err := events.Emit("verifying"); err != nil {
		_ = identity.RestoreActive(p, rollbackTarget)
		_ = platform.RestartStunnel(p)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err = tunnel.WaitForProxy(ctx, p)
	cancel()
	if err != nil {
		_ = identity.RestoreActive(p, rollbackTarget)
		_ = platform.RestartStunnel(p)
		return fmt.Errorf("renewed certificate did not pass the mTLS and HTTPS check: %w", err)
	}
	newCertificate, err := identity.LoadActive(p)
	if err != nil {
		return err
	}
	currentState.ActiveSerial = request.IssuedSerial
	currentState.LastError = ""
	if err := state.SaveState(p, *currentState); err != nil {
		return err
	}
	if err := api.NewClient().ConfirmRotation(request.RequestID, newCertificate); err != nil {
		currentState.LastError = "certificate rotation confirmation is pending"
		_ = state.SaveState(p, *currentState)
		return nil
	}
	currentState.ActiveSerial = request.IssuedSerial
	currentState.LastError = ""
	if err := state.SaveState(p, *currentState); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(request.CSRPath)); err != nil {
		return err
	}
	pending.Renewal = nil
	if err := state.SavePending(p, *pending); err != nil {
		return err
	}
	return nil
}
