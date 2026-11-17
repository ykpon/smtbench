// Package report prints the plan, progress and summary to the console in Russian.
package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/runner"
	"smtbench/internal/summary"
	"smtbench/internal/topology"
)

// Int formats 12340 as "12 340".
func Int(v float64) string {
	s := strconv.FormatInt(int64(math.Round(v)), 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Dec formats 2.14 as "2,1".
func Dec(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1)
}

func Duration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d с", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d мин", int(math.Round(d.Minutes())))
	}
	return fmt.Sprintf("%d ч %d мин", int(d.Hours()), int(d.Minutes())%60)
}

// Machine prints the CPU line.
func Machine(w io.Writer, t *topology.Topology) {
	ht := "нет"
	if t.HasSMT() {
		ht = "да"
	}
	fmt.Fprintf(w, "Процессор: %s · сокетов: %d · ядер: %d · потоков: %d · HT: %s\n", t.Model, t.Packages, len(t.Cores), len(t.CPUs), ht)
}

// Plan prints the settings of one run and its estimate.
func Plan(w io.Writer, ws []plan.Window, sw []plan.StabWindow, p result.Params, est time.Duration) {
	cores := map[int]bool{}
	for _, x := range ws {
		cores[x.Core] = true
	}
	fmt.Fprintf(w, "Профиль: %s (%s)", p.Profile, mixString(p.Mix))
	if len(ws) > 0 {
		fmt.Fprintf(w, " · ядер в тесте: %d · окон: %d · замер %s + прогрев %s", len(cores), len(ws),
			Duration(time.Duration(p.DurationMS)*time.Millisecond), Duration(time.Duration(p.WarmupMS)*time.Millisecond))
		bg := map[int]string{0: "столько же потоков, сколько у тестируемого", 1: "один поток", 2: "оба потока"}[p.BgThreads]
		fmt.Fprintf(w, "\nПорядок загрузки: %s · у соседних ядер нагружается: %s", p.Order, bg)
	}
	fmt.Fprintln(w)
	if len(sw) > 0 {
		fmt.Fprintf(w, "Тест тикрейта: тикрейт %d, работа сервера — %d%% бюджета тика, сценариев: %d по %s\n",
			p.TickRate, p.TickLoadPct, len(sw), Duration(time.Duration(p.StabMS)*time.Millisecond))
	}
	fmt.Fprintf(w, "Оценка времени: ~%s\n", Duration(est))
}

func mixString(m map[string]int) string {
	var parts []string
	for _, k := range []string{"int", "fp", "avx", "mem"} {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
		}
	}
	return strings.Join(parts, ",")
}

var modeText = map[plan.Mode]string{
	plan.Single: "1-й поток один",
	plan.SoloB:  "2-й поток один",
	plan.Pair:   "оба потока    ",
	plan.Final:  "повтор        ",
}

// Printer prints one progress line per window, showing speed as a share
// of the core's idle single-thread speed.
type Printer struct {
	W     io.Writer
	Cores int
	base  map[int]float64
}

func (pr *Printer) Line(p runner.Progress) {
	if pr.base == nil {
		pr.base = map[int]float64{}
	}
	tps := 0.0
	mhz := 0.0
	for _, th := range p.Point.Threads {
		tps += th.TicksPerSec
		mhz = max(mhz, th.MHz)
	}
	tps /= float64(max(len(p.Point.Threads), 1))
	if p.Window.Mode == plan.Single && p.Window.LoadedCores == 0 {
		pr.base[p.Window.Core] = tps
	}
	share := ""
	if b := pr.base[p.Window.Core]; b > 0 {
		share = fmt.Sprintf(" (%d%%)", int(math.Round(100*tps/b)))
	}
	freq := ""
	if mhz > 0 {
		freq = fmt.Sprintf(" · %d МГц", int(math.Round(mhz)))
	}
	noisy := ""
	if p.Point.Noisy {
		noisy = " · помехи"
	}
	width := len(strconv.Itoa(p.Total))
	fmt.Fprintf(pr.W, "[%*d/%d] ядро %d · %s · загружено %d/%d · %s тик/с%s%s%s · осталось ~%s\n",
		width, p.Done, p.Total, p.Window.Core, modeText[p.Window.Mode], p.Window.LoadedCores, pr.Cores-1,
		Int(tps), share, freq, noisy, Duration(p.ETA))
}

