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

func TestSummaryShowsThreadsAndDensity(t *testing.T) {
	var b strings.Builder
	s := summary.Summary{HasPair: true, HTPerThreadPct: 80, HTAPct: 82, HTBPct: 78, HTTotalPct: 160, KneeSingle: -1,
		Node: summary.Node{Uniform: true,
			Density:     []summary.Density{{MinPct: 90, Isolated: 12, Packed: 0}, {MinPct: 70, Isolated: 12, Packed: 24}},
			Conclusions: []string{"вывод про плотность"}}}
	Summary(&b, &result.Run{}, s)
	out := b.String()
	for _, want := range []string{"1-й поток 82%", "2-й поток 78%", "вместе 160%", "вывод про плотность", "не медленнее 90%", "12", "24", "ни одного"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}
