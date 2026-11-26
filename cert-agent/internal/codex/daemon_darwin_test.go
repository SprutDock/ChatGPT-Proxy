package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigureRestartsStaleDaemonOnce(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("managed Codex runs as the regular user")
	}
	home := t.TempDir()
	path := filepath.Join(home, ".env")
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}

	executable := filepath.Join(home, "packages/app-server-daemon/current/bin/codex")
	pid := filepath.Join(home, "app-server-daemon/daemon.pid")
	log := filepath.Join(home, "restart.log")
	for _, dir := range []string{filepath.Dir(executable), filepath.Dir(pid)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf("#!/bin/sh\n[ \"$1 $2\" = 'app-server daemon' ] || exit 1\ncase \"$3\" in\nversion) exit 0;;\nrestart) printf 'restart\\n' >> %q; /usr/bin/touch %q;;\n*) exit 1;;\nesac\n", log, pid)
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pid, []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	earlier := time.Now().Add(-time.Minute)
	if err := os.Chtimes(pid, earlier, earlier); err != nil {
		t.Fatal(err)
	}
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}
	if err := configureFile(path); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(log); err != nil || string(content) != "restart\n" {
		t.Fatalf("expected exactly one restart, got %q, %v", content, err)
	}
}
