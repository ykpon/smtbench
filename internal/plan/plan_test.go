package plan

import (
	"io/fs"
	"reflect"
	"testing"
	"time"

	"smtbench/internal/result"
	"smtbench/internal/topology"
	"smtbench/internal/topology/topotest"
)

func topo(t *testing.T, sys fs.FS) *topology.Topology {
	t.Helper()
	top, err := topology.ParseSysfs(sys)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func xeon(t *testing.T) *topology.Topology {
	return topo(t, topotest.Xeon())
}

func defaults() result.Params {
	return result.Params{DurationMS: 3000, WarmupMS: 1000, Reps: 1, Step: 1, Order: "near", BgThreads: 0, Modes: []string{"single", "solo_b", "pair"}}
}

func TestLoadOrderXeonNear(t *testing.T) {
	top := xeon(t)
	order, err := LoadOrder(top, 0, "near")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 4, 6, 8, 10, 12, 14, 1, 3, 5, 7, 9, 11, 13, 15}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order %v", order)
	}
	if b := Boundary(top, 0, order); b != 7 {
		t.Fatalf("boundary %d", b)
	}
}

func TestLoadOrderX3DFar(t *testing.T) {
	top := topo(t, topotest.DualCCDX3D())
	order, err := LoadOrder(top, 0, "far")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order[:8], []int{8, 9, 10, 11, 12, 13, 14, 15}) {
		t.Fatalf("order %v", order)
	}
	if b := Boundary(top, 0, order); b != 8 {
		t.Fatalf("boundary %d", b)
	}
}

func TestSteps(t *testing.T) {
	all := make([]int, 16)
	for i := range all {
		all[i] = i
	}
	if s := Steps(15, 1, 7); !reflect.DeepEqual(s, all) {
		t.Fatalf("%v", s)
	}
	if s := Steps(15, 4, 7); !reflect.DeepEqual(s, []int{0, 4, 7, 8, 12, 15}) {
		t.Fatalf("%v", s)
	}
	if s := Steps(15, 20, 0); !reflect.DeepEqual(s, []int{0, 15}) {
		t.Fatalf("%v", s)
	}
	if s := Steps(0, 1, 0); !reflect.DeepEqual(s, []int{0}) {
		t.Fatalf("%v", s)
	}
}

