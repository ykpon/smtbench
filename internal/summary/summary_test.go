package summary

import (
	"math"
	"strings"
	"testing"

	"smtbench/internal/result"
)

type pt struct {
	core    int
	mode    string
	k       int
	outside int
	tps     []float64 // per measured thread
	p99     []float64
	mhz     float64
	noisy   bool
}

func mkRun(pts []pt) *result.Run {
	r := &result.Run{SchemaVersion: 1, RunID: strings.Repeat("a", 32)}
	for _, p := range pts {
		rp := result.Point{Core: p.core, Mode: p.mode, LoadedCores: p.k, OutsideL3: p.outside, Noisy: p.noisy}
		// neighbours are loaded like the tested core: 1 thread, or 2 in pair mode
		bg := p.k
		if p.mode == "pair" {
			bg = 2 * p.k
		}
		rp.LoadedCPUs = make([]int, bg)
		for i, v := range p.tps {
			p99 := 100.0
			if i < len(p.p99) {
				p99 = p.p99[i]
			}
			rp.Threads = append(rp.Threads, result.ThreadResult{CPU: p.core*2 + i, TicksPerSec: v, TickP50us: 90, TickP99us: p99, MHz: p.mhz})
		}
		r.Points = append(r.Points, rp)
	}
	return r
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestHT(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "solo_b", 0, 0, []float64{1000}, nil, 0, false},
		{0, "pair", 0, 0, []float64{600, 600}, nil, 0, false},
	}))
	if !s.HasPair || !approx(s.HTPerThreadPct, 60) || !approx(s.HTTotalPct, 120) {
		t.Fatalf("%+v", s)
	}
	if s.Ratings["ht"] != Warn {
		t.Fatalf("ratings %v", s.Ratings)
	}
}

func TestFullAndKnee(t *testing.T) {
	var pts []pt
	for k, v := range []float64{1000, 990, 960, 900, 850} {
		pts = append(pts, pt{0, "single", k, 0, []float64{v}, nil, 0, false})
	}
	s := Compute(mkRun(pts))
	if !approx(s.FullSinglePct, 85) || s.Ratings["full_single"] != Warn || s.KneeSingle != 2 {
		t.Fatalf("full %v rating %v knee %d", s.FullSinglePct, s.Ratings["full_single"], s.KneeSingle)
	}
	if len(s.Avg.Loaded) != 5 || !approx(s.Avg.Single[3], 90) {
		t.Fatalf("avg %+v", s.Avg)
	}
}

func TestLag(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, []float64{100}, 0, false},
		{0, "single", 1, 0, []float64{900}, []float64{120}, 0, false},
		{0, "pair", 0, 0, []float64{600, 600}, []float64{150, 150}, 0, false},
		{0, "pair", 1, 0, []float64{500, 500}, []float64{180, 200}, 0, false},
	}))
	if !approx(s.LagRatio, 2.0) || s.Ratings["lag"] != Warn {
		t.Fatalf("lag %v %v", s.LagRatio, s.Ratings["lag"])
	}
}

func TestWeakAndDrift(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "final", 0, 0, []float64{960}, nil, 0, false},
		{1, "single", 0, 0, []float64{900}, nil, 0, false},
		{1, "final", 0, 0, []float64{900}, nil, 0, false},
		{2, "single", 0, 0, []float64{1000}, nil, 0, false},
		{2, "final", 0, 0, []float64{1000}, nil, 0, false},
	}))
	if len(s.WeakCores) != 1 || s.WeakCores[0] != 1 {
		t.Fatalf("weak %v", s.WeakCores)
	}
	if !s.Cores[0].Drift || s.Cores[1].Drift {
		t.Fatalf("drift %+v", s.Cores)
	}
	found := false
	for _, w := range s.Warnings {
		found = found || strings.Contains(w, "ядрах 0:")
	}
	if !found {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

func TestNoSMT(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "single", 1, 0, []float64{950}, nil, 0, false},
	}))
	if s.HasPair {
		t.Fatal("HasPair")
	}
	if _, ok := s.Ratings["ht"]; ok {
		t.Fatal("ht rating without pair")
	}
	if _, ok := s.Ratings["full_pair"]; ok {
		t.Fatal("full_pair rating without pair")
	}
}

func TestBoundary(t *testing.T) {
	near := []int{0, 0, 0, 1, 2}
	far := []int{0, 1, 2, 2, 2}
	same := []int{0, 0, 0, 0, 0}
	for name, c := range map[string]struct {
		outside []int
		want    int
	}{"near": {near, 2}, "far": {far, 2}, "none": {same, -1}} {
		var pts []pt
		for k, o := range c.outside {
			pts = append(pts, pt{0, "single", k, o, []float64{1000 - float64(k)}, nil, 0, false})
		}
		if b := Compute(mkRun(pts)).Avg.Boundary; b != c.want {
			t.Errorf("%s: boundary %d want %d", name, b, c.want)
		}
	}
}

