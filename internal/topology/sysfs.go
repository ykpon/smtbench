package topology

import (
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// ParseSysfs reads topology from a tree rooted at /sys/devices/system.
func ParseSysfs(sys fs.FS) (*Topology, error) {
	online, err := readString(sys, "cpu/online")
	if err != nil {
		return nil, err
	}
	cpus, err := ParseCPUList(online)
	if err != nil {
		return nil, err
	}

	numa := map[int]int{}
	nodes, _ := fs.Glob(sys, "node/node*/cpulist")
	for _, n := range nodes {
		id, err := strconv.Atoi(strings.TrimPrefix(path.Base(path.Dir(n)), "node"))
		if err != nil {
			continue
		}
		s, err := readString(sys, n)
		if err != nil || strings.TrimSpace(s) == "" {
			continue
		}
		list, err := ParseCPUList(s)
		if err != nil {
			continue
		}
		for _, c := range list {
			numa[c] = id
		}
	}

	raw := make([]rawCPU, 0, len(cpus))
	for _, id := range cpus {
		base := fmt.Sprintf("cpu/cpu%d/", id)
		r := rawCPU{ID: id, NUMA: numa[id], Siblings: []int{id}}
		if s, err := readString(sys, base+"topology/physical_package_id"); err == nil {
			r.Package, _ = strconv.Atoi(strings.TrimSpace(s))
		}
		if s, err := readString(sys, base+"topology/thread_siblings_list"); err == nil {
			if l, err := ParseCPUList(s); err == nil {
				r.Siblings = l
			}
		}
		for i := 0; i < 10; i++ {
			dir := fmt.Sprintf("%scache/index%d/", base, i)
			lvl, err := readString(sys, dir+"level")
			if err != nil {
				continue
			}
			typ, _ := readString(sys, dir+"type")
			typ = strings.TrimSpace(typ)
			if strings.TrimSpace(lvl) != "3" || (typ != "Unified" && typ != "Data") {
				continue
			}
			if s, err := readString(sys, dir+"shared_cpu_list"); err == nil {
				r.L3CPUs, _ = ParseCPUList(s)
			}
			if s, err := readString(sys, dir+"size"); err == nil {
				r.L3SizeKB = parseCacheSize(s)
			}
			break
		}
		raw = append(raw, r)
	}
	return build(raw)
}

func readString(sys fs.FS, name string) (string, error) {
	b, err := fs.ReadFile(sys, name)
	return string(b), err
}

// parseCacheSize parses "25600K" or "32M" into KB.
func parseCacheSize(s string) int {
	s = strings.TrimSpace(s)
	mult := 1
	switch {
	case strings.HasSuffix(s, "K"):
		s = strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		s, mult = strings.TrimSuffix(s, "M"), 1024
	}
	n, _ := strconv.Atoi(s)
	return n * mult
}
