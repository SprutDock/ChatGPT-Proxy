package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func setActive(p Paths, serial string) (string, error) {
	previous, _ := activeSerial(p)
	if err := activateRelease(p, serial); err != nil {
		return previous, err
	}
	return previous, nil
}

func restoreActive(p Paths, previous string) error {
	if previous == "" {
		return os.RemoveAll(p.ActiveLink)
	}
	return activateRelease(p, previous)
}

func activateRelease(p Paths, serial string) error {
	release := filepath.Join(p.Releases, serial)
	if _, err := os.Stat(filepath.Join(release, "client.pem")); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Dir(p.ActiveLink)); err != nil {
		return err
	}
	temporary := p.ActiveLink + ".new"
	backup := p.ActiveLink + ".old"
	_ = os.RemoveAll(temporary)
	_ = os.RemoveAll(backup)
	if err := ensurePrivateDir(temporary); err != nil {
		return err
	}
	for _, name := range []string{"client.pem", "client.key"} {
		source, err := os.Open(filepath.Join(release, name))
		if err != nil {
			_ = os.RemoveAll(temporary)
			return err
		}
		destination, err := os.OpenFile(filepath.Join(temporary, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = io.Copy(destination, source)
		}
		if closeError := source.Close(); err == nil {
			err = closeError
		}
		if destination != nil {
			if closeError := destination.Close(); err == nil {
				err = closeError
			}
		}
		if err != nil {
			_ = os.RemoveAll(temporary)
			return err
		}
	}
	if _, err := os.Stat(p.ActiveLink); err == nil {
		if err := os.Rename(p.ActiveLink, backup); err != nil {
			_ = os.RemoveAll(temporary)
			return err
		}
	}
	if err := os.Rename(temporary, p.ActiveLink); err != nil {
		_ = os.Rename(backup, p.ActiveLink)
		_ = os.RemoveAll(temporary)
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func activeSerial(p Paths) (string, error) {
	data, err := os.ReadFile(filepath.Join(p.ActiveLink, "client.pem"))
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("active certificate is invalid")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	return normalizeSerial(certificate.SerialNumber), nil
}
