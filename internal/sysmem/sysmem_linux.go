//go:build linux

// Package sysmem reports available physical memory.
package sysmem

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func Available() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024, err
		}
	}
	return 0, errors.New("в /proc/meminfo нет MemAvailable")
}
