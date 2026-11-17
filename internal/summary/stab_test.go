package summary

import (
	"strings"
	"testing"

	"smtbench/internal/result"
)

func stabServer(cpu int, perSec []int, workUs, p99Us float64, overruns int) result.StabServer {
	ticks := 0
	maxWork := make([]float64, len(perSec))
	for i, n := range perSec {
		ticks += n
		maxWork[i] = p99Us
	}
	return result.StabServer{CPU: cpu, Ticks: ticks, PerSec: perSec, MaxWorkPerSec: maxWork,
		WorkMeanUs: workUs, WorkP50us: workUs, WorkP99us: p99Us, WorkMaxUs: p99Us, IntervalSDus: 500, IntervalP99us: 20000, Overruns: overruns}
}

func TestStab(t *testing.T) {
	st := &result.Stability{TickRate: 64, TickLoadPct: 50, WorkTicks: 78, IdleWorkUs: 7812.5, DurationMS: 2000,
		Scenarios: []result.StabScenario{
			{Name: "alone", Servers: []result.StabServer{stabServer(0, []int{64, 64}, 7812.5, 9375, 0)}},
			{Name: "all_busy", Servers: []result.StabServer{
				stabServer(0, []int{64, 64}, 9375, 12500, 0),
				stabServer(2, []int{64, 56}, 12500, 17187.5, 3),
			}},
		}}
	s := Compute(&result.Run{Stability: st})
	if s.Stab == nil || len(s.Stab.Rows) != 2 || s.Stab.Key != "all_busy" || s.Stab.BudgetMS != 15.625 {
		t.Fatalf("%+v", s.Stab)
	}
	a, b := s.Stab.Rows[0], s.Stab.Rows[1]
	if a.RatePct != 100 || a.WorstSecPct != 100 || a.WorkPct != 50 || a.WorkP99Pct != 60 || a.Rating != Good {
		t.Fatalf("alone %+v", a)
	}
	// 248 of 256 ticks; worst second 56 of 64; worst p99 110% of budget
	if b.RatePct != 96.875 || b.WorstSecPct != 87.5 || b.WorkP99Pct != 110 || b.Rating != Bad || b.Servers != 2 {
		t.Fatalf("all_busy %+v", b)
	}
	if b.SecRatePct[1] != 87.5 || b.SecMaxWorkPct[0] != 110 || b.Low1Pct != 78.125 || b.VarMS != 0.5 {
		t.Fatalf("series %+v", b)
	}
	if s.Ratings["stab"] != Bad || len(s.Cores) != 0 {
		t.Fatalf("ratings %v cores %v", s.Ratings, s.Cores)
	}
	all := strings.Join(s.Stab.Conclusions, "\n")
	for _, want := range []string{"Тикрейт 64, бюджет тика 15,6 мс", "(на пустой машине 50%)", "в худшую секунду 56 тиков из 64"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in\n%s", want, all)
		}
	}
	for _, w := range s.Warnings {
		if strings.Contains(w, "частота") {
			t.Fatalf("frequency warning for a run without ramp: %v", s.Warnings)
		}
	}
}

func TestStabNoise(t *testing.T) {
	quiet := stabServer(0, []int{64}, 7812.5, 9375, 0)
	loud := stabServer(2, []int{64}, 7812.5, 9375, 0)
	loud.NoisePct = 12
	st := &result.Stability{TickRate: 64, TickLoadPct: 50, IdleWorkUs: 7812.5, DurationMS: 1000,
		Scenarios: []result.StabScenario{
			{Name: "alone", Servers: []result.StabServer{quiet}},
			{Name: "all_busy", Servers: []result.StabServer{quiet, loud}},
		}}
	s := Compute(&result.Run{Params: result.Params{NoiseMaxPct: 3}, Stability: st})
	if s.Stab.Rows[0].Noisy || !s.Stab.Rows[1].Noisy || s.Stab.Rows[1].NoisePct != 12 {
		t.Fatalf("%+v", s.Stab.Rows)
	}
	all := strings.Join(s.Warnings, "\n")
	if !strings.Contains(all, "до 12% процессора") || !strings.Contains(all, "«все остальные потоки загружены на 100%»") {
		t.Fatalf("%v", s.Warnings)
	}
}
