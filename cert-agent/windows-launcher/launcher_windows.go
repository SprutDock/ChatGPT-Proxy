package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func main() {
	executable, err := os.Executable()
	if err != nil {
		showError(err.Error())
		return
	}
	root := filepath.Dir(executable)
	script := filepath.Join(root, "windows-launcher.ps1")
	if _, err := os.Stat(script); err != nil {
		showError("Не найден файл запуска. Распакуйте весь архив ChatGPT Proxy и повторите запуск.")
		return
	}

	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(powershell); err != nil {
		powershell, err = exec.LookPath("powershell.exe")
		if err != nil {
			showError("Не найден Windows PowerShell.")
			return
		}
	}

	command := exec.Command(powershell, "-NoLogo", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", script, "-LauncherPid", strconv.Itoa(os.Getpid()))
	command.Dir = root
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = fmt.Sprintf("Не удалось запустить лаунчер PowerShell: %v", err)
		}
		showError(writeFailureLog(message))
	}
}

func showError(message string) {
	if len([]rune(message)) > 3000 {
		message = string([]rune(message)[len([]rune(message))-3000:])
	}
	text, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, err := syscall.UTF16PtrFromString("ChatGPT Proxy не запущен")
	if err != nil {
		return
	}
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10|0x10000)
}

func writeFailureLog(message string) string {
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
		_, _ = fmt.Fprintf(logFile, "%s launcher error: %s\n", time.Now().Format(time.RFC3339), message)
		_ = logFile.Close()
	}
	return message + "\n\nЛог: " + logPath
}
