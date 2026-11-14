// Package kernels contains synthetic workloads. One work unit takes about
// 1 µs on the reference machine (Xeon E5-2667 v2, single core, idle system).
package kernels

import (
	"errors"
	"fmt"
)

// Version changes whenever the amount of work per unit changes.
// Results with different versions are not comparable.
const Version = 1

// Iterations per unit, calibrated on the reference machine (see calibrate_test.go).
// Changing any of them requires Version++.
const (
	intItersPerUnit = 1000
	fpItersPerUnit  = 200
	avxItersPerUnit = 200
	memItersPerUnit = 10
)

var ErrUnsupported = errors.New("не поддерживается этим процессором")

type Kernel interface {
	Name() string
	// Run executes units of work and returns a value that depends on all of it.
	Run(units int) uint64
}

type factory func(memBytes int) (Kernel, error)

var registry = map[string]factory{
	"int": func(int) (Kernel, error) { return newInt(), nil },
	"fp":  func(int) (Kernel, error) { return newFP(), nil },
	"mem": newMem,
}

var order = []string{"int", "fp", "avx", "mem"}

func Names() []string { return order }

func New(name string, memBytes int) (Kernel, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("неизвестная нагрузка %q", name)
	}
	return f(memBytes)
}
