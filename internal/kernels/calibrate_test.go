package kernels

import (
	"flag"
	"math"
	"runtime"
	"testing"
	"time"

	"smtbench/internal/affinity"
)

var calibrate = flag.Bool("calibrate", false, "measure ns per unit and suggest itersPerUnit")

// TestCalibrate prints ns per unit for every kernel on CPU 0.
// Run on the reference machine: scripts/remote-test.sh internal/kernels -test.run Calibrate -calibrate
func TestCalibrate(t *testing.T) {
	if !*calibrate {
		t.Skip("use -calibrate")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := affinity.Pin(0); err != nil {
		t.Fatal(err)
	}
	iters := map[string]int{"int": intItersPerUnit, "fp": fpItersPerUnit, "avx": avxItersPerUnit, "mem": memItersPerUnit}
	for _, name := range Names() {
		k, err := New(name, 64<<20)
		if err != nil {
			t.Logf("%s: %v", name, err)
			continue
		}
		run := func(d time.Duration) (units int, elapsed time.Duration) {
			start := time.Now()
			for time.Since(start) < d {
				k.Run(10)
				units += 10
			}
			return units, time.Since(start)
		}
		run(time.Second)
		units, elapsed := run(2 * time.Second)
		nsPerUnit := float64(elapsed.Nanoseconds()) / float64(units)
		nsPerIter := nsPerUnit / float64(iters[name])
		t.Logf("%-4s %8.1f ns/unit  suggested itersPerUnit = %d", name, nsPerUnit, int(math.Round(1000/nsPerIter)))
	}
}
