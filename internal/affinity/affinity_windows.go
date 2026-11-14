//go:build windows

package affinity

import (
	"fmt"
	"math/bits"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                         = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadGroupAffinity       = kernel32.NewProc("SetThreadGroupAffinity")
	procGetCurrentProcessorNumberEx  = kernel32.NewProc("GetCurrentProcessorNumberEx")
	procGetThreadTimes               = kernel32.NewProc("GetThreadTimes")
	procGetProcessAffinityMask       = kernel32.NewProc("GetProcessAffinityMask")
	procGetActiveProcessorGroupCount = kernel32.NewProc("GetActiveProcessorGroupCount")
)

type groupAffinity struct {
	Mask     uint64
	Group    uint16
	Reserved [3]uint16
}

type processorNumber struct {
	Group    uint16
	Number   uint8
	Reserved uint8
}

func Pin(cpu int) error {
	ga := groupAffinity{Mask: 1 << (cpu % 64), Group: uint16(cpu / 64)}
	if cpu < 0 || cpu/64 > 0xFFFF {
		return fmt.Errorf("CPU %d вне допустимого диапазона", cpu)
	}
	r, _, err := procSetThreadGroupAffinity.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(&ga)), 0)
	if r == 0 {
		return fmt.Errorf("привязка к CPU %d: %w", cpu, err)
	}
	return nil
}

func Current() (int, error) {
	var pn processorNumber
	procGetCurrentProcessorNumberEx.Call(uintptr(unsafe.Pointer(&pn)))
	return int(pn.Group)*64 + int(pn.Number), nil
}

func Allowed() ([]int, error) {
	groups, _, _ := procGetActiveProcessorGroupCount.Call()
	var out []int
	if groups <= 1 {
		var procMask, sysMask uintptr
		r, _, err := procGetProcessAffinityMask.Call(uintptr(windows.CurrentProcess()),
			uintptr(unsafe.Pointer(&procMask)), uintptr(unsafe.Pointer(&sysMask)))
		if r == 0 {
			return nil, err
		}
		m := uint64(procMask)
		for m != 0 {
			b := bits.TrailingZeros64(m)
			out = append(out, b)
			m &^= 1 << b
		}
		return out, nil
	}
	for g := 0; g < int(groups); g++ {
		n := int(windows.GetActiveProcessorCount(uint16(g)))
		for i := 0; i < n; i++ {
			out = append(out, g*64+i)
		}
	}
	return out, nil
}

func ThreadCPUTime() (time.Duration, error) {
	var creation, exit, kernel, user windows.Filetime
	r, _, err := procGetThreadTimes.Call(uintptr(windows.CurrentThread()),
		uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0, err
	}
	ft := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return time.Duration((ft(kernel) + ft(user)) * 100), nil
}
