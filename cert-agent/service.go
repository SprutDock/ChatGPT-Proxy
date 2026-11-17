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
	"strconv"
	"strings"
	"time"
)

func installLaunchAgents(p Paths, interactive bool) error {
	if err := ensurePrivateDir(filepath.Dir(p.StunnelAgent)); err != nil {
		return err
	}
	stunnel := filepath.Join(p.Resources, "stunnel")
	if _, err := os.Stat(stunnel); err != nil {
		return fmt.Errorf("bundled stunnel is unavailable")
	}
	uid, err := currentUID()
	if err != nil {
		return err
	}
	domain := "gui/" + uid
	stunnelPlist := launchAgentPlist(stunnelLabel, []string{stunnel, p.StunnelConf}, p.StunnelLog, true, 0)
	if err := writeAtomic(p.StunnelAgent, []byte(stunnelPlist), 0o600); err != nil {
		return err
	}
	agent := filepath.Join(p.Resources, "cert-agent")
	schedulerPlist := launchAgentPlist(agentLabel, []string{agent, "ensure", "--background"}, p.Log, false, 12*60*60)
	if !interactive && available(p.SchedulerAgent) {
		return ensureJobLoaded(domain, agentLabel, p.SchedulerAgent)
	}
	changed, err := writeIfChanged(p.SchedulerAgent, []byte(schedulerPlist), 0o600)
	if err != nil {
		return err
	}
	if changed {
		return reloadJob(domain, agentLabel, p.SchedulerAgent)
	}
	return ensureJobLoaded(domain, agentLabel, p.SchedulerAgent)
}

func launchAgentPlist(label string, arguments []string, logPath string, keepAlive bool, interval int) string {
	var values strings.Builder
	for _, argument := range arguments {
		values.WriteString("<string>" + plistEscape(argument) + "</string>")
	}
	var schedule strings.Builder
	if keepAlive {
		schedule.WriteString("<key>KeepAlive</key><true/><key>RunAtLoad</key><true/>")
	}
	if interval > 0 {
		schedule.WriteString("<key>StartInterval</key><integer>" + strconv.Itoa(interval) + "</integer>")
	}
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\"><plist version=\"1.0\"><dict><key>Label</key><string>" + plistEscape(label) + "</string><key>ProgramArguments</key><array>" + values.String() + "</array><key>StandardOutPath</key><string>" + plistEscape(logPath) + "</string><key>StandardErrorPath</key><string>" + plistEscape(logPath) + "</string>" + schedule.String() + "</dict></plist>\n"
}

func ensureJobLoaded(domain, label, plistPath string) error {
	if command("launchctl", "print", domain+"/"+label).Run() == nil {
		return nil
	}
	if err := command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not load the certificate agent")
	}
	return nil
}

func reloadJob(domain, label, plistPath string) error {
	target := domain + "/" + label
	if command("launchctl", "print", target).Run() == nil {
		if err := command("launchctl", "bootout", target).Run(); err != nil {
			return fmt.Errorf("could not reload the certificate agent")
		}
	}
	if err := command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("could not reload the certificate agent")
	}
	return nil
}

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

func ensureStunnelRunning(p Paths) error {
	uid, err := currentUID()
	if err != nil {
		return err
	}
	domain := "gui/" + uid
	if err := ensureJobLoaded(domain, stunnelLabel, p.StunnelAgent); err != nil {
		return err
	}
	return nil
}

func restartStunnel(p Paths) error {
	if err := ensureStunnelRunning(p); err != nil {
		return err
	}
	uid, err := currentUID()
	if err != nil {
		return err
	}
	domain := "gui/" + uid
	if err := command("launchctl", "kickstart", "-k", domain+"/"+stunnelLabel).Run(); err != nil {
		return fmt.Errorf("could not start the local proxy tunnel")
	}
	return nil
}

func writeStunnelConfig(p Paths) (bool, error) {
	config := []string{
		"foreground = yes",
		"debug = 4",
		"output = " + p.StunnelLog,
		"[sprutdock-proxy]",
		"client = yes",
		"sessionResume = no",
		"accept = " + proxyAddress,
		"connect = " + proxyHost + ":3128",
		"cert = " + filepath.Join(p.ActiveLink, "client.pem"),
		"key = " + filepath.Join(p.ActiveLink, "client.key"),
		"CAfile = " + filepath.Join(p.Resources, "server-ca.pem"),
		"verifyChain = yes",
		"checkHost = " + proxyHost,
		"sslVersionMin = TLSv1.2",
		"TIMEOUTconnect = 10",
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