func TestFreqAndNoise(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 4000, false},
		{0, "single", 1, 0, []float64{900}, nil, 3600, true},
	}))
	if !s.HasFreq || !approx(s.Avg.FreqPct[1], 90) || !s.Avg.Noisy[1] || s.Avg.Noisy[0] {
		t.Fatalf("%+v", s.Avg)
	}
}

func TestRateThresholds(t *testing.T) {
	cases := []struct {
		m    string
		v    float64
		want Rating
	}{
		{"full_single", 90, Good}, {"full_single", 89.9, Warn}, {"full_single", 74.9, Bad},
		{"ht", 70, Good}, {"ht", 55, Warn}, {"ht", 54.9, Bad},
		{"full_pair", 65, Good}, {"full_pair", 50, Warn}, {"full_pair", 49, Bad},
		{"lag", 1.5, Good}, {"lag", 3, Warn}, {"lag", 3.01, Bad},
	}
	for _, c := range cases {
		if got := Rate(c.m, c.v); got != c.want {
			t.Errorf("%s %v: %s want %s", c.m, c.v, got, c.want)
		}
	}
}

func TestWarningsUseDecimalComma(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "final", 0, 0, []float64{838}, nil, 0, false},
	}))
	for _, w := range s.Warnings {
		if strings.Contains(w, "16.2") || (strings.Contains(w, "ядрах 0") && !strings.Contains(w, "16,2%")) {
			t.Fatalf("warning %q", w)
		}
	}
}

func TestBaseAveragesFinal(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "solo_b", 0, 0, []float64{1050}, nil, 0, false},
		{0, "pair", 0, 0, []float64{1050, 1050}, nil, 0, false},
		{0, "final", 0, 0, []float64{1100}, nil, 0, false},
	}))
	if !approx(s.Cores[0].BaseTPS, 1050) || !approx(s.HTPerThreadPct, 100) {
		t.Fatalf("base %v ht %v", s.Cores[0].BaseTPS, s.HTPerThreadPct)
	}
}

func TestDriftWarningAggregated(t *testing.T) {
	var pts []pt
	for c := 0; c < 3; c++ {
		pts = append(pts, pt{c, "single", 0, 0, []float64{1000}, nil, 3600, false},
			pt{c, "final", 0, 0, []float64{1000 + 30*float64(c+2)}, nil, 3900, false})
	}
	s := Compute(mkRun(pts))
	n := 0
	for _, w := range s.Warnings {
		if strings.Contains(w, "ядрах") {
			n++
			if !strings.Contains(w, "ядрах 0, 1, 2:") || !strings.Contains(w, "до 12,0%") || !strings.Contains(w, "частот") {
				t.Fatalf("warning %q", w)
			}
		}
	}
	if n != 1 {
		t.Fatalf("drift warnings %d: %v", n, s.Warnings)
	}
}

func TestFrequencyMHz(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 3600, false},
		{0, "single", 1, 0, []float64{950}, nil, 3500, false},
		{0, "pair", 0, 0, []float64{800, 800}, nil, 3580, false},
		{0, "pair", 1, 0, []float64{760, 760}, nil, 3400, false},
		{0, "final", 0, 0, []float64{1000}, nil, 3800, false},
	}))
	c := s.Cores[0]
	if !approx(c.MHzIdle, 3700) || !approx(c.MHzFullSingle, 3500) || !approx(c.MHzFullPair, 3400) {
		t.Fatalf("core mhz %v %v %v", c.MHzIdle, c.MHzFullSingle, c.MHzFullPair)
	}
	if !approx(c.Curve.MHz[1], 3500) || !approx(c.Curve.PairMHz[0], 3580) {
		t.Fatalf("curve mhz %v %v", c.Curve.MHz, c.Curve.PairMHz)
	}
	if !approx(s.MHzIdle, 3700) || !approx(s.MHzFullPair, 3400) || !approx(s.Avg.MHz[0], 3600) {
		t.Fatalf("summary mhz %v %v %v", s.MHzIdle, s.MHzFullPair, s.Avg.MHz)
	}
}

func TestFrequencyMissing(t *testing.T) {
	s := Compute(mkRun([]pt{{0, "single", 0, 0, []float64{1000}, nil, 0, false}}))
	if s.MHzIdle != -1 || s.Cores[0].MHzIdle != -1 || s.Avg.MHz[0] != -1 {
		t.Fatalf("%v %v %v", s.MHzIdle, s.Cores[0].MHzIdle, s.Avg.MHz)
	}
}

