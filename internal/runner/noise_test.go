package runner

import (
	"context"
	"flag"
	"testing"

	"smtbench/internal/cpuload"
	"smtbench/internal/plan"
)

var noiseFlag = flag.Bool("noise", false, "print noise levels of real windows on core 0")

// TestNoiseLevels measures how much other processes disturb windows on this machine.
func TestNoiseLevels(t *testing.T) {
	if !*noiseFlag {
		t.Skip("use -noise")
	}
	cfg, _ := quickConfig(t, 3000)
	cfg.Params.WarmupMS = 1000
	cfg.Params.Step = 1
	cfg.Load = cpuload.NewSampler()
	ws, err := plan.Build(cfg.Topo, cfg.Params)
	if err != nil {
		t.Fatal(err)
	}
	var pick []plan.Window
	for _, w := range ws {
		last := w.LoadedCores == len(cfg.Topo.Cores)-1
		if (w.Mode == plan.Single && w.LoadedCores == 0) || (w.Mode == plan.Pair && (w.LoadedCores == 0 || last)) {
			pick = append(pick, w)
		}
	}
	cfg.Params.NoiseMaxPct = 100
	for i := 0; i < 3; i++ {
		points, err := Run(context.Background(), cfg, pick)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range points {
			t.Logf("%-6s k=%-2d noise %.2f%%", p.Mode, p.LoadedCores, p.NoisePct)
		}
	}
}
