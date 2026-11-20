package platform

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"sprutdock/cert-agent/internal/state"
)

const (
	certificateTaskName = "SprutDock ChatGPT Proxy Certificate Agent"
	certificateRunKey   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

func InstallBackgroundCheck(p state.Paths, interactive bool) error {
	_ = interactive
	stunnel := filepath.Join(p.Resources, "stunnel.exe")
	if _, err := os.Stat(stunnel); err != nil {
		return fmt.Errorf("bundled stunnel is unavailable")
	}
	agent := filepath.Join(p.Resources, "cert-agent.exe")
	if _, err := os.Stat(agent); err != nil {
		return fmt.Errorf("certificate agent is unavailable")
	}
	if currentUserSID() == "" {
		return fmt.Errorf("current user id is unavailable")
	}
	arguments := "ensure --background"
	if Command(registryPath(), "query", certificateRunKey, "/v", certificateTaskName).Run() == nil {
		if err := registerLogonFallback(agent, arguments); err == nil {
			return nil
		}
	}
	startBoundary := time.Now().Truncate(12 * time.Hour).Format("2006-01-02T15:04:05")
	definition := taskXML(agent, arguments, p.Resources, startBoundary)
	schedulerPath := SchedulerAgentPath(p)
	changed, err := state.WriteIfChanged(schedulerPath, taskXMLBytes(definition), 0o600)
	if err != nil {
		return err
	}
	if !changed && Command("schtasks.exe", "/Query", "/TN", certificateTaskName).Run() == nil {
		return nil
	}
	output, err := Command("schtasks.exe", "/Create", "/TN", certificateTaskName, "/XML", schedulerPath, "/F").CombinedOutput()
	if err != nil {
		reason := strings.TrimSpace(string(output))
		if reason == "" {
			reason = err.Error()
		}
		if fallbackErr := registerLogonFallback(agent, arguments); fallbackErr != nil {
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

func registerLogonFallback(agent, arguments string) error {
	commandLine := `"` + agent + `" ` + arguments
	output, err := Command(registryPath(), "add", certificateRunKey, "/v", certificateTaskName, "/t", "REG_SZ", "/d", commandLine, "/f").CombinedOutput()
	if err == nil {
		return nil
	}
	reason := strings.TrimSpace(string(output))
	if reason == "" {
		reason = err.Error()
	}
	return fmt.Errorf("could not register the per-user logon fallback: %s", reason)
}

func logAgentNotice(p state.Paths, message string) {
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
