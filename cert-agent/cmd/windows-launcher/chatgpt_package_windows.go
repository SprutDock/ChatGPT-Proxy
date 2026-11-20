package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	appModelBufferError      = 122
	openAICodexPackageFamily = "OpenAI.Codex_2p2nqsd0c76g0"
	openAICodexApplicationID = "App"
)

var (
	getPackagesByFamily        = kernel32.NewProc("GetPackagesByPackageFamily")
	openPackageInfo            = kernel32.NewProc("OpenPackageInfoByFullName")
	closePackageInfo           = kernel32.NewProc("ClosePackageInfo")
	getPackageApplicationIDs   = kernel32.NewProc("GetPackageApplicationIds")
	formatApplicationUserModel = kernel32.NewProc("FormatApplicationUserModelId")
)

func findChatGPTAppID() (string, error) {
	families := []string{
		openAICodexPackageFamily,
		"OpenAI.ChatGPT-Desktop_2p2nqsd0c76g0",
	}
	for _, family := range families {
		fullNames, err := packageFullNames(family)
		if err != nil {
			continue
		}
		for _, fullName := range fullNames {
			applicationIDs, err := packageApplicationIDs(fullName)
			if err != nil {
				continue
			}
			if appID := preferredApplicationID(family, applicationIDs); appID != "" {
				return appID, nil
			}
		}
	}
	return openAICodexPackageFamily + "!" + openAICodexApplicationID, nil
}

func packageFullNames(family string) ([]string, error) {
	familyName, err := syscall.UTF16PtrFromString(family)
	if err != nil {
		return nil, err
	}
	var count, bufferLength uint32
	status, _, _ := getPackagesByFamily.Call(
		uintptr(unsafe.Pointer(familyName)),
		uintptr(unsafe.Pointer(&count)),
		0,
		uintptr(unsafe.Pointer(&bufferLength)),
		0,
	)
	if status != appModelBufferError || count == 0 || bufferLength == 0 || count > 64 || bufferLength > 1<<16 {
		return nil, fmt.Errorf("package family is unavailable: %s", family)
	}
	names := make([]uintptr, count)
	buffer := make([]uint16, bufferLength)
	status, _, _ = getPackagesByFamily.Call(
		uintptr(unsafe.Pointer(familyName)),
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&names[0])),
		uintptr(unsafe.Pointer(&bufferLength)),
		uintptr(unsafe.Pointer(&buffer[0])),
	)
	if status != 0 {
		return nil, fmt.Errorf("could not read installed package names: %w", syscall.Errno(status))
	}
	if int(count) > len(names) || int(bufferLength) > len(buffer) {
		return nil, fmt.Errorf("installed package name data changed while being read")
	}
	result := make([]string, 0, count)
	for _, name := range names[:count] {
		if name != 0 {
			value, ok := utf16FromBuffer(buffer, name)
			if !ok {
				return nil, fmt.Errorf("installed package name data is invalid")
			}
			result = append(result, value)
		}
	}
	runtime.KeepAlive(buffer)
	return result, nil
}

func packageApplicationIDs(fullName string) ([]string, error) {
	name, err := syscall.UTF16PtrFromString(fullName)
	if err != nil {
		return nil, err
	}
	var packageReference uintptr
	status, _, _ := openPackageInfo.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&packageReference)))
	if status != 0 {
		return nil, fmt.Errorf("could not open installed ChatGPT package: %w", syscall.Errno(status))
	}
	defer closePackageInfo.Call(packageReference)

	var length uint32
	status, _, _ = getPackageApplicationIDs.Call(packageReference, uintptr(unsafe.Pointer(&length)), 0, 0)
	if status != appModelBufferError || length == 0 || length > 1<<16 {
		return nil, fmt.Errorf("could not read ChatGPT application ids")
	}
	buffer := make([]byte, length)
	var count uint32
	status, _, _ = getPackageApplicationIDs.Call(packageReference, uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&count)))
	if status != 0 || count == 0 || count > 64 {
		return nil, fmt.Errorf("could not read ChatGPT application ids")
	}
	pointerSize := int(unsafe.Sizeof(uintptr(0)))
	if int(length) > len(buffer) || uint64(length) < uint64(count)*uint64(pointerSize) {
		return nil, fmt.Errorf("ChatGPT application id data is truncated")
	}
	result := make([]string, 0, count)
	for index := uint32(0); index < count; index++ {
		address := *(*uintptr)(unsafe.Pointer(&buffer[int(index)*pointerSize]))
		if address == 0 {
			continue
		}
		value, ok := utf16FromBytes(buffer, address)
		if !ok {
			return nil, fmt.Errorf("ChatGPT application id data is invalid")
		}
		result = append(result, value)
	}
	runtime.KeepAlive(buffer)
	return result, nil
}

func utf16FromBuffer(buffer []uint16, address uintptr) (string, bool) {
	if len(buffer) == 0 {
		return "", false
	}
	base := uintptr(unsafe.Pointer(&buffer[0]))
	if address < base || address >= base+uintptr(len(buffer))*2 || (address-base)%2 != 0 {
		return "", false
	}
	start := int((address - base) / 2)
	for index := start; index < len(buffer); index++ {
		if buffer[index] == 0 {
			return syscall.UTF16ToString(buffer[start : index+1]), true
		}
	}
	return "", false
}

func utf16FromBytes(buffer []byte, address uintptr) (string, bool) {
	if len(buffer) < 2 {
		return "", false
	}
	base := uintptr(unsafe.Pointer(&buffer[0]))
	if address < base || address >= base+uintptr(len(buffer)) || (address-base)%2 != 0 {
		return "", false
	}
	start := int((address - base) / 2)
	units := unsafe.Slice((*uint16)(unsafe.Pointer(address)), (len(buffer)-start)/2)
	for index, value := range units {
		if value == 0 {
			return syscall.UTF16ToString(units[:index+1]), true
		}
	}
	return "", false
}

func preferredApplicationID(family string, applicationIDs []string) string {
	wanted := "app"
	if strings.Contains(strings.ToLower(family), "chatgpt-desktop") {
		wanted = "chatgpt"
	}
	for _, id := range applicationIDs {
		if strings.EqualFold(id, wanted) {
			formatted, err := formatAppUserModelID(family, id)
			if err == nil {
				return formatted
			}
		}
	}
	if len(applicationIDs) == 1 {
		formatted, err := formatAppUserModelID(family, applicationIDs[0])
		if err == nil {
			return formatted
		}
	}
	return ""
}

func formatAppUserModelID(family, relativeID string) (string, error) {
	familyName, err := syscall.UTF16PtrFromString(family)
	if err != nil {
		return "", err
	}
	appID, err := syscall.UTF16PtrFromString(relativeID)
	if err != nil {
		return "", err
	}
	var length uint32
	status, _, _ := formatApplicationUserModel.Call(uintptr(unsafe.Pointer(familyName)), uintptr(unsafe.Pointer(appID)), uintptr(unsafe.Pointer(&length)), 0)
	if status != appModelBufferError || length == 0 || length > 1<<16 {
		return "", fmt.Errorf("could not format ChatGPT app identity")
	}
	buffer := make([]uint16, length)
	status, _, _ = formatApplicationUserModel.Call(uintptr(unsafe.Pointer(familyName)), uintptr(unsafe.Pointer(appID)), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buffer[0])))
	if status != 0 {
		return "", fmt.Errorf("could not format ChatGPT app identity: %w", syscall.Errno(status))
	}
	return syscall.UTF16ToString(buffer), nil
}
