package topology

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"sort"
)

// Relationship values of SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX.
const (
	relProcessorCore    = 0
	relNumaNode         = 1
	relCache            = 2
	relProcessorPackage = 3
)

var errGLPIEx = errors.New("повреждённый буфер GetLogicalProcessorInformationEx")

// groupMasks decodes count GROUP_AFFINITY entries (16 bytes each) at off
// into logical CPU ids (group*64 + bit).
func groupMasks(rec []byte, off, count int) ([]int, error) {
	if off+16*count > len(rec) {
		return nil, errGLPIEx
	}
	var ids []int
	for i := 0; i < count; i++ {
		e := rec[off+16*i:]
		mask := binary.LittleEndian.Uint64(e[0:8])
		group := int(binary.LittleEndian.Uint16(e[8:10]))
		for mask != 0 {
			b := bits.TrailingZeros64(mask)
			ids = append(ids, group*64+b)
			mask &^= 1 << b
		}
	}
	sort.Ints(ids)
	return ids, nil
}

// ParseGLPIEx parses a GetLogicalProcessorInformationEx(RelationAll) buffer.
func ParseGLPIEx(buf []byte) (*Topology, error) {
	var cores [][]int
	pkgOf := map[int]int{}
	numaOf := map[int]int{}
	type l3 struct {
		cpus   []int
		sizeKB int
	}
	var caches []l3
	pkgCount := 0

	for off := 0; off < len(buf); {
		if off+8 > len(buf) {
			return nil, errGLPIEx
		}
		rel := binary.LittleEndian.Uint32(buf[off:])
		size := int(binary.LittleEndian.Uint32(buf[off+4:]))
		if size < 8 || off+size > len(buf) {
			return nil, errGLPIEx
		}
		rec := buf[off : off+size]
		off += size

		switch rel {
		case relProcessorCore, relProcessorPackage:
			if len(rec) < 32 {
				return nil, errGLPIEx
			}
			ids, err := groupMasks(rec, 32, int(binary.LittleEndian.Uint16(rec[30:])))
			if err != nil {
				return nil, err
			}
			if rel == relProcessorCore {
				cores = append(cores, ids)
			} else {
				for _, id := range ids {
					pkgOf[id] = pkgCount
				}
				pkgCount++
			}
		case relCache:
			if len(rec) < 40 {
				return nil, errGLPIEx
			}
			level := rec[8]
			typ := binary.LittleEndian.Uint32(rec[16:])
			if level != 3 || (typ != 0 && typ != 2) {
				continue
			}
			count := int(binary.LittleEndian.Uint16(rec[38:]))
			if count == 0 {
				count = 1
			}
			ids, err := groupMasks(rec, 40, count)
			if err != nil {
				return nil, err
			}
			caches = append(caches, l3{ids, int(binary.LittleEndian.Uint32(rec[12:]) / 1024)})
		case relNumaNode:
			if len(rec) < 32 {
				return nil, errGLPIEx
			}
			node := int(binary.LittleEndian.Uint32(rec[8:]))
			count := int(binary.LittleEndian.Uint16(rec[30:]))
			if count == 0 {
				count = 1
			}
			ids, err := groupMasks(rec, 32, count)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				numaOf[id] = node
			}
		}
	}
	if len(cores) == 0 {
		return nil, fmt.Errorf("%w: нет записей о ядрах", errGLPIEx)
	}

	var raw []rawCPU
	for _, threads := range cores {
		for _, id := range threads {
			r := rawCPU{ID: id, Package: pkgOf[id], NUMA: numaOf[id], Siblings: threads}
			for _, c := range caches {
				if containsInt(c.cpus, id) {
					r.L3CPUs, r.L3SizeKB = c.cpus, c.sizeKB
					break
				}
			}
			raw = append(raw, r)
		}
	}
	return build(raw)
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
