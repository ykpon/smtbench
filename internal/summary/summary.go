// Package summary turns raw points into the numbers shown to people:
// percentages of the idle single-thread speed, the knee of the ramp and
// traffic-light ratings. Missing values are -1.
package summary

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"smtbench/internal/result"
)

const Version = 1

type Rating string

const (
	Good Rating = "good"
	Warn Rating = "warn"
	Bad  Rating = "bad"
)

// Thresholds: {good if >= / <=, warn if >= / <=}.
var thresholds = map[string]struct {
	good, warn  float64
	lowerBetter bool
}{
	"full_single": {90, 75, false},
	"ht":          {70, 55, false},
	"full_pair":   {65, 50, false},
	"lag":         {1.5, 3, true},
}

func Rate(metric string, v float64) Rating {
	th := thresholds[metric]
	if th.lowerBetter {
		switch {
		case v <= th.good:
			return Good
		case v <= th.warn:
			return Warn
		}
		return Bad
	}
	switch {
	case v >= th.good:
		return Good
	case v >= th.warn:
		return Warn
	}
	return Bad
}

type Curve struct {
	Loaded   []int     `json:"loaded"`
	Single   []float64 `json:"single"`
	Pair     []float64 `json:"pair"`
	FreqPct  []float64 `json:"freq_pct"`
	Noisy    []bool    `json:"noisy"`
	Boundary int       `json:"boundary"`
}

type Core struct {
	Core           int     `json:"core"`
	BaseTPS        float64 `json:"base_tps"`
	SoloPct        float64 `json:"solo_pct"`
	HTPerThreadPct float64 `json:"ht_per_thread_pct"`
	HTTotalPct     float64 `json:"ht_total_pct"`
	FullSinglePct  float64 `json:"full_single_pct"`
	FullPairPct    float64 `json:"full_pair_pct"`
	LagRatio       float64 `json:"lag_ratio"`
	DriftPct       float64 `json:"drift_pct"`
	Weak           bool    `json:"weak"`
	Drift          bool    `json:"drift"`
	Noisy          bool    `json:"noisy"`
	Curve          Curve   `json:"curve"`
}

type Summary struct {
	Version        int               `json:"version"`
	HasPair        bool              `json:"has_pair"`
	HasFreq        bool              `json:"has_freq"`
	BaselineTPS    float64           `json:"baseline_tps"`
	HTPerThreadPct float64           `json:"ht_per_thread_pct"`
	HTTotalPct     float64           `json:"ht_total_pct"`
	FullSinglePct  float64           `json:"full_single_pct"`
	FullPairPct    float64           `json:"full_pair_pct"`
	LagRatio       float64           `json:"lag_ratio"`
	KneeSingle     int               `json:"knee_single"`
	Ratings        map[string]Rating `json:"ratings"`
	Avg            Curve             `json:"avg"`
	Cores          []Core            `json:"cores"`
	WeakCores      []int             `json:"weak_cores"`
	NoisyPoints    int               `json:"noisy_points"`
	Warnings       []string          `json:"warnings"`
}

const (
	weakPct  = 95
	driftPct = 3
	kneePct  = 95
)

type key struct {
	mode string
	k    int
}

func mean(xs []float64) float64 {
	var s float64
	n := 0
	for _, x := range xs {
		if x >= 0 {
			s += x
			n++
		}
	}
	if n == 0 {
		return -1
	}
	return s / float64(n)
}

