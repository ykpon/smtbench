package kernels

import (
	"errors"
	"math"
	"testing"

	"golang.org/x/sys/cpu"
)

func TestAVXUnsupported(t *testing.T) {
	saved := hasAVX
	hasAVX = false
	defer func() { hasAVX = saved }()
	if _, err := New("avx", 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("New: %v", err)
	}
	if _, err := NewTick(Mix{"avx": 1}, 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("NewTick: %v", err)
	}
}

func TestAVXRuns(t *testing.T) {
	if !cpu.X86.HasAVX {
		t.Skip("no AVX")
	}
	a, err := New("avx", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New("avx", 0)
	sa, sb := a.Run(3), b.Run(3)
	if sa != sb || sa == 0 || math.IsNaN(math.Float64frombits(sa)) {
		t.Fatalf("sinks %x %x", sa, sb)
	}
}
