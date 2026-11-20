package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const localProxyURL = "http://127.0.0.1:3128"

var errOperationCancelled = errors.New("operation cancelled")

func runApplication(root string) error {
	cleanupPreviousUpdateStage()
	if err := checkPackageFiles(root); err != nil {
		return err
	}
	if updateStarted, err := checkForUpdate(root); err != nil {
		return err
	} else if updateStarted {
		return nil
	}
	appID, err := findChatGPTAppID()
	if err != nil {
		return err
	}
	alias, err := chatGPTAlias(appID)
	if err != nil {
		return err
	}
	if !confirmProxyOverride() {
		return nil
	}
	ui, err := newSetupUI()
	if err != nil {
		return fmt.Errorf("не удалось открыть окно настройки ChatGPT Proxy")
	}
	defer ui.close()

	for {
		ui.begin("Проверка сертификата и защищённого соединения…")
		if err := runCertificateAgent(root, ui); err != nil {
			if errors.Is(err, errOperationCancelled) {
				return nil
			}
			stage := "checking"
			var agentErr agentRunError
			if errors.As(err, &agentErr) && agentErr.stage != "" {
				stage = agentErr.stage
			}
			launcherLog("certificate ensure failed stage=" + stage)
			failure := "Настройка сертификата или защищённого соединения не завершилась. Повторите попытку."
			if stage == "proxy_conflict" {
				failure = "Локальный порт 3128 занят другим процессом. Закройте это приложение и повторите запуск."
			}
			ui.fail(failure)
			action := <-ui.actions
			if action.kind != "retry" {
				return nil
			}
			continue
		}
		break
	}

	if err := waitForChatGPTClosed(appID, ui); err != nil {
		if errors.Is(err, errOperationCancelled) {
			return nil
		}
		return err
	}
	command := exec.Command(alias, "--proxy-server="+localProxyURL)
	command.Env = localProxyEnvironment(command.Environ())
	if err := command.Start(); err != nil {
		return fmt.Errorf("не удалось запустить ChatGPT через локальный сертификатный туннель: %w", err)
	}
	defer command.Process.Release()
	processID := uint32(command.Process.Pid)
	if !waitForActivatedApplication(processID, appID) {
		return fmt.Errorf("Windows не подтвердил запуск приложения ChatGPT")
	}
	launcherLog(fmt.Sprintf("ChatGPT activation succeeded pid=%d", processID))
	return nil
}

func waitForActivatedApplication(processID uint32, appID string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		actualAppID := processApplicationID(processID)
		if actualAppID == appID {
			return true
		}
		if actualAppID != "" {
			return false
		}
		if time.Now().After(deadline) {
			return false
		}
		process, err := syscall.OpenProcess(0x1000, false, processID)
		if err != nil {
			return false
		}
		_ = syscall.CloseHandle(process)
		time.Sleep(100 * time.Millisecond)
	}
}

func checkPackageFiles(root string) error {
	launcher, err := os.Stat(filepath.Join(root, "ChatGPT Proxy.exe"))
	if err != nil || !launcher.Mode().IsRegular() {
		return fmt.Errorf("не найдены обязательные файлы ChatGPT Proxy. Распакуйте полный архив приложения и повторите запуск")
	}
	resources := supportDirectory(root)
	required := []string{
		"cert-agent.exe", "stunnel.exe", "windows-launcher.ps1", "windows-update.ps1",
		"client-ca-chain.pem", "server-ca.pem", "version.txt", filepath.Join("config", "openssl.cnf"),
		filepath.Join("config", "fipsmodule.cnf"), filepath.Join("ossl-modules", "fips.dll"),
	}
	for _, name := range required {
		info, err := os.Stat(filepath.Join(resources, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("не найдены обязательные файлы ChatGPT Proxy. Распакуйте полный архив приложения и повторите запуск")
		}
	}
	return nil
}

func confirmProxyOverride() bool {
	conflicts := false
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && !strings.EqualFold(value, localProxyURL) {
			conflicts = true
			break
		}
	}
	if !conflicts {
		return true
	}
	return showMessage("Прокси ChatGPT Proxy", "Для этого запуска ChatGPT будет использовать локальный сертификатный туннель. Настройки системы и файлы приложения останутся без изменений. Продолжить?", 0x4|0x30) == 6
}

func localProxyEnvironment(environment []string) []string {
	noProxy := "localhost,127.0.0.1,::1"
	if previous := strings.TrimSpace(os.Getenv("NO_PROXY")); previous != "" {
		noProxy = previous + "," + noProxy
	}
	return append(environment, "HTTP_PROXY="+localProxyURL, "HTTPS_PROXY="+localProxyURL, "ALL_PROXY="+localProxyURL, "NO_PROXY="+noProxy)
}

func waitForChatGPTClosed(appID string, ui *setupUI) error {
	for {
		processes, err := runningChatGPTProcesses(appID)
		if err != nil {
			return fmt.Errorf("не удалось проверить состояние приложения ChatGPT")
		}
		if len(processes) == 0 {
			return nil
		}
		ui.fail("ChatGPT уже запущен. Полностью выйдите из ChatGPT и нажмите «Повторить», чтобы запустить его через защищённый туннель.")
		action := <-ui.actions
		if action.kind != "retry" {
			return errOperationCancelled
		}
		ui.begin("Ожидание завершения ChatGPT…")
	}
}

func launcherLog(message string) {
	logDirectory := userStateDirectory()
	if os.MkdirAll(logDirectory, 0o700) != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(logDirectory, "launcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(file, "%s %s\n", timeStamp(), message)
	_ = file.Close()
}

func timeStamp() string {
	return time.Now().Format("2006-01-02T15:04:05Z07:00")
}