func TestBuildXeonDefaults(t *testing.T) {
	ws, err := Build(xeon(t), defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 16*(16+16+16+1) {
		t.Fatalf("windows %d", len(ws))
	}
	w0 := ws[0]
	if w0.Core != 0 || w0.Mode != Single || w0.LoadedCores != 0 || !reflect.DeepEqual(w0.Measure, []int{0}) || len(w0.Bg) != 0 {
		t.Fatalf("first %+v", w0)
	}
	var soloB, pair, final *Window
	for i := range ws[:49] {
		switch ws[i].Mode {
		case SoloB:
			if soloB == nil {
				soloB = &ws[i]
			}
		case Pair:
			if pair == nil {
				pair = &ws[i]
			}
		case Final:
			final = &ws[i]
		}
	}
	if !reflect.DeepEqual(soloB.Measure, []int{16}) || !reflect.DeepEqual(pair.Measure, []int{0, 16}) || !reflect.DeepEqual(final.Measure, []int{0}) {
		t.Fatalf("soloB %+v pair %+v final %+v", soloB, pair, final)
	}
	last := ws[15] // single, k=15
	if last.LoadedCores != 15 || len(last.Bg) != 15 || last.OutsideL3 != 8 {
		t.Fatalf("single kmax %+v", last)
	}
	if ws[48].Mode != Final || ws[49].Core != 1 {
		t.Fatalf("ordering: %+v %+v", ws[48], ws[49])
	}
}

func TestBuildBgThreads1(t *testing.T) {
	p := defaults()
	p.BgThreads = 1
	ws, _ := Build(xeon(t), p)
	for _, w := range ws {
		if w.Mode == Pair && w.LoadedCores == 3 {
			if len(w.Bg) != 3 {
				t.Fatalf("bg %v", w.Bg)
			}
			return
		}
	}
	t.Fatal("no pair k=3 window")
}

func TestBuildNoSMT(t *testing.T) {
	ws, err := Build(topo(t, topotest.NoSMT(8)), defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		if w.Mode != Single && w.Mode != Final {
			t.Fatalf("unexpected %s", w.Mode)
		}
	}
	if len(ws) != 8*(8+1) {
		t.Fatalf("windows %d", len(ws))
	}
}

func TestBuildRestricted(t *testing.T) {
	top := xeon(t)
	var allowed []int
	for i := 0; i < 32; i++ {
		if i != 17 {
			allowed = append(allowed, i)
		}
	}
	r, _ := top.Restrict(allowed)
	ws, err := Build(r, defaults())
	if err != nil {
		t.Fatal(err)
	}
	pairs := map[int]bool{}
	for _, w := range ws {
		if w.Mode == Pair || w.Mode == SoloB {
			pairs[w.Core] = true
		}
	}
	if pairs[1] || !pairs[0] || !pairs[2] {
		t.Fatalf("pair cores %v", pairs)
	}
}

func TestBuildCoresFilter(t *testing.T) {
	p := defaults()
	p.Cores = []int{0, 3}
	ws, _ := Build(xeon(t), p)
	for _, w := range ws {
		if w.Core != 0 && w.Core != 3 {
			t.Fatalf("core %d", w.Core)
		}
	}
	if len(ws) != 2*49 {
		t.Fatalf("windows %d", len(ws))
	}
}

func TestEstimate(t *testing.T) {
	ws, _ := Build(xeon(t), defaults())
	if d := Estimate(ws, defaults()); d != 52*time.Minute+16*time.Second {
		t.Fatalf("estimate %v", d)
	}
}

func TestBuildErrors(t *testing.T) {
	top := xeon(t)
	muts := map[string]func(*result.Params){
		"core":    func(p *result.Params) { p.Cores = []int{99} },
		"order":   func(p *result.Params) { p.Order = "x" },
		"bg":      func(p *result.Params) { p.BgThreads = 3 },
		"modes":   func(p *result.Params) { p.Modes = nil },
		"badmode": func(p *result.Params) { p.Modes = []string{"zzz"} },
	}
	for name, m := range muts {
		p := defaults()
		m(&p)
		if _, err := Build(top, p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// With bg-threads auto, neighbours are loaded the same way as the tested core.
func TestBuildBgAuto(t *testing.T) {
	ws, _ := Build(xeon(t), defaults())
	seen := map[Mode]int{}
	for _, w := range ws {
		if w.Core == 0 && w.LoadedCores == 3 {
			seen[w.Mode] = len(w.Bg)
		}
	}
	if seen[Single] != 3 || seen[SoloB] != 3 || seen[Pair] != 6 {
		t.Fatalf("bg cpus at k=3: %v", seen)
	}
}

func TestBuildBgForced2(t *testing.T) {
	p := defaults()
	p.BgThreads = 2
	ws, _ := Build(xeon(t), p)
	if w := ws[3]; w.Mode != Single || w.LoadedCores != 3 || len(w.Bg) != 6 {
		t.Fatalf("%+v", w)
	}
}

// Without solo_b in modes, thread B alone is measured only on the idle machine.
func TestBuildWithoutSoloBRamp(t *testing.T) {
	p := defaults()
	p.Modes = []string{"single", "pair"}
	ws, _ := Build(xeon(t), p)
	if len(ws) != 16*(16+1+16+1) {
		t.Fatalf("windows %d", len(ws))
	}
	for _, w := range ws {
		if w.Mode == SoloB && w.LoadedCores != 0 {
			t.Fatalf("%+v", w)
		}
	}
}
