// Package x86 exposes RDTSC and CPUID.
package x86

import (
	"encoding/binary"
	"strings"
)

// RDTSC returns the time stamp counter (serialized with LFENCE).
func RDTSC() uint64

// CPUID executes the CPUID instruction.
func CPUID(eax, ecx uint32) (a, b, c, d uint32)

func Vendor() string {
	_, b, c, d := CPUID(0, 0)
	var buf [12]byte
	binary.LittleEndian.PutUint32(buf[0:], b)
	binary.LittleEndian.PutUint32(buf[4:], d)
	binary.LittleEndian.PutUint32(buf[8:], c)
	return string(buf[:])
}

func Brand() string {
	if maxExt, _, _, _ := CPUID(0x80000000, 0); maxExt < 0x80000004 {
		return ""
	}
	var buf [48]byte
	for i := uint32(0); i < 3; i++ {
		a, b, c, d := CPUID(0x80000002+i, 0)
		for j, r := range []uint32{a, b, c, d} {
			binary.LittleEndian.PutUint32(buf[i*16+uint32(j)*4:], r)
		}
	}
	return strings.TrimSpace(strings.TrimRight(string(buf[:]), "\x00"))
}

// InvariantTSC reports whether the TSC runs at a constant rate in all P/C states.
func InvariantTSC() bool {
	if maxExt, _, _, _ := CPUID(0x80000000, 0); maxExt < 0x80000007 {
		return false
	}
	_, _, _, d := CPUID(0x80000007, 0)
	return d&(1<<8) != 0
}
