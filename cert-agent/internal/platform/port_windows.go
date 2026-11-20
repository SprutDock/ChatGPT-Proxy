package platform

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"sprutdock/cert-agent/internal/state"
)

var getExtendedTCPTable = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func VerifyProxyOwner(p state.Paths) error {
	owners, err := tcpListenerOwners(3128)
	if err != nil {
		return err
	}
	if len(owners) == 0 {
		return fmt.Errorf("bundled stunnel is not listening on the local proxy port")
	}

	pid, pidErr := readStunnelPID(p)
	for _, owner := range owners {
		if pidErr == nil && owner == pid && isTrackedStunnelProcess(owner) {
			continue
		}
		return ErrProxyOwnerConflict
	}
	return nil
}

func stunnelProcessOwnsProxyPort(pid uint32) bool {
	if !isTrackedStunnelProcess(pid) {
		return false
	}
	owners, err := tcpListenerOwners(3128)
	if err != nil {
		return false
	}
	for _, owner := range owners {
		if owner == pid {
			return true
		}
	}
	return false
}

func isTrackedStunnelProcess(pid uint32) bool {
	image := stunnelProcessPath(pid)
	return image == "" || strings.EqualFold(filepath.Base(image), "stunnel.exe")
}

func tcpListenerOwners(port uint16) ([]uint32, error) {
	wantedAddress := binary.LittleEndian.Uint32([]byte{127, 0, 0, 1})
	wantedPort := uint16(port>>8) | port<<8
	for attempt := 0; attempt < 3; attempt++ {
		var size uint32
		status, _, _ := getExtendedTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 3, 0)
		if status != 122 && status != 0 {
			return nil, fmt.Errorf("could not inspect the local proxy listener: %w", syscall.Errno(status))
		}
		if size < 4 {
			return nil, fmt.Errorf("local proxy listener table is invalid")
		}
		table := make([]byte, size)
		status, _, _ = getExtendedTCPTable.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 3, 0)
		if status == 122 {
			continue
		}
		if status != 0 {
			return nil, fmt.Errorf("could not inspect the local proxy listener: %w", syscall.Errno(status))
		}
		count := binary.LittleEndian.Uint32(table[:4])
		if uint64(size) < 4+uint64(count)*24 {
			return nil, fmt.Errorf("local proxy listener table is truncated")
		}
		owners := make([]uint32, 0, count)
		for index := uint32(0); index < count; index++ {
			row := table[4+index*24 : 4+(index+1)*24]
			address := binary.LittleEndian.Uint32(row[4:8])
			localPort := uint16(binary.LittleEndian.Uint32(row[8:12]))
			if (address == wantedAddress || address == 0) && localPort == wantedPort {
				owners = append(owners, binary.LittleEndian.Uint32(row[20:24]))
			}
		}
		return owners, nil
	}
	return nil, fmt.Errorf("local proxy listener table changed while being read")
}
