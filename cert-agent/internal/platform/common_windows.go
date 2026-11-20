package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"sprutdock/cert-agent/internal/state"
)

func Paths() (state.Paths, error) {
	executable, err := os.Executable()
	if err != nil {
		return state.Paths{}, err
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return state.Paths{}, fmt.Errorf("current user home is unavailable")
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	stateDirectory := filepath.Join(localAppData, "SprutDock", "ChatGPT Proxy")
	return state.Paths{
		Resources:   filepath.Dir(executable),
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

func StunnelPath(path string) string {
	return filepath.ToSlash(path)
}

func StunnelStorePath(resources, path string) (string, error) {
	configDirectory := filepath.Join(resources, "config")
	relativePath, err := filepath.Rel(configDirectory, path)
	if err != nil {
		return "", fmt.Errorf("stunnel certificate files must be on the same drive as the app")
	}
	return filepath.ToSlash(relativePath), nil
}

func StunnelForegroundOption() string {
	return ""
}

func StunnelPIDFile(p state.Paths) string {
	return filepath.Join(p.State, "stunnel.pid")
}

func SchedulerAgentPath(p state.Paths) string {
	return filepath.Join(p.State, "certificate-task.xml")
}