var ratingText = map[summary.Rating]string{summary.Good: "хорошо", summary.Warn: "средне", summary.Bad: "плохо"}

func rated(s summary.Summary, key string) string {
	if r, ok := s.Ratings[key]; ok {
		return " [" + ratingText[r] + "]"
	}
	return ""
}

func round(v float64) int { return int(math.Round(v)) }

// pctCell formats a percentage; missing values are "—".
func pctCell(v float64) string {
	if v < 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", round(v))
}

func tpsCell(v float64) string {
	if v <= 0 {
		return "—"
	}
	return Int(v)
}

// othersLoaded names the "all other cores loaded" condition for a run.
func othersLoaded(bgThreads int) string {
	switch bgThreads {
	case 1:
		return "остальные ядра нагружены одним потоком"
	case 2:
		return "все остальные ядра нагружены"
	}
	return "остальные ядра нагружены так же, как тестируемое"
}

func Summary(w io.Writer, r *result.Run, s summary.Summary) {
	fmt.Fprintf(w, "\nИтог (%s, профиль %s)\n", r.Label, r.Params.Profile)
	if len(r.Points) > 0 || s.Stab == nil {
		throughput(w, r, s)
	}
	stab(w, s.Stab)
	for _, warn := range s.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
}

func throughput(w io.Writer, r *result.Run, s summary.Summary) {
	if s.BaselineTPS > 0 {
		fmt.Fprintf(w, "100%% — скорость потока, когда он один на пустой машине: %s тиков/с\n", Int(s.BaselineTPS))
	}
	full := othersLoaded(r.Params.BgThreads)

	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintf(tw, "  \tостальные ядра свободны\t%s\n", full)
	fmt.Fprintf(tw, "  Поток один, второй поток ядра свободен\t100%%\t%s%s\n", pctCell(s.FullAlonePct), rated(s, "full_single"))
	if s.HasPair {
		fmt.Fprintf(tw, "  Оба потока ядра нагружены, каждый\t%s%s\t%s%s\n", pctCell(s.HTPerThreadPct), rated(s, "ht"), pctCell(s.FullPairPct), rated(s, "full_pair"))
	}
	tw.Flush()

	if len(s.Node.Conclusions) > 0 {
		fmt.Fprintln(w)
		for _, c := range s.Node.Conclusions {
			fmt.Fprintf(w, "  %s\n", c)
		}
	}

	fmt.Fprintln(w)
	if s.LagRatio > 0 {
		fmt.Fprintf(w, "  Лаги при полной загрузке: самые долгие тики в %s раза длиннее%s\n", Dec(s.LagRatio), rated(s, "lag"))
	}
	if s.HasFreq && s.MHzIdle > 0 {
		line := fmt.Sprintf("  Частота ядра: %d МГц на пустой машине", round(s.MHzIdle))
		if s.MHzFullSingle > 0 {
			line += fmt.Sprintf(", %d МГц при полной загрузке (поток один)", round(s.MHzFullSingle))
		}
		if s.HasPair && s.MHzFullPair > 0 {
			line += fmt.Sprintf(", %d МГц (оба потока)", round(s.MHzFullPair))
		}
		fmt.Fprintln(w, line)
	}
	if len(s.WeakCores) > 0 {
		var ids []string
		for _, c := range s.WeakCores {
			ids = append(ids, strconv.Itoa(c))
		}
		fmt.Fprintf(w, "  Ядра медленнее остальных больше чем на 5%%: %s\n", strings.Join(ids, ", "))
	}

	pairs(w, s, full)
}

// StabLine prints one finished scenario of the tick rate test.
func StabLine(w io.Writer, p runner.StabProgress) {
	if p.Done == 1 {
		fmt.Fprintf(w, "Тест тикрейта: тикрейт %d, бюджет тика %s, работа тика рассчитана на %s\n",
			p.Stab.TickRate, Ms(1e3/float64(p.Stab.TickRate)), Ms(p.Stab.IdleWorkUs/1000))
	}
	r := summary.StabScenario(p.Stab, p.Scenario)
	fmt.Fprintf(w, "[%d/%d] %s: тикрейт %s, худшая секунда %s · тик в среднем %s бюджета, 1 из 100 дольше %s\n",
		p.Done, p.Total, r.Title, pctCell(r.RatePct), pctCell(r.WorstSecPct), pctCell(r.WorkPct), pctCell(r.WorkP99Pct))
}

