package runner

import (
	"math"
	"math/rand/v2"
	"time"

	"smtbench/internal/kernels"
	"smtbench/internal/result"
)

// gameCmd starts a server loop: every period the server does work profile
// ticks, then sleeps until the next tick is due. Slots are numbered from
// origin; the first warm slots are not recorded.
type gameCmd struct {
	origin uint64 // clock value of slot 0
	period uint64 // clock units
	rate   int    // slots per second
	work   int    // profile ticks per server tick
	warm   int    // slots before recording starts
	slots  int    // recorded slots
	done   chan<- result.StabServer
}

// game runs the server loop. A tick that overruns its period delays the next
// one; missed slots are skipped, as a game server drops tick rate when it
// cannot keep up.
func (w *worker) game(tick *kernels.Tick, g *gameCmd, local *uint64) (*cmd, result.StabServer) {
	w.h.Reset()
	w.hInt.Reset()
	w.hWake.Reset()
	secs := (g.slots + g.rate - 1) / g.rate
	s := result.StabServer{CPU: w.cpu, PerSec: make([]int, secs), MaxWorkPerSec: make([]float64, secs)}
	// Each server gets its own phase, like independent servers on a node.
	phase := uint64(rand.Float64() * float64(g.period))
	origin := g.origin + phase
	periodUs := w.clock.Us(g.period)
	total := g.warm + g.slots

	var next *cmd
	var prevStart uint64
	var sumWork, sumInt, sumInt2 float64
	nInt := 0
	slot, last := 0, -1
	due := origin
	for slot < total {
		w.sleepUntil(due)
		t0 := w.clock.Now()
		for i := 0; i < g.work; i++ {
			*local += tick.Run()
		}
		t1 := w.clock.Now()

		// the slot this tick belongs to: the nearest one, and never the same twice
		slot = int(math.Floor(float64(t0-origin)/float64(g.period) + 0.5))
		if slot <= last {
			slot = last + 1
		}
		last = slot
		if slot >= g.warm && slot < total {
			workUs := w.clock.Us(t1 - t0)
			w.h.Add(uint64(workUs * 1000))
			lateUs := 0.0
			if t0 > due {
				lateUs = w.clock.Us(t0 - due)
			}
			w.hWake.Add(uint64(lateUs * 1000))
			if lateUs > periodUs/10 {
				s.Late++
			}
			if workUs > periodUs {
				s.Overruns++
			}
			sec := (slot - g.warm) / g.rate
			s.PerSec[sec]++
			s.MaxWorkPerSec[sec] = max(s.MaxWorkPerSec[sec], workUs)
			s.Ticks++
			sumWork += workUs
			if prevStart != 0 {
				iv := w.clock.Us(t0 - prevStart)
				w.hInt.Add(uint64(iv * 1000))
				sumInt += iv
				sumInt2 += iv * iv
				nInt++
			}
			prevStart = t0
		}

		slot++
		due = origin + uint64(slot)*g.period
		if t1 > due { // behind schedule: start the next tick right away
			due = t1
		}
		select {
		case c := <-w.cmds:
			next = &c
		default:
		}
		if next != nil {
			break
		}
	}
	if s.Ticks > 0 {
		s.WorkMeanUs = sumWork / float64(s.Ticks)
		s.WorkP50us = w.h.Quantile(0.5) / 1000
		s.WorkP99us = w.h.Quantile(0.99) / 1000
		s.WorkMaxUs = w.h.Quantile(1) / 1000
		s.WakeP99us = w.hWake.Quantile(0.99) / 1000
	}
	if nInt > 1 {
		m := sumInt / float64(nInt)
		s.IntervalSDus = math.Sqrt(max(0, sumInt2/float64(nInt)-m*m))
		s.IntervalP99us = w.hInt.Quantile(0.99) / 1000
	}
	return next, s
}

// sleepUntil sleeps until the clock reaches deadline. The last spinUs before
// it are spent spinning, because sleep on Windows overshoots by up to a millisecond.
func (w *worker) sleepUntil(deadline uint64) {
	for {
		now := w.clock.Now()
		if now >= deadline {
			return
		}
		rem := w.clock.Us(deadline - now)
		if rem <= spinUs+50 {
			for w.clock.Now() < deadline {
			}
			return
		}
		osSleep(time.Duration(rem-spinUs) * time.Microsecond)
	}
}

const (
	burstMinMS = 100
	burstMaxMS = 1000
)

// burst alternates full load and idle for random periods until a new command arrives.
func (w *worker) burst(tick *kernels.Tick, local *uint64) cmd {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	randMS := func() time.Duration {
		return time.Duration(burstMinMS+rand.IntN(burstMaxMS-burstMinMS)) * time.Millisecond
	}
	for {
		end := w.clock.Now() + uint64(float64(randMS().Microseconds())*w.clock.TicksPerUs)
		for w.clock.Now() < end {
			*local += tick.Run()
			select {
			case c := <-w.cmds:
				return c
			default:
			}
		}
		timer.Reset(randMS())
		select {
		case c := <-w.cmds:
			return c
		case <-timer.C:
		}
	}
}
