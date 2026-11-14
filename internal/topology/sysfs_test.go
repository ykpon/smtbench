package topology

import (
	"io/fs"
	"reflect"
	"testing"

	"smtbench/internal/topology/topotest"
)

func TestParseCPUList(t *testing.T) {
	got, err := ParseCPUList("0-3,8,10-11\n")
	if err != nil || !reflect.DeepEqual(got, []int{0, 1, 2, 3, 8, 10, 11}) {
		t.Fatalf("got %v %v", got, err)
	}
	got, err = ParseCPUList("5")
	if err != nil || !reflect.DeepEqual(got, []int{5}) {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range []string{"", "3-1", "a", "1-"} {
		if _, err := ParseCPUList(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func mustParse(t *testing.T, sys fs.FS) *Topology {
	t.Helper()
	top, err := ParseSysfs(sys)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func TestXeon(t *testing.T) {
	top := mustParse(t, topotest.Xeon())
	if len(top.CPUs) != 32 || len(top.Cores) != 16 || top.Packages != 2 || !top.HasSMT() {
		t.Fatalf("counts: cpus %d cores %d pkgs %d", len(top.CPUs), len(top.Cores), top.Packages)
	}
	if !reflect.DeepEqual(top.Cores[0].Threads, []int{0, 16}) || !reflect.DeepEqual(top.Cores[1].Threads, []int{1, 17}) {
		t.Fatalf("threads: %v %v", top.Cores[0].Threads, top.Cores[1].Threads)
	}
	if top.Cores[1].Package != 1 {
		t.Fatalf("core 1 package %d", top.Cores[1].Package)
	}
	if len(top.L3) != 2 || top.L3[0].SizeKB != 25600 || top.L3[0].Big {
		t.Fatalf("l3: %+v", top.L3)
	}
	if top.CPUs[1].NUMA != 1 {
		t.Fatalf("cpu1 numa %d", top.CPUs[1].NUMA)
	}
	if top.Distance(0, 2) != 0 || top.Distance(0, 1) != 2 {
		t.Fatalf("distance %d %d", top.Distance(0, 2), top.Distance(0, 1))
	}
}

func TestDualCCDX3D(t *testing.T) {
	top := mustParse(t, topotest.DualCCDX3D())
	if len(top.Cores) != 16 || len(top.L3) != 2 {
		t.Fatalf("cores %d l3 %d", len(top.Cores), len(top.L3))
	}
	if !top.L3[0].Big || top.L3[1].Big {
		t.Fatalf("big flags: %+v", top.L3)
	}
	if !reflect.DeepEqual(top.L3[0].Cores, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("l3[0] cores %v", top.L3[0].Cores)
	}
	if top.Distance(0, 8) != 1 || top.Distance(0, 7) != 0 {
		t.Fatalf("distance %d %d", top.Distance(0, 8), top.Distance(0, 7))
	}
}

func TestNoSMT(t *testing.T) {
	top := mustParse(t, topotest.NoSMT(8))
	if top.HasSMT() || len(top.Cores) != 8 {
		t.Fatalf("smt %v cores %d", top.HasSMT(), len(top.Cores))
	}
	for _, c := range top.Cores {
		if len(c.Threads) != 1 {
			t.Fatalf("core %d threads %v", c.ID, c.Threads)
		}
	}
}

func TestRestrict(t *testing.T) {
	top := mustParse(t, topotest.Xeon())
	r, err := top.Restrict([]int{0, 16, 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cores) != 2 || r.Cores[0].ID != 0 || r.Cores[1].ID != 1 {
		t.Fatalf("cores %+v", r.Cores)
	}
	if !reflect.DeepEqual(r.Cores[1].Threads, []int{1}) {
		t.Fatalf("core 1 threads %v", r.Cores[1].Threads)
	}
	if c, ok := r.CoreByID(1); !ok || c.ID != 1 {
		t.Fatal("CoreByID(1)")
	}
	if _, ok := r.CoreByID(2); ok {
		t.Fatal("CoreByID(2) should be gone")
	}
	if _, err := top.Restrict(nil); err == nil {
		t.Fatal("expected error for empty allowed set")
	}
}
