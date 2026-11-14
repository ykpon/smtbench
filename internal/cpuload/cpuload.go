// Package cpuload samples per-CPU busy time to detect load from other processes.
package cpuload

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
)

// Times are cumulative per-CPU counters in arbitrary units.
type Times struct{ Busy, Total uint64 }

type Sampler interface {
	Sample() (map[int]Times, error)
}

// ParseProcStat parses per-CPU lines of /proc/stat. Idle and iowait count as not busy.
func ParseProcStat(b []byte) (map[int]Times, error) {
	m := map[int]Times{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		id, err := strconv.Atoi(f[0][3:])
		if err != nil {
			continue
		}
		var t Times
		var idle uint64
		for i, s := range f[1:] {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return nil, err
			}
			if i >= 8 { // guest and guest_nice are already included in user
				break
			}
			t.Total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		t.Busy = t.Total - idle
		m[id] = t
	}
	if len(m) == 0 {
		return nil, errors.New("в /proc/stat нет строк cpuN")
	}
	return m, nil
}

const ntRecord = 48

// ParseNtProcPerf parses SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION records:
// IdleTime@0, KernelTime@8 (includes idle), UserTime@16.
func ParseNtProcPerf(b []byte) (map[int]Times, error) {
	if len(b) == 0 || len(b)%ntRecord != 0 {
		return nil, errors.New("неверный размер данных NtQuerySystemInformation")
	}
	m := map[int]Times{}
	for i := 0; i*ntRecord < len(b); i++ {
		r := b[i*ntRecord:]
		idle := binary.LittleEndian.Uint64(r[0:])
		kernel := binary.LittleEndian.Uint64(r[8:])
		user := binary.LittleEndian.Uint64(r[16:])
		m[i] = Times{Busy: kernel - idle + user, Total: kernel + user}
	}
	return m, nil
}

// BusyPct returns the busy share of cpu between samples a and b, in percent.
func BusyPct(a, b map[int]Times, cpu int) float64 {
	x, ok1 := a[cpu]
	y, ok2 := b[cpu]
	if !ok1 || !ok2 || y.Total <= x.Total {
		return 0
	}
	return 100 * float64(y.Busy-x.Busy) / float64(y.Total-x.Total)
}
