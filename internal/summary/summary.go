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

const Version = 2

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

// Curve holds values per number of loaded other cores. Percentages are of
// the same thread running alone on an idle machine.
type Curve struct {
	Loaded  []int     `json:"loaded"`
	Single  []float64 `json:"single"`   // thread A alone, sibling idle
	SoloB   []float64 `json:"solo_b"`   // thread B alone, sibling idle
	PairA   []float64 `json:"pair_a"`   // thread A while thread B is loaded
	PairB   []float64 `json:"pair_b"`   // thread B while thread A is loaded
	Pair    []float64 `json:"pair"`     // mean of PairA and PairB
	PairSum []float64 `json:"pair_sum"` // both threads together, in % of thread A alone
	// Servers on the node at this step: measured threads plus loaded neighbours.
	SingleServers []int     `json:"single_servers"`
	PairServers   []int     `json:"pair_servers"`
	FreqPct       []float64 `json:"freq_pct"`
	MHz           []float64 `json:"mhz"`      // thread A, 1 server per core
	PairMHz       []float64 `json:"pair_mhz"` // 2 servers per core
	Noisy         []bool    `json:"noisy"`
	Boundary      int       `json:"boundary"`
}

type Core struct {
	Core           int     `json:"core"`
	BaseTPS        float64 `json:"base_tps"`
	SoloPct        float64 `json:"solo_pct"`
	HTPerThreadPct float64 `json:"ht_per_thread_pct"`
	HTAPct         float64 `json:"ht_a_pct"`
	HTBPct         float64 `json:"ht_b_pct"`
	HTTotalPct     float64 `json:"ht_total_pct"`
	FullSinglePct  float64 `json:"full_single_pct"`
	FullPairPct    float64 `json:"full_pair_pct"`
	LagRatio       float64 `json:"lag_ratio"`
	DriftPct       float64 `json:"drift_pct"`
	MHzIdle        float64 `json:"mhz_idle"`
	MHzFullSingle  float64 `json:"mhz_full_single"`
	MHzFullPair    float64 `json:"mhz_full_pair"`
	Weak           bool    `json:"weak"`
	Drift          bool    `json:"drift"`
	DriftFreq      bool    `json:"drift_freq"`
	Noisy          bool    `json:"noisy"`
	Curve          Curve   `json:"curve"`
}

