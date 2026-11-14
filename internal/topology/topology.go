// Package topology describes physical cores, their hardware threads,
// L3 groups (chiplets) and packages.
package topology

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"smtbench/internal/result"
)

type Topology struct {
	Vendor, Model string
	CPUs          []CPU
	Cores         []Core
	L3            []L3Group
	Packages      int
}

type CPU struct{ ID, Core, Package, NUMA, L3 int }

// Core is a physical core. Threads are logical CPU ids in ascending order:
// Threads[0] is thread A, Threads[1] (if present) is thread B.
type Core struct {
	ID, Package, L3 int
	Threads         []int
}

type L3Group struct {
	ID, SizeKB, Package int
	Cores               []int
	Big                 bool
}

// rawCPU is what OS-specific parsers produce.
type rawCPU struct {
	ID, Package, NUMA int
	Siblings, L3CPUs  []int
	L3SizeKB          int
}

func key(ids []int) string { return fmt.Sprint(ids) }

func build(raw []rawCPU) (*Topology, error) {
	if len(raw) == 0 {
		return nil, errors.New("не найдено ни одного CPU")
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].ID < raw[j].ID })
	present := map[int]*rawCPU{}
	for i := range raw {
		present[raw[i].ID] = &raw[i]
	}

	// packages normalized to 0..n-1 in order of first appearance by cpu id
	pkgIdx := map[int]int{}
	for _, r := range raw {
		if _, ok := pkgIdx[r.Package]; !ok {
			pkgIdx[r.Package] = len(pkgIdx)
		}
	}

	t := &Topology{Packages: len(pkgIdx)}

	// cores: unique sibling sets, ordered by smallest thread id
	coreOf := map[int]int{}
	for _, r := range raw {
		if _, done := coreOf[r.ID]; done {
			continue
		}
		var threads []int
		for _, s := range r.Siblings {
			if _, ok := present[s]; ok {
				threads = append(threads, s)
			}
		}
		if !slices.Contains(threads, r.ID) {
			threads = append(threads, r.ID)
		}
		sort.Ints(threads)
		id := len(t.Cores)
		t.Cores = append(t.Cores, Core{ID: id, Package: pkgIdx[r.Package], Threads: threads})
		for _, th := range threads {
			coreOf[th] = id
		}
	}

	// L3 groups: unique shared sets; without L3 info, one group per package
	l3Of := map[int]int{}
	l3ByKey := map[string]int{}
	for _, r := range raw {
		k := key(r.L3CPUs)
		size := r.L3SizeKB
		if len(r.L3CPUs) == 0 {
			k = fmt.Sprintf("pkg%d", r.Package)
			size = 0
		}
		id, ok := l3ByKey[k]
		if !ok {
			id = len(t.L3)
			l3ByKey[k] = id
			t.L3 = append(t.L3, L3Group{ID: id, SizeKB: size, Package: pkgIdx[r.Package]})
		}
		l3Of[r.ID] = id
	}

	for _, r := range raw {
		t.CPUs = append(t.CPUs, CPU{ID: r.ID, Core: coreOf[r.ID], Package: pkgIdx[r.Package], NUMA: r.NUMA, L3: l3Of[r.ID]})
	}
	for i := range t.Cores {
		c := &t.Cores[i]
		c.L3 = l3Of[c.Threads[0]]
		t.L3[c.L3].Cores = append(t.L3[c.L3].Cores, c.ID)
	}
	markBig(t.L3)
	return t, nil
}

func markBig(groups []L3Group) {
	if len(groups) == 0 {
		return
	}
	minSize := groups[0].SizeKB
	for _, g := range groups {
		minSize = min(minSize, g.SizeKB)
	}
	for i := range groups {
		groups[i].Big = groups[i].SizeKB > minSize
	}
}

// Distance: 0 same L3 group, 1 same package, 2 different packages.
func (t *Topology) Distance(a, b int) int {
	ca, _ := t.CoreByID(a)
	cb, _ := t.CoreByID(b)
	switch {
	case ca.L3 == cb.L3:
		return 0
	case ca.Package == cb.Package:
		return 1
	}
	return 2
}

func (t *Topology) CoreByID(id int) (Core, bool) {
	for _, c := range t.Cores {
		if c.ID == id {
			return c, true
		}
	}
	return Core{}, false
}

func (t *Topology) HasSMT() bool {
	for _, c := range t.Cores {
		if len(c.Threads) > 1 {
			return true
		}
	}
	return false
}

// Restrict keeps only the allowed logical CPUs. Core and group ids are kept.
func (t *Topology) Restrict(allowed []int) (*Topology, error) {
	ok := map[int]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	n := &Topology{Vendor: t.Vendor, Model: t.Model, Packages: t.Packages}
	for _, c := range t.CPUs {
		if ok[c.ID] {
			n.CPUs = append(n.CPUs, c)
		}
	}
	if len(n.CPUs) == 0 {
		return nil, errors.New("процессу не разрешён ни один CPU")
	}
	keptCores := map[int]bool{}
	for _, c := range t.Cores {
		var th []int
		for _, x := range c.Threads {
			if ok[x] {
				th = append(th, x)
			}
		}
		if len(th) > 0 {
			c.Threads = th
			n.Cores = append(n.Cores, c)
			keptCores[c.ID] = true
		}
	}
	for _, g := range t.L3 {
		var cores []int
		for _, c := range g.Cores {
			if keptCores[c] {
				cores = append(cores, c)
			}
		}
		if len(cores) > 0 {
			g.Cores = cores
			n.L3 = append(n.L3, g)
		}
	}
	return n, nil
}

func (t *Topology) Info() result.CPUInfo {
	info := result.CPUInfo{Model: t.Model, Vendor: t.Vendor, Packages: t.Packages}
	for _, c := range t.CPUs {
		info.Logical = append(info.Logical, result.LogicalCPU{ID: c.ID, Core: c.Core, Package: c.Package, NUMA: c.NUMA, L3Group: c.L3})
	}
	for _, c := range t.Cores {
		info.Cores = append(info.Cores, result.CoreInfo{ID: c.ID, Package: c.Package, L3Group: c.L3, Threads: slices.Clone(c.Threads)})
	}
	for _, g := range t.L3 {
		info.L3Groups = append(info.L3Groups, result.L3Group{ID: g.ID, SizeKB: g.SizeKB, Package: g.Package, Cores: slices.Clone(g.Cores), Big: g.Big})
	}
	return info
}
