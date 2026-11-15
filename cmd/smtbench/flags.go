package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"smtbench/internal/kernels"
	"smtbench/internal/result"
	"smtbench/internal/topology"
)

type runOpts struct {
	Params   result.Params
	Mix      kernels.Mix
	MemBytes int
	Label    string
	Out      string
	Send     string
	DryRun   bool
}

const defaultMemMB = 64

func profileNames() string {
	var names []string
	for _, p := range kernels.Profiles() {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

func parseRunFlags(args []string, hostname string) (runOpts, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	profile := fs.String("profile", "int", "профиль нагрузки: "+profileNames())
	mix := fs.String("mix", "", "своя смесь, например int=50,mem=40,fp=10")
	memSize := fs.String("mem-size", "", "размер буфера памяти на поток, например 64MB")
	duration := fs.Duration("duration", 3*time.Second, "длительность замера одного окна")
	warmup := fs.Duration("warmup", time.Second, "прогрев перед замером")
	reps := fs.Int("reps", 1, "повторы окна (берётся медиана)")
	cores := fs.String("cores", "", "какие ядра тестировать, например 0-3,8 (по умолчанию все)")
	modes := fs.String("modes", "single,solo_b,pair", "что мерить: single (1-й поток ядра один), solo_b (2-й поток один), pair (оба потока вместе)")
	step := fs.Int("step", 1, "шаг нарастания нагрузки, в ядрах")
	order := fs.String("order", "near", "порядок загрузки: near (сначала свой чиплет) или far")
	bgThreads := fs.String("bg-threads", "2", "сколько потоков грузить у соседних ядер: 2 (ядро занято целиком), 1 или auto (как у тестируемого ядра)")
	noiseMax := fs.Float64("noise-max", 3, "порог помех, %")
	label := fs.String("label", hostname, "имя ноды в интерфейсе")
	out := fs.String("out", "", "файл результата (по умолчанию smtbench-<label>-<дата>.json)")
	send := fs.String("send", "", "адрес коллектора, например http://10.0.0.5:8080")
	dry := fs.Bool("dry-run", false, "показать план и оценку времени без запуска")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fmt.Println("smtbench run [флаги]\n\nФлаги:")
			fs.PrintDefaults()
			return runOpts{}, err
		}
		return runOpts{}, fmt.Errorf("%v (smtbench run --help)", err)
	}

	o := runOpts{Label: *label, Out: *out, Send: *send, DryRun: *dry}
	p := &o.Params
	memMB := 0
	if *mix != "" {
		m, err := kernels.ParseMix(*mix)
		if err != nil {
			return o, err
		}
		o.Mix, p.Profile, memMB = m, "custom", defaultMemMB
	} else {
		prof, ok := kernels.ProfileByName(*profile)
		if !ok {
			return o, fmt.Errorf("неизвестный профиль %q, доступны: %s", *profile, profileNames())
		}
		o.Mix, p.Profile, memMB = prof.Mix, prof.Name, prof.MemSizeMB
	}
	o.MemBytes = memMB << 20
	if *memSize != "" {
		n, err := parseSize(*memSize)
		if err != nil {
			return o, err
		}
		o.MemBytes = n
	}
	if !o.Mix.NeedsMem() {
		o.MemBytes = 0
	}
	p.Mix = o.Mix
	p.MemSizeMB = o.MemBytes >> 20

	if *duration <= 0 || *warmup < 0 {
		return o, errors.New("--duration должен быть больше нуля, --warmup не меньше нуля")
	}
	if *reps < 1 {
		return o, errors.New("--reps должен быть не меньше 1")
	}
	if *step < 1 {
		return o, errors.New("--step должен быть не меньше 1")
	}
	if *order != "near" && *order != "far" {
		return o, fmt.Errorf("--order должен быть near или far, получено %q", *order)
	}
	bg, ok := map[string]int{"auto": 0, "1": 1, "2": 2}[*bgThreads]
	if !ok {
		return o, fmt.Errorf("--bg-threads должен быть auto, 1 или 2, получено %q", *bgThreads)
	}
	for _, m := range strings.Split(*modes, ",") {
		m = strings.TrimSpace(m)
		if m != "single" && m != "solo_b" && m != "pair" {
			return o, fmt.Errorf("неизвестный режим %q, доступны single, solo_b и pair", m)
		}
		p.Modes = append(p.Modes, m)
	}
	if c := strings.TrimSpace(*cores); c != "" && c != "all" {
		list, err := topology.ParseCPUList(c)
		if err != nil {
			return o, fmt.Errorf("--cores: %v", err)
		}
		p.Cores = list
	}
	p.DurationMS = int(duration.Milliseconds())
	p.WarmupMS = int(warmup.Milliseconds())
	p.Reps, p.Step, p.Order, p.BgThreads, p.NoiseMaxPct = *reps, *step, *order, bg, *noiseMax
	return o, nil
}

// parseSize parses sizes like 512KB, 64MB, 1GB (powers of 1024).
func parseSize(s string) (int, error) {
	u := strings.ToUpper(strings.TrimSpace(s))
	mult := 0
	for suffix, m := range map[string]int{"KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30} {
		if strings.HasSuffix(u, suffix) {
			u, mult = strings.TrimSuffix(u, suffix), m
			break
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(u))
	if mult == 0 || err != nil || n <= 0 {
		return 0, fmt.Errorf("неверный размер %q, пример: 64MB", s)
	}
	return n * mult, nil
}

const memReserve = 256 << 20

// preflight checks things that would make the run fail after load has started.
func preflight(o runOpts, t *topology.Topology, availMem uint64, hasAVX bool) error {
	if o.Mix["avx"] > 0 && !hasAVX {
		return errors.New("профиль использует AVX, а этот процессор его не поддерживает")
	}
	if o.Mix.NeedsMem() {
		need := uint64(len(t.CPUs))*uint64(o.MemBytes) + memReserve
		if need > availMem {
			return fmt.Errorf("не хватает памяти: нужно %d МБ (%d потоков × %d МБ), доступно %d МБ. Уменьшите --mem-size",
				need>>20, len(t.CPUs), o.MemBytes>>20, availMem>>20)
		}
	}
	return nil
}
