package summary

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"smtbench/internal/result"
)

// StabRow is one scenario of the tick rate test over all its servers.
// Percentages of the rate are of the target tick rate, work percentages of
// the tick period (the budget).
type StabRow struct {
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	Servers     int     `json:"servers"`
	RatePct     float64 `json:"rate_pct"`      // average tick rate
	WorstSecPct float64 `json:"worst_sec_pct"` // worst second of the worst server
	Low1Pct     float64 `json:"low1_pct"`      // rate at the 99th percentile of the interval, worst server
	WorkPct     float64 `json:"work_pct"`      // mean tick work
	WorkP99Pct  float64 `json:"work_p99_pct"`  // worst server
	WorkMaxPct  float64 `json:"work_max_pct"`
	OverrunPct  float64 `json:"overrun_pct"` // ticks longer than the period
	LatePct     float64 `json:"late_pct"`    // ticks started late by more than 10% of the period
	VarMS       float64 `json:"var_ms"`      // mean standard deviation of the interval between ticks
	WakeP99us   float64 `json:"wake_p99_us"`
	Rating      Rating  `json:"rating"`
	// Per second of the scenario: the slowest server's tick rate and the
	// longest tick on any server.
	SecRatePct    []float64 `json:"sec_rate_pct"`
	SecMaxWorkPct []float64 `json:"sec_max_work_pct"`
}

type Stab struct {
	TickRate    int       `json:"tick_rate"`
	TickLoadPct int       `json:"tick_load_pct"`
	BudgetMS    float64   `json:"budget_ms"`
	IdleWorkMS  float64   `json:"idle_work_ms"`
	DurationMS  int       `json:"duration_ms"`
	Rows        []StabRow `json:"rows"`
	// Key is the row shown in run lists: everything else loaded, or the last row.
	Key         string   `json:"key"`
	Conclusions []string `json:"conclusions"`
}

// KeyRow returns the row shown in run lists. s may be nil.
func (s *Stab) KeyRow() (StabRow, bool) {
	if s == nil {
		return StabRow{}, false
	}
	for _, r := range s.Rows {
		if r.Name == s.Key {
			return r, true
		}
	}
	return StabRow{}, false
}

// StabTitle names a scenario of the tick rate test.
var StabTitle = map[string]string{
	"alone":        "один сервер на пустой машине",
	"one_per_core": "по одному серверу на ядро",
	"sibling_busy": "второй поток ядра загружен на 100%",
	"two_per_core": "по два сервера на ядро",
	"all_busy":     "все остальные потоки загружены на 100%",
	"bursty":       "остальные потоки то загружены, то простаивают",
}

// Thresholds of the tick rate test.
const (
	stabRateGood, stabRateWarn = 95.0, 85.0
	stabWorkGood, stabWorkWarn = 80.0, 100.0
)

