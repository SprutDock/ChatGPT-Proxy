package codex

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"sprutdock/cert-agent/internal/state"
	"sprutdock/cert-agent/internal/tunnel"
)

const maxEnvironmentSize = 1 << 20

var (
	errUnsupported = errors.New("unsupported Codex .env format; configure the proxy manually")
	assignment     = regexp.MustCompile(`(?m)^[\t ]*(?:export[\t ]+)?([a-zA-Z_][a-zA-Z0-9_.]*)[\t ]*=[\t ]*(?:'([^']*)'|"((?:[^"\\]|\\[\s\S])*)"|((?:[^\t \r\n'"\\#]|\\[^\r\n])*))[\t ]*(#[^\r\n]*)?\r?(?:\n|$)`)
	noProxyEntry   = regexp.MustCompile(`^[a-zA-Z0-9_.:/%*\[\]-]+$`)
)

func Configure() error {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".codex")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("CODEX_HOME must be an absolute path")
	}
	return configureFile(filepath.Join(dir, ".env"))
}

func configureFile(path string) error {
	info, err := os.Lstat(path)
	var content []byte
	mode := os.FileMode(0o600)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case !info.Mode().IsRegular() || info.Size() > maxEnvironmentSize:
		return errUnsupported
	default:
		content, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
	}
	updated, err := proxyEnvironment(content)
	if err != nil {
		return err
	}
	changed := !bytes.Equal(content, updated)
	if changed && mode&0o200 == 0 {
		return os.ErrPermission
	}
	var configuredAt time.Time
	if !changed {
		configuredAt = info.ModTime()
	}
	daemon := staleDaemon(filepath.Dir(path), configuredAt)
	if changed {
		if err := writeEnvironment(path, updated, info, mode); err != nil {
			return err
		}
	}
	if daemon != "" {
		return runDaemon(daemon, "restart", 40*time.Second)
	}
	return nil
}

func writeEnvironment(path string, updated []byte, info os.FileInfo, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if info == nil && !errors.Is(err, os.ErrNotExist) ||
		info != nil && (err != nil || !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime())) {
		return errors.New("Codex .env changed during configuration")
	}
	return state.ReplaceAtomic(path, updated, mode)
}

func proxyEnvironment(content []byte) ([]byte, error) {
	if len(content) > maxEnvironmentSize || !utf8.Valid(content) || bytes.ContainsRune(content, 0) {
		return nil, errUnsupported
	}
	const bom = "\uFEFF"
	input := string(content)
	newline := "\n"
	if first := strings.IndexByte(input, '\n'); first > 0 && input[first-1] == '\r' {
		newline = "\r\n"
	}
	var output strings.Builder
	if strings.HasPrefix(input, bom) {
		output.WriteString(bom)
		input = strings.TrimPrefix(input, bom)
	}
	noProxy := []string{"localhost", "127.0.0.1", "::1"}
	for input != "" {
		line, rest, hasNewline := strings.Cut(input, "\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			output.WriteString(line)
			if hasNewline {
				output.WriteByte('\n')
			}
			input = rest
			continue
		}
		match := assignment.FindStringSubmatch(input)
		if match == nil || !strings.HasPrefix(input, match[0]) {
			return nil, errUnsupported
		}
		input = input[len(match[0]):]
		switch strings.ToUpper(match[1]) {
		case "NO_PROXY":
			for entry := range strings.SplitSeq(match[2]+match[3]+match[4], ",") {
				entry = strings.TrimSpace(entry)
				if entry == "" {
					continue
				}
				if !noProxyEntry.MatchString(entry) {
					return nil, errUnsupported
				}
				if !slices.Contains(noProxy, entry) {
					noProxy = append(noProxy, entry)
				}
			}
		case "HTTP_PROXY", "HTTPS_PROXY":
		default:
			output.WriteString(match[0])
			continue
		}
		if match[5] != "" {
			output.WriteString(match[5] + newline)
		}
	}
	if prefix := output.String(); prefix != "" && prefix != bom && !strings.HasSuffix(prefix, "\n") {
		output.WriteString(newline)
	}
	for _, line := range []string{
		"HTTP_PROXY=" + tunnel.ProxyURL,
		"HTTPS_PROXY=" + tunnel.ProxyURL,
		"NO_PROXY=" + strings.Join(noProxy, ","),
	} {
		output.WriteString(line + newline)
	}
	return []byte(output.String()), nil
}
