package main

import (
	"syscall"
	"unsafe"
)

func replaceFile(source, destination string) error {
	sourcePath, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	moveFile := syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
	result, _, callError := moveFile.Call(uintptr(unsafe.Pointer(sourcePath)), uintptr(unsafe.Pointer(destinationPath)), 0x1|0x8)
	if result == 0 {
		return callError
	}
	return nil
}
