package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"smtbench/internal/kernels"
	"smtbench/internal/topology"
	"smtbench/internal/topology/topotest"
)

func TestParseRunFlagsDefaults(t *testing.T) {
	o, err := parseOne(nil, "host1")
	if err != nil {
		t.Fatal(err)
	}
	p := o.Params
	if p.Profile != "int" || p.DurationMS != 3000 || p.WarmupMS != 1000 || p.Reps != 1 || p.Step != 1 ||
		p.Order != "near" || p.BgThreads != 2 || !reflect.DeepEqual(p.Modes, []string{"single", "solo_b", "pair", "stability"}) ||
		p.NoiseMaxPct != 3 || o.Label != "host1" || p.Cores != nil {
		t.Fatalf("%+v label %q", p, o.Label)
	}
	if !reflect.DeepEqual(o.Mix, kernels.Mix{"int": 100}) {
		t.Fatalf("mix %v", o.Mix)
	}
}

func TestParseRunFlags(t *testing.T) {
	o, err := parseOne([]string{"--cores", "0-2,5"}, "h")
	if err != nil || !reflect.DeepEqual(o.Params.Cores, []int{0, 1, 2, 5}) {
		t.Fatalf("%v %v", o.Params.Cores, err)
	}
	o, _ = parseOne([]string{"--mix", "int=50,mem=50"}, "h")
	if o.Params.Profile != "custom" || o.Mix["mem"] != 50 || o.MemBytes != 64<<20 {
		t.Fatalf("custom: %+v mem %d", o.Params, o.MemBytes)
	}
	o, _ = parseOne([]string{"--profile", "minecraft"}, "h")
	if o.MemBytes != 64<<20 || o.Params.MemSizeMB != 64 || o.Params.Mix["mem"] != 45 {
		t.Fatalf("minecraft: %+v mem %d", o.Params, o.MemBytes)
	}
	o, _ = parseOne([]string{"--profile", "minecraft", "--mem-size", "1GB"}, "h")
	if o.MemBytes != 1<<30 || o.Params.MemSizeMB != 1024 {
		t.Fatalf("mem-size override %d", o.MemBytes)
	}
	o, err = parseOne([]string{"--bg-threads", "2", "--modes", "single,pair"}, "h")
	if err != nil || o.Params.BgThreads != 2 || len(o.Params.Modes) != 2 {
		t.Fatalf("%+v %v", o.Params, err)
	}
	o, err = parseOne([]string{"--bg-threads", "auto"}, "h")
	if err != nil || o.Params.BgThreads != 0 {
		t.Fatalf("%+v %v", o.Params, err)
	}
	o, _ = parseOne([]string{"--duration", "1s", "--warmup", "300ms", "--send", "http://x:8080", "--dry-run"}, "h")
	if o.Params.DurationMS != 1000 || o.Params.WarmupMS != 300 || o.Send != "http://x:8080" || !o.DryRun {
		t.Fatalf("%+v", o)
	}
}

func TestParseRunFlagsErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--order", "up"}, {"--bg-threads", "3"}, {"--profile", "nope"}, {"--duration", "0s"},
		{"--mix", "zzz=1"}, {"--cores", "3-1"}, {"--modes", "triple"}, {"--mem-size", "abc"}, {"--reps", "0"},
	} {
		if _, err := parseOne(args, "h"); err == nil {
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
	o, _ := parseOne([]string{"--profile", "minecraft"}, "h") // 32 cpus x 64 MB = 2 GiB
	err := preflight(o, xeonTopo(t), 1<<30, true)
	if err == nil || !strings.Contains(err.Error(), "--mem-size") {
		t.Fatalf("err %v", err)
	}
	if err := preflight(o, xeonTopo(t), 4<<30, true); err != nil {
		t.Fatal(err)
	}
	o, _ = parseOne([]string{"--profile", "int"}, "h")
	if err := preflight(o, xeonTopo(t), 1<<20, true); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightAVX(t *testing.T) {
	o, _ := parseOne([]string{"--profile", "avx"}, "h")
	err := preflight(o, xeonTopo(t), 1<<34, false)
	if err == nil || !strings.Contains(err.Error(), "AVX") {
		t.Fatalf("err %v", err)
	}
}

func parseOne(args []string, host string) (runOpts, error) {
	runs, err := parseRunFlags(args, host)
	if err != nil {
		return runOpts{}, err
	}
	if len(runs) != 1 {
		return runOpts{}, fmt.Errorf("%d variants", len(runs))
	}
	return runs[0], nil
}

func TestParseRunFlagsVariants(t *testing.T) {
	runs, err := parseRunFlags([]string{"--profile", "source,minecraft", "--bg-threads", "1,2,auto"}, "h")
	if err != nil || len(runs) != 6 {
		t.Fatalf("%d %v", len(runs), err)
	}
	var names, suffixes []string
	for _, r := range runs {
		names, suffixes = append(names, r.Variant), append(suffixes, r.Suffix)
	}
	if names[0] != "source · соседи 1" || suffixes[5] != "minecraft-bgauto" || runs[5].Params.BgThreads != 0 {
		t.Fatalf("%q %q", names, suffixes)
	}
	if runs[0].Params.TickRate != 64 || runs[3].Params.TickRate != 20 || runs[3].MemBytes != 64<<20 || runs[0].MemBytes != 16<<20 {
		t.Fatalf("%+v", runs[3].Params)
	}
	runs[0].Params.Modes[0] = "x"
	if runs[1].Params.Modes[0] != "single" {
		t.Fatal("variants share the modes slice")
	}

	runs, _ = parseRunFlags([]string{"--profile", "all", "--order", "near,far", "--tickrate", "30"}, "h")
	if len(runs) != 2*len(kernels.Profiles()) || !runs[0].AllProfiles || runs[0].Params.TickRate != 30 {
		t.Fatalf("all: %d", len(runs))
	}
	o, _ := parseOne([]string{"--modes", "single,pair"}, "h")
	if o.Params.TickRate != 0 || o.Params.StabMS != 0 || o.Variant != "" || o.Suffix != "" {
		t.Fatalf("no stability: %+v", o.Params)
	}
	o, _ = parseOne([]string{"--modes", "stability", "--stab-duration", "5500ms", "--tick-load", "30"}, "h")
	if o.Params.StabMS != 5000 || o.Params.TickLoadPct != 30 {
		t.Fatalf("%+v", o.Params)
	}
	for _, args := range [][]string{
		{"--profile", "int", "--mix", "int=1"}, {"--bg-threads", "1,3"}, {"--order", "near,up"}, {"--profile", ","},
		{"--tick-load", "0"}, {"--stab-duration", "500ms"}, {"--modes", ""},
	} {
		if _, err := parseRunFlags(args, "h"); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}

func TestOutPath(t *testing.T) {
	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		out, suffix, want string
	}{
		{"", "", "smtbench-n1-20261117-100000.json"},
		{"", "source-bg2", "smtbench-n1-source-bg2-20261117-100000.json"},
		{"r.json", "", "r.json"},
		{"r.json", "fp", "r-fp.json"},
		{"res", "fp", "res-fp"},
	} {
		if got := outPath(runOpts{Label: "n1", Out: c.out, Suffix: c.suffix}, at); got != c.want {
			t.Errorf("%q %q: %q", c.out, c.suffix, got)
		}
	}
}
