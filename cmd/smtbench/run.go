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
	"strings"
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
	stab     func(context.Context, runner.Config, []plan.StabWindow) (*result.Stability, error)
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
		stab:     runner.Stability,
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
	runs, err := parseRunFlags(args, d.hostname)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execRuns(ctx, runs, d)
}

// job is one variant ready to run.
type job struct {
	o  runOpts
	ws []plan.Window
	sw []plan.StabWindow
}

func (j job) estimate() time.Duration {
	return plan.Estimate(j.ws, j.o.Params) + plan.StabEstimate(j.sw, j.o.Params)
}

// outPath is the result file of a variant: --out or the default name,
// with the variant suffix when the command runs several variants.
func outPath(o runOpts, started time.Time) string {
	if o.Out == "" {
		name := "smtbench-" + o.Label
		if o.Suffix != "" {
			name += "-" + o.Suffix
		}
		return name + "-" + started.Format("20060102-150405") + ".json"
	}
	if o.Suffix == "" {
		return o.Out
	}
	ext := ".json"
	base := strings.TrimSuffix(o.Out, ext)
	if base == o.Out {
		ext = ""
	}
	return base + "-" + o.Suffix + ext
}

// execRuns runs every variant in turn. Each result is saved and sent as soon
// as it is ready; Ctrl+C stops the current variant and skips the rest.
func execRuns(ctx context.Context, runs []runOpts, d runDeps) int {
	w := d.out
	fail := func(err error) int {
		fmt.Fprintln(w, "ошибка:", err)
		return 1
	}
	t, err := d.detect()
	if err != nil {
		return fail(err)
	}
	avail := ^uint64(0)
	if !runs[0].DryRun {
		if a, err := d.availMem(); err == nil {
			avail = a
		}
	}

	var jobs []job
	var skipped []string
	for _, o := range runs {
		ws, err := plan.Build(t, o.Params)
		if err != nil {
			return fail(err)
		}
		if err := preflight(o, t, avail, d.hasAVX); err != nil {
			if errors.Is(err, errNoAVX) && o.AllProfiles {
				skipped = append(skipped, o.Variant)
				continue
			}
			return fail(err)
		}
		jobs = append(jobs, job{o: o, ws: ws, sw: plan.Stability(t, o.Params)})
	}
	if len(jobs) == 0 {
		return fail(errNoAVX)
	}

	report.Machine(w, t)
	if len(jobs) == 1 {
		report.Plan(w, jobs[0].ws, jobs[0].sw, jobs[0].o.Params, jobs[0].estimate())
	} else {
		var total time.Duration
		for _, j := range jobs {
			total += j.estimate()
		}
		fmt.Fprintf(w, "Вариантов: %d, тест пройдёт по разу для каждого. Общая оценка времени: ~%s\n", len(jobs), report.Duration(total))
		for i, j := range jobs {
			fmt.Fprintf(w, "\n%d. %s\n", i+1, j.o.Variant)
			report.Plan(w, j.ws, j.sw, j.o.Params, j.estimate())
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(w, "Пропущено: %s — процессор не поддерживает AVX\n", strings.Join(skipped, ", "))
	}
	fmt.Fprintln(w)
	if runs[0].DryRun {
		return 0
	}

	var warnings []string
	if busy := d.busyPct(); busy > 10 {
		msg := fmt.Sprintf("перед тестом машина уже была загружена на %d%%: результаты могут быть занижены", int(busy))
		fmt.Fprintln(w, "Внимание:", msg)
		warnings = append(warnings, msg)
	}
	clock := timing.Calibrate(200 * time.Millisecond)
	if !clock.TSC {
		warnings = append(warnings, "нет инвариантного TSC, длительность тиков измерена системным таймером")
	}
	group := ""
	if len(jobs) > 1 {
		group = result.NewRunID()
	}
	started := time.Now()

	code := 0
	var done []report.GroupRow
	for i, j := range jobs {
		if ctx.Err() != nil {
			code = 130
			break
		}
		if len(jobs) > 1 {
			fmt.Fprintf(w, "\n=== Вариант %d из %d: %s ===\n", i+1, len(jobs), j.o.Variant)
		}
		run, stopped, err := runOne(ctx, j, t, clock, warnings, d)
		if err != nil {
			return fail(err)
		}
		if group != "" {
			run.GroupID, run.GroupIndex, run.GroupTotal = group, i+1, len(jobs)
		}
		o := j.o
		o.Out = outPath(o, started)
		if err := result.Save(o.Out, run); err != nil {
			return fail(fmt.Errorf("не удалось сохранить результат: %w", err))
		}
		sum := summary.Compute(run)
		report.Summary(w, run, sum)
		fmt.Fprintf(w, "\nРезультат сохранён: %s\n", o.Out)
		done = append(done, report.GroupRow{Variant: o.Variant, Run: run, Sum: sum})
		if o.Send != "" && !sendRun(w, d, o, run) {
			code = 2
		}
		if stopped {
			code = 130
			break
		}
	}
	if len(jobs) > 1 {
		report.Group(w, done)
		if len(done) < len(jobs) {
			fmt.Fprintf(w, "Пройдено вариантов: %d из %d\n", len(done), len(jobs))
		}
	}
	return code
}

// runOne runs the ramp and the tick rate test of one variant. When
// stopped is true, the run was interrupted and run holds what was measured.
func runOne(ctx context.Context, j job, t *topology.Topology, clock timing.Clock, warnings []string, d runDeps) (run *result.Run, stopped bool, err error) {
	w := d.out
	o := j.o
	run = &result.Run{
		SchemaVersion:  result.SchemaVersion,
		KernelsVersion: kernels.Version,
		ToolVersion:    buildinfo.Version,
		RunID:          result.NewRunID(),
		Label:          o.Label,
		Hostname:       d.hostname,
		OS:             runtime.GOOS,
		CPU:            t.Info(),
		Params:         o.Params,
		Points:         []result.Point{},
		Warnings:       append([]string{}, warnings...),
	}
	run.CPU.Features = features()
	run.CPU.FreqSource = "none"
	if d.freq != nil {
		run.CPU.FreqSource = d.freq.Name()
	}
	cfg := runner.Config{Topo: t, Params: o.Params, Mix: o.Mix, MemBytes: o.MemBytes, Clock: clock, Load: d.load, Freq: d.freq, PrewarmMS: 5000}
	if d.freq != nil && d.freq.Name() == "none" {
		cfg.Freq = nil
	}
	pr := &report.Printer{W: w, Cores: len(t.Cores)}
	cfg.Progress = pr.Line
	cfg.StabProgress = func(p runner.StabProgress) { report.StabLine(w, p) }

	run.StartedAt = time.Now().UTC()
	defer func() { run.FinishedAt = time.Now().UTC() }()
	if len(j.ws) > 0 {
		points, err := d.run(ctx, cfg, j.ws)
		if points != nil {
			run.Points = points
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				return nil, false, err
			}
			return interrupted(w, run)
		}
	}
	if len(j.sw) > 0 {
		fmt.Fprintln(w)
		st, err := d.stab(ctx, cfg, j.sw)
		run.Stability = st
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				return nil, false, err
			}
			return interrupted(w, run)
		}
	}
	return run, false, nil
}

func interrupted(w io.Writer, run *result.Run) (*result.Run, bool, error) {
	run.Partial = true
	fmt.Fprintln(w, "\nТест прерван, сохраняю то, что успели измерить.")
	return run, true, nil
}

// sendRun sends a saved run and reports whether it succeeded.
func sendRun(w io.Writer, d runDeps, o runOpts, run *result.Run) bool {
	raw, _ := json.Marshal(run)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	url, err := d.send(ctx, o.Send, raw)
	if err != nil {
		fmt.Fprintf(w, "Не удалось отправить: %v\nПовторить: smtbench send %s --to %s\n", err, o.Out, o.Send)
		return false
	}
	fmt.Fprintf(w, "Отправлено: %s\n", url)
	return true
}
