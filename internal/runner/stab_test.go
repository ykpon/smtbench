package runner

import (
	"context"
	"testing"
	"time"

	"smtbench/internal/kernels"
	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/timing"
)

func TestStability(t *testing.T) {
	top := detect(t)
	p := result.Params{Cores: []int{top.Cores[0].ID, top.Cores[1].ID}, Modes: []string{"stability"},
		TickRate: 64, TickLoadPct: 40, StabMS: 2000}
	sw := plan.Stability(top, p)
	cfg := Config{Topo: top, Params: p, Mix: kernels.Mix{"int": 10}, Clock: timing.Calibrate(50 * time.Millisecond)}
	var progress int
	cfg.StabProgress = func(StabProgress) { progress++ }
	st, err := Stability(context.Background(), cfg, sw)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Scenarios) != len(sw) || progress != len(sw) || st.WorkTicks < 1 {
		t.Fatalf("scenarios %d/%d progress %d work %d", len(st.Scenarios), len(sw), progress, st.WorkTicks)
	}
	alone := st.Scenarios[0]
	if alone.Name != plan.StabAlone || len(alone.Servers) != 1 {
		t.Fatalf("%+v", alone)
	}
	for _, sc := range st.Scenarios {
		for _, s := range sc.Servers {
			t.Logf("%-13s cpu %2d ticks %d per_sec %v work p50 %.0f p99 %.0f max %.0f µs (idle %.0f) sd %.0f µs wake p99 %.0f µs late %d overruns %d",
				sc.Name, s.CPU, s.Ticks, s.PerSec, s.WorkP50us, s.WorkP99us, s.WorkMaxUs, st.IdleWorkUs, s.IntervalSDus, s.WakeP99us, s.Late, s.Overruns)
		}
	}
	a := alone.Servers[0]
	if a.Ticks < 120 || len(a.PerSec) != 2 || a.PerSec[0] > 64 {
		t.Fatalf("alone: %+v", a)
	}
}
