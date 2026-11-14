// Package plan turns topology and parameters into the ordered list of
// measurement windows ("ramp"): for each tested core, other cores are loaded
// one step at a time while thread A alone, then both threads, are measured.
package plan

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"smtbench/internal/result"
	"smtbench/internal/topology"
)

type Mode string

const (
	Single Mode = "single" // thread A measured, thread B idle
	SoloB  Mode = "solo_b" // thread B measured, thread A idle, k = 0
	Pair   Mode = "pair"   // both threads measured
	Final  Mode = "final"  // repeat of single k = 0 to detect drift
)

type Window struct {
	Core        int
	Mode        Mode
	Measure     []int // logical CPUs measured
	LoadedCores int
	Bg          []int // logical CPUs running background load
	OutsideL3   int   // loaded cores outside the target core's L3 group
}

// LoadOrder returns the other cores in the order they get loaded.
// near: same L3 group first, then same package, then other packages; far: reverse.
func LoadOrder(t *topology.Topology, target int, order string) ([]int, error) {
	if order != "near" && order != "far" {
		return nil, fmt.Errorf("порядок должен быть near или far, получено %q", order)
	}
	var others []int
	for _, c := range t.Cores {
		if c.ID != target {
			others = append(others, c.ID)
		}
	}
	sort.SliceStable(others, func(i, j int) bool {
		di, dj := t.Distance(target, others[i]), t.Distance(target, others[j])
		if di == dj {
			return others[i] < others[j]
		}
		if order == "near" {
			return di < dj
		}
		return di > dj
	})
	return others, nil
}

// Boundary is the number of leading cores in order that share the first core's distance.
func Boundary(t *topology.Topology, target int, order []int) int {
	if len(order) == 0 {
		return 0
	}
	d := t.Distance(target, order[0])
	n := 0
	for _, c := range order {
		if t.Distance(target, c) != d {
			break
		}
		n++
	}
	return n
}

// Steps returns loaded-core counts: multiples of step, n, and boundary if 0 < boundary < n.
func Steps(n, step, boundary int) []int {
	if step < 1 {
		step = 1
	}
	var ks []int
	for k := 0; k < n; k += step {
		ks = append(ks, k)
	}
	ks = append(ks, n)
	if boundary > 0 && boundary < n {
		ks = append(ks, boundary)
	}
	sort.Ints(ks)
	return slices.Compact(ks)
}

func validate(t *topology.Topology, p result.Params) error {
	if len(p.Modes) == 0 {
		return errors.New("не выбран ни один режим (--modes single,pair)")
	}
	for _, m := range p.Modes {
		if m != "single" && m != "pair" {
			return fmt.Errorf("неизвестный режим %q, доступны single и pair", m)
		}
	}
	if p.Order != "near" && p.Order != "far" {
		return fmt.Errorf("порядок должен быть near или far, получено %q", p.Order)
	}
	if p.BgThreads != 1 && p.BgThreads != 2 {
		return fmt.Errorf("--bg-threads должен быть 1 или 2, получено %d", p.BgThreads)
	}
	if p.Step < 1 {
		return errors.New("--step должен быть не меньше 1")
	}
	for _, c := range p.Cores {
		if _, ok := t.CoreByID(c); !ok {
			return fmt.Errorf("ядра %d нет (смотрите smtbench topo)", c)
		}
	}
	return nil
}

func Build(t *topology.Topology, p result.Params) ([]Window, error) {
	if err := validate(t, p); err != nil {
		return nil, err
	}
	targets := p.Cores
	if len(targets) == 0 {
		for _, c := range t.Cores {
			targets = append(targets, c.ID)
		}
	}
	doSingle := slices.Contains(p.Modes, "single")
	doPair := slices.Contains(p.Modes, "pair")

	var ws []Window
	for _, id := range targets {
		core, _ := t.CoreByID(id)
		order, _ := LoadOrder(t, id, p.Order)
		ks := Steps(len(order), p.Step, Boundary(t, id, order))

		at := func(mode Mode, k int, measure []int) Window {
			w := Window{Core: id, Mode: mode, Measure: measure, LoadedCores: k, Bg: []int{}}
			for _, oc := range order[:k] {
				c, _ := t.CoreByID(oc)
				w.Bg = append(w.Bg, c.Threads[:min(p.BgThreads, len(c.Threads))]...)
				if c.L3 != core.L3 {
					w.OutsideL3++
				}
			}
			return w
		}
		a := core.Threads[:1]
		smt := len(core.Threads) > 1
		if doSingle {
			for _, k := range ks {
				ws = append(ws, at(Single, k, a))
			}
		}
		if doPair && smt {
			ws = append(ws, at(SoloB, 0, core.Threads[1:2]))
			for _, k := range ks {
				ws = append(ws, at(Pair, k, core.Threads[:2]))
			}
		}
		if doSingle {
			ws = append(ws, at(Final, 0, a))
		}
	}
	return ws, nil
}

func Estimate(ws []Window, p result.Params) time.Duration {
	per := time.Duration(p.WarmupMS+p.DurationMS) * time.Millisecond
	return time.Duration(len(ws)*max(p.Reps, 1)) * per
}
