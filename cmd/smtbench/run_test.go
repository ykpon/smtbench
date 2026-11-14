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
	o, err := parseRunFlags(append([]string{"--cores", "0", "--out", filepath.Join(t.TempDir(), "r.json")}, args...), "host")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestExecRunOK(t *testing.T) {
	d, out := fakeDeps(t, okPoints)
	o := opts(t, "--send", "http://c")
	if code := execRun(context.Background(), o, d); code != 0 {
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
	if code := execRun(context.Background(), o, d); code != 2 {
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
	if code := execRun(context.Background(), o, d); code != 130 {
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
	if code := execRun(context.Background(), opts(t, "--dry-run"), d); code != 0 || called {
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
	if code := execRun(context.Background(), opts(t, "--profile", "minecraft"), d); code != 1 || called {
		t.Fatalf("code %d called %v", code, called)
	}
}
