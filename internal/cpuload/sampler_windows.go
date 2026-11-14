//go:build windows

package cpuload

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procNtQSI = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQuerySystemInformation")

const systemProcessorPerformanceInformation = 8

// ntSampler reads per-CPU times. NtQuerySystemInformation(8) only reports
// the processor group of the calling thread, so machines with more than
// 64 logical CPUs get data for group 0 only.
type ntSampler struct{ buf []byte }

func NewSampler() Sampler {
	return &ntSampler{buf: make([]byte, ntRecord*runtime.NumCPU())}
}

func (s *ntSampler) Sample() (map[int]Times, error) {
	var got uint32
	r, _, _ := procNtQSI.Call(systemProcessorPerformanceInformation,
		uintptr(unsafe.Pointer(&s.buf[0])), uintptr(len(s.buf)), uintptr(unsafe.Pointer(&got)))
	if r != 0 {
		return nil, fmt.Errorf("NtQuerySystemInformation: 0x%x", r)
	}
	return ParseNtProcPerf(s.buf[:got-got%ntRecord])
}
