package kernels

import "fmt"

const nodeWords = 16 // 64-byte node = one cache line

// memKernel walks a random single-cycle chain of cache lines (pointer chasing).
type memKernel struct {
	buf []uint32
	pos uint32
	acc uint64
}

func newMem(memBytes int) (Kernel, error) {
	n := memBytes / (nodeWords * 4)
	if n < 1024 {
		return nil, fmt.Errorf("размер буфера памяти слишком мал: %d байт", memBytes)
	}
	chain := newChain(n, 0x5DEECE66D)
	buf := make([]uint32, n*nodeWords)
	for i, next := range chain {
		buf[i*nodeWords] = next
	}
	return &memKernel{buf: buf}, nil
}

func (k *memKernel) Name() string { return "mem" }

func (k *memKernel) Run(units int) uint64 {
	pos, acc := k.pos, k.acc
	for n := units * memItersPerUnit; n > 0; n-- {
		pos = k.buf[pos*nodeWords]
		acc += uint64(pos)
	}
	k.pos, k.acc = pos, acc
	return acc | 1
}

// newChain returns next[] forming one cycle through all n nodes (Sattolo's algorithm).
func newChain(n int, seed uint64) []uint32 {
	perm := make([]uint32, n)
	for i := range perm {
		perm[i] = uint32(i)
	}
	s := seed
	for i := n - 1; i > 0; i-- {
		s = s*6364136223846793005 + 1442695040888963407
		j := int((s >> 33) % uint64(i))
		perm[i], perm[j] = perm[j], perm[i]
	}
	next := make([]uint32, n)
	for i := 0; i < n; i++ {
		next[perm[i]] = perm[(i+1)%n]
	}
	return next
}
