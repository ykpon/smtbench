// Package hist is a fixed-size histogram of durations in nanoseconds
// with about 1% relative precision.
package hist

import "math/bits"

const (
	subBits = 6
	sub     = 1 << subBits
	nBucket = (64 - subBits + 1) * sub
)

type Hist struct {
	counts [nBucket]uint32
	total  uint64
}

func index(v uint64) int {
	if v < sub {
		return int(v)
	}
	e := bits.Len64(v) - subBits - 1
	return (e+1)*sub + int(v>>e) - sub
}

func value(i int) float64 {
	if i < sub {
		return float64(i)
	}
	e := i/sub - 1
	m := uint64(i%sub + sub)
	lo := m << e
	return float64(lo) + float64(uint64(1)<<e)/2
}

func (h *Hist) Add(ns uint64) {
	h.counts[index(ns)]++
	h.total++
}

func (h *Hist) Count() uint64 { return h.total }

func (h *Hist) Reset() { *h = Hist{} }

// Quantile returns the value at quantile q (0..1) in nanoseconds.
func (h *Hist) Quantile(q float64) float64 {
	if h.total == 0 {
		return 0
	}
	rank := uint64(q*float64(h.total-1)) + 1
	var seen uint64
	for i, c := range h.counts {
		seen += uint64(c)
		if seen >= rank {
			return value(i)
		}
	}
	return value(nBucket - 1)
}
