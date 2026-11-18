package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func promptInvitation() (string, error) {
	value, err := exec.Command("osascript", "-e", `text returned of (display dialog "Введите приглашение для регистрации устройства SprutDock." default answer "" buttons {"Отмена", "Продолжить"} default button "Продолжить" with hidden answer)`).Output()
	if err != nil {
		return "", fmt.Errorf("invitation entry was cancelled")
	}
	invitation := strings.TrimSpace(string(value))
	if len(invitation) < 32 || len(invitation) > 256 {
		return "", fmt.Errorf("invitation is invalid")
	}
	return invitation, nil
}

func notifyUser(title, message string) {
	_ = exec.Command("osascript", "-e", "display notification "+appleScriptString(message)+" with title "+appleScriptString(title)).Run()
}

func appleScriptString(value string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value) + "\""
}
