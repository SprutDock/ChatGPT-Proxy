package main

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
)

func writeIfChanged(path string, value []byte, mode os.FileMode) (bool, error) {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, value) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := writeAtomic(path, value, mode); err != nil {
		return false, err
	}
	return true, nil
}

func writeStunnelConfig(p Paths) (bool, error) {
	certificatePath, err := stunnelStorePath(p.Resources, filepath.Join(p.ActiveLink, "client.pem"))
	if err != nil {
		return false, err
	}
	keyPath, err := stunnelStorePath(p.Resources, filepath.Join(p.ActiveLink, "client.key"))
	if err != nil {
		return false, err
	}
	caPath, err := stunnelStorePath(p.Resources, filepath.Join(p.Resources, "server-ca.pem"))
	if err != nil {
		return false, err
	}
	config := []string{
		"debug = 4",
		"output = " + stunnelPath(p.StunnelLog),
		"[sprutdock-proxy]",
		"client = yes",
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
	if foreground := stunnelForegroundOption(); foreground != "" {
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
	if err := writeAtomic(p.StunnelConf, content, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func waitForProxy(ctx context.Context) error {
	var lastError error
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := probeProxy(ctx); err == nil {
			return nil
		} else {
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

func available(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
