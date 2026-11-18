package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ensure(p Paths, interactive bool) error {
	if err := ensurePrivateDir(p.State); err != nil {
		return err
	}
	state, err := loadState(p)
	if err != nil {
		return err
	}
	if err := installLaunchAgents(p, interactive); err != nil {
		return err
	}
	configChanged, err := writeStunnelConfig(p)
	if err != nil {
		return err
	}
	pending, err := loadPending(p)
	if err != nil {
		return err
	}
	proxyChecked := false
	if pending.Enrollment != nil {
		err = completeEnrollment(p, &state, &pending)
		proxyChecked = err == nil
	} else if !available(filepath.Join(p.ActiveLink, "client.pem")) {
		err = beginEnrollment(p, &state, &pending, interactive)
		proxyChecked = err == nil
	} else {
		proxyChecked, err = renewIfDue(p, &state, &pending)
	}
	if err != nil {
		state.LastError = safeError(err)
		_ = saveState(p, state)
		return err
	}
	if !available(filepath.Join(p.ActiveLink, "client.pem")) {
		return fmt.Errorf("device enrollment has not completed")
	}
	if configChanged && !proxyChecked {
		err = restartStunnel(p)
	} else {
		err = ensureStunnelRunning(p)
	}
	if err != nil {
		state.LastError = safeError(err)
		_ = saveState(p, state)
		return err
	}
	if !proxyChecked {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err = waitForProxy(ctx)
		cancel()
		if err != nil {
			state.LastError = safeError(err)
			_ = saveState(p, state)
			notifyUser("Прокси SprutDock недоступен", "Проверьте подключение к сети; сертификат сохранён, агент повторит проверку.")
			return fmt.Errorf("local proxy tunnel did not pass its mTLS and HTTPS check")
		}
	}
	if pending.Renewal == nil {
		state.LastError = ""
	}
	return saveState(p, state)
}

func beginEnrollment(p Paths, state *State, pending *PendingState, interactive bool) error {
	if !interactive {
		return fmt.Errorf("first enrollment requires an invitation entered through the launcher")
	}
	invitation, err := promptInvitation()
	if err != nil {
		return err
	}
	requestID, err := newUUID()
	if err != nil {
		return err
	}
	credential, err := randomHex(32)
	if err != nil {
		return err
	}
	requestDir := filepath.Join(p.State, "identity", "pending", requestID)
	csrPath, keyPath, err := createCSR(requestDir)
	if err != nil {
		return err
	}
	pending.Enrollment = &PendingEnrollment{
		RequestID:   requestID,
		Credential:  credential,
		Invitation:  invitation,
		CSRPath:     csrPath,
		PrivatePath: keyPath,
	}
	if err := savePending(p, *pending); err != nil {
		return err
	}
	state.LastError = ""
	if err := saveState(p, *state); err != nil {
		return err
	}
	return completeEnrollment(p, state, pending)
}

func completeEnrollment(p Paths, state *State, pending *PendingState) error {
	request := pending.Enrollment
	if request == nil {
		return nil
	}
	csr, err := readCSR(request.CSRPath)
	if err != nil {
		return err
	}
	api := newAPIClient()
	result, err := api.enroll(request.Invitation, request.RequestID, request.Credential, csr)
	if err != nil {
		return err
	}
	if result.Status != "issued" {
		return fmt.Errorf("enrollment service did not issue a certificate")
	}
	serial, err := saveIdentity(p, result.CertificateChain, request.PrivatePath)
	if err != nil {
		return err
	}
	previous, err := setActive(p, serial)
	if err != nil {
		return err
	}
	if err := restartStunnel(p); err != nil {
		_ = restoreActive(p, previous)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err = waitForProxy(ctx)
	cancel()
	if err != nil {
		_ = restoreActive(p, previous)
		_ = restartStunnel(p)
		return fmt.Errorf("issued certificate did not pass the mTLS and HTTPS check: %w", err)
	}
	state.DeviceID = result.DeviceID
	state.ActiveSerial = serial
	state.LastError = ""
	if err := saveState(p, *state); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(request.CSRPath)); err != nil {
		return err
	}
	pending.Enrollment = nil
	if err := savePending(p, *pending); err != nil {
		return err
	}
	notifyUser("Прокси SprutDock готов", "Устройство зарегистрировано; ChatGPT можно запускать через launcher.")
	return nil
}

