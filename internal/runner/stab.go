package runner

import (
	"context"
	"errors"
	"math"
	"runtime"
	"runtime/debug"
	"time"

	"smtbench/internal/plan"
	"smtbench/internal/result"
)

type StabProgress struct {
	Done, Total int
	Stab        *result.Stability
	Scenario    result.StabScenario
}

// Stability runs the tick rate test. The server tick is first calibrated on
// the first server CPU of an idle machine so that it takes TickLoadPct of the
// tick period. On cancellation it returns the scenarios finished so far.
func Stability(ctx context.Context, cfg Config, sw []plan.StabWindow) (*result.Stability, error) {
	if len(sw) == 0 {
		return nil, nil
	}
	p := cfg.Params
	if p.TickRate <= 0 || p.TickLoadPct <= 0 || p.StabMS <= 0 {
		return nil, errors.New("не заданы параметры теста тикрейта")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU() + 4))

	r := &runner{cfg: cfg, workers: map[int]*worker{}, state: map[int]state{}}
	defer r.shutdown()
	cpus := map[int]bool{}
	for _, w := range sw {
		for _, list := range [][]int{w.Servers, w.Busy, w.Bursty} {
			for _, c := range list {
				cpus[c] = true
			}
		}
	}
	if err := r.startCPUs(cpus); err != nil {
		return nil, err
	}

	calib := sw[0].Servers[0]
	r.set(calib, stLoad)
	if err := sleepCtx(ctx, time.Duration(plan.StabWarmupMS)*time.Millisecond); err != nil {
		return nil, err
	}
	ths, _, err := r.measure(ctx, []int{calib}, plan.StabCalibMS)
	if err != nil {
		return nil, err
	}
	r.set(calib, stIdle)
	tickUs := 1e6 / ths[0].TicksPerSec
	periodUs := 1e6 / float64(p.TickRate)
	work := max(1, int(math.Round(periodUs*float64(p.TickLoadPct)/100/tickUs)))
	st := &result.Stability{
		TickRate:    p.TickRate,
		TickLoadPct: p.TickLoadPct,
		WorkTicks:   work,
		IdleWorkUs:  float64(work) * tickUs,
		CalibCPU:    calib,
		DurationMS:  p.StabMS,
		Scenarios:   []result.StabScenario{},
	}

	for i, w := range sw {
		sc, err := r.scenario(ctx, w, st)
		if err != nil {
			return st, err
		}
		st.Scenarios = append(st.Scenarios, sc)
		if cfg.StabProgress != nil {
			cfg.StabProgress(StabProgress{Done: i + 1, Total: len(sw), Stab: st, Scenario: sc})
		}
	}
	for c := range r.workers {
		r.set(c, stIdle)
	}
	return st, nil
}

func (r *runner) scenario(ctx context.Context, w plan.StabWindow, st *result.Stability) (result.StabScenario, error) {
	want := map[int]state{}
	for _, c := range w.Busy {
		want[c] = stLoad
	}
	for _, c := range w.Bursty {
		want[c] = stBurst
	}
	for c := range r.workers {
		r.set(c, want[c])
	}
	clock := r.cfg.Clock
	period := uint64(clock.TicksPerUs * 1e6 / float64(st.TickRate))
	done := make(chan result.StabServer, len(w.Servers))
	g := &gameCmd{
		origin: clock.Now() + uint64(clock.TicksPerUs*2000),
		period: period,
		rate:   st.TickRate,
		work:   st.WorkTicks,
		warm:   plan.StabWarmupMS * st.TickRate / 1000,
		slots:  st.DurationMS * st.TickRate / 1000,
		done:   done,
	}
	for _, c := range w.Servers {
		r.workers[c].cmds <- cmd{st: stGame, game: g}
		r.state[c] = stIdle // workers go idle after the loop
	}
	sc := result.StabScenario{Name: w.Name, Busy: orEmpty(w.Busy), Bursty: orEmpty(w.Bursty)}
	got := map[int]result.StabServer{}
	for len(got) < len(w.Servers) {
		select {
		case <-ctx.Done():
			return sc, ctx.Err()
		case s := <-done:
			got[s.CPU] = s
		}
	}
	for _, c := range w.Servers {
		sc.Servers = append(sc.Servers, got[c])
	}
	return sc, nil
}

func orEmpty(x []int) []int {
	if x == nil {
		return []int{}
	}
	return x
}
