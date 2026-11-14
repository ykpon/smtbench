package hist

import (
	"math"
	"testing"
)

func near(got, want float64) bool { return math.Abs(got-want)/want <= 0.02 }

func TestHistQuantiles(t *testing.T) {
	var h Hist
	for i := 0; i < 990; i++ {
		h.Add(100_000)
	}
	for i := 0; i < 10; i++ {
		h.Add(1_000_000)
	}
	if h.Count() != 1000 {
		t.Fatalf("count %d", h.Count())
	}
	if q := h.Quantile(0.5); !near(q, 100_000) {
		t.Fatalf("p50 %v", q)
	}
	if q := h.Quantile(0.995); !near(q, 1_000_000) {
		t.Fatalf("p99.5 %v", q)
	}
	h.Reset()
	if h.Count() != 0 {
		t.Fatal("reset")
	}
}

func TestHistPrecision(t *testing.T) {
	for _, v := range []uint64{1_000, 1_234, 77_777, 123_456_789, 9_999_999_999} {
		var h Hist
		h.Add(v)
		if q := h.Quantile(0.5); !near(q, float64(v)) {
			t.Errorf("%d -> %v", v, q)
		}
	}
}

func TestHistAddNoAlloc(t *testing.T) {
	var h Hist
	if n := testing.AllocsPerRun(1000, func() { h.Add(12345) }); n != 0 {
		t.Fatalf("allocs %v", n)
	}
}
