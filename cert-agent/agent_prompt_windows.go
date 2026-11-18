package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func promptInvitation() (string, error) {
	const script = `Add-Type -AssemblyName System.Windows.Forms; $form = New-Object Windows.Forms.Form; $form.Text = 'SprutDock'; $form.Width = 460; $form.Height = 150; $form.StartPosition = 'CenterScreen'; $form.TopMost = $true; $label = New-Object Windows.Forms.Label; $label.Text = 'Введите приглашение для регистрации устройства SprutDock.'; $label.Left = 12; $label.Top = 12; $label.Width = 420; $form.Controls.Add($label); $invitationField = New-Object Windows.Forms.TextBox; $invitationField.Left = 12; $invitationField.Top = 40; $invitationField.Width = 420; $invitationField.UseSystemPasswordChar = $true; $form.Controls.Add($invitationField); $ok = New-Object Windows.Forms.Button; $ok.Text = 'Продолжить'; $ok.Left = 270; $ok.Top = 75; $ok.DialogResult = [Windows.Forms.DialogResult]::OK; $form.Controls.Add($ok); $cancel = New-Object Windows.Forms.Button; $cancel.Text = 'Отмена'; $cancel.Left = 355; $cancel.Top = 75; $cancel.DialogResult = [Windows.Forms.DialogResult]::Cancel; $form.Controls.Add($cancel); $form.AcceptButton = $ok; $form.CancelButton = $cancel; if ($form.ShowDialog() -eq [Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($invitationField.Text) } else { exit 1 }`
	process := exec.Command("powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", script)
	process.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	value, err := process.Output()
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
	title = strings.ReplaceAll(title, "'", "''")
	message = strings.ReplaceAll(message, "'", "''")
	script := `Add-Type -AssemblyName System.Windows.Forms; $notification = New-Object Windows.Forms.NotifyIcon; $notification.Icon = [Drawing.SystemIcons]::Information; $notification.Visible = $true; $notification.ShowBalloonTip(5000, '` + title + `', '` + message + `', [Windows.Forms.ToolTipIcon]::Info); Start-Sleep -Seconds 6; $notification.Visible = $false; $notification.Dispose()`
	process := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", script)
	process.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if process.Start() == nil {
		_ = process.Process.Release()
	}
}
