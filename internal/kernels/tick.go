package kernels

type part struct {
	k     Kernel
	units int
}

// Tick is a fixed amount of mixed work, like one game server tick.
// Create it on the thread that will run it, after pinning, so memory
// is allocated on the local NUMA node.
type Tick struct{ parts []part }

func NewTick(m Mix, memBytes int) (*Tick, error) {
	t := &Tick{}
	for _, name := range order {
		u := m[name]
		if u == 0 {
			continue
		}
		k, err := New(name, memBytes)
		if err != nil {
			return nil, err
		}
		t.parts = append(t.parts, part{k, u})
	}
	return t, nil
}

func (t *Tick) Run() uint64 {
	var s uint64
	for _, p := range t.parts {
		s += p.k.Run(p.units)
	}
	return s
}
