package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"sprutdock/cert-agent/internal/state"
)

var (
	getVolumePathNameW               = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumePathNameW")
	getVolumeNameForVolumeMountPoint = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeNameForVolumeMountPointW")
	getVolumeInformationW            = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeInformationW")
	getDriveTypeW                    = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")
)

func StunnelDirectories(p state.Paths) (string, string, error) {
	if isASCIIPath(p.Resources) && isASCIIPath(p.State) {
		return p.Resources, p.State, nil
	}
	resources, stateDirectory, err := latinStunnelDirectories(p)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrStunnelPathUnavailable, err)
	}
	return resources, stateDirectory, nil
}

func latinStunnelDirectories(p state.Paths) (string, string, error) {
	resources, err := filepath.Abs(p.Resources)
	if err != nil {
		return "", "", err
	}
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = os.Getenv("ALLUSERSPROFILE")
	}
	if programData == "" {
		return "", "", fmt.Errorf("ProgramData is unavailable")
	}
	programData, err = filepath.Abs(programData)
	if err != nil || !isASCIIPath(programData) {
		return "", "", fmt.Errorf("ProgramData does not provide an ASCII path")
	}
	if err := sameLocalNTFSVolume(resources, programData); err != nil {
		return "", "", err
	}
	sid := currentUserSID()
	if sid == "" || !isASCIIPath(sid) || strings.ContainsAny(sid, `\\/`) {
		return "", "", fmt.Errorf("current user id is unavailable")
	}
	privateDirectory := filepath.Join(programData, "SprutDockProxy-"+sid)
	if err := secureStunnelDirectory(privateDirectory, sid); err != nil {
		return "", "", err
	}
	stateTarget, err := filepath.Abs(p.State)
	if err != nil {
		return "", "", err
	}
	resourcesAlias := filepath.Join(privateDirectory, "files")
	stateAlias := filepath.Join(privateDirectory, "state")
	if err := ensureStunnelJunction(resourcesAlias, resources); err != nil {
		return "", "", err
	}
	if err := ensureStunnelJunction(stateAlias, stateTarget); err != nil {
		return "", "", err
	}
	return resourcesAlias, stateAlias, nil
}

func secureStunnelDirectory(directory, sid string) error {
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := stunnelPathAttributes(directory)
	if err != nil {
		return err
	}
	const directoryAttributes = syscall.FILE_ATTRIBUTE_DIRECTORY | syscall.FILE_ATTRIBUTE_REPARSE_POINT
	if info.FileAttributes&directoryAttributes != syscall.FILE_ATTRIBUTE_DIRECTORY {
		return fmt.Errorf("protected stunnel directory is not a regular directory")
	}
	icacls, err := systemTool("icacls.exe")
	if err != nil {
		return err
	}
	for _, arguments := range [][]string{
		{directory, "/setowner", "*" + sid},
		{directory, "/reset"},
		{directory, "/inheritance:r", "/grant:r", "*" + sid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F"},
	} {
		command := Command(icacls, arguments...)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if _, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("could not restrict stunnel directory permissions")
		}
	}
	return nil
}

func ensureStunnelJunction(alias, target string) error {
	targetInfo, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !targetInfo.IsDir() {
		return fmt.Errorf("stunnel junction target is not a directory")
	}
	if matches, err := junctionMatches(alias, targetInfo); err == nil {
		if matches {
			return nil
		}
		if err := os.Remove(alias); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cmd, err := systemTool("cmd.exe")
	if err != nil {
		return err
	}
	command := Command(cmd)
	command.Env = stunnelJunctionEnvironment(alias, target)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
		CmdLine:       syscall.EscapeArg(cmd) + ` /d /v:off /c mklink /J "%SPRU_STUNNEL_STATE_LINK%" "%SPRU_STUNNEL_STATE_TARGET%"`,
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create stunnel junction: %w: %s", err, strings.TrimSpace(string(output)))
	}
	matches, err := junctionMatches(alias, targetInfo)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("stunnel state junction points to a different directory")
	}
	return nil
}

