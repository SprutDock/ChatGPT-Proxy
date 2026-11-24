package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var errElevatedDaemon = errors.New("run Codex with the regular user account")

func staleDaemon(home string, configuredAt time.Time) string {
	binary := "codex"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	for _, install := range []struct{ packageName, pidFile string }{
		{"app-server-daemon", "daemon.pid"},
		{"standalone", "app-server.pid"},
	} {
		pid, err := os.Stat(filepath.Join(home, "app-server-daemon", install.pidFile))
		if err != nil || !pid.ModTime().Before(configuredAt) && !configuredAt.IsZero() {
			continue
		}
		current := filepath.Join(home, "packages", install.packageName, "current")
		for _, executable := range []string{filepath.Join(current, "bin", binary), filepath.Join(current, binary)} {
			if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() &&
				runDaemon(executable, "version", 3*time.Second) == nil {
				return executable
			}
		}
	}
	return ""
}

func runDaemon(executable, operation string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "app-server", "daemon", operation)
	if err := prepareDaemonCommand(command); err != nil {
		return err
	}
	return command.Run()
}
