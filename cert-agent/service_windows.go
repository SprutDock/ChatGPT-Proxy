package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	certificateTaskName = "SprutDock ChatGPT Proxy Certificate Agent"
	certificateRunKey   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

func installLaunchAgents(p Paths, interactive bool) error {
	stunnel := filepath.Join(p.Resources, "stunnel.exe")
	if _, err := os.Stat(stunnel); err != nil {
		return fmt.Errorf("bundled stunnel is unavailable")
	}
	if currentUserSID() == "" {
		return fmt.Errorf("current user id is unavailable")
	}
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	launcher := filepath.Join(p.Resources, "windows-launcher.ps1")
	arguments := `-NoProfile -STA -WindowStyle Hidden -ExecutionPolicy Bypass -File "` + launcher + `" -Background`
	if !interactive && available(p.SchedulerAgent) && command("schtasks.exe", "/Query", "/TN", certificateTaskName).Run() == nil {
		return nil
	}
	if command(registryPath(), "query", certificateRunKey, "/v", certificateTaskName).Run() == nil {
		if err := registerLogonFallback(powershell, arguments); err == nil {
			return nil
		}
	}
	startBoundary := time.Now().Truncate(12 * time.Hour).Format("2006-01-02T15:04:05")
	definition := taskXML(powershell, arguments, filepath.Dir(launcher), startBoundary)
	changed, err := writeIfChanged(p.SchedulerAgent, taskXMLBytes(definition), 0o600)
	if err != nil {
		return err
	}
	if !changed && command("schtasks.exe", "/Query", "/TN", certificateTaskName).Run() == nil {
		return nil
	}
	output, err := command("schtasks.exe", "/Create", "/TN", certificateTaskName, "/XML", p.SchedulerAgent, "/F").CombinedOutput()
	if err != nil {
		reason := strings.TrimSpace(string(output))
		if reason == "" {
			reason = err.Error()
		}
		if fallbackErr := registerLogonFallback(powershell, arguments); fallbackErr != nil {
			logAgentNotice(p, fmt.Sprintf("certificate agent task was not registered: %s; %v", reason, fallbackErr))
		} else {
			logAgentNotice(p, fmt.Sprintf("certificate agent task was not registered: %s; per-user logon fallback is enabled", reason))
		}
	}
	return nil
}

func registryPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "reg.exe")
}

func registerLogonFallback(powershell, arguments string) error {
	commandLine := `"` + powershell + `" ` + arguments
	output, err := command(registryPath(), "add", certificateRunKey, "/v", certificateTaskName, "/t", "REG_SZ", "/d", commandLine, "/f").CombinedOutput()
	if err == nil {
		return nil
	}
	reason := strings.TrimSpace(string(output))
	if reason == "" {
		reason = err.Error()
	}
	return fmt.Errorf("could not register the per-user logon fallback: %s", reason)
}

func logAgentNotice(p Paths, message string) {
	logFile, err := os.OpenFile(p.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(logFile, "%s %s\n", time.Now().Format(time.RFC3339), message)
	_ = logFile.Close()
}

func taskXML(commandPath, arguments, workingDirectory, startBoundary string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>` +
		`<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">` +
		`<Triggers><LogonTrigger><Enabled>true</Enabled></LogonTrigger>` +
		`<CalendarTrigger><StartBoundary>` + xmlEscape(startBoundary) + `</StartBoundary><Repetition><Interval>PT12H</Interval><Duration>P1D</Duration><StopAtDurationEnd>false</StopAtDurationEnd></Repetition><Enabled>true</Enabled><ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay></CalendarTrigger></Triggers>` +
		`<Principals><Principal id="Author"><UserId>` + xmlEscape(currentUserSID()) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>` +
		`<Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><StartWhenAvailable>true</StartWhenAvailable><ExecutionTimeLimit>PT20M</ExecutionTimeLimit><Enabled>true</Enabled></Settings>` +
		`<Actions Context="Author"><Exec><Command>` + xmlEscape(commandPath) + `</Command><Arguments>` + xmlEscape(arguments) + `</Arguments><WorkingDirectory>` + xmlEscape(workingDirectory) + `</WorkingDirectory></Exec></Actions></Task>`
}

func taskXMLBytes(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	result := make([]byte, 2+2*len(encoded))
	result[0], result[1] = 0xff, 0xfe
	for index, unit := range encoded {
		result[2+2*index], result[3+2*index] = byte(unit), byte(unit>>8)
	}
	return result
}

func currentUserSID() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}
	return current.Uid
}

func xmlEscape(value string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

func ensureStunnelRunning(p Paths) error {
	pid, err := readStunnelPID(p)
	if err == nil && stunnelProcessRunning(pid, filepath.Join(p.Resources, "stunnel.exe")) {
		return nil
	}
	_ = os.Remove(p.StunnelPIDFile)
	return startStunnel(p)
}

func restartStunnel(p Paths) error {
	if err := stopStunnel(p); err != nil {
		return err
	}
	return startStunnel(p)
}

func stopStunnel(p Paths) error {
	pid, err := readStunnelPID(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		_ = os.Remove(p.StunnelPIDFile)
		return nil
	}
	if stunnelProcessRunning(pid, filepath.Join(p.Resources, "stunnel.exe")) {
		process, err := os.FindProcess(int(pid))
		if err != nil {
			return err
		}
		if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && stunnelProcessRunning(pid, filepath.Join(p.Resources, "stunnel.exe")) {
			time.Sleep(100 * time.Millisecond)
		}
		if stunnelProcessRunning(pid, filepath.Join(p.Resources, "stunnel.exe")) {
			return fmt.Errorf("could not stop the local proxy tunnel")
		}
	}
	return os.Remove(p.StunnelPIDFile)
}

func startStunnel(p Paths) error {
	logFile, err := os.OpenFile(p.StunnelLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	stunnel := command(filepath.Join(p.Resources, "stunnel.exe"), p.StunnelConf)
	stunnel.Env = stunnelEnvironment(p.Resources)
	stunnel.Stdout = logFile
	stunnel.Stderr = logFile
	stunnel.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := stunnel.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("could not start the local proxy tunnel: %w", err)
	}
	pid := strconv.Itoa(stunnel.Process.Pid)
	if err := writeAtomic(p.StunnelPIDFile, []byte(pid), 0o600); err != nil {
		_ = stunnel.Process.Kill()
		_ = stunnel.Process.Release()
		_ = logFile.Close()
		return err
	}
	_ = stunnel.Process.Release()
	_ = logFile.Close()
	return nil
}

func stunnelEnvironment(resources string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if strings.EqualFold(name, "OPENSSL_CONF") || strings.EqualFold(name, "OPENSSL_MODULES") {
			continue
		}
		environment = append(environment, variable)
	}
	return append(environment,
		"OPENSSL_CONF="+filepath.Join(resources, "config", "openssl.cnf"),
		"OPENSSL_MODULES="+filepath.Join(resources, "ossl-modules"),
	)
}

func readStunnelPID(p Paths) (uint32, error) {
	data, err := os.ReadFile(p.StunnelPIDFile)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("stunnel process id is invalid")
	}
	return uint32(value), nil
}

func stunnelProcessRunning(pid uint32, executable string) bool {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	path := make([]uint16, 32768)
	pathLength := uint32(len(path))
	queryImage := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
	result, _, _ := queryImage.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&pathLength)))
	if result == 0 {
		return false
	}
	actual := filepath.Clean(syscall.UTF16ToString(path[:pathLength]))
	return strings.EqualFold(actual, filepath.Clean(executable))
}
