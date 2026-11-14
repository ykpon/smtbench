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
