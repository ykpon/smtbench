// Package timing provides a cheap clock for per-tick measurements.
package timing

import (
	"time"

	"smtbench/internal/x86"
)

// Clock converts raw counter values to microseconds.
// TSC is false when the CPU has no invariant TSC; Now then uses time.Now.
type Clock struct {
	TicksPerUs float64
	TSC        bool
}

var epoch = time.Now()

// Calibrate measures the TSC rate against the system clock over d.
func Calibrate(d time.Duration) Clock {
	if !x86.InvariantTSC() {
		return Clock{TicksPerUs: 1000, TSC: false}
	}
	t0, c0 := time.Now(), x86.RDTSC()
	time.Sleep(d)
	t1, c1 := time.Now(), x86.RDTSC()
	return Clock{TicksPerUs: float64(c1-c0) / float64(t1.Sub(t0).Microseconds()), TSC: true}
}

func (c Clock) Now() uint64 {
	if c.TSC {
		return x86.RDTSC()
	}
	return uint64(time.Since(epoch).Nanoseconds())
}

func (c Clock) Us(delta uint64) float64 { return float64(delta) / c.TicksPerUs }
