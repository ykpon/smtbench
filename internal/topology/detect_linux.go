//go:build linux

package topology

import "os"

// Detect reads the topology of the current machine.
func Detect() (*Topology, error) {
	return ParseSysfs(os.DirFS("/sys/devices/system"))
}
