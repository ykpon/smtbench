package main

import (
	"reflect"
	"strings"
	"testing"

	"smtbench/internal/kernels"
	"smtbench/internal/topology"
	"smtbench/internal/topology/topotest"
)

func TestParseRunFlagsDefaults(t *testing.T) {
	o, err := parseRunFlags(nil, "host1")
	if err != nil {
		t.Fatal(err)
	}
	p := o.Params
	if p.Profile != "int" || p.DurationMS != 3000 || p.WarmupMS != 1000 || p.Reps != 1 || p.Step != 1 ||
		p.Order != "near" || p.BgThreads != 0 || !reflect.DeepEqual(p.Modes, []string{"single", "solo_b", "pair"}) ||
		p.NoiseMaxPct != 3 || o.Label != "host1" || p.Cores != nil {
		t.Fatalf("%+v label %q", p, o.Label)
	}
	if !reflect.DeepEqual(o.Mix, kernels.Mix{"int": 100}) {
		t.Fatalf("mix %v", o.Mix)
	}
}

func TestParseRunFlags(t *testing.T) {
	o, err := parseRunFlags([]string{"--cores", "0-2,5"}, "h")
	if err != nil || !reflect.DeepEqual(o.Params.Cores, []int{0, 1, 2, 5}) {
		t.Fatalf("%v %v", o.Params.Cores, err)
	}
	o, _ = parseRunFlags([]string{"--mix", "int=50,mem=50"}, "h")
	if o.Params.Profile != "custom" || o.Mix["mem"] != 50 || o.MemBytes != 64<<20 {
		t.Fatalf("custom: %+v mem %d", o.Params, o.MemBytes)
	}
	o, _ = parseRunFlags([]string{"--profile", "minecraft"}, "h")
	if o.MemBytes != 64<<20 || o.Params.MemSizeMB != 64 || o.Params.Mix["mem"] != 45 {
		t.Fatalf("minecraft: %+v mem %d", o.Params, o.MemBytes)
	}
	o, _ = parseRunFlags([]string{"--profile", "minecraft", "--mem-size", "1GB"}, "h")
	if o.MemBytes != 1<<30 || o.Params.MemSizeMB != 1024 {
		t.Fatalf("mem-size override %d", o.MemBytes)
	}
	o, err = parseRunFlags([]string{"--bg-threads", "2", "--modes", "single,pair"}, "h")
	if err != nil || o.Params.BgThreads != 2 || len(o.Params.Modes) != 2 {
		t.Fatalf("%+v %v", o.Params, err)
	}
	o, err = parseRunFlags([]string{"--bg-threads", "auto"}, "h")
	if err != nil || o.Params.BgThreads != 0 {
		t.Fatalf("%+v %v", o.Params, err)
	}
	o, _ = parseRunFlags([]string{"--duration", "1s", "--warmup", "300ms", "--send", "http://x:8080", "--dry-run"}, "h")
	if o.Params.DurationMS != 1000 || o.Params.WarmupMS != 300 || o.Send != "http://x:8080" || !o.DryRun {
		t.Fatalf("%+v", o)
	}
}

func TestParseRunFlagsErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--order", "up"}, {"--bg-threads", "3"}, {"--profile", "nope"}, {"--duration", "0s"},
		{"--mix", "zzz=1"}, {"--cores", "3-1"}, {"--modes", "triple"}, {"--mem-size", "abc"}, {"--reps", "0"},
	} {
		if _, err := parseRunFlags(args, "h"); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int{"64MB": 64 << 20, "1GB": 1 << 30, "512KB": 512 << 10, "16mb": 16 << 20} {
		if got, err := parseSize(in); err != nil || got != want {
			t.Errorf("%s: %d %v", in, got, err)
		}
	}
	if _, err := parseSize("12"); err == nil {
		t.Error("expected error without unit")
	}
}

func xeonTopo(t *testing.T) *topology.Topology {
	top, err := topology.ParseSysfs(topotest.Xeon())
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func TestPreflightMemory(t *testing.T) {
	o, _ := parseRunFlags([]string{"--profile", "minecraft"}, "h") // 32 cpus x 64 MB = 2 GiB
	err := preflight(o, xeonTopo(t), 1<<30, true)
	if err == nil || !strings.Contains(err.Error(), "--mem-size") {
		t.Fatalf("err %v", err)
	}
	if err := preflight(o, xeonTopo(t), 4<<30, true); err != nil {
		t.Fatal(err)
	}
	o, _ = parseRunFlags([]string{"--profile", "int"}, "h")
	if err := preflight(o, xeonTopo(t), 1<<20, true); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightAVX(t *testing.T) {
	o, _ := parseRunFlags([]string{"--profile", "avx"}, "h")
	err := preflight(o, xeonTopo(t), 1<<34, false)
	if err == nil || !strings.Contains(err.Error(), "AVX") {
		t.Fatalf("err %v", err)
	}
}
