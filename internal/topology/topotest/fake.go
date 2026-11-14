// Package topotest provides sysfs trees for topology tests.
package topotest

import (
	_ "embed"
	"fmt"
	"io/fs"
	"strings"
	"testing/fstest"
)

//go:embed xeon-e5-2667v2.dump
var xeonDump string

// Xeon returns the sysfs tree captured on a 2x Xeon E5-2667 v2 (2 sockets, 8 cores, HT).
func Xeon() fs.FS {
	m := fstest.MapFS{}
	var name string
	var body strings.Builder
	flush := func() {
		if name != "" {
			m[name] = &fstest.MapFile{Data: []byte(body.String())}
		}
		body.Reset()
	}
	for _, line := range strings.Split(xeonDump, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "==") {
			flush()
			name = strings.TrimPrefix(line, "==")
			continue
		}
		if line != "" {
			body.WriteString(line + "\n")
		}
	}
	flush()
	return m
}

// DualCCDX3D returns a synthetic 16-core/32-thread dual-CCD Ryzen X3D:
// cpu N and N+16 are siblings, cores 0-7 share a 96 MB L3, cores 8-15 a 32 MB L3.
func DualCCDX3D() fstest.MapFS {
	m := fstest.MapFS{}
	put := func(path, data string) { m[path] = &fstest.MapFile{Data: []byte(data + "\n")} }
	put("cpu/online", "0-31")
	put("node/node0/cpulist", "0-31")
	for cpu := 0; cpu < 32; cpu++ {
		core := cpu % 16
		p := fmt.Sprintf("cpu/cpu%d/", cpu)
		put(p+"topology/physical_package_id", "0")
		put(p+"topology/core_id", fmt.Sprint(core))
		put(p+"topology/thread_siblings_list", fmt.Sprintf("%d,%d", core, core+16))
		put(p+"cache/index0/level", "1")
		put(p+"cache/index0/type", "Data")
		put(p+"cache/index0/size", "32K")
		put(p+"cache/index0/shared_cpu_list", fmt.Sprintf("%d,%d", core, core+16))
		put(p+"cache/index3/level", "3")
		put(p+"cache/index3/type", "Unified")
		if core < 8 {
			put(p+"cache/index3/size", "98304K")
			put(p+"cache/index3/shared_cpu_list", "0-7,16-23")
		} else {
			put(p+"cache/index3/size", "32768K")
			put(p+"cache/index3/shared_cpu_list", "8-15,24-31")
		}
	}
	return m
}

// NoSMT returns n single-threaded cores in one package sharing one L3.
func NoSMT(n int) fstest.MapFS {
	m := fstest.MapFS{}
	put := func(path, data string) { m[path] = &fstest.MapFile{Data: []byte(data + "\n")} }
	put("cpu/online", fmt.Sprintf("0-%d", n-1))
	for cpu := 0; cpu < n; cpu++ {
		p := fmt.Sprintf("cpu/cpu%d/", cpu)
		put(p+"topology/physical_package_id", "0")
		put(p+"topology/core_id", fmt.Sprint(cpu))
		put(p+"topology/thread_siblings_list", fmt.Sprint(cpu))
		put(p+"cache/index3/level", "3")
		put(p+"cache/index3/type", "Unified")
		put(p+"cache/index3/size", "16384K")
		put(p+"cache/index3/shared_cpu_list", fmt.Sprintf("0-%d", n-1))
	}
	return m
}

//go:embed ryzen-5500x3d.glpiex.bin
var ryzen5500X3D []byte

// Ryzen5500X3D returns a raw GetLogicalProcessorInformationEx(RelationAll)
// buffer captured on a Ryzen 5 5500X3D (6 cores, 12 threads, 96 MB L3).
func Ryzen5500X3D() []byte { return ryzen5500X3D }