func median(xs []float64) float64 {
	var v []float64
	for _, x := range xs {
		if x > 0 {
			v = append(v, x)
		}
	}
	if len(v) == 0 {
		return -1
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func ratio(a, b float64) float64 {
	if a < 0 || b <= 0 {
		return -1
	}
	return a / b
}

func pct(a, b float64) float64 {
	r := ratio(a, b)
	if r < 0 {
		return -1
	}
	return 100 * r
}

func Compute(r *result.Run) Summary {
	s := Summary{Version: Version, Ratings: map[string]Rating{}, WeakCores: []int{}, Warnings: []string{}, KneeSingle: -1}

	byCore := map[int]map[key]result.Point{}
	var coreIDs []int
	for _, p := range r.Points {
		if byCore[p.Core] == nil {
			byCore[p.Core] = map[key]result.Point{}
			coreIDs = append(coreIDs, p.Core)
		}
		byCore[p.Core][key{p.Mode, p.LoadedCores}] = p
		if p.Mode == "pair" {
			s.HasPair = true
		}
		for _, th := range p.Threads {
			if th.MHz > 0 {
				s.HasFreq = true
			}
		}
		if p.Noisy {
			s.NoisyPoints++
		}
	}
	sort.Ints(coreIDs)

	var bases []float64
	for _, id := range coreIDs {
		c := computeCore(id, byCore[id])
		s.Cores = append(s.Cores, c)
		bases = append(bases, c.BaseTPS)
	}
	s.BaselineTPS = median(bases)

	var ht, htTotal, fullS, fullP, lag []float64
	for i := range s.Cores {
		c := &s.Cores[i]
		c.SoloPct = pct(c.BaseTPS, s.BaselineTPS)
		c.Weak = c.SoloPct >= 0 && c.SoloPct < weakPct
		if c.Weak {
			s.WeakCores = append(s.WeakCores, c.Core)
		}
		if c.Drift {
			s.Warnings = append(s.Warnings, fmt.Sprintf("ядро %d: скорость в начале и в конце теста различается на %s%% (нагрев или фоновая нагрузка)", c.Core, strings.Replace(fmt.Sprintf("%.1f", c.DriftPct), ".", ",", 1)))
		}
		ht = append(ht, c.HTPerThreadPct)
		htTotal = append(htTotal, c.HTTotalPct)
		fullS = append(fullS, c.FullSinglePct)
		fullP = append(fullP, c.FullPairPct)
		lag = append(lag, c.LagRatio)
	}
	s.HTPerThreadPct, s.HTTotalPct = mean(ht), mean(htTotal)
	s.FullSinglePct, s.FullPairPct, s.LagRatio = mean(fullS), mean(fullP), mean(lag)
	s.Avg = averageCurve(s.Cores)

	for i, k := range s.Avg.Loaded {
		if s.Avg.Single[i] < kneePct {
			break
		}
		s.KneeSingle = k
	}

	rate := func(name string, v float64) {
		if v >= 0 {
			s.Ratings[name] = Rate(name, v)
		}
	}
	rate("full_single", s.FullSinglePct)
	if s.HasPair {
		rate("ht", s.HTPerThreadPct)
		rate("full_pair", s.FullPairPct)
	}
	rate("lag", s.LagRatio)

	if s.NoisyPoints > 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("%d точек измерены с помехами от других процессов", s.NoisyPoints))
	}
	if !s.HasFreq {
		s.Warnings = append(s.Warnings, "частота не измерялась")
	}
	if r.Partial {
		s.Warnings = append(s.Warnings, "тест был прерван, результаты неполные")
	}
	return s
}

func tps(p result.Point, ok bool, i int) float64 {
	if !ok || i >= len(p.Threads) {
		return -1
	}
	return p.Threads[i].TicksPerSec
}

