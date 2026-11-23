package tunnel

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sprutdock/cert-agent/internal/platform"
	"sprutdock/cert-agent/internal/state"
)

const (
	proxyHost    = "sprutdock.com"
	proxyAddress = "127.0.0.1:3128"
)

func WriteStunnelConfig(p state.Paths) (bool, error) {
	stunnelResources, stunnelState, err := platform.StunnelDirectories(p)
	if err != nil {
		return false, err
	}
	activeLink := filepath.Join(stunnelState, "identity", "active")
	certificatePath, err := platform.StunnelStorePath(stunnelResources, filepath.Join(activeLink, "client.pem"))
	if err != nil {
		return false, err
	}
	keyPath, err := platform.StunnelStorePath(stunnelResources, filepath.Join(activeLink, "client.key"))
	if err != nil {
		return false, err
	}
	caPath, err := platform.StunnelStorePath(stunnelResources, filepath.Join(stunnelResources, "server-ca.pem"))
	if err != nil {
		return false, err
	}
	stunnelLog := filepath.Join(stunnelState, "stunnel.log")
	for _, path := range []string{stunnelLog, certificatePath, keyPath, caPath} {
		if strings.ContainsAny(path, "\r\n") {
			return false, fmt.Errorf("%w: stunnel paths cannot contain line breaks", platform.ErrStunnelPathUnavailable)
		}
	}
	config := []string{
		"debug = 4",
		"output = " + platform.StunnelPath(stunnelLog),
		"[sprutdock-proxy]",
		"client = yes",
		"delay = yes",
		"sessionResume = no",
		"accept = " + proxyAddress,
		"connect = " + proxyHost + ":3128",
		"cert = " + certificatePath,
		"key = " + keyPath,
		"CAfile = " + caPath,
		"verifyChain = yes",
		"checkHost = " + proxyHost,
		"sslVersionMin = TLSv1.2",
		"TIMEOUTconnect = 10",
	}
	if foreground := platform.StunnelForegroundOption(); foreground != "" {
		config = append([]string{foreground}, config...)
	}
	content := []byte(strings.Join(config, "\n") + "\n")
	current, err := os.ReadFile(p.StunnelConf)
	if err == nil && bytes.Equal(current, content) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := state.WriteAtomic(p.StunnelConf, content, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func WaitForProxy(ctx context.Context, p state.Paths) error {
	var lastError error
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := platform.VerifyProxyOwner(p); err == nil {
			if err := probeProxy(ctx); err == nil {
				return nil
			} else {
				lastError = err
			}
		} else {
			if errors.Is(err, platform.ErrProxyOwnerConflict) {
				return err
			}
			lastError = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastError == nil {
		lastError = fmt.Errorf("local proxy did not become ready")
	}
	return lastError
}

func probeProxy(ctx context.Context) error {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(8 * time.Second))
	if _, err := fmt.Fprintf(connection, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: "CONNECT"})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy tunnel returned HTTP %d", response.StatusCode)
	}
	tlsConnection := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "example.com"})
	return tlsConnection.HandshakeContext(ctx)
}
