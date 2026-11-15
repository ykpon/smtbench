package report

import (
	"strings"
	"testing"

	"smtbench/internal/plan"
	"smtbench/internal/result"
	"smtbench/internal/runner"
	"smtbench/internal/summary"
)

func TestProgressLineShowsMHz(t *testing.T) {
	var b strings.Builder
	pr := &Printer{W: &b, Cores: 16}
	pr.Line(runner.Progress{Done: 1, Total: 10, Window: plan.Window{Core: 0, Mode: plan.Single},
		Point: result.Point{Threads: []result.ThreadResult{{TicksPerSec: 1000, MHz: 3616.4}}}})
	if !strings.Contains(b.String(), "3616 МГц") || strings.Contains(b.String(), "ГГц") {
		t.Fatalf("%q", b.String())
	}
}

func TestSummaryShowsMHz(t *testing.T) {
	var b strings.Builder
	s := summary.Summary{HasFreq: true, HasPair: true, MHzIdle: 3923.2, MHzFullSingle: 3601, MHzFullPair: 3580, KneeSingle: -1}
	Summary(&b, &result.Run{}, s)
	out := b.String()
	for _, want := range []string{"3923 МГц", "3601 МГц", "3580 МГц"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestSummaryMatrixPairsAndDensity(t *testing.T) {
	var b strings.Builder
	s := summary.Summary{HasPair: true, BaselineTPS: 10000, KneeSingle: -1,
		HTPerThreadPct: 87, HTTotalPct: 173, FullAlonePct: 91, FullPairPct: 88, FullPairSumPct: 176,
		Cores: []summary.Core{{Core: 3, CPUA: 3, CPUB: 19, BaseTPS: 9964, BaseBTPS: 9980,
			HTAPct: 85, HTBPct: 89, HTTotalPct: 175, FullSinglePct: 92, FullSoloBPct: 90,
			FullPairAPct: 84, FullPairBPct: 86, FullPairSumPct: 171}},
		Node: summary.Node{
			Density:     []summary.Density{{MinPct: 90, Isolated: -1, Packed: 0}, {MinPct: 70, Isolated: -1, Packed: 24}},
			Conclusions: []string{"вывод про плотность"}}}
	Summary(&b, &result.Run{Params: result.Params{BgThreads: 2}}, s)
	out := b.String()
	for _, want := range []string{
		"все остальные ядра нагружены", "Поток один", "91%", "Оба потока ядра нагружены, каждый", "87%", "88%", "173%", "176%",
		"вывод про плотность", "не медленнее 90%: по два на ядро — ни одного", "по два на ядро — 24",
		"CPU 3", "CPU 19", "9 964", "9 980", "85%", "89%", "175%", "92%", "90%", "84%", "86%", "171%",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "по одному на ядро") {
		t.Fatalf("one-per-core density must be hidden when it was not measured:\n%s", out)
	}
}
