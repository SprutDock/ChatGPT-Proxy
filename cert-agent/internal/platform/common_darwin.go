package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"sprutdock/cert-agent/internal/state"
)

const (
	StunnelLabel = "com.sprutdock.proxy.stunnel"
	AgentLabel   = "com.sprutdock.cert-agent.scheduled"
)

func Paths() (state.Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return state.Paths{}, err
	}
	resources := filepath.Dir(executable)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return state.Paths{}, fmt.Errorf("current user home is unavailable")
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return state.Paths{}, fmt.Errorf("current user home is unavailable")
	}
	stateDirectory := filepath.Join(home, "Library", "Application Support", "SprutDock Proxy")
	return state.Paths{
		Resources:   resources,
		State:       stateDirectory,
		Releases:    filepath.Join(stateDirectory, "identity", "releases"),
		ActiveLink:  filepath.Join(stateDirectory, "identity", "active"),
		StunnelConf: filepath.Join(stateDirectory, "stunnel.conf"),
		Log:         filepath.Join(stateDirectory, "agent.log"),
		StunnelLog:  filepath.Join(stateDirectory, "stunnel.log"),
		StateFile:   filepath.Join(stateDirectory, "state.json"),
		PendingFile: filepath.Join(stateDirectory, "pending.json"),
	}, nil
}

func CurrentUID() (string, error) {
	uid := os.Getuid()
	if uid < 0 {
		return "", fmt.Errorf("current user id is unavailable")
	}
	return strconv.Itoa(uid), nil
}

func StunnelPath(path string) string {
	return path
}

func StunnelDirectories(p state.Paths) (string, string, error) {
	return p.Resources, p.State, nil
}

func StunnelStorePath(_, path string) (string, error) {
	return path, nil
}

func StunnelForegroundOption() string {
	return "foreground = yes"
}

func StunnelAgentPath(p state.Paths) string {
	return filepath.Join(homeDirectory(p), "Library", "LaunchAgents", StunnelLabel+".plist")
}

func SchedulerAgentPath(p state.Paths) string {
	return filepath.Join(homeDirectory(p), "Library", "LaunchAgents", AgentLabel+".plist")
}

func homeDirectory(p state.Paths) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if absolute, absErr := filepath.Abs(home); absErr == nil {
			return absolute
		}
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(p.State)))
}
