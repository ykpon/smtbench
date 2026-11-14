// Package runner executes measurement windows on pinned worker threads.
package runner

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"smtbench/internal/cpuload"
	"smtbench/internal/kernels"
	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/timing"
	"smtbench/internal/topology"
)

type Config struct {
	Topo     *topology.Topology
	Params   result.Params
	Mix      kernels.Mix
	MemBytes int
	Clock    timing.Clock
	Load     cpuload.Sampler // nil: idle siblings are not checked
	Progress func(Progress)
}

const maxRetries = 2

type Progress struct {
	Done, Total int
	Window      plan.Window
	Point       result.Point
	ETA         time.Duration
}

type runner struct {
	cfg     Config
	workers map[int]*worker
	state   map[int]state
	wg      sync.WaitGroup
}

// Run executes the windows in order. On context cancellation it stops all
// load and returns the points measured so far together with ctx.Err().
func Run(ctx context.Context, cfg Config, ws []plan.Window) ([]result.Point, error) {
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU() + 4))

	r := &runner{cfg: cfg, workers: map[int]*worker{}, state: map[int]state{}}
	defer r.shutdown()
	if err := r.start(ws); err != nil {
		return nil, err
	}

	points := make([]result.Point, 0, len(ws))
	began := time.Now()
	for i, w := range ws {
		p, err := r.window(ctx, w)
		if err != nil {
			return points, err
		}
		points = append(points, p)
		if cfg.Progress != nil {
			done := i + 1
			avg := time.Since(began) / time.Duration(done)
			cfg.Progress(Progress{Done: done, Total: len(ws), Window: w, Point: p, ETA: avg * time.Duration(len(ws)-done)})
		}
	}
	return points, nil
}

func (r *runner) start(ws []plan.Window) error {
	cpus := map[int]bool{}
	for _, w := range ws {
		for _, c := range w.Measure {
			cpus[c] = true
		}
		for _, c := range w.Bg {
			cpus[c] = true
		}
	}
	ready := make(chan error, len(cpus))
	for c := range cpus {
		r.workers[c] = startWorker(c, r.cfg.Mix, r.cfg.MemBytes, r.cfg.Clock, &r.wg, ready)
		r.state[c] = stIdle
	}
	var firstErr error
	for range cpus {
		if err := <-ready; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("подготовка нагрузки: %w", firstErr)
	}
	return nil
}

func (r *runner) shutdown() {
	for _, w := range r.workers {
		w.cmds <- cmd{st: stExit}
	}
	r.wg.Wait()
}

func (r *runner) set(cpu int, st state) {
	if r.state[cpu] != st {
		r.workers[cpu].cmds <- cmd{st: st}
		r.state[cpu] = st
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *runner) window(ctx context.Context, w plan.Window) (result.Point, error) {
	p := r.cfg.Params
	want := map[int]state{}
	for _, c := range w.Bg {
		want[c] = stLoad
	}
	for _, c := range w.Measure {
		want[c] = stLoad // warm-up
	}
	for c := range r.workers {
		r.set(c, want[c]) // missing entries are stIdle
	}
	if err := sleepCtx(ctx, time.Duration(p.WarmupMS)*time.Millisecond); err != nil {
		return result.Point{}, err
	}

	pt := result.Point{
		Core:        w.Core,
		Mode:        string(w.Mode),
		LoadedCores: w.LoadedCores,
		LoadedCPUs:  w.Bg,
		OutsideL3:   w.OutsideL3,
	}
	idle := r.idleSiblings(w)
	for attempt := 0; ; attempt++ {
		ths, noise, err := r.measureReps(ctx, w.Measure, idle)
		if err != nil {
			return result.Point{}, err
		}
		pt.Threads, pt.NoisePct, pt.Retries = ths, noise, attempt
		if noise <= p.NoiseMaxPct {
			pt.Noisy = false
			break
		}
		pt.Noisy = true
		if attempt == maxRetries {
			break
		}
	}
	return pt, nil
}

// idleSiblings returns threads of the measured core that are not measured.
func (r *runner) idleSiblings(w plan.Window) []int {
	core, _ := r.cfg.Topo.CoreByID(w.Core)
	var idle []int
	for _, th := range core.Threads {
		measured := false
		for _, m := range w.Measure {
			measured = measured || m == th
		}
		if !measured {
			idle = append(idle, th)
		}
	}
	return idle
}

// measureReps runs Reps measurements and returns the median one and the
// worst noise seen: time other processes took from measured threads, or
// busy share of idle siblings, in percent.
func (r *runner) measureReps(ctx context.Context, cpus, idle []int) ([]result.ThreadResult, float64, error) {
	reps := max(r.cfg.Params.Reps, 1)
	runs := make([][]result.ThreadResult, 0, reps)
	noise := 0.0
	for i := 0; i < reps; i++ {
		var before map[int]cpuload.Times
		if r.cfg.Load != nil {
			before, _ = r.cfg.Load.Sample()
		}
		ths, foreign, err := r.measure(ctx, cpus)
		if err != nil {
			return nil, 0, err
		}
		noise = max(noise, foreign)
		if r.cfg.Load != nil && before != nil {
			if after, err := r.cfg.Load.Sample(); err == nil {
				for _, c := range idle {
					noise = max(noise, cpuload.BusyPct(before, after, c))
				}
			}
		}
		runs = append(runs, ths)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i][0].TicksPerSec < runs[j][0].TicksPerSec })
	return runs[len(runs)/2], noise, nil
}

func (r *runner) measure(ctx context.Context, cpus []int) ([]result.ThreadResult, float64, error) {
	done := make(chan sample, len(cpus))
	clock := r.cfg.Clock
	deadline := clock.Now() + uint64(float64(r.cfg.Params.DurationMS)*1000*clock.TicksPerUs)
	for _, c := range cpus {
		r.workers[c].cmds <- cmd{st: stMeasure, deadline: deadline, done: done}
		r.state[c] = stLoad // workers fall back to load after measuring
	}
	got := map[int]sample{}
	for len(got) < len(cpus) {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case s := <-done:
			got[s.cpu] = s
		}
	}
	out := make([]result.ThreadResult, len(cpus))
	foreign := 0.0
	for i, c := range cpus {
		s := got[c]
		if s.wallUs > 0 {
			foreign = max(foreign, 100*(1-float64(s.cpuTime.Microseconds())/s.wallUs))
		}
		out[i] = result.ThreadResult{
			CPU:         c,
			TicksPerSec: float64(s.ticks) / (s.wallUs / 1e6),
			TickP50us:   s.p50,
			TickP99us:   s.p99,
			TickP999us:  s.p999,
		}
	}
	return out, foreign, nil
}