// Ms formats milliseconds: "15,6 мс", "120 мс".
func Ms(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%d мс", round(v))
	}
	return Dec(v) + " мс"
}

func stab(w io.Writer, s *summary.Stab) {
	if s == nil || len(s.Rows) == 0 {
		return
	}
	fmt.Fprintf(w, "\nТест тикрейта: тикрейт %d, бюджет тика %s.\n", s.TickRate, Ms(s.BudgetMS))
	fmt.Fprintf(w, "Тикрейт — в процентах от %d, время тика — в процентах бюджета, var — разброс интервала между тиками.\n\n", s.TickRate)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  сценарий\tсерверов\tтикрейт\tхудшая секунда\tтик, среднее\tтик, 1 из 100\tне уложились\tvar\t")
	for _, r := range s.Rows {
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t[%s]\n", r.Title, r.Servers, pctCell(r.RatePct), pctCell(r.WorstSecPct),
			pctCell(r.WorkPct), pctCell(r.WorkP99Pct), fracPct(r.OverrunPct), Ms(r.VarMS), ratingText[r.Rating])
	}
	tw.Flush()
	fmt.Fprintln(w)
	for _, c := range s.Conclusions {
		fmt.Fprintf(w, "  %s\n", c)
	}
	fmt.Fprintln(w)
}

// fracPct formats a small share: "0", "0,4%", "3%".
func fracPct(v float64) string {
	switch {
	case v <= 0:
		return "0"
	case v < 1:
		return Dec(v) + "%"
	}
	return fmt.Sprintf("%d%%", round(v))
}

// GroupRow is one run of a command with several variants.
type GroupRow struct {
	Variant string
	Run     *result.Run
	Sum     summary.Summary
}

// Group prints the headline numbers of all variants side by side.
func Group(w io.Writer, rows []GroupRow) {
	fmt.Fprintf(w, "\nВсе варианты (%d)\n", len(rows))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  вариант\tпоток один, всё загружено\tоба потока, каждый\tлаги\tтикрейт, худшая секунда\tтик, 1 из 100\t")
	for _, g := range rows {
		s := g.Sum
		alone, pair, lag := "—", "—", "—"
		if len(g.Run.Points) > 0 {
			alone = pctCell(s.FullAlonePct)
		}
		if s.HasPair {
			pair = pctCell(s.FullPairPct)
		}
		if s.LagRatio > 0 {
			lag = Dec(s.LagRatio) + "×"
		}
		worst, work := "—", "—"
		if r, ok := s.Stab.KeyRow(); ok {
			worst, work = pctCell(r.WorstSecPct), pctCell(r.WorkP99Pct)
		}
		name := g.Variant
		if g.Run.Partial {
			name += " (прерван)"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t\n", name, alone, pair, lag, worst, work)
	}
	tw.Flush()
}

// pairs prints every core as a pair of its threads: each thread alone and
// with its sibling loaded, on an idle machine and with other cores loaded.
func pairs(w io.Writer, s summary.Summary, full string) {
	if len(s.Cores) == 0 {
		return
	}
	fmt.Fprintf(w, "\nПо парам потоков. «Один» — второй поток ядра свободен, «сосед нагружен» — занят.\n")
	fmt.Fprintf(w, "«Пусто» — остальные ядра свободны, «загружено» — %s.\n", full)
	fmt.Fprintf(w, "Проценты — от скорости этого же потока одного на пустой машине.\n\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  \t\tпусто:\tпусто:\tзагружено:\tзагружено:\t")
	fmt.Fprintln(tw, "  ядро\tпоток\tодин, тиков/с\tсосед нагружен\tодин\tсосед нагружен\t")
	for _, c := range s.Cores {
		note := ""
		if c.Noisy {
			note = "помехи"
		}
		fmt.Fprintf(tw, "  %d\tCPU %d\t%s\t%s\t%s\t%s\t%s\n", c.Core, c.CPUA, tpsCell(c.BaseTPS),
			pctCell(c.HTAPct), pctCell(c.FullSinglePct), pctCell(c.FullPairAPct), note)
		if c.CPUB >= 0 {
			fmt.Fprintf(tw, "  \tCPU %d\t%s\t%s\t%s\t%s\t\n", c.CPUB, tpsCell(c.BaseBTPS),
				pctCell(c.HTBPct), pctCell(c.FullSoloBPct), pctCell(c.FullPairBPct))
		}
	}
	tw.Flush()
	fmt.Fprintln(w)
}
