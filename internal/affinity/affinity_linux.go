//go:build linux

package affinity

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func Pin(cpu int) error {
	var set unix.CPUSet
	set.Set(cpu)
	if set.Count() == 0 {
		return fmt.Errorf("CPU %d вне допустимого диапазона", cpu)
	}
	if err := unix.SchedSetaffinity(0, &set); err != nil {
		return fmt.Errorf("привязка к CPU %d: %w", cpu, err)
	}
	return nil
}

func Current() (int, error) {
	var cpu uint32
	_, _, errno := unix.RawSyscall(unix.SYS_GETCPU, uintptr(unsafe.Pointer(&cpu)), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(cpu), nil
}

func Allowed() ([]int, error) {
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(0, &set); err != nil {
		return nil, err
	}
	var out []int
	for i := 0; i < len(set)*64; i++ {
		if set.IsSet(i) {
			out = append(out, i)
		}
	}
	return out, nil
}

func ThreadCPUTime() (time.Duration, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &ts); err != nil {
		return 0, err
	}
	return time.Duration(ts.Nano()), nil
}
