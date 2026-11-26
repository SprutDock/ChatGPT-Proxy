package codex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureFilePreservesOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := "\ufeff# пользовательские настройки\r\n" +
		"CUSTOM='first line\r\nHTTP_PROXY=part of custom value\r\nlast line'\r\n" +
		"export http_proxy=http://old.example # previous proxy\r\n" +
		"HTTPS_PROXY=\"https://old.example\"\r\n" +
		"no_proxy=\"localhost, .internal.example\""
	expected := "\ufeff# пользовательские настройки\r\n" +
		"CUSTOM='first line\r\nHTTP_PROXY=part of custom value\r\nlast line'\r\n" +
		"# previous proxy\r\n" +
		"HTTP_PROXY=http://127.0.0.1:3128\r\nHTTPS_PROXY=http://127.0.0.1:3128\r\n" +
		"NO_PROXY=localhost,127.0.0.1,::1,.internal.example\r\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("unexpected .env contents: %q", content)
	}
	configured, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(configured, current) || !configured.ModTime().Equal(current.ModTime()) {
		t.Fatal("already configured .env was rewritten")
	}
}

func TestConfigureFileCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", ".env")
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "HTTP_PROXY=http://127.0.0.1:3128\nHTTPS_PROXY=http://127.0.0.1:3128\nNO_PROXY=localhost,127.0.0.1,::1\n"
	if string(content) != expected {
		t.Fatalf("unexpected .env contents: %q", content)
	}
}

func TestConfigureFileLeavesUnsupportedFileIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := "CUSTOM=\"unfinished\nHTTPS_PROXY=http://old.example"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := configureFile(path); !errors.Is(err, errUnsupported) {
		t.Fatalf("expected unsupported environment, got %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("unexpected .env contents: %q", content)
	}
}