func worse(a, b Rating) Rating {
	rank := map[Rating]int{Good: 0, Warn: 1, Bad: 2}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

func rateStab(r StabRow) Rating {
	rate := Bad
	switch {
	case r.WorstSecPct >= stabRateGood:
		rate = Good
	case r.WorstSecPct >= stabRateWarn:
		rate = Warn
	}
	work := Bad
	switch {
	case r.WorkP99Pct <= stabWorkGood:
		work = Good
	case r.WorkP99Pct <= stabWorkWarn:
		work = Warn
	}
	return worse(rate, work)
}

// StabScenario summarises one scenario.
func StabScenario(st *result.Stability, sc result.StabScenario) StabRow {
	periodUs := 1e6 / float64(st.TickRate)
	row := StabRow{Name: sc.Name, Title: StabTitle[sc.Name], Servers: len(sc.Servers),
		WorstSecPct: -1, Low1Pct: -1, SecRatePct: []float64{}, SecMaxWorkPct: []float64{}}
	if row.Title == "" {
		row.Title = sc.Name
	}
	if len(sc.Servers) == 0 {
		return row
	}
	var ticks, overruns, late int
	var work, sd float64
	secs := 0
	for _, s := range sc.Servers {
		ticks += s.Ticks
		overruns += s.Overruns
		late += s.Late
		work += s.WorkMeanUs * float64(s.Ticks)
		sd += s.IntervalSDus
		row.WorkP99Pct = max(row.WorkP99Pct, 100*s.WorkP99us/periodUs)
		row.WorkMaxPct = max(row.WorkMaxPct, 100*s.WorkMaxUs/periodUs)
		row.WakeP99us = max(row.WakeP99us, s.WakeP99us)
		if s.IntervalP99us > 0 {
			low := 100 * periodUs / s.IntervalP99us
			if row.Low1Pct < 0 || low < row.Low1Pct {
				row.Low1Pct = min(low, 100)
			}
		}
		secs = max(secs, len(s.PerSec))
	}
	expected := float64(len(sc.Servers)) * float64(st.DurationMS) / 1000 * float64(st.TickRate)
	row.RatePct = min(100, 100*float64(ticks)/expected)
	if ticks > 0 {
		row.WorkPct = 100 * work / float64(ticks) / periodUs
		row.OverrunPct = 100 * float64(overruns) / float64(ticks)
		row.LatePct = 100 * float64(late) / float64(ticks)
	}
	row.VarMS = sd / float64(len(sc.Servers)) / 1000
	for i := 0; i < secs; i++ {
		minRate, maxWork := math.Inf(1), 0.0
		for _, s := range sc.Servers {
			if i < len(s.PerSec) {
				minRate = math.Min(minRate, 100*float64(s.PerSec[i])/float64(st.TickRate))
				maxWork = math.Max(maxWork, 100*s.MaxWorkPerSec[i]/periodUs)
			}
		}
		row.SecRatePct = append(row.SecRatePct, minRate)
		row.SecMaxWorkPct = append(row.SecMaxWorkPct, maxWork)
		if row.WorstSecPct < 0 || minRate < row.WorstSecPct {
			row.WorstSecPct = minRate
		}
	}
	row.Rating = rateStab(row)
	return row
}

func computeStab(st *result.Stability) *Stab {
	if st == nil || st.TickRate <= 0 {
		return nil
	}
	periodUs := 1e6 / float64(st.TickRate)
	s := &Stab{TickRate: st.TickRate, TickLoadPct: st.TickLoadPct, BudgetMS: periodUs / 1000,
		IdleWorkMS: st.IdleWorkUs / 1000, DurationMS: st.DurationMS, Rows: []StabRow{}, Conclusions: []string{}}
	for _, sc := range st.Scenarios {
		s.Rows = append(s.Rows, StabScenario(st, sc))
	}
	if len(s.Rows) == 0 {
		return s
	}
	s.Key = s.Rows[len(s.Rows)-1].Name
	for _, r := range s.Rows {
		if r.Name == "all_busy" {
			s.Key = r.Name
		}
	}
	s.Conclusions = stabConclusions(s)
	return s
}

func ms(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%d мс", int(math.Round(v)))
	}
	return dec1(v) + " мс"
}

func dec1(v float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1)
}

func stabConclusions(s *Stab) []string {
	out := []string{fmt.Sprintf("Тикрейт %d, бюджет тика %s. Работа тика подобрана так, чтобы при непрерывной нагрузке занимать %d%% бюджета (%s).",
		s.TickRate, ms(s.BudgetMS), s.TickLoadPct, ms(s.IdleWorkMS))}
	base := -1.0
	for _, r := range s.Rows {
		line := fmt.Sprintf("%s: тик занимает в среднем %s бюджета", capFirst(r.Title), pctText(r.WorkPct))
		if r.Name == "alone" {
			base = r.WorkPct
		} else if base > 0 {
			line += fmt.Sprintf(" (на пустой машине %s)", pctText(base))
		}
		line += fmt.Sprintf(", 1 тик из 100 — дольше %s", pctText(r.WorkP99Pct))
		switch {
		case r.WorstSecPct >= 0 && r.WorstSecPct < 100:
			line += fmt.Sprintf("; в худшую секунду %d тиков из %d", int(math.Round(r.WorstSecPct*float64(s.TickRate)/100)), s.TickRate)
		case r.OverrunPct > 0:
			line += fmt.Sprintf("; %s тиков не уложились в бюджет, но сервер наверстал", pctTextFine(r.OverrunPct))
		default:
			line += "; тикрейт держится"
		}
		if r.Name == "alone" && r.WorkPct-float64(s.TickLoadPct) > 10 {
			line += ". Это больше расчётного: после простоя между тиками ядро работает медленнее, чем под непрерывной нагрузкой"
		}
		out = append(out, line+".")
	}
	return out
}

func pctTextFine(v float64) string {
	if v < 1 {
		return dec1(v) + "%"
	}
	return pctText(v)
}

func capFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
