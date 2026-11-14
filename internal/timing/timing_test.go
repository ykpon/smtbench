package timing

import (
	"testing"
	"time"
)

func TestCalibrate(t *testing.T) {
	c := Calibrate(200 * time.Millisecond)
	if c.TicksPerUs <= 0 {
		t.Fatalf("ticks/us %v", c.TicksPerUs)
	}
	a := c.Now()
	time.Sleep(100 * time.Millisecond)
	us := c.Us(c.Now() - a)
	if us < 90_000 || us > 130_000 {
		t.Fatalf("measured %v us", us)
	}
}
