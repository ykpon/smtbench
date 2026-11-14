package kernels

import (
	"math"

	"golang.org/x/sys/cpu"
)

// hasAVX is a variable so tests can simulate a CPU without AVX.
var hasAVX = cpu.X86.HasAVX

//go:noescape
func avxLoop(n int, acc *[64]float32, c *[16]float32)

// avxKernel runs 256-bit AVX1 multiply and add chains (no AVX2/FMA, so the
// same work is done on every CPU with AVX).
type avxKernel struct {
	acc [64]float32
	c   [16]float32
}

func newAVX(int) (Kernel, error) {
	if !hasAVX {
		return nil, ErrUnsupported
	}
	k := &avxKernel{}
	for i := range k.acc {
		k.acc[i] = float32(i) + 1
	}
	for i := 0; i < 8; i++ {
		k.c[i] = 0.999999
		k.c[8+i] = 1e-6
	}
	return k, nil
}

func (k *avxKernel) Name() string { return "avx" }

func (k *avxKernel) Run(units int) uint64 {
	avxLoop(units*avxItersPerUnit, &k.acc, &k.c)
	var s float64
	for _, v := range k.acc {
		s += float64(v)
	}
	return math.Float64bits(s)
}

func init() { registry["avx"] = newAVX }
