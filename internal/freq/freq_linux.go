//go:build linux

package freq

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const msrAPERF = 0xE8

// Detect prefers APERF via /dev/cpu/N/msr (root and the msr module),
// then the perf cycles counter of the calling thread.
func Detect() Source {
	if c, err := (msrSource{}).Open(0); err == nil {
		_, err = c.Cycles()
		c.Close()
		if err == nil {
			return msrSource{}
		}
	}
	if c, err := (perfSource{}).Open(-1); err == nil {
		c.Close()
		return perfSource{}
	}
	return none{}
}

type msrSource struct{}

func (msrSource) Name() string { return "msr" }

func (msrSource) Open(cpu int) (Counter, error) {
	fd, err := unix.Open(fmt.Sprintf("/dev/cpu/%d/msr", cpu), unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return &fdCounter{fd: fd, off: msrAPERF}, nil
}

type perfSource struct{}

func (perfSource) Name() string { return "perf" }

// Open counts cycles of the calling thread on whatever CPU it runs.
func (perfSource) Open(int) (Counter, error) {
	attr := unix.PerfEventAttr{
		Type:   unix.PERF_TYPE_HARDWARE,
		Config: unix.PERF_COUNT_HW_CPU_CYCLES,
	}
	attr.Size = uint32(unsafe.Sizeof(attr))
	fd, err := unix.PerfEventOpen(&attr, 0, -1, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &fdCounter{fd: fd, off: -1}, nil
}

// fdCounter reads 8 bytes: with pread at off for msr, with read for perf.
type fdCounter struct {
	fd  int
	off int64
}

func (c *fdCounter) Cycles() (uint64, error) {
	var b [8]byte
	var err error
	if c.off >= 0 {
		_, err = unix.Pread(c.fd, b[:], c.off)
	} else {
		_, err = unix.Read(c.fd, b[:])
	}
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b[:]), nil
}

func (c *fdCounter) Close() error { return unix.Close(c.fd) }
