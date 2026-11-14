//go:build linux

package cpuload

import "os"

type procStatSampler struct{}

func NewSampler() Sampler { return procStatSampler{} }

func (procStatSampler) Sample() (map[int]Times, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	return ParseProcStat(b)
}