func computeCore(id int, pts map[key]result.Point) Core {
	c := Core{Core: id, HTPerThreadPct: -1, HTTotalPct: -1, FullSinglePct: -1, FullPairPct: -1, LagRatio: -1}
	single0, okS := pts[key{"single", 0}]
	c.BaseTPS = tps(single0, okS, 0)
	soloB, okB := pts[key{"solo_b", 0}]
	baseB := tps(soloB, okB, 0)
	if baseB < 0 {
		baseB = c.BaseTPS
	}
	perThread := func(p result.Point) float64 {
		if len(p.Threads) < 2 {
			return -1
		}
		return mean([]float64{pct(p.Threads[0].TicksPerSec, c.BaseTPS), pct(p.Threads[1].TicksPerSec, baseB)})
	}

	if pair0, ok := pts[key{"pair", 0}]; ok && len(pair0.Threads) == 2 {
		c.HTPerThreadPct = perThread(pair0)
		c.HTTotalPct = pct(pair0.Threads[0].TicksPerSec+pair0.Threads[1].TicksPerSec, c.BaseTPS)
	}
	if final, ok := pts[key{"final", 0}]; ok && c.BaseTPS > 0 {
		c.DriftPct = 100 * math.Abs(tps(final, ok, 0)-c.BaseTPS) / c.BaseTPS
		c.Drift = c.DriftPct > driftPct
	}

	ks := map[int]bool{}
	for k := range pts {
		if k.mode == "single" || k.mode == "pair" {
			ks[k.k] = true
		}
		if pts[k].Noisy {
			c.Noisy = true
		}
	}
	cv := Curve{Boundary: -1}
	for k := range ks {
		cv.Loaded = append(cv.Loaded, k)
	}
	sort.Ints(cv.Loaded)
	mhz0 := -1.0
	if okS && len(single0.Threads) > 0 && single0.Threads[0].MHz > 0 {
		mhz0 = single0.Threads[0].MHz
	}
	for _, k := range cv.Loaded {
		sp, okSP := pts[key{"single", k}]
		pp, okPP := pts[key{"pair", k}]
		cv.Single = append(cv.Single, pct(tps(sp, okSP, 0), c.BaseTPS))
		pv := -1.0
		if okPP {
			pv = perThread(pp)
		}
		cv.Pair = append(cv.Pair, pv)
		f := -1.0
		if okSP && len(sp.Threads) > 0 && sp.Threads[0].MHz > 0 {
			f = pct(sp.Threads[0].MHz, mhz0)
		}
		cv.FreqPct = append(cv.FreqPct, f)
		cv.Noisy = append(cv.Noisy, (okSP && sp.Noisy) || (okPP && pp.Noisy))
	}
	cv.Boundary = boundary(pts, cv.Loaded)
	c.Curve = cv

	if n := len(cv.Loaded); n > 0 {
		kmax := cv.Loaded[n-1]
		c.FullSinglePct = cv.Single[n-1]
		c.FullPairPct = cv.Pair[n-1]
		base99 := -1.0
		if okS && len(single0.Threads) > 0 {
			base99 = single0.Threads[0].TickP99us
		}
		worst := -1.0
		if pp, ok := pts[key{"pair", kmax}]; ok {
			for _, th := range pp.Threads {
				worst = max(worst, th.TickP99us)
			}
		} else if sp, ok := pts[key{"single", kmax}]; ok && len(sp.Threads) > 0 {
			worst = sp.Threads[0].TickP99us
		}
		c.LagRatio = ratio(worst, base99)
	}
	return c
}

// boundary is the last k up to which all loaded cores are of one kind
// (all inside the target's L3 group, or all outside it); -1 if never crossed.
func boundary(pts map[key]result.Point, loaded []int) int {
	outside := func(k int) (int, bool) {
		if p, ok := pts[key{"single", k}]; ok {
			return p.OutsideL3, true
		}
		if p, ok := pts[key{"pair", k}]; ok {
			return p.OutsideL3, true
		}
		return 0, false
	}
	var first = -1
	for _, k := range loaded {
		if k > 0 {
			first = k
			break
		}
	}
	if first < 0 {
		return -1
	}
	o, _ := outside(first)
	inside := o == 0
	last := 0
	crossed := false
	for _, k := range loaded {
		if k == 0 {
			continue
		}
		o, ok := outside(k)
		if !ok {
			continue
		}
		same := (inside && o == 0) || (!inside && o == k)
		if !same {
			crossed = true
			break
		}
		last = k
	}
	if !crossed {
		return -1
	}
	return last
}

// averageCurve averages curves over cores at the k values present in all of them.
func averageCurve(cores []Core) Curve {
	avg := Curve{Boundary: -1, Loaded: []int{}, Single: []float64{}, Pair: []float64{}, FreqPct: []float64{}, Noisy: []bool{}}
	if len(cores) == 0 {
		return avg
	}
	count := map[int]int{}
	for _, c := range cores {
		for _, k := range c.Curve.Loaded {
			count[k]++
		}
	}
	for _, k := range cores[0].Curve.Loaded {
		if count[k] != len(cores) {
			continue
		}
		var sv, pv, fv []float64
		noisy := false
		for _, c := range cores {
			i := sort.SearchInts(c.Curve.Loaded, k)
			sv = append(sv, c.Curve.Single[i])
			pv = append(pv, c.Curve.Pair[i])
			fv = append(fv, c.Curve.FreqPct[i])
			noisy = noisy || c.Curve.Noisy[i]
		}
		avg.Loaded = append(avg.Loaded, k)
		avg.Single = append(avg.Single, mean(sv))
		avg.Pair = append(avg.Pair, mean(pv))
		avg.FreqPct = append(avg.FreqPct, mean(fv))
		avg.Noisy = append(avg.Noisy, noisy)
	}
	avg.Boundary = cores[0].Curve.Boundary
	return avg
}
