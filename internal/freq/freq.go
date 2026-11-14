// Package freq reads core cycle counters to compute the effective frequency.
package freq

// Counter returns cycles executed by one logical CPU (or the calling thread).
type Counter interface {
	Cycles() (uint64, error)
	Close() error
}

// Source opens counters. Open must be called from the pinned thread that
// will be measured.
type Source interface {
	Name() string // "msr", "perf" or "none"
	Open(cpu int) (Counter, error)
}

type none struct{}

func (none) Name() string              { return "none" }
func (none) Open(int) (Counter, error) { return nil, errNone }

type noneErr struct{}

func (noneErr) Error() string { return "частота не измеряется" }

var errNone error = noneErr{}
