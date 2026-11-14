package result

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func sampleRun() *Run {
	t0 := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	return &Run{
		SchemaVersion:  SchemaVersion,
		KernelsVersion: 1,
		ToolVersion:    "dev",
		RunID:          "0123456789abcdef0123456789abcdef",
		Label:          "node-1",
		Hostname:       "host",
		OS:             "linux",
		StartedAt:      t0,
		FinishedAt:     t0.Add(time.Minute),
		CPU: CPUInfo{
			Model:    "Test CPU",
			Vendor:   "GenuineIntel",
			Features: []string{"avx"},
			Logical: []LogicalCPU{
				{ID: 0, Core: 0}, {ID: 1, Core: 0},
			},
			Cores:    []CoreInfo{{ID: 0, Threads: []int{0, 1}}},
			L3Groups: []L3Group{{ID: 0, SizeKB: 8192, Cores: []int{0}}},
			Packages: 1,
		},
		Params: Params{
			Profile: "int", Mix: map[string]int{"int": 100},
			DurationMS: 3000, WarmupMS: 1000, Reps: 1, Step: 1,
			Order: "near", BgThreads: 2, Modes: []string{"single", "pair"},
			Cores: []int{0}, NoiseMaxPct: 3,
		},
		Points: []Point{{
			Core: 0, Mode: "single", LoadedCPUs: []int{},
			Threads: []ThreadResult{{CPU: 0, TicksPerSec: 1000, TickP50us: 100, TickP99us: 120, TickP999us: 150, MHz: 3500}},
		}},
		Warnings: []string{},
	}
}

func TestRoundTrip(t *testing.T) {
	r := sampleRun()
	path := filepath.Join(t.TempDir(), "r.json")
	if err := Save(path, r); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, got) {
		t.Fatalf("round trip mismatch:\n%+v\n%+v", r, got)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(r *Run){
		"empty id":    func(r *Run) { r.RunID = "" },
		"bad id":      func(r *Run) { r.RunID = "xyz" },
		"schema 0":    func(r *Run) { r.SchemaVersion = 0 },
		"schema next": func(r *Run) { r.SchemaVersion = SchemaVersion + 1 },
		"no cpus":     func(r *Run) { r.CPU.Logical = nil },
		"bad mode":    func(r *Run) { r.Points[0].Mode = "bogus" },
	}
	for name, mut := range cases {
		r := sampleRun()
		mut(r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if err := sampleRun().Validate(); err != nil {
		t.Errorf("valid run rejected: %v", err)
	}
}

func TestNewRunIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewRunID()
		if len(id) != 32 {
			t.Fatalf("len %d", len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
	}
}
