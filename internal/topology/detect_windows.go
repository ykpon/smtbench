//go:build windows

package topology

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGLPIEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLogicalProcessorInformationEx")

const relationAll = 0xFFFF

// readGLPIEx returns the raw GetLogicalProcessorInformationEx(RelationAll) buffer.
func readGLPIEx() ([]byte, error) {
	var size uint32
	procGLPIEx.Call(relationAll, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return nil, fmt.Errorf("GetLogicalProcessorInformationEx: пустой размер буфера")
	}
	for {
		buf := make([]byte, size)
		r, _, err := procGLPIEx.Call(relationAll, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r != 0 {
			return buf[:size], nil
		}
		if err != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, fmt.Errorf("GetLogicalProcessorInformationEx: %w", err)
		}
	}
}

// Detect reads the topology of the current machine.
func Detect() (*Topology, error) {
	buf, err := readGLPIEx()
	if err != nil {
		return nil, err
	}
	return ParseGLPIEx(buf)
}
