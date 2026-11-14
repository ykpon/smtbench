package affinity

import (
	"runtime"
	"testing"
	"time"
)

func spin(d time.Duration) {
	end := time.Now().Add(d)
	x := 0
	for time.Now().Before(end) {
		x++
	}
	_ = x
}

func TestPinAndCurrent(t *testing.T) {
	allowed, err := Allowed()
	if err != nil || len(allowed) == 0 {
		t.Fatalf("Allowed: %v %v", allowed, err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, c := range allowed[:min(2, len(allowed))] {
		if err := Pin(c); err != nil {
			t.Fatal(err)
		}
		spin(20 * time.Millisecond)
		got, err := Current()
		if err != nil || got != c {
			t.Fatalf("pinned to %d, running on %d (%v)", c, got, err)
		}
	}
}

func TestThreadCPUTime(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a, err := ThreadCPUTime()
	if err != nil {
		t.Fatal(err)
	}
	spin(100 * time.Millisecond)
	b, _ := ThreadCPUTime()
	if d := b - a; d < 60*time.Millisecond || d > 150*time.Millisecond {
		t.Fatalf("cpu time delta %v", d)
	}
}

func TestPinInvalid(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := Pin(100000); err == nil {
		t.Fatal("expected error")
	}
}
