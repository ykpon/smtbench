package kernels

import "math/bits"

// intKernel mimics game logic: PRNG-driven unpredictable branches,
// small table lookups in L1 and integer multiplies.
type intKernel struct {
	state uint64
	acc   uint64
	table [1024]uint32
}

func newInt() *intKernel {
	k := &intKernel{state: 0x9E3779B97F4A7C15}
	for i := range k.table {
		k.table[i] = uint32(i * 2654435761)
	}
	return k
}

func (k *intKernel) Name() string { return "int" }

func (k *intKernel) Run(units int) uint64 {
	s, acc := k.state, k.acc
	for i := units * intItersPerUnit; i > 0; i-- {
		s ^= s >> 12
		s ^= s << 25
		s ^= s >> 27
		r := s * 0x2545F4914F6CDD1D
		idx := r >> 54
		v := uint64(k.table[idx])
		if r&1 != 0 {
			acc += v * 3
		} else {
			acc ^= v << 1
		}
		if r&0xF0 == 0 {
			acc = acc*31 + r
		}
		switch (r >> 8) & 3 {
		case 0:
			acc += r >> 3
		case 1:
			acc -= v
		case 2:
			acc ^= r
		default:
			acc = bits.RotateLeft64(acc, 7)
		}
		k.table[idx] = uint32(acc)
	}
	k.state, k.acc = s, acc
	return acc | 1
}
