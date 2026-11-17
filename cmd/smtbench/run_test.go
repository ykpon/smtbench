package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/runner"
	"smtbench/internal/topology"
)

func fakeDeps(t *testing.T, run func(context.Context, runner.Config, []plan.Window) ([]result.Point, error)) (runDeps, *strings.Builder) {
	out := &strings.Builder{}
	return runDeps{
		detect:   func() (*topology.Topology, error) { return xeonTopo(t), nil },
		run:      run,
		stab:     okStab,
		send:     func(context.Context, string, []byte) (string, error) { return "http://c/#/run/x", nil },
		availMem: func() (uint64, error) { return 1 << 40, nil },
		busyPct:  func() float64 { return 0 },
		hasAVX:   true,
		out:      out,
		hostname: "host",
	}, out
}

func okPoints(_ context.Context, _ runner.Config, ws []plan.Window) ([]result.Point, error) {
	var ps []result.Point
	for _, w := range ws {
		p := result.Point{Core: w.Core, Mode: string(w.Mode), LoadedCores: w.LoadedCores, LoadedCPUs: w.Bg}
		for _, c := range w.Measure {
			p.Threads = append(p.Threads, result.ThreadResult{CPU: c, TicksPerSec: 1000, TickP50us: 100, TickP99us: 120})
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func opts(t *testing.T, args ...string) runOpts {
	o, err := parseOne(append([]string{"--cores", "0", "--out", filepath.Join(t.TempDir(), "r.json")}, args...), "host")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestExecRunOK(t *testing.T) {
	d, out := fakeDeps(t, okPoints)
	o := opts(t, "--send", "http://c")
	if code := execRuns(context.Background(), []runOpts{o}, d); code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	r, err := result.Load(o.Out)
	if err != nil {
		t.Fatal(err)
	}
	if r.Partial || len(r.Points) != 49 || r.Label != "host" || r.CPU.Model == "" && len(r.CPU.Logical) == 0 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(out.String(), "http://c/#/run/x") {
		t.Fatalf("no link in output:\n%s", out)
	}
}

func TestExecRunSendFails(t *testing.T) {
	d, out := fakeDeps(t, okPoints)
	d.send = func(context.Context, string, []byte) (string, error) { return "", errors.New("connection refused") }
	o := opts(t, "--send", "http://c")
	if code := execRuns(context.Background(), []runOpts{o}, d); code != 2 {
		t.Fatalf("code %d", code)
	}
	if _, err := result.Load(o.Out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "smtbench send") {
		t.Fatalf("no resend hint:\n%s", out)
	}
}

func TestExecRunInterrupted(t *testing.T) {
	d, _ := fakeDeps(t, func(ctx context.Context, c runner.Config, ws []plan.Window) ([]result.Point, error) {
		ps, _ := okPoints(ctx, c, ws[:3])
		return ps, context.Canceled
	})
	o := opts(t)
	if code := execRuns(context.Background(), []runOpts{o}, d); code != 130 {
		t.Fatalf("code %d", code)
	}
	r, err := result.Load(o.Out)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Partial || len(r.Points) != 3 {
		t.Fatalf("partial %v points %d", r.Partial, len(r.Points))
	}
}

func TestExecRunDryRun(t *testing.T) {
	called := false
	d, out := fakeDeps(t, func(context.Context, runner.Config, []plan.Window) ([]result.Point, error) {
		called = true
		return nil, nil
	})
	if code := execRuns(context.Background(), []runOpts{opts(t, "--dry-run")}, d); code != 0 || called {
		t.Fatalf("code %d called %v", code, called)
	}
	if !strings.Contains(out.String(), "мин") {
		t.Fatalf("no estimate:\n%s", out)
	}
}

func TestExecRunPreflightStops(t *testing.T) {
	called := false
	d, _ := fakeDeps(t, func(context.Context, runner.Config, []plan.Window) ([]result.Point, error) {
		called = true
		return nil, nil
	})
	d.availMem = func() (uint64, error) { return 1 << 20, nil }
	d.out = io.Discard
	if code := execRuns(context.Background(), []runOpts{opts(t, "--profile", "minecraft")}, d); code != 1 || called {
		t.Fatalf("code %d called %v", code, called)
	}
}

func okStab(_ context.Context, c runner.Config, sw []plan.StabWindow) (*result.Stability, error) {
	p := c.Params
	st := &result.Stability{TickRate: p.TickRate, TickLoadPct: p.TickLoadPct, WorkTicks: 10, IdleWorkUs: 7800, DurationMS: p.StabMS}
	secs := p.StabMS / 1000
	for _, w := range sw {
		sc := result.StabScenario{Name: w.Name}
		for _, cpu := range w.Servers {
			s := result.StabServer{CPU: cpu, Ticks: secs * p.TickRate, WorkMeanUs: 7800, WorkP50us: 7800, WorkP99us: 9000, WorkMaxUs: 9500}
			for i := 0; i < secs; i++ {
				s.PerSec = append(s.PerSec, p.TickRate)
				s.MaxWorkPerSec = append(s.MaxWorkPerSec, 9500)
			}
			sc.Servers = append(sc.Servers, s)
		}
		st.Scenarios = append(st.Scenarios, sc)
	}
	return st, nil
}

func TestExecRunsGroup(t *testing.T) {
	d, out := fakeDeps(t, okPoints)
	var sent []*result.Run
	d.send = func(_ context.Context, _ string, raw []byte) (string, error) {
		r, err := result.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		sent = append(sent, r)
		return "http://c/#/run/" + r.RunID, nil
	}
	dir := t.TempDir()
	runs, err := parseRunFlags([]string{"--cores", "0", "--profile", "int,fp", "--bg-threads", "1,2", "--send", "c",
		"--out", filepath.Join(dir, "r.json")}, "host")
	if err != nil {
		t.Fatal(err)
	}
	if code := execRuns(context.Background(), runs, d); code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if len(sent) != 4 {
		t.Fatalf("sent %d", len(sent))
	}
	for i, r := range sent {
		if r.GroupID == "" || r.GroupID != sent[0].GroupID || r.GroupIndex != i+1 || r.GroupTotal != 4 || r.Stability == nil {
			t.Fatalf("run %d: group %q %d/%d stab %v", i, r.GroupID, r.GroupIndex, r.GroupTotal, r.Stability != nil)
		}
	}
	if sent[3].Params.Profile != "fp" || sent[3].Params.BgThreads != 2 {
		t.Fatalf("%+v", sent[3].Params)
	}
	if _, err := result.Load(filepath.Join(dir, "r-fp-bg2.json")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Вариантов: 4", "Вариант 4 из 4: fp · соседи 2", "Все варианты (4)", "Тест тикрейта"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestExecRunsInterruptedSkipsRest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	d, out := fakeDeps(t, func(c context.Context, cfg runner.Config, ws []plan.Window) ([]result.Point, error) {
		calls++
		if calls == 2 {
			cancel()
			ps, _ := okPoints(c, cfg, ws[:2])
			return ps, context.Canceled
		}
		return okPoints(c, cfg, ws)
	})
	runs, _ := parseRunFlags([]string{"--cores", "0", "--profile", "int,fp,source", "--out", filepath.Join(t.TempDir(), "r.json")}, "host")
	if code := execRuns(ctx, runs, d); code != 130 || calls != 2 {
		t.Fatalf("code %d calls %d\n%s", code, calls, out)
	}
	if !strings.Contains(out.String(), "Пройдено вариантов: 2 из 3") {
		t.Fatalf("\n%s", out)
	}
}

func TestExecRunsSkipsAVXForAll(t *testing.T) {
	d, out := fakeDeps(t, okPoints)
	d.hasAVX = false
	runs, _ := parseRunFlags([]string{"--cores", "0", "--profile", "all", "--dry-run"}, "host")
	if code := execRuns(context.Background(), runs, d); code != 0 || !strings.Contains(out.String(), "Пропущено: avx") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestExecRunsStabilityOnly(t *testing.T) {
	d, out := fakeDeps(t, func(context.Context, runner.Config, []plan.Window) ([]result.Point, error) {
		t.Fatal("ramp must not run")
		return nil, nil
	})
	o := opts(t, "--modes", "stability", "--stab-duration", "2s")
	if code := execRuns(context.Background(), []runOpts{o}, d); code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	r, err := result.Load(o.Out)
	if err != nil || r.Stability == nil || len(r.Points) != 0 {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.Contains(out.String(), "один сервер на пустой машине") {
		t.Fatalf("\n%s", out)
	}
}

func TestExecRunsExplicitAVXFails(t *testing.T) {
	d, _ := fakeDeps(t, okPoints)
	d.hasAVX = false
	d.out = io.Discard
	runs, _ := parseRunFlags([]string{"--cores", "0", "--profile", "avx,int", "--dry-run"}, "host")
	if code := execRuns(context.Background(), runs, d); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestExecRunsInterruptedInStability(t *testing.T) {
	d, _ := fakeDeps(t, okPoints)
	d.stab = func(ctx context.Context, c runner.Config, sw []plan.StabWindow) (*result.Stability, error) {
		st, _ := okStab(ctx, c, sw[:2])
		return st, context.Canceled
	}
	o := opts(t, "--stab-duration", "2s")
	if code := execRuns(context.Background(), []runOpts{o}, d); code != 130 {
		t.Fatalf("code %d", code)
	}
	r, err := result.Load(o.Out)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Partial || len(r.Points) != 49 || r.Stability == nil || len(r.Stability.Scenarios) != 2 {
		t.Fatalf("partial %v points %d stab %+v", r.Partial, len(r.Points), r.Stability)
	}
}
