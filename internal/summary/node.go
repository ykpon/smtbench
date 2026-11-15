package summary

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NodePoint is the node with a given number of servers placed on it.
type NodePoint struct {
	Servers      int     `json:"servers"`
	PerServerPct float64 `json:"per_server_pct"` // speed of each server, % of a server alone on an idle machine
	TotalEq      float64 `json:"total_eq"`       // node output, in servers running alone on an idle machine
	MHz          float64 `json:"mhz"`
}

// Density is how many servers fit while each keeps at least MinPct of its speed.
type Density struct {
	MinPct   int `json:"min_pct"`
	Isolated int `json:"isolated"`
	Packed   int `json:"packed"`
}

// Node answers how the node degrades as servers are added, for two placements:
// one server per core (Isolated) and two servers per core (Packed).
type Node struct {
	// Uniform: neighbours were loaded the same way as the tested core, so both
	// lines describe a node filled with one placement. With neighbours always
	// loaded on both threads only Packed does; Isolated is then a server that
	// has a whole core on a packed node.
	Uniform     bool        `json:"uniform"`
	Isolated    []NodePoint `json:"isolated"`
	Packed      []NodePoint `json:"packed"`
	Density     []Density   `json:"density"`
	Conclusions []string    `json:"conclusions"`
}

var densityLevels = []int{95, 90, 80, 70}

// isolationGainPct: below this difference, spreading servers over cores is
// reported as not worth it.
const isolationGainPct = 3

// nodeView builds the node lines. bg is Params.BgThreads: 0 means neighbours
// are loaded like the tested core, 1 or 2 always that many threads per core.
func nodeView(avg Curve, bg int) Node {
	n := Node{Uniform: bg == 0, Isolated: []NodePoint{}, Packed: []NodePoint{}, Density: []Density{}, Conclusions: []string{}}
	add := func(dst *[]NodePoint, servers []int, pcts, mhz []float64) {
		for i := range avg.Loaded {
			if pcts[i] < 0 || servers[i] <= 0 {
				continue
			}
			*dst = append(*dst, NodePoint{Servers: servers[i], PerServerPct: pcts[i],
				TotalEq: float64(servers[i]) * pcts[i] / 100, MHz: mhz[i]})
		}
	}
	add(&n.Isolated, avg.SingleServers, avg.Single, avg.MHz)
	add(&n.Packed, avg.PairServers, avg.Pair, avg.PairMHz)
	if len(n.Isolated) == 0 {
		return n
	}
	for _, lvl := range densityLevels {
		d := Density{MinPct: lvl, Isolated: -1, Packed: -1}
		if bg != 2 {
			d.Isolated = fits(n.Isolated, lvl)
		}
		if bg != 1 && len(n.Packed) > 0 {
			d.Packed = fits(n.Packed, lvl)
		}
		n.Density = append(n.Density, d)
	}
	n.Conclusions = conclusions(n, bg)
	return n
}

// fits returns the largest server count up to which every measured step
// keeps per-server speed at or above minPct.
func fits(points []NodePoint, minPct int) int {
	best := 0
	for _, p := range points {
		if math.Round(p.PerServerPct) < float64(minPct) {
			break
		}
		best = p.Servers
	}
	return best
}

func conclusions(n Node, bg int) []string {
	var out []string
	iso := n.Isolated[len(n.Isolated)-1]
	if bg != 2 {
		out = append(out, fmt.Sprintf("По одному серверу на ядро: когда заняты все ядра (%s), каждый работает на %s от скорости на пустой машине. В сумме нода выдаёт столько же, сколько %s сервера на пустой машине.",
			serversWord(iso.Servers), pctText(iso.PerServerPct), dec(iso.TotalEq)))
	}
	if len(n.Packed) == 0 || bg == 1 {
		return out
	}
	pk := n.Packed[len(n.Packed)-1]
	out = append(out, fmt.Sprintf("По два сервера на ядро: когда заняты все потоки (%s), каждый работает на %s. В сумме нода выдаёт столько же, сколько %s сервера на пустой машине.",
		serversWord(pk.Servers), pctText(pk.PerServerPct), dec(pk.TotalEq)))
	if bg == 2 {
		return append(out, wholeCore(n)...)
	}
	return append(out, sameCount(n)...)
}

// wholeCore compares a server that has a core to itself with servers sharing
// a core, on a node filled two servers per core.
func wholeCore(n Node) []string {
	iso, pk := n.Isolated[len(n.Isolated)-1], n.Packed[len(n.Packed)-1]
	idle := int(math.Round(n.Isolated[0].PerServerPct) - math.Round(n.Packed[0].PerServerPct))
	full := int(math.Round(iso.PerServerPct) - math.Round(pk.PerServerPct))
	out := []string{
		fmt.Sprintf("Сервер, которому отдано ядро целиком (второй поток свободен): когда все остальные ядра заняты по два сервера, он работает на %s.", pctText(iso.PerServerPct)),
		fmt.Sprintf("Не занимать второй поток ядра — это %+d п.п. к скорости сервера на пустой ноде и %+d п.п. на забитой.", idle, full),
	}
	if full < isolationGainPct {
		out = append(out, "На забитой ноде выделять серверу целое ядро почти ничего не даёт.")
	}
	return out
}

// sameCount compares the same number of servers placed one and two per core.
func sameCount(n Node) []string {
	var out []string
	isoBy := map[int]float64{}
	for _, p := range n.Isolated {
		isoBy[p.Servers] = p.PerServerPct
	}
	minGain, maxGain := math.Inf(1), math.Inf(-1)
	var firstN, lastN int
	for _, p := range n.Packed {
		v, ok := isoBy[p.Servers]
		if !ok {
			continue
		}
		g := math.Round(v) - math.Round(p.PerServerPct)
		minGain, maxGain = math.Min(minGain, g), math.Max(maxGain, g)
		if firstN == 0 {
			firstN = p.Servers
		}
		lastN = p.Servers
	}
	if firstN == 0 {
		return out
	}
	packedBy := map[int]float64{}
	for _, p := range n.Packed {
		packedBy[p.Servers] = p.PerServerPct
	}
	for _, c := range uniqueInts(firstN, lastN) {
		out = append(out, fmt.Sprintf("%s: по одному на ядро каждый работает на %s, по два на ядро — на %s.",
			serversWord(c), pctText(isoBy[c]), pctText(packedBy[c])))
	}
	if maxGain < isolationGainPct {
		out = append(out, fmt.Sprintf("Разносить серверы по разным ядрам почти ничего не даёт: разница не больше %d п.п.", int(math.Max(maxGain, 0))))
	} else {
		gain := strconv.Itoa(int(maxGain))
		if minGain != maxGain {
			gain = fmt.Sprintf("%d–%d", int(minGain), int(maxGain))
		}
		out = append(out, fmt.Sprintf("Пока серверов не больше, чем ядер, разносить их по разным ядрам выгодно: каждый работает быстрее на %s п.п.", gain))
	}
	return out
}

func uniqueInts(a, b int) []int {
	if a == b {
		return []int{a}
	}
	return []int{a, b}
}

func pctText(v float64) string { return fmt.Sprintf("%d%%", int(math.Round(v))) }

func dec(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1)
}

// serversWord formats a count with the right Russian plural form.
func serversWord(n int) string {
	word := "серверов"
	switch {
	case n%100 >= 11 && n%100 <= 14:
	case n%10 == 1:
		word = "сервер"
	case n%10 >= 2 && n%10 <= 4:
		word = "сервера"
	}
	return fmt.Sprintf("%d %s", n, word)
}
