package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sprutdock/cert-agent/internal/api"
	"sprutdock/cert-agent/internal/identity"
	"sprutdock/cert-agent/internal/platform"
	"sprutdock/cert-agent/internal/state"
	"sprutdock/cert-agent/internal/tunnel"
)

func beginEnrollment(p state.Paths, currentState *state.State, pending *state.PendingState, interactive bool, events *EventStream) error {
	if !interactive {
		return fmt.Errorf("first enrollment requires an invitation entered through the launcher")
	}
	if events == nil {
		return fmt.Errorf("first enrollment requires an invitation entered through the launcher")
	}
	invitation, err := events.RequestInvitation()
	if err != nil {
		return err
	}
	if !validInvitation(invitation) {
		return fmt.Errorf("invitation is invalid")
	}
	requestID, err := newUUID()
	if err != nil {
		return err
	}
	credential, err := state.RandomHex(32)
	if err != nil {
		return err
	}
	requestDir := filepath.Join(p.State, "identity", "pending", requestID)
	csrPath, keyPath, err := identity.CreateCSR(requestDir)
	if err != nil {
		return err
	}
	pending.Enrollment = &state.PendingEnrollment{
		RequestID:   requestID,
		Credential:  credential,
		Invitation:  invitation,
		CSRPath:     csrPath,
		PrivatePath: keyPath,
	}
	if err := state.SavePending(p, *pending); err != nil {
		return err
	}
	currentState.LastError = ""
	if err := state.SaveState(p, *currentState); err != nil {
		return err
	}
	if err := events.Emit("request_prepared"); err != nil {
		return err
	}
	return completeEnrollment(p, currentState, pending, events)
}

func completeEnrollment(p state.Paths, currentState *state.State, pending *state.PendingState, events *EventStream) error {
	request := pending.Enrollment
	if request == nil {
		return nil
	}
	csr, err := identity.ReadCSR(request.CSRPath)
	if err != nil {
		return err
	}
	if err := events.Emit("enrolling"); err != nil {
		return err
	}
	result, err := api.NewClient().Enroll(request.Invitation, request.RequestID, request.Credential, csr)
	if err != nil {
		return err
	}
	if result.Status != "issued" {
		return fmt.Errorf("enrollment service did not issue a certificate")
	}
	serial, err := identity.SaveIdentity(p, result.CertificateChain, request.PrivatePath)
	if err != nil {
		return err
	}
	if err := events.Emit("certificate_saved"); err != nil {
		return err
	}
	previous, err := identity.SetActive(p, serial)
	if err != nil {
		return err
	}
	if err := events.Emit("tunnel_starting"); err != nil {
		_ = identity.RestoreActive(p, previous)
		return err
	}
	if err := platform.RestartStunnel(p); err != nil {
		_ = identity.RestoreActive(p, previous)
		return err
	}
	if err := events.Emit("verifying"); err != nil {
		_ = identity.RestoreActive(p, previous)
		_ = platform.RestartStunnel(p)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err = tunnel.WaitForProxy(ctx, p)
	cancel()
	if err != nil {
		_ = identity.RestoreActive(p, previous)
		_ = platform.RestartStunnel(p)
		return fmt.Errorf("issued certificate did not pass the mTLS and HTTPS check: %w", err)
	}
	currentState.DeviceID = result.DeviceID
	currentState.ActiveSerial = serial
	currentState.LastError = ""
	if err := state.SaveState(p, *currentState); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(request.CSRPath)); err != nil {
		return err
	}
	pending.Enrollment = nil
	if err := state.SavePending(p, *pending); err != nil {
		return err
	}
	return nil
}

func newUUID() (string, error) {
	value, err := state.RandomHex(16)
	if err != nil {
		return "", err
	}
	value = value[:12] + "4" + value[13:16] + value[16:]
	variant := strings.IndexByte("0123456789abcdef", value[16])
	value = value[:16] + "89ab"[variant&3:variant&3+1] + value[17:]
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}