func renewIfDue(p Paths, state *State, pending *PendingState) (bool, error) {
	current, err := loadActive(p)
	if err != nil {
		return false, err
	}
	leaf := current.Leaf
	if leaf == nil && len(current.Certificate) > 0 {
		leaf, err = parseLeaf(current.Certificate[0])
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
		csrPath, keyPath, err := createCSR(requestDir)
		if err != nil {
			return false, err
		}
		pending.Renewal = &PendingRenewal{RequestID: requestID, CSRPath: csrPath, PrivatePath: keyPath, Previous: state.ActiveSerial}
		if err := savePending(p, *pending); err != nil {
			return false, err
		}
	}
	request := pending.Renewal
	if request.IssuedSerial != "" {
		if err := activateRenewal(p, state, pending, current); err != nil {
			return false, err
		}
		return true, nil
	}
	csr, err := readCSR(request.CSRPath)
	if err != nil {
		return false, err
	}
	result, err := newAPIClient().renew(request.RequestID, csr, current)
	if err != nil {
		return false, err
	}
	serial, err := saveIdentity(p, result.CertificateChain, request.PrivatePath)
	if err != nil {
		return false, err
	}
	request.IssuedSerial = serial
	if err := savePending(p, *pending); err != nil {
		return false, err
	}
	if err := activateRenewal(p, state, pending, current); err != nil {
		return false, err
	}
	return true, nil
}

func activateRenewal(p Paths, state *State, pending *PendingState, current *tls.Certificate) error {
	request := pending.Renewal
	currentSerial, err := certificateSerial(current)
	if err != nil {
		return err
	}
	rollbackTarget := ""
	if request.Previous != "" {
		rollbackTarget = filepath.Join("releases", request.Previous)
	}
	if currentSerial != request.IssuedSerial {
		rollbackTarget, err = setActive(p, request.IssuedSerial)
		if err != nil {
			return err
		}
	}
	if err := restartStunnel(p); err != nil {
		_ = restoreActive(p, rollbackTarget)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err = waitForProxy(ctx)
	cancel()
	if err != nil {
		_ = restoreActive(p, rollbackTarget)
		_ = restartStunnel(p)
		return fmt.Errorf("renewed certificate did not pass the mTLS and HTTPS check")
	}
	newCertificate, err := loadActive(p)
	if err != nil {
		return err
	}
	state.ActiveSerial = request.IssuedSerial
	state.LastError = ""
	if err := saveState(p, *state); err != nil {
		return err
	}
	if err := newAPIClient().confirmRotation(request.RequestID, newCertificate); err != nil {
		state.LastError = "certificate rotation confirmation is pending"
		_ = saveState(p, *state)
		notifyUser("Обновление сертификата ожидает подтверждения", "Туннель работает; агент повторит подтверждение при следующей проверке.")
		return nil
	}
	state.ActiveSerial = request.IssuedSerial
	state.LastError = ""
	if err := saveState(p, *state); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(request.CSRPath)); err != nil {
		return err
	}
	pending.Renewal = nil
	if err := savePending(p, *pending); err != nil {
		return err
	}
	return nil
}

func newUUID() (string, error) {
	value, err := randomHex(16)
	if err != nil {
		return "", err
	}
	value = value[:12] + "4" + value[13:16] + value[16:]
	variant := strings.IndexByte("0123456789abcdef", value[16])
	value = value[:16] + "89ab"[variant&3:variant&3+1] + value[17:]
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}

func parseLeaf(der []byte) (*x509.Certificate, error) {
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}

func certificateSerial(certificate *tls.Certificate) (string, error) {
	leaf := certificate.Leaf
	if leaf == nil && len(certificate.Certificate) > 0 {
		parsed, err := parseLeaf(certificate.Certificate[0])
		if err != nil {
			return "", err
		}
		leaf = parsed
	}
	if leaf == nil {
		return "", fmt.Errorf("active certificate is invalid")
	}
	return normalizeSerial(leaf.SerialNumber), nil
}

func safeError(err error) string {
	return strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, err.Error())
}