type Summary struct {
	Version        int               `json:"version"`
	HasPair        bool              `json:"has_pair"`
	HasFreq        bool              `json:"has_freq"`
	BaselineTPS    float64           `json:"baseline_tps"`
	HTPerThreadPct float64           `json:"ht_per_thread_pct"`
	HTAPct         float64           `json:"ht_a_pct"`
	HTBPct         float64           `json:"ht_b_pct"`
	HTTotalPct     float64           `json:"ht_total_pct"`
	FullSinglePct  float64           `json:"full_single_pct"`
	FullPairPct    float64           `json:"full_pair_pct"`
	LagRatio       float64           `json:"lag_ratio"`
	MHzIdle        float64           `json:"mhz_idle"`
	MHzFullSingle  float64           `json:"mhz_full_single"`
	MHzFullPair    float64           `json:"mhz_full_pair"`
	KneeSingle     int               `json:"knee_single"`
	Ratings        map[string]Rating `json:"ratings"`
	Avg            Curve             `json:"avg"`
	Node           Node              `json:"node"`
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

	var ht, htA, htB, htTotal, fullS, fullP, lag []float64
	for i := range s.Cores {
		c := &s.Cores[i]
		c.SoloPct = pct(c.BaseTPS, s.BaselineTPS)
		c.Weak = c.SoloPct >= 0 && c.SoloPct < weakPct
		if c.Weak {
			s.WeakCores = append(s.WeakCores, c.Core)
		}
		ht = append(ht, c.HTPerThreadPct)
		htA, htB = append(htA, c.HTAPct), append(htB, c.HTBPct)
		htTotal = append(htTotal, c.HTTotalPct)
		fullS = append(fullS, c.FullSinglePct)
		fullP = append(fullP, c.FullPairPct)
		lag = append(lag, c.LagRatio)
	}
	var mi, mfs, mfp []float64
	for _, c := range s.Cores {
		mi, mfs, mfp = append(mi, c.MHzIdle), append(mfs, c.MHzFullSingle), append(mfp, c.MHzFullPair)
	}
	s.MHzIdle, s.MHzFullSingle, s.MHzFullPair = mean(mi), mean(mfs), mean(mfp)
	if w := driftWarning(s.Cores); w != "" {
		s.Warnings = append(s.Warnings, w)
	}
	s.HTPerThreadPct, s.HTTotalPct = mean(ht), mean(htTotal)
	s.HTAPct, s.HTBPct = mean(htA), mean(htB)
	s.FullSinglePct, s.FullPairPct, s.LagRatio = mean(fullS), mean(fullP), mean(lag)
	s.Avg = averageCurve(s.Cores)
	s.Node = nodeView(s.Avg, r.Params.BgThreads == 0)

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

// driftWarning describes all cores whose speed at the start and end of their
// ramp differs by more than driftPct, in one sentence.
func driftWarning(cores []Core) string {
	var ids []string
	worst := 0.0
	freq := 0
	for _, c := range cores {
		if c.Drift {
			ids = append(ids, fmt.Sprint(c.Core))
			worst = max(worst, c.DriftPct)
			if c.DriftFreq {
				freq++
			}
		}
	}
	if len(ids) == 0 {
		return ""
	}
	reason := "нагрев или фоновая нагрузка"
	if freq*2 >= len(ids) {
		reason = "разная частота: турбобуст зависит от фоновой нагрузки на машине"
	}
	return fmt.Sprintf("на ядрах %s: скорость в начале и в конце теста различается до %s%% (%s); за 100%% взято среднее двух замеров",
		strings.Join(ids, ", "), strings.Replace(fmt.Sprintf("%.1f", worst), ".", ",", 1), reason)
}

func tps(p result.Point, ok bool, i int) float64 {
	if !ok || i >= len(p.Threads) {
		return -1
	}
	return p.Threads[i].TicksPerSec
}

func computeCore(id int, pts map[key]result.Point) Core {
	c := Core{Core: id, HTPerThreadPct: -1, HTAPct: -1, HTBPct: -1, HTTotalPct: -1, FullSinglePct: -1, FullPairPct: -1, LagRatio: -1,
		MHzIdle: -1, MHzFullSingle: -1, MHzFullPair: -1}
	single0, okS := pts[key{"single", 0}]
	c.BaseTPS = tps(single0, okS, 0)
	// The final point repeats single k=0 after the ramp. Turbo frequency on a
	// live machine moves between the two, so the base is their average and the
	// difference is reported as drift.
	if final, ok := pts[key{"final", 0}]; ok && c.BaseTPS > 0 && len(final.Threads) > 0 {
		s0, f := c.BaseTPS, final.Threads[0].TicksPerSec
		c.DriftPct = 100 * math.Abs(f-s0) / s0
		c.Drift = c.DriftPct > driftPct
		if m0, m1 := single0.Threads[0].MHz, final.Threads[0].MHz; m0 > 0 && m1 > 0 {
			c.DriftFreq = 100*math.Abs(m1-m0)/m0 > driftPct
			c.MHzIdle = (m0 + m1) / 2
		}
		c.BaseTPS = (s0 + f) / 2
	}
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
		c.HTAPct = pct(pair0.Threads[0].TicksPerSec, c.BaseTPS)
		c.HTBPct = pct(pair0.Threads[1].TicksPerSec, baseB)
		c.HTTotalPct = pct(pair0.Threads[0].TicksPerSec+pair0.Threads[1].TicksPerSec, c.BaseTPS)
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
		if c.MHzIdle < 0 {
			c.MHzIdle = mhz0
		}
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
		bp, okBP := pts[key{"solo_b", k}]
		cv.SoloB = append(cv.SoloB, pct(tps(bp, okBP, 0), baseB))
		cv.PairA = append(cv.PairA, pct(tps(pp, okPP, 0), c.BaseTPS))
		cv.PairB = append(cv.PairB, pct(tps(pp, okPP, 1), baseB))
		sum := -1.0
		if okPP && len(pp.Threads) == 2 {
			sum = pct(pp.Threads[0].TicksPerSec+pp.Threads[1].TicksPerSec, c.BaseTPS)
		}
		cv.PairSum = append(cv.PairSum, sum)
		cv.SingleServers = append(cv.SingleServers, serversAt(sp, okSP))
		cv.PairServers = append(cv.PairServers, serversAt(pp, okPP))
		f := -1.0
		if okSP && len(sp.Threads) > 0 && sp.Threads[0].MHz > 0 {
			f = pct(sp.Threads[0].MHz, mhz0)
		}
		cv.FreqPct = append(cv.FreqPct, f)
		cv.MHz = append(cv.MHz, mhzOf(sp, okSP))
		cv.PairMHz = append(cv.PairMHz, mhzOf(pp, okPP))
		cv.Noisy = append(cv.Noisy, (okSP && sp.Noisy) || (okPP && pp.Noisy))
	}
	cv.Boundary = boundary(pts, cv.Loaded)
	c.Curve = cv

	if n := len(cv.Loaded); n > 0 {
		kmax := cv.Loaded[n-1]
		c.FullSinglePct = cv.Single[n-1]
		c.FullPairPct = cv.Pair[n-1]
		c.MHzFullSingle = cv.MHz[n-1]
		c.MHzFullPair = cv.PairMHz[n-1]
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
	avg := Curve{Boundary: -1, Loaded: []int{}, Noisy: []bool{}, SingleServers: []int{}, PairServers: []int{}}
	fields := func(c *Curve) []*[]float64 {
		return []*[]float64{&c.Single, &c.SoloB, &c.PairA, &c.PairB, &c.Pair, &c.PairSum, &c.FreqPct, &c.MHz, &c.PairMHz}
	}
	for _, f := range fields(&avg) {
		*f = []float64{}
	}
	if len(cores) == 0 {
		return avg
	}
	count := map[int]int{}
	for _, c := range cores {
		for _, k := range c.Curve.Loaded {
			count[k]++
		}
	}
	first := cores[0].Curve
	for i0, k := range first.Loaded {
		if count[k] != len(cores) {
			continue
		}
		vals := make([][]float64, len(fields(&avg)))
		noisy := false
		for ci := range cores {
			cv := &cores[ci].Curve
			i := sort.SearchInts(cv.Loaded, k)
			for fi, f := range fields(cv) {
				vals[fi] = append(vals[fi], (*f)[i])
			}
			noisy = noisy || cv.Noisy[i]
		}
		avg.Loaded = append(avg.Loaded, k)
		for fi, f := range fields(&avg) {
			*f = append(*f, mean(vals[fi]))
		}
		avg.Noisy = append(avg.Noisy, noisy)
		avg.SingleServers = append(avg.SingleServers, first.SingleServers[i0])
		avg.PairServers = append(avg.PairServers, first.PairServers[i0])
	}
	avg.Boundary = first.Boundary
	return avg
}

// serversAt is the number of servers on the node in a window: measured
// threads plus loaded background CPUs. 0 when the window was not measured.
func serversAt(p result.Point, ok bool) int {
	if !ok {
		return 0
	}
	return len(p.Threads) + len(p.LoadedCPUs)
}

// mhzOf returns the frequency of the first measured thread, or -1.
func mhzOf(p result.Point, ok bool) float64 {
	if !ok || len(p.Threads) == 0 || p.Threads[0].MHz <= 0 {
		return -1
	}
	return p.Threads[0].MHz
}
