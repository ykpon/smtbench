package plan

import (
	"slices"
	"time"

	"smtbench/internal/result"
	"smtbench/internal/topology"
)

// StabWindow is one scenario of the tick rate test: server loops run on
// Servers, Busy CPUs run constant load, Bursty CPUs switch load on and off.
type StabWindow struct {
	Name    string
	Servers []int
	Busy    []int
	Bursty  []int
}

// Scenario names of the tick rate test.
const (
	StabAlone       = "alone"        // one server on an idle machine
	StabOnePerCore  = "one_per_core" // a server on the first thread of every tested core
	StabSiblingBusy = "sibling_busy" // as above, second threads under full load
	StabTwoPerCore  = "two_per_core" // servers on both threads of every tested core
	StabAllBusy     = "all_busy"     // servers on first threads, every other CPU under full load
	StabBursty      = "bursty"       // servers on first threads, every other CPU loaded on and off
)

// StabWarmupMS is the warm-up before each scenario of the tick rate test.
const StabWarmupMS = 1000

// StabCalibMS is how long the server tick is calibrated before the scenarios.
const StabCalibMS = 1000

// HasStability reports whether the tick rate test is selected.
func HasStability(p result.Params) bool {
	return slices.Contains(p.Modes, "stability") && p.StabMS > 0
}

// Stability returns the scenarios of the tick rate test, or nil when it is off.
func Stability(t *topology.Topology, p result.Params) []StabWindow {
	if !HasStability(p) {
		return nil
	}
	targets := p.Cores
	if len(targets) == 0 {
		for _, c := range t.Cores {
			targets = append(targets, c.ID)
		}
	}
	var first, second, tested []int
	for _, id := range targets {
		c, ok := t.CoreByID(id)
		if !ok {
			continue
		}
		first = append(first, c.Threads[0])
		tested = append(tested, c.Threads...)
		if len(c.Threads) > 1 {
			second = append(second, c.Threads[1])
		}
	}
	if len(first) == 0 {
		return nil
	}
	var others []int // every CPU except the first threads of tested cores
	for _, cpu := range t.CPUs {
		if !slices.Contains(first, cpu.ID) {
			others = append(others, cpu.ID)
		}
	}
	ws := []StabWindow{{Name: StabAlone, Servers: first[:1]}}
	if len(first) > 1 {
		ws = append(ws, StabWindow{Name: StabOnePerCore, Servers: first})
	}
	if len(second) > 0 {
		ws = append(ws,
			StabWindow{Name: StabSiblingBusy, Servers: first, Busy: second},
			StabWindow{Name: StabTwoPerCore, Servers: tested})
	}
	if len(others) > 0 {
		ws = append(ws,
			StabWindow{Name: StabAllBusy, Servers: first, Busy: others},
			StabWindow{Name: StabBursty, Servers: first, Bursty: others})
	}
	return ws
}

// StabEstimate is the time the tick rate test takes.
func StabEstimate(sw []StabWindow, p result.Params) time.Duration {
	if len(sw) == 0 {
		return 0
	}
	return time.Duration(StabCalibMS+len(sw)*(StabWarmupMS+p.StabMS)) * time.Millisecond
}
