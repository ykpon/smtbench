package topology

import (
	"reflect"
	"testing"

	"smtbench/internal/topology/topotest"
)

func TestRyzen5500X3D(t *testing.T) {
	top, err := ParseGLPIEx(topotest.Ryzen5500X3D())
	if err != nil {
		t.Fatal(err)
	}
	if len(top.CPUs) != 12 || len(top.Cores) != 6 || top.Packages != 1 || !top.HasSMT() {
		t.Fatalf("cpus %d cores %d pkgs %d", len(top.CPUs), len(top.Cores), top.Packages)
	}
	if !reflect.DeepEqual(top.Cores[0].Threads, []int{0, 1}) || !reflect.DeepEqual(top.Cores[5].Threads, []int{10, 11}) {
		t.Fatalf("threads %v %v", top.Cores[0].Threads, top.Cores[5].Threads)
	}
	if len(top.L3) != 1 || top.L3[0].SizeKB != 98304 || top.L3[0].Big {
		t.Fatalf("l3 %+v", top.L3)
	}
}

func TestGLPIExTruncated(t *testing.T) {
	buf := topotest.Ryzen5500X3D()
	if _, err := ParseGLPIEx(buf[:len(buf)-5]); err == nil {
		t.Fatal("expected error")
	}
}
