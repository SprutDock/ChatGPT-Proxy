package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func paths() (Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("current user home is unavailable")
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	state := filepath.Join(localAppData, "SprutDock", "ChatGPT Proxy")
	return Paths{
		Resources:      filepath.Dir(executable),
		State:          state,
		Releases:       filepath.Join(state, "identity", "releases"),
		ActiveLink:     filepath.Join(state, "identity", "active"),
		StunnelConf:    filepath.Join(state, "stunnel.conf"),
		Log:            filepath.Join(state, "agent.log"),
		StunnelLog:     filepath.Join(state, "stunnel.log"),
		StunnelPIDFile: filepath.Join(state, "stunnel.pid"),
		SchedulerAgent: filepath.Join(state, "certificate-task.xml"),
		StateFile:      filepath.Join(state, "state.json"),
		PendingFile:    filepath.Join(state, "pending.json"),
	}, nil
}

func stunnelPath(path string) string {
	return filepath.ToSlash(path)
}

func stunnelStorePath(resources, path string) (string, error) {
	configDirectory := filepath.Join(resources, "config")
	relativePath, err := filepath.Rel(configDirectory, path)
	if err != nil {
		return "", fmt.Errorf("stunnel certificate files must be on the same drive as the app")
	}
	return filepath.ToSlash(relativePath), nil
}

func stunnelForegroundOption() string {
	return ""
}
