package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func paths() (Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	resources := filepath.Dir(executable)
	home := os.Getenv("HOME")
	if home == "" {
		return Paths{}, fmt.Errorf("current user home is unavailable")
	}
	state := filepath.Join(home, "Library", "Application Support", "SprutDock Proxy")
	return Paths{
		Resources:      resources,
		State:          state,
		Releases:       filepath.Join(state, "identity", "releases"),
		ActiveLink:     filepath.Join(state, "identity", "active"),
		StunnelConf:    filepath.Join(state, "stunnel.conf"),
		Log:            filepath.Join(state, "agent.log"),
		StunnelLog:     filepath.Join(state, "stunnel.log"),
		StunnelAgent:   filepath.Join(home, "Library", "LaunchAgents", stunnelLabel+".plist"),
		SchedulerAgent: filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"),
		StateFile:      filepath.Join(state, "state.json"),
		PendingFile:    filepath.Join(state, "pending.json"),
	}, nil
}

func currentUID() (string, error) {
	uid := os.Getuid()
	if uid < 0 {
		return "", fmt.Errorf("current user id is unavailable")
	}
	return strconv.Itoa(uid), nil
}

func stunnelPath(path string) string {
	return path
}

func stunnelStorePath(_, path string) (string, error) {
	return path, nil
}

func stunnelForegroundOption() string {
	return "foreground = yes"
}
