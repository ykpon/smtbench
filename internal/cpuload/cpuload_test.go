package cpuload

import (
	"encoding/binary"
	"testing"
)

const procStat = `cpu  100 0 50 1000 10 0 0 0 0 0
cpu0 10 1 5 100 4 2 3 0 0 0
cpu1 20 0 10 70 0 0 0 0 0 0
intr 12345
ctxt 999
`

func TestParseProcStat(t *testing.T) {
	m, err := ParseProcStat([]byte(procStat))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("cpus %v", m)
	}
	// cpu0: total 125, idle 100 + iowait 4
	if m[0] != (Times{Busy: 21, Total: 125}) {
		t.Fatalf("cpu0 %+v", m[0])
	}
	if m[1] != (Times{Busy: 30, Total: 100}) {
		t.Fatalf("cpu1 %+v", m[1])
	}
}

func TestParseNtProcPerf(t *testing.T) {
	buf := make([]byte, 96)
	put := func(rec, off int, v int64) { binary.LittleEndian.PutUint64(buf[rec*48+off:], uint64(v)) }
	put(0, 0, 70)  // idle
	put(0, 8, 90)  // kernel incl. idle
	put(0, 16, 10) // user
	put(1, 0, 0)
	put(1, 8, 5)
	put(1, 16, 5)
	m, err := ParseNtProcPerf(buf)
	if err != nil {
		t.Fatal(err)
	}
	if m[0] != (Times{Busy: 30, Total: 100}) || m[1] != (Times{Busy: 10, Total: 10}) {
		t.Fatalf("%+v", m)
	}
	if _, err := ParseNtProcPerf(buf[:50]); err == nil {
		t.Fatal("expected error for partial record")
	}
}

func TestBusyPct(t *testing.T) {
	a := map[int]Times{0: {10, 100}}
	b := map[int]Times{0: {60, 200}}
	if p := BusyPct(a, b, 0); p != 50 {
		t.Fatalf("%v", p)
	}
	if p := BusyPct(a, b, 7); p != 0 {
		t.Fatalf("missing cpu %v", p)
	}
}
