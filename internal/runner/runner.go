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
	Progress func(Progress)
}

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

	reps := max(p.Reps, 1)
	runs := make([][]result.ThreadResult, 0, reps)
	for i := 0; i < reps; i++ {
		ths, err := r.measure(ctx, w.Measure)
		if err != nil {
			return result.Point{}, err
		}
		runs = append(runs, ths)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i][0].TicksPerSec < runs[j][0].TicksPerSec })

	return result.Point{
		Core:        w.Core,
		Mode:        string(w.Mode),
		LoadedCores: w.LoadedCores,
		LoadedCPUs:  w.Bg,
		OutsideL3:   w.OutsideL3,
		Threads:     runs[len(runs)/2],
	}, nil
}

func (r *runner) measure(ctx context.Context, cpus []int) ([]result.ThreadResult, error) {
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
			return nil, ctx.Err()
		case s := <-done:
			got[s.cpu] = s
		}
	}
	out := make([]result.ThreadResult, len(cpus))
	for i, c := range cpus {
		s := got[c]
		out[i] = result.ThreadResult{
			CPU:         c,
			TicksPerSec: float64(s.ticks) / (s.wallUs / 1e6),
			TickP50us:   s.p50,
			TickP99us:   s.p99,
			TickP999us:  s.p999,
		}
	}
	return out, nil
}
