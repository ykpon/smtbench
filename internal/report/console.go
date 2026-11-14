// Package report prints the plan, progress and summary to the console in Russian.
package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
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

func Header(w io.Writer, t *topology.Topology, ws []plan.Window, p result.Params, est time.Duration) {
	ht := "нет"
	if t.HasSMT() {
		ht = "да"
	}
	fmt.Fprintf(w, "Процессор: %s · сокетов: %d · ядер: %d · потоков: %d · HT: %s\n", t.Model, t.Packages, len(t.Cores), len(t.CPUs), ht)
	cores := map[int]bool{}
	for _, x := range ws {
		cores[x.Core] = true
	}
	fmt.Fprintf(w, "Профиль: %s (%s) · ядер в тесте: %d · окон: %d · замер %s + прогрев %s\n",
		p.Profile, mixString(p.Mix), len(cores), len(ws),
		Duration(time.Duration(p.DurationMS)*time.Millisecond), Duration(time.Duration(p.WarmupMS)*time.Millisecond))
	bg := "так же, как тестируемое (1 или 2 потока)"
	if p.BgThreads != 0 {
		bg = fmt.Sprintf("потоков на ядро: %d", p.BgThreads)
	}
	fmt.Fprintf(w, "Порядок загрузки: %s · соседние ядра загружаются %s\n", p.Order, bg)
	fmt.Fprintf(w, "Оценка времени: ~%s\n\n", Duration(est))
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

func countOrNone(n int) string {
	if n <= 0 {
		return "ни одного"
	}
	return strconv.Itoa(n)
}

func Summary(w io.Writer, r *result.Run, s summary.Summary) {
	fmt.Fprintf(w, "\nИтог (%s, профиль %s):\n", r.Label, r.Params.Profile)
	if s.BaselineTPS > 0 {
		fmt.Fprintf(w, "  Скорость одного потока ядра на пустой машине: %s тиков/с — это 100%%\n", Int(s.BaselineTPS))
	}
	if s.HasPair && s.HTPerThreadPct >= 0 {
		fmt.Fprintf(w, "  Оба потока одного ядра нагружены, остальные ядра свободны: 1-й поток %d%%, 2-й поток %d%%, вместе %d%%%s\n",
			round(s.HTAPct), round(s.HTBPct), round(s.HTTotalPct), rated(s, "ht"))
	}
	if s.Node.Uniform && len(s.Node.Conclusions) > 0 {
		for _, c := range s.Node.Conclusions {
			fmt.Fprintf(w, "  %s\n", c)
		}
	} else {
		if s.FullSinglePct >= 0 {
			fmt.Fprintf(w, "  1 сервер на ядре, остальные ядра загружены: %d%%%s\n", round(s.FullSinglePct), rated(s, "full_single"))
		}
		if s.HasPair && s.FullPairPct >= 0 {
			fmt.Fprintf(w, "  2 сервера на ядре, остальные ядра загружены: каждый %d%%%s\n", round(s.FullPairPct), rated(s, "full_pair"))
		}
	}
	if len(s.Node.Density) > 0 {
		fmt.Fprintln(w, "  Сколько серверов помещается на ноду, чтобы каждый работал:")
		for _, d := range s.Node.Density {
			line := fmt.Sprintf("    не медленнее %d%%: по одному на ядро — %s", d.MinPct, countOrNone(d.Isolated))
			if s.HasPair {
				line += fmt.Sprintf(", по два на ядро — %s", countOrNone(d.Packed))
			}
			fmt.Fprintln(w, line)
		}
	}
	if s.LagRatio > 0 {
		fmt.Fprintf(w, "  Лаги при полной загрузке: самые долгие тики в %s раза длиннее%s\n", Dec(s.LagRatio), rated(s, "lag"))
	}
	if s.HasFreq && s.MHzIdle > 0 {
		line := fmt.Sprintf("  Частота ядра: %d МГц на пустой машине", round(s.MHzIdle))
		if s.MHzFullSingle > 0 {
			line += fmt.Sprintf(", %d МГц при полной загрузке (1 сервер на ядро)", round(s.MHzFullSingle))
		}
		if s.HasPair && s.MHzFullPair > 0 {
			line += fmt.Sprintf(", %d МГц (2 сервера на ядро)", round(s.MHzFullPair))
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
	for _, warn := range s.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
}
