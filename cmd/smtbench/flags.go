package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"smtbench/internal/kernels"
	"smtbench/internal/plan"
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
	// Variant names this run among the runs of one command, e.g.
	// "source · соседи 2"; empty when the command runs one variant.
	Variant string
	// Suffix is added to the output file name of a variant.
	Suffix string
	// AllProfiles: the profile came from --profile all, so a profile the CPU
	// cannot run is skipped instead of failing the command.
	AllProfiles bool
}

const defaultMemMB = 64

func profileNames() string {
	var names []string
	for _, p := range kernels.Profiles() {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// defaultTickRate is the target tick rate of the tick rate test for a profile.
func defaultTickRate(profile string) int {
	if profile == "minecraft" {
		return 20
	}
	return 64
}

// splitList splits "a,b,,a" into unique non-empty values in order.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

type profileChoice struct {
	name  string
	mix   kernels.Mix
	memMB int
}

// parseRunFlags returns one runOpts per combination of the list flags
// --profile, --bg-threads and --order.
func parseRunFlags(args []string, hostname string) ([]runOpts, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	profile := fs.String("profile", "int", "профиль нагрузки, несколько через запятую или all: "+profileNames())
	mix := fs.String("mix", "", "своя смесь вместо профиля, например int=50,mem=40,fp=10")
	memSize := fs.String("mem-size", "", "размер буфера памяти на поток, например 64MB")
	duration := fs.Duration("duration", 3*time.Second, "длительность замера одного окна")
	warmup := fs.Duration("warmup", time.Second, "прогрев перед замером")
	reps := fs.Int("reps", 1, "повторы окна (берётся медиана)")
	cores := fs.String("cores", "", "какие ядра тестировать, например 0-3,8 (по умолчанию все)")
	modes := fs.String("modes", "single,solo_b,pair,stability", "что мерить: single (1-й поток ядра один), solo_b (2-й поток один), pair (оба потока вместе), stability (тест тикрейта)")
	step := fs.Int("step", 1, "шаг нарастания нагрузки, в ядрах")
	order := fs.String("order", "near", "порядок загрузки: near (сначала свой чиплет), far или near,far")
	bgThreads := fs.String("bg-threads", "2", "сколько потоков грузить у соседних ядер: 2 (ядро занято целиком), 1, auto (как у тестируемого ядра) или несколько через запятую")
	noiseMax := fs.Float64("noise-max", 3, "порог помех, %")
	tickRate := fs.Int("tickrate", 0, "тест тикрейта: тиков в секунду (по умолчанию 20 для minecraft, 64 для остальных)")
	tickLoad := fs.Int("tick-load", 50, "тест тикрейта: сколько процентов бюджета тика занимает работа сервера при непрерывной нагрузке на пустой машине")
	stabDur := fs.Duration("stab-duration", 10*time.Second, "тест тикрейта: длительность одного сценария")
	label := fs.String("label", hostname, "имя ноды в интерфейсе")
	out := fs.String("out", "", "файл результата (по умолчанию smtbench-<label>-<дата>.json)")
	send := fs.String("send", "", "адрес коллектора, например http://10.0.0.5:8080")
	dry := fs.Bool("dry-run", false, "показать план и оценку времени без запуска")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fmt.Println("smtbench run [флаги]\n\nФлаги:")
			fs.PrintDefaults()
			fmt.Println("\nУ --profile, --bg-threads и --order можно указать несколько значений через запятую:\n" +
				"тест пройдёт по разу для каждого сочетания, например --profile all --bg-threads 1,2.")
			return nil, err
		}
		return nil, fmt.Errorf("%v (smtbench run --help)", err)
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	var base runOpts
	base.Label, base.Out, base.Send, base.DryRun = *label, *out, *send, *dry
	p := &base.Params

	var profs []profileChoice
	all := false
	if *mix != "" {
		if set["profile"] {
			return nil, errors.New("укажите либо --profile, либо --mix")
		}
		m, err := kernels.ParseMix(*mix)
		if err != nil {
			return nil, err
		}
		profs = append(profs, profileChoice{"custom", m, defaultMemMB})
	} else {
		names := splitList(*profile)
		if slices.Contains(names, "all") {
			all = true
			names = nil
			for _, pr := range kernels.Profiles() {
				names = append(names, pr.Name)
			}
		}
		if len(names) == 0 {
			return nil, errors.New("--profile: не указан профиль")
		}
		for _, n := range names {
			prof, ok := kernels.ProfileByName(n)
			if !ok {
				return nil, fmt.Errorf("неизвестный профиль %q, доступны: %s, all", n, profileNames())
			}
			profs = append(profs, profileChoice{prof.Name, prof.Mix, prof.MemSizeMB})
		}
	}
	memOverride := 0
	if *memSize != "" {
		n, err := parseSize(*memSize)
		if err != nil {
			return nil, err
		}
		memOverride = n
	}

	if *duration <= 0 || *warmup < 0 {
		return nil, errors.New("--duration должен быть больше нуля, --warmup не меньше нуля")
	}
	if *reps < 1 {
		return nil, errors.New("--reps должен быть не меньше 1")
	}
	if *step < 1 {
		return nil, errors.New("--step должен быть не меньше 1")
	}
	orders := splitList(*order)
	if len(orders) == 0 {
		return nil, errors.New("--order: не указан порядок")
	}
	for _, o := range orders {
		if o != "near" && o != "far" {
			return nil, fmt.Errorf("--order должен быть near или far, получено %q", o)
		}
	}
	var bgs []int
	for _, v := range splitList(*bgThreads) {
		bg, ok := map[string]int{"auto": 0, "1": 1, "2": 2}[v]
		if !ok {
			return nil, fmt.Errorf("--bg-threads должен быть auto, 1 или 2, получено %q", v)
		}
		if !slices.Contains(bgs, bg) {
			bgs = append(bgs, bg)
		}
	}
	if len(bgs) == 0 {
		return nil, errors.New("--bg-threads: не указано значение")
	}
	for _, m := range splitList(*modes) {
		if !plan.ValidMode(m) {
			return nil, fmt.Errorf("неизвестный режим %q, доступны single, solo_b, pair и stability", m)
		}
		p.Modes = append(p.Modes, m)
	}
	if len(p.Modes) == 0 {
		return nil, errors.New("--modes: не выбран ни один режим")
	}
	if c := strings.TrimSpace(*cores); c != "" && c != "all" {
		list, err := topology.ParseCPUList(c)
		if err != nil {
			return nil, fmt.Errorf("--cores: %v", err)
		}
		p.Cores = list
	}
	if *tickRate < 0 || *tickRate > 1000 {
		return nil, errors.New("--tickrate должен быть от 1 до 1000")
	}
	if *tickLoad < 1 || *tickLoad > 100 {
		return nil, errors.New("--tick-load должен быть от 1 до 100")
	}
	if *stabDur < time.Second {
		return nil, errors.New("--stab-duration должен быть не меньше 1s")
	}
	p.DurationMS = int(duration.Milliseconds())
	p.WarmupMS = int(warmup.Milliseconds())
	p.Reps, p.Step, p.NoiseMaxPct = *reps, *step, *noiseMax
	if slices.Contains(p.Modes, "stability") {
		// whole seconds, so every second of the test has the same number of slots
		p.TickLoadPct, p.StabMS = *tickLoad, int(stabDur.Milliseconds())/1000*1000
	}

	var runs []runOpts
	for _, pr := range profs {
		for _, bg := range bgs {
			for _, ord := range orders {
				o := base
				o.Params.Modes = slices.Clone(base.Params.Modes)
				o.Mix, o.Params.Profile, o.Params.Mix = pr.mix, pr.name, pr.mix
				o.MemBytes = pr.memMB << 20
				if memOverride > 0 {
					o.MemBytes = memOverride
				}
				if !o.Mix.NeedsMem() {
					o.MemBytes = 0
				}
				o.Params.MemSizeMB = o.MemBytes >> 20
				o.Params.BgThreads, o.Params.Order = bg, ord
				if o.Params.StabMS > 0 {
					o.Params.TickRate = *tickRate
					if o.Params.TickRate == 0 {
						o.Params.TickRate = defaultTickRate(pr.name)
					}
				}
				o.AllProfiles = all
				var name, suffix []string
				if len(profs) > 1 {
					name, suffix = append(name, pr.name), append(suffix, pr.name)
				}
				if len(bgs) > 1 {
					name, suffix = append(name, "соседи "+bgName(bg)), append(suffix, "bg"+bgName(bg))
				}
				if len(orders) > 1 {
					name, suffix = append(name, ord), append(suffix, ord)
				}
				o.Variant, o.Suffix = strings.Join(name, " · "), strings.Join(suffix, "-")
				runs = append(runs, o)
			}
		}
	}
	return runs, nil
}

func bgName(bg int) string {
	if bg == 0 {
		return "auto"
	}
	return strconv.Itoa(bg)
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

var errNoAVX = errors.New("профиль использует AVX, а этот процессор его не поддерживает")

// preflight checks things that would make the run fail after load has started.
func preflight(o runOpts, t *topology.Topology, availMem uint64, hasAVX bool) error {
	if o.Mix["avx"] > 0 && !hasAVX {
		return errNoAVX
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
