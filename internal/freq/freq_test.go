package freq

import (
	"runtime"
	"testing"
	"time"

	"smtbench/internal/affinity"
)

func TestFreqPlausible(t *testing.T) {
	src := Detect()
	t.Logf("source %s", src.Name())
	if src.Name() == "none" {
		t.Skip("no frequency source on this machine")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	allowed, _ := affinity.Allowed()
	if err := affinity.Pin(allowed[0]); err != nil {
		t.Fatal(err)
	}
	c, err := src.Open(allowed[0])
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c0, err := c.Cycles()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	x := 0
	for time.Since(start) < 200*time.Millisecond {
		x++
	}
	c1, _ := c.Cycles()
	mhz := float64(c1-c0) / float64(time.Since(start).Microseconds())
	t.Logf("%.0f MHz", mhz)
	if mhz < 800 || mhz > 6500 {
		t.Fatalf("implausible %.0f MHz", mhz)
	}
}
