package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	enablePerMonitorDPI()
	executable, err := os.Executable()
	if err != nil {
		showError("Не удалось определить папку приложения.")
		return
	}
	root := filepath.Dir(executable)
	if len(os.Args) > 1 && os.Args[1] == "--apply-update" {
		if err := runUpdateHelper(os.Args[2:]); err != nil {
			showError(writeFailureLog("update apply failed: " + err.Error()))
		}
		return
	}
	if err := runApplication(root); err != nil {
		showError(writeFailureLog(err.Error()))
	}
}

func showError(message string) {
	showMessage("ChatGPT Proxy не запущен", message, 0x10)
}

func writeFailureLog(message string) string {
	message = strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return ' '
		}
		return character
	}, message)
	if len([]rune(message)) > 1000 {
		message = string([]rune(message)[:1000])
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return message
	}
	logDirectory := filepath.Join(localAppData, "SprutDock", "ChatGPT Proxy")
	if os.MkdirAll(logDirectory, 0o700) != nil {
		return message
	}
	logPath := filepath.Join(logDirectory, "launcher.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = fmt.Fprintf(logFile, "%s %s\n", time.Now().Format(time.RFC3339), message)
		_ = logFile.Close()
	}
	return message + "\n\nЛог: " + logPath
}

func parseProcessID(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("invalid update parent process")
	}
	return uint32(parsed), nil
}
