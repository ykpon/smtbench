package runner

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"smtbench/internal/affinity"
	"smtbench/internal/cpuload"
	"smtbench/internal/freq"
	"smtbench/internal/kernels"
	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/timing"
	"smtbench/internal/topology"
)

func detect(t *testing.T) *topology.Topology {
	t.Helper()
	top, err := topology.Detect()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := affinity.Allowed()
	if err != nil {
		t.Fatal(err)
	}
	top, err = top.Restrict(allowed)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func quickConfig(t *testing.T, durMS int) (Config, []plan.Window) {
	top := detect(t)
	p := result.Params{Cores: []int{top.Cores[0].ID}, DurationMS: durMS, WarmupMS: 50, Reps: 1, Step: 100,
		Order: "near", BgThreads: 2, Modes: []string{"single", "pair"}, NoiseMaxPct: 100}
	ws, err := plan.Build(top, p)
	if err != nil {
		t.Fatal(err)
	}
	return Config{Topo: top, Params: p, Mix: kernels.Mix{"int": 10}, Clock: timing.Calibrate(50 * time.Millisecond)}, ws
}

func TestRunTwoWindows(t *testing.T) {
	cfg, ws := quickConfig(t, 200)
	points, err := Run(context.Background(), cfg, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != len(ws) {
		t.Fatalf("points %d windows %d", len(points), len(ws))
	}
	for i, p := range points {
		if string(ws[i].Mode) != p.Mode || len(p.Threads) != len(ws[i].Measure) {
			t.Fatalf("point %d: %+v vs window %+v", i, p, ws[i])
		}
		for _, th := range p.Threads {
			if th.TicksPerSec <= 0 || th.TickP99us < th.TickP50us || th.TickP50us <= 0 {
				t.Fatalf("point %d thread %+v", i, th)
			}
		}
		if p.Mode == "pair" && p.Threads[0].CPU == p.Threads[1].CPU {
			t.Fatalf("pair measured one cpu twice: %+v", p)
		}
	}
}

func TestRunCancel(t *testing.T) {
	base := runtime.NumGoroutine()
	cfg, ws := quickConfig(t, 5000)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	_, err := Run(ctx, cfg, ws)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Fatalf("returned after %v", d)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("goroutines %d > baseline %d", n, base)
	}
}

func TestRunInitError(t *testing.T) {
	cfg, ws := quickConfig(t, 100)
	cfg.Mix = kernels.Mix{"mem": 1}
	cfg.MemBytes = 10 // too small: kernel init fails
	if _, err := Run(context.Background(), cfg, ws); err == nil {
		t.Fatal("expected init error")
	}
}

// busySampler reports every CPU as 50% busy between any two samples.
type busySampler struct{ n uint64 }

func (s *busySampler) Sample() (map[int]cpuload.Times, error) {
	s.n++
	m := map[int]cpuload.Times{}
	for c := 0; c < 1024; c++ {
		m[c] = cpuload.Times{Busy: 50 * s.n, Total: 100 * s.n}
	}
	return m, nil
}

func TestNoisyRetry(t *testing.T) {
	cfg, ws := quickConfig(t, 100)
	if len(cfg.Topo.Cores[0].Threads) < 2 {
		t.Skip("needs SMT")
	}
	cfg.Load = &busySampler{}
	cfg.Params.NoiseMaxPct = 3
	points, err := Run(context.Background(), cfg, ws[:1]) // single k=0, sibling idle
	if err != nil {
		t.Fatal(err)
	}
	if p := points[0]; p.Retries != 2 || !p.Noisy || p.NoisePct < 49 {
		t.Fatalf("%+v", p)
	}
}

func TestETAMatchesRemaining(t *testing.T) {
	cfg, ws := quickConfig(t, 30)
	cfg.Params.WarmupMS = 0
	var etaAt time.Time
	var eta time.Duration
	cfg.Progress = func(p Progress) {
		if p.Done == p.Total/2 {
			etaAt, eta = time.Now(), p.ETA
		}
	}
	if _, err := Run(context.Background(), cfg, ws); err != nil {
		t.Fatal(err)
	}
	actual := time.Since(etaAt)
	if r := float64(eta) / float64(actual); r < 0.7 || r > 1.3 {
		t.Fatalf("eta %v actual %v", eta, actual)
	}
}

type fakeCounter struct{ n uint64 }

func (c *fakeCounter) Cycles() (uint64, error) { c.n += 1_000_000; return c.n, nil }
func (c *fakeCounter) Close() error            { return nil }

type fakeFreq struct{}

func (fakeFreq) Name() string                   { return "fake" }
func (fakeFreq) Open(int) (freq.Counter, error) { return &fakeCounter{}, nil }

func TestRunReportsMHz(t *testing.T) {
	cfg, ws := quickConfig(t, 100)
	cfg.Freq = fakeFreq{}
	points, err := Run(context.Background(), cfg, ws[:1])
	if err != nil {
		t.Fatal(err)
	}
	// 1e6 cycles over ~100 ms = ~10 MHz
	if mhz := points[0].Threads[0].MHz; mhz < 5 || mhz > 20 {
		t.Fatalf("mhz %v", mhz)
	}
}
