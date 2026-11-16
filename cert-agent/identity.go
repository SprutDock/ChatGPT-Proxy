package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func createCSR(directory string) (string, string, error) {
	if err := ensurePrivateDir(directory); err != nil {
		return "", "", err
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return "", "", err
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	keyPath := filepath.Join(directory, "client.key")
	if err := writeAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		return "", "", err
	}
	requestID, err := randomHex(16)
	if err != nil {
		return "", "", err
	}
	request := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "device-request-" + requestID}}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, request, key)
	if err != nil {
		return "", "", err
	}
	csrPath := filepath.Join(directory, "client.csr")
	if err := writeAtomic(csrPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}), 0o600); err != nil {
		return "", "", err
	}
	return csrPath, keyPath, nil
}

func readCSR(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	request, _ := pem.Decode(data)
	if request == nil || request.Type != "CERTIFICATE REQUEST" {
		return "", fmt.Errorf("stored CSR is invalid")
	}
	if _, err := x509.ParseCertificateRequest(request.Bytes); err != nil {
		return "", fmt.Errorf("stored CSR is invalid")
	}
	return string(data), nil
}

func parsePrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("private key is invalid")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key is invalid")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key type is unsupported")
	}
	return key, nil
}

func saveIdentity(p Paths, chain, keyPath string) (string, error) {
	certificates, err := parseCertificateChain(chain)
	if err != nil {
		return "", err
	}
	leaf := certificates[0]
	key, err := parsePrivateKey(keyPath)
	if err != nil {
		return "", err
	}
	if leaf.NotBefore.After(time.Now().Add(5*time.Minute)) || !leaf.NotAfter.After(time.Now().Add(24*time.Hour)) || leaf.NotAfter.After(time.Now().Add(31*24*time.Hour)) {
		return "", fmt.Errorf("issued certificate lifetime is outside policy")
	}
	if leaf.IsCA || !containsUsage(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return "", fmt.Errorf("issued certificate is not a client certificate")
	}
	if err := verifyClientChain(p, certificates); err != nil {
		return "", err
	}
	certificateKey, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok || !bytes.Equal(certificateKey.N.Bytes(), key.PublicKey.N.Bytes()) || certificateKey.E != key.PublicKey.E {
		return "", fmt.Errorf("issued certificate does not match the local private key")
	}
	serial := normalizeSerial(leaf.SerialNumber)
	if !validSerial(serial) {
		return "", fmt.Errorf("issued certificate serial is invalid")
	}
	releasePath := filepath.Join(p.Releases, serial)
	if err := ensurePrivateDir(p.Releases); err != nil {
		return "", err
	}
	if err := os.Mkdir(releasePath, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(releasePath, "client.pem"), []byte(chain), 0o600); err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(releasePath, "client.key"), keyBytes, 0o600); err != nil {
		return "", err
	}
	return serial, nil
}

func parseCertificateChain(chain string) ([]*x509.Certificate, error) {
	remaining := []byte(chain)
	certificates := make([]*x509.Certificate, 0, 3)
	for len(bytes.TrimSpace(remaining)) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("certificate chain is invalid")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate chain is invalid")
		}
		certificates = append(certificates, certificate)
		remaining = rest
	}
	if len(certificates) < 2 {
		return nil, fmt.Errorf("certificate chain is incomplete")
	}
	return certificates, nil
}

func verifyClientChain(p Paths, certificates []*x509.Certificate) error {
	trust, err := os.ReadFile(filepath.Join(p.Resources, "client-ca-chain.pem"))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust) {
		return fmt.Errorf("bundled client CA chain is invalid")
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		intermediates.AddCert(certificate)
	}
	_, err = certificates[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   time.Now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return fmt.Errorf("issued certificate chain is not trusted")
	}
	return nil
}

func containsUsage(usages []x509.ExtKeyUsage, wanted x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == wanted || usage == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

func normalizeSerial(value *big.Int) string {
	serial := strings.ToLower(value.Text(16))
	serial = strings.TrimLeft(serial, "0")
	if serial == "" {
		return "0"
	}
	return serial
}

func validSerial(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func loadActive(p Paths) (*tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(filepath.Join(p.ActiveLink, "client.pem"), filepath.Join(p.ActiveLink, "client.key"))
	if err != nil {
		return nil, err
	}
	return &certificate, nil
}

func setActive(p Paths, serial string) (string, error) {
	releasePath := filepath.Join(p.Releases, serial)
	if _, err := os.Stat(filepath.Join(releasePath, "client.pem")); err != nil {
		return "", err
	}
	if err := ensurePrivateDir(filepath.Dir(p.ActiveLink)); err != nil {
		return "", err
	}
	previous, _ := os.Readlink(p.ActiveLink)
	relativePath, err := filepath.Rel(filepath.Dir(p.ActiveLink), releasePath)
	if err != nil {
		return previous, err
	}
	temporary := p.ActiveLink + ".new"
	_ = os.Remove(temporary)
	if err := os.Symlink(relativePath, temporary); err != nil {
		return previous, err
	}
	if err := os.Rename(temporary, p.ActiveLink); err != nil {
		_ = os.Remove(temporary)
		return previous, err
	}
	return previous, nil
}

func restoreActive(p Paths, previous string) error {
	if previous == "" {
		if err := os.Remove(p.ActiveLink); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	temporary := p.ActiveLink + ".restore"
	_ = os.Remove(temporary)
	if err := os.Symlink(previous, temporary); err != nil {
		return err
	}
	return os.Rename(temporary, p.ActiveLink)
}
