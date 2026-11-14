package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/cpu"

	"smtbench/internal/buildinfo"
	"smtbench/internal/client"
	"smtbench/internal/cpuload"
	"smtbench/internal/freq"
	"smtbench/internal/kernels"
	"smtbench/internal/plan"
	"smtbench/internal/report"
	"smtbench/internal/result"
	"smtbench/internal/runner"
	"smtbench/internal/summary"
	"smtbench/internal/sysmem"
	"smtbench/internal/timing"
	"smtbench/internal/topology"
)

type runDeps struct {
	detect   func() (*topology.Topology, error)
	run      func(context.Context, runner.Config, []plan.Window) ([]result.Point, error)
	send     func(context.Context, string, []byte) (string, error)
	availMem func() (uint64, error)
	busyPct  func() float64
	freq     freq.Source
	load     cpuload.Sampler
	hasAVX   bool
	out      io.Writer
	hostname string
}

func realDeps() runDeps {
	host, _ := os.Hostname()
	return runDeps{
		detect:   detectTopology,
		run:      runner.Run,
		send:     client.Send,
		availMem: sysmem.Available,
		busyPct:  systemBusy,
		freq:     freq.Detect(),
		load:     cpuload.NewSampler(),
		hasAVX:   cpu.X86.HasAVX,
		out:      os.Stdout,
		hostname: host,
	}
}

// systemBusy returns the average busy share of all CPUs over one second.
func systemBusy() float64 {
	s := cpuload.NewSampler()
	a, err := s.Sample()
	if err != nil {
		return 0
	}
	time.Sleep(time.Second)
	b, err := s.Sample()
	if err != nil {
		return 0
	}
	sum := 0.0
	for c := range a {
		sum += cpuload.BusyPct(a, b, c)
	}
	return sum / float64(max(len(a), 1))
}

func features() []string {
	var f []string
	for name, ok := range map[string]bool{"avx": cpu.X86.HasAVX, "avx2": cpu.X86.HasAVX2, "fma": cpu.X86.HasFMA, "avx512f": cpu.X86.HasAVX512F} {
		if ok {
			f = append(f, name)
		}
	}
	return f
}

func cmdRun(args []string) int {
	d := realDeps()
	o, err := parseRunFlags(args, d.hostname)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execRun(ctx, o, d)
}

func execRun(ctx context.Context, o runOpts, d runDeps) int {
	w := d.out
	fail := func(err error) int {
		fmt.Fprintln(w, "ошибка:", err)
		return 1
	}
	t, err := d.detect()
	if err != nil {
		return fail(err)
	}
	ws, err := plan.Build(t, o.Params)
	if err != nil {
		return fail(err)
	}
	report.Header(w, t, ws, o.Params, plan.Estimate(ws, o.Params))
	if o.DryRun {
		return 0
	}
	avail, err := d.availMem()
	if err != nil {
		avail = ^uint64(0)
	}
	if err := preflight(o, t, avail, d.hasAVX); err != nil {
		return fail(err)
	}

	run := &result.Run{
		SchemaVersion:  result.SchemaVersion,
		KernelsVersion: kernels.Version,
		ToolVersion:    buildinfo.Version,
		RunID:          result.NewRunID(),
		Label:          o.Label,
		Hostname:       d.hostname,
		OS:             runtime.GOOS,
		CPU:            t.Info(),
		Params:         o.Params,
		Warnings:       []string{},
	}
	run.CPU.Features = features()
	run.CPU.FreqSource = "none"
	if d.freq != nil {
		run.CPU.FreqSource = d.freq.Name()
	}
	if busy := d.busyPct(); busy > 10 {
		msg := fmt.Sprintf("перед тестом машина уже была загружена на %d%%: результаты могут быть занижены", int(busy))
		fmt.Fprintln(w, "Внимание:", msg)
		run.Warnings = append(run.Warnings, msg)
	}
	clock := timing.Calibrate(200 * time.Millisecond)
	if !clock.TSC {
		run.Warnings = append(run.Warnings, "нет инвариантного TSC, длительность тиков измерена системным таймером")
	}
	if o.Out == "" {
		o.Out = fmt.Sprintf("smtbench-%s-%s.json", o.Label, time.Now().Format("20060102-150405"))
	}

	cfg := runner.Config{Topo: t, Params: o.Params, Mix: o.Mix, MemBytes: o.MemBytes, Clock: clock, Load: d.load, Freq: d.freq, PrewarmMS: 5000}
	if d.freq != nil && d.freq.Name() == "none" {
		cfg.Freq = nil
	}
	pr := &report.Printer{W: w, Cores: len(t.Cores)}
	cfg.Progress = pr.Line

	run.StartedAt = time.Now().UTC()
	points, err := d.run(ctx, cfg, ws)
	run.FinishedAt = time.Now().UTC()
	interrupted := errors.Is(err, context.Canceled)
	if err != nil && !interrupted {
		return fail(err)
	}
	run.Points = points
	if run.Points == nil {
		run.Points = []result.Point{}
	}
	run.Partial = interrupted
	if interrupted {
		fmt.Fprintln(w, "\nТест прерван, сохраняю то, что успели измерить.")
	}
	if err := result.Save(o.Out, run); err != nil {
		return fail(fmt.Errorf("не удалось сохранить результат: %w", err))
	}
	report.Summary(w, run, summary.Compute(run))
	fmt.Fprintf(w, "\nРезультат сохранён: %s\n", o.Out)

	code := 0
	if o.Send != "" {
		raw, _ := json.Marshal(run)
		sendCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		url, err := d.send(sendCtx, o.Send, raw)
		cancel()
		if err != nil {
			fmt.Fprintf(w, "Не удалось отправить: %v\nПовторить: smtbench send %s --to %s\n", err, o.Out, o.Send)
			code = 2
		} else {
			fmt.Fprintf(w, "Отправлено: %s\n", url)
		}
	}
	if interrupted {
		return 130
	}
	return code
}
