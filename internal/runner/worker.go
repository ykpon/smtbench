package runner

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"smtbench/internal/affinity"
	"smtbench/internal/freq"
	"smtbench/internal/hist"
	"smtbench/internal/kernels"
	"smtbench/internal/timing"
)

type state int

const (
	stIdle state = iota
	stLoad
	stMeasure
	stExit
)

type cmd struct {
	st       state
	deadline uint64        // stMeasure: clock value to stop at
	done     chan<- sample // stMeasure: receives the result
}

type sample struct {
	cpu      int
	ticks    uint64
	wallUs   float64
	cpuTime  time.Duration
	p50, p99 float64 // µs
	p999     float64
	mhz      float64
	aborted  bool
}

// sink keeps tick results alive so the compiler cannot drop the work.
var sink uint64

type worker struct {
	cpu   int
	cmds  chan cmd
	clock timing.Clock
	h     hist.Hist
	freq  freq.Counter // nil when frequency is not measured
}

func startWorker(cpu int, mix kernels.Mix, memBytes int, clock timing.Clock, fsrc freq.Source, wg *sync.WaitGroup, ready chan<- error) *worker {
	w := &worker{cpu: cpu, cmds: make(chan cmd, 4), clock: clock}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The goroutine exits while still locked, so its OS thread is discarded
		// together with the pinned affinity.
		runtime.LockOSThread()
		if err := affinity.Pin(cpu); err != nil {
			ready <- err
			return
		}
		// Allocate after pinning so memory lands on the local NUMA node.
		tick, err := kernels.NewTick(mix, memBytes)
		if err != nil {
			ready <- err
			return
		}
		if fsrc != nil {
			if c, err := fsrc.Open(cpu); err == nil {
				w.freq = c
				defer c.Close()
			}
		}
		ready <- nil
		w.loop(tick)
	}()
	return w
}

func (w *worker) loop(tick *kernels.Tick) {
	cur := cmd{st: stIdle}
	var local uint64
	defer func() { atomic.AddUint64(&sink, local) }()
	for {
		switch cur.st {
		case stExit:
			return
		case stIdle:
			cur = <-w.cmds
		case stLoad:
		load:
			for {
				local += tick.Run()
				select {
				case cur = <-w.cmds:
					break load
				default:
				}
			}
		case stMeasure:
			next, s := w.measure(tick, cur.deadline, &local)
			cur.done <- s
			if next != nil {
				cur = *next
			} else {
				cur = cmd{st: stLoad} // keep running until told otherwise
			}
		}
	}
}

func (w *worker) measure(tick *kernels.Tick, deadline uint64, local *uint64) (*cmd, sample) {
	w.h.Reset()
	s := sample{cpu: w.cpu}
	cpu0, _ := affinity.ThreadCPUTime()
	var cyc0 uint64
	if w.freq != nil {
		cyc0, _ = w.freq.Cycles()
	}
	start := w.clock.Now()
	last := start
	var next *cmd
	for {
		*local += tick.Run()
		now := w.clock.Now()
		w.h.Add(uint64(w.clock.Us(now-last) * 1000))
		last = now
		s.ticks++
		if now >= deadline {
			break
		}
		select {
		case c := <-w.cmds:
			next, s.aborted = &c, true
		default:
		}
		if next != nil {
			break
		}
	}
	cpu1, _ := affinity.ThreadCPUTime()
	s.wallUs = w.clock.Us(last - start)
	if w.freq != nil {
		if cyc1, err := w.freq.Cycles(); err == nil && s.wallUs > 0 {
			s.mhz = float64(cyc1-cyc0) / s.wallUs
		}
	}
	s.cpuTime = cpu1 - cpu0
	s.p50 = w.h.Quantile(0.5) / 1000
	s.p99 = w.h.Quantile(0.99) / 1000
	s.p999 = w.h.Quantile(0.999) / 1000
	return next, s
}