func TestThreadsSeparately(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "single", 1, 0, []float64{950}, nil, 0, false},
		{0, "solo_b", 0, 0, []float64{1000}, nil, 0, false},
		{0, "solo_b", 1, 0, []float64{940}, nil, 0, false},
		{0, "pair", 0, 0, []float64{820, 780}, nil, 0, false},
		{0, "pair", 1, 0, []float64{760, 740}, nil, 0, false},
	}))
	cv := s.Cores[0].Curve
	check := func(name string, got []float64, want ...float64) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %v", name, got)
		}
		for i := range want {
			if !approx(got[i], want[i]) {
				t.Fatalf("%s: %v want %v", name, got, want)
			}
		}
	}
	check("single", cv.Single, 100, 95)
	check("solo_b", cv.SoloB, 100, 94)
	check("pair a", cv.PairA, 82, 76)
	check("pair b", cv.PairB, 78, 74)
	check("pair sum", cv.PairSum, 160, 150)
	check("pair", cv.Pair, 80, 75)
	check("avg pair a", s.Avg.PairA, 82, 76)
	if !approx(s.Cores[0].HTAPct, 82) || !approx(s.HTBPct, 78) {
		t.Fatalf("ht a %v b %v", s.Cores[0].HTAPct, s.HTBPct)
	}
}

func nodeRun() *result.Run {
	var pts []pt
	for k, v := range []float64{1000, 980, 950, 900} {
		pts = append(pts, pt{0, "single", k, 0, []float64{v}, nil, 0, false})
	}
	pts = append(pts, pt{0, "solo_b", 0, 0, []float64{1000}, nil, 0, false})
	for k, v := range []float64{850, 800, 750, 700} {
		pts = append(pts, pt{0, "pair", k, 0, []float64{v, v}, nil, 0, false})
	}
	return mkRun(pts)
}

func TestNodeView(t *testing.T) {
	n := Compute(nodeRun()).Node
	if !n.Uniform || len(n.Isolated) != 4 || len(n.Packed) != 4 {
		t.Fatalf("%+v", n)
	}
	if p := n.Isolated[3]; p.Servers != 4 || !approx(p.PerServerPct, 90) || !approx(p.TotalEq, 3.6) {
		t.Fatalf("isolated %+v", p)
	}
	if p := n.Packed[1]; p.Servers != 4 || !approx(p.PerServerPct, 80) || !approx(p.TotalEq, 3.2) {
		t.Fatalf("packed %+v", p)
	}
	if p := n.Packed[3]; p.Servers != 8 || !approx(p.TotalEq, 5.6) {
		t.Fatalf("packed full %+v", p)
	}
	want := []Density{{95, 3, 0}, {90, 4, 0}, {80, 4, 4}, {70, 4, 8}}
	if len(n.Density) != len(want) {
		t.Fatalf("density %+v", n.Density)
	}
	for i := range want {
		if n.Density[i] != want[i] {
			t.Fatalf("density %+v want %+v", n.Density[i], want[i])
		}
	}
	all := strings.Join(n.Conclusions, "\n")
	for _, s := range []string{"(4 сервера)", "90%", "3,6 сервера", "(8 серверов)", "70%", "5,6 сервера", "на 10–13 п.п.", "выгодно"} {
		if !strings.Contains(all, s) {
			t.Fatalf("missing %q in:\n%s", s, all)
		}
	}
}

func TestNodeForcedBackground(t *testing.T) {
	r := nodeRun()
	r.Params.BgThreads = 2
	n := Compute(r).Node
	if n.Uniform || len(n.Density) != 0 || len(n.Conclusions) != 0 || len(n.Isolated) != 4 {
		t.Fatalf("%+v", n)
	}
}

func TestIsolationUseless(t *testing.T) {
	s := Compute(mkRun([]pt{
		{0, "single", 0, 0, []float64{1000}, nil, 0, false},
		{0, "single", 1, 0, []float64{1000}, nil, 0, false},
		{0, "solo_b", 0, 0, []float64{1000}, nil, 0, false},
		{0, "pair", 0, 0, []float64{990, 990}, nil, 0, false},
		{0, "pair", 1, 0, []float64{990, 990}, nil, 0, false},
	}))
	if all := strings.Join(s.Node.Conclusions, "\n"); !strings.Contains(all, "почти ничего не даёт") {
		t.Fatalf("%s", all)
	}
}

func TestServersWord(t *testing.T) {
	for n, want := range map[int]string{1: "1 сервер", 2: "2 сервера", 5: "5 серверов", 11: "11 серверов", 21: "21 сервер", 24: "24 сервера", 32: "32 сервера"} {
		if got := serversWord(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}
