package main

import (
	"fmt"
	"os"

	"smtbench/internal/affinity"
	"smtbench/internal/buildinfo"
	"smtbench/internal/topology"
)

const usage = `smtbench — тест производительности ядер и потоков (HT/SMT) под нагрузкой

Команды:
  smtbench topo [--json]           показать ядра, потоки, чиплеты и сокеты
  smtbench profiles                список профилей нагрузки
  smtbench run [флаги]             запустить тест (smtbench run --help)
  smtbench send FILE --to URL      отправить сохранённый результат на коллектор
  smtbench version                 версия
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}
	args := os.Args[2:]
	var code int
	switch os.Args[1] {
	case "topo":
		code = cmdTopo(args)
	case "profiles":
		code = cmdProfiles(args)
	case "run":
		code = cmdRun(args)
	case "send":
		code = cmdSend(args)
	case "version":
		fmt.Println(buildinfo.Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n%s", os.Args[1], usage)
		code = 1
	}
	os.Exit(code)
}

// detectTopology returns the machine topology limited to CPUs this process may use.
func detectTopology() (*topology.Topology, error) {
	t, err := topology.Detect()
	if err != nil {
		return nil, err
	}
	allowed, err := affinity.Allowed()
	if err != nil {
		return nil, err
	}
	return t.Restrict(allowed)
}