func junctionMatches(alias string, target os.FileInfo) (bool, error) {
	data, err := stunnelPathAttributes(alias)
	if err != nil {
		return false, err
	}
	const junctionTag = 0xa0000003
	const junctionAttributes = syscall.FILE_ATTRIBUTE_DIRECTORY | syscall.FILE_ATTRIBUTE_REPARSE_POINT
	if data.FileAttributes&junctionAttributes != junctionAttributes || data.Reserved0 != junctionTag {
		return false, fmt.Errorf("stunnel state alias is not a junction")
	}
	info, err := os.Stat(alias)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(info, target), nil
}

func stunnelPathAttributes(path string) (syscall.Win32finddata, error) {
	var data syscall.Win32finddata
	searchPath := path
	if volume := filepath.VolumeName(path); len(volume) == 2 && volume[1] == ':' {
		searchPath = `\\?\` + path
	}
	name, err := syscall.UTF16PtrFromString(searchPath)
	if err != nil {
		return data, err
	}
	handle, err := syscall.FindFirstFile(name, &data)
	if err != nil {
		return data, &os.PathError{Op: "FindFirstFile", Path: path, Err: err}
	}
	_ = syscall.FindClose(handle)
	return data, nil
}

func sameLocalNTFSVolume(first, second string) error {
	firstVolume, err := localNTFSVolume(first)
	if err != nil {
		return err
	}
	secondVolume, err := localNTFSVolume(second)
	if err != nil {
		return err
	}
	if !strings.EqualFold(firstVolume, secondVolume) {
		return fmt.Errorf("application and ProgramData are not on the same local NTFS volume")
	}
	return nil
}

func localNTFSVolume(path string) (string, error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	volumePath := make([]uint16, 32768)
	result, _, _ := getVolumePathNameW.Call(uintptr(unsafe.Pointer(pathPointer)), uintptr(unsafe.Pointer(&volumePath[0])), uintptr(len(volumePath)))
	if result == 0 {
		return "", fmt.Errorf("could not identify the application disk")
	}
	if driveType, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(&volumePath[0]))); driveType != 3 {
		return "", fmt.Errorf("stunnel paths require a local disk")
	}
	volumeName := make([]uint16, 128)
	result, _, _ = getVolumeNameForVolumeMountPoint.Call(uintptr(unsafe.Pointer(&volumePath[0])), uintptr(unsafe.Pointer(&volumeName[0])), uintptr(len(volumeName)))
	if result == 0 {
		return "", fmt.Errorf("could not identify the application disk")
	}
	fileSystem := make([]uint16, 64)
	result, _, _ = getVolumeInformationW.Call(uintptr(unsafe.Pointer(&volumePath[0])), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&fileSystem[0])), uintptr(len(fileSystem)))
	if result == 0 || !strings.EqualFold(syscall.UTF16ToString(fileSystem), "NTFS") {
		return "", fmt.Errorf("stunnel state requires an NTFS disk")
	}
	return syscall.UTF16ToString(volumeName), nil
}

func stunnelJunctionEnvironment(alias, target string) []string {
	const linkName = "SPRU_STUNNEL_STATE_LINK"
	const targetName = "SPRU_STUNNEL_STATE_TARGET"
	environment := make([]string, 0, len(os.Environ())+2)
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !strings.EqualFold(name, linkName) && !strings.EqualFold(name, targetName) {
			environment = append(environment, variable)
		}
	}
	return append(environment, linkName+"="+alias, targetName+"="+target)
}

func systemTool(name string) (string, error) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		return "", fmt.Errorf("Windows system directory is unavailable")
	}
	path := filepath.Join(systemRoot, "System32", name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func isASCIIPath(path string) bool {
	for _, character := range path {
		if character > 0x7f {
			return false
		}
	}
	return true
}
