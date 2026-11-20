package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	processQueryLimited = 0x1000
	processSnapshot     = 0x00000002
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	getApplicationUserModelID = kernel32.NewProc("GetApplicationUserModelId")
	createToolhelpSnapshot    = kernel32.NewProc("CreateToolhelp32Snapshot")
	process32First            = kernel32.NewProc("Process32FirstW")
	process32Next             = kernel32.NewProc("Process32NextW")
	processIdToSessionID      = kernel32.NewProc("ProcessIdToSessionId")
)

type processEntry32 struct {
	Size              uint32
	Usage             uint32
	ProcessID         uint32
	DefaultHeapID     uintptr
	ModuleID          uint32
	Threads           uint32
	ParentProcessID   uint32
	PriorityClassBase int32
	Flags             uint32
	ExecutableName    [260]uint16
}

func runningChatGPTProcesses(appID string) ([]uint32, error) {
	var sessionID uint32
	if result, _, err := processIdToSessionID.Call(uintptr(os.Getpid()), uintptr(unsafe.Pointer(&sessionID))); result == 0 {
		return nil, fmt.Errorf("could not inspect the current Windows session: %w", err)
	}
	snapshot, _, callErr := createToolhelpSnapshot.Call(processSnapshot, 0)
	if snapshot == ^uintptr(0) {
		return nil, fmt.Errorf("could not inspect running applications: %w", callErr)
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))
	entry := processEntry32{Size: uint32(unsafe.Sizeof(processEntry32{}))}
	result, _, _ := process32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	var processes []uint32
	for result != 0 {
		if strings.EqualFold(syscall.UTF16ToString(entry.ExecutableName[:]), "ChatGPT.exe") {
			var otherSession uint32
			inSession, _, _ := processIdToSessionID.Call(uintptr(entry.ProcessID), uintptr(unsafe.Pointer(&otherSession)))
			if inSession != 0 && otherSession == sessionID && strings.EqualFold(processApplicationID(entry.ProcessID), appID) {
				processes = append(processes, entry.ProcessID)
			}
		}
		entry.Size = uint32(unsafe.Sizeof(processEntry32{}))
		result, _, _ = process32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	return processes, nil
}

func processApplicationID(processID uint32) string {
	process, err := syscall.OpenProcess(processQueryLimited, false, processID)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(process)
	var length uint32
	status, _, _ := getApplicationUserModelID.Call(uintptr(process), uintptr(unsafe.Pointer(&length)), 0)
	if status != appModelBufferError || length == 0 || length > 1<<16 {
		return ""
	}
	buffer := make([]uint16, length)
	status, _, _ = getApplicationUserModelID.Call(uintptr(process), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buffer[0])))
	if status != 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}

func chatGPTAlias(appID string) (string, error) {
	localAppData, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	family, _, _ := strings.Cut(appID, "!")
	path := filepath.Join(localAppData, "Microsoft", "WindowsApps", family, "chatgpt-desktop.exe")
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	var data syscall.Win32finddata
	handle, err := syscall.FindFirstFile(pathPointer, &data)
	if err == nil {
		_ = syscall.FindClose(handle)
		const appExecutionAliasTag = 0x8000001b
		if data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 && data.Reserved0 == appExecutionAliasTag {
			return path, nil
		}
	}
	return "", fmt.Errorf("команда запуска ChatGPT недоступна. Проверьте установку Codex из Microsoft Store и псевдоним chatgpt-desktop.exe в настройках Windows")
}
