package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sprutdock/cert-agent/internal/state"
)

func TestActiveCertificateRollback(t *testing.T) {
	root := t.TempDir()
	p := state.Paths{
		Releases:   filepath.Join(root, "identity", "releases"),
		ActiveLink: filepath.Join(root, "identity", "active"),
	}
	for _, serial := range []int64{1, 2} {
		directory := filepath.Join(p.Releases, NormalizeSerial(big.NewInt(serial)))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "client.pem"), testCertificate(t, serial), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "client.key"), []byte("test key"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SetActive(p, "1"); err != nil {
		t.Fatal(err)
	}
	previous, err := SetActive(p, "2")
	if err != nil {
		t.Fatal(err)
	}
	if previous == "" {
		t.Fatal("previous active certificate was not recorded")
	}
	if err := RestoreActive(p, previous); err != nil {
		t.Fatal(err)
	}
	activeFile, err := os.ReadFile(filepath.Join(p.ActiveLink, "client.pem"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(activeFile)
	if block == nil {
		t.Fatal("active certificate is missing PEM data")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	active := NormalizeSerial(certificate.SerialNumber)
	if active != "1" {
		t.Fatalf("active certificate serial after rollback = %q, want 1", active)
	}
}

func testCertificate(t *testing.T, serial int64) []byte {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
