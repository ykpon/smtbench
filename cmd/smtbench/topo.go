package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"smtbench/internal/topology"
)

func cmdTopo(args []string) int {
	fs := flag.NewFlagSet("topo", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "вывести в JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	t, err := detectTopology()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(t.Info())
		return 0
	}
	printTopology(os.Stdout, t)
	return 0
}

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}

func joinInts(xs []int, sep string) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, sep)
}

// groupName returns "Сокет N" when the package has one L3 group, otherwise "Чиплет N".
func groupName(t *topology.Topology, g topology.L3Group) string {
	n := 0
	for _, o := range t.L3 {
		if o.Package == g.Package {
			n++
		}
	}
	if n == 1 && t.Packages > 1 {
		return fmt.Sprintf("Сокет %d", g.Package)
	}
	if n == 1 {
		return "Процессор"
	}
	return fmt.Sprintf("Чиплет %d", g.ID)
}

func printTopology(w io.Writer, t *topology.Topology) {
	fmt.Fprintf(w, "Процессор: %s · сокетов: %d · ядер: %d · потоков: %d · HT: %s\n",
		t.Model, t.Packages, len(t.Cores), len(t.CPUs), yesNo(t.HasSMT()))
	for _, g := range t.L3 {
		big := ""
		if g.Big {
			big = " · большой L3"
		}
		fmt.Fprintf(w, "%s · L3 %d МБ%s: ядра %s\n", groupName(t, g), g.SizeKB/1024, big, joinInts(g.Cores, ","))
	}
	fmt.Fprintln(w)
	numa := map[int]int{}
	for _, c := range t.CPUs {
		numa[c.ID] = c.NUMA
	}
	fmt.Fprintf(w, "%-5s %-8s %-6s %-4s %s\n", "Ядро", "Потоки", "Сокет", "L3", "NUMA")
	for _, c := range t.Cores {
		fmt.Fprintf(w, "%-5d %-8s %-6d %-4d %d\n", c.ID, joinInts(c.Threads, ","), c.Package, c.L3, numa[c.Threads[0]])
	}
}
