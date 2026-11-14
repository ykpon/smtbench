package kernels

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMix(t *testing.T) {
	m, err := ParseMix("int=50,mem=40")
	if err != nil || !reflect.DeepEqual(m, Mix{"int": 50, "mem": 40}) {
		t.Fatalf("%v %v", m, err)
	}
	for _, bad := range []string{"", "int", "foo=1", "int=0", "int=10001", "int=5,int=6", "int=x"} {
		if _, err := ParseMix(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	if s := (Mix{"int": 50, "fp": 10}).String(); s != "fp=10,int=50" {
		t.Fatalf("String %q", s)
	}
}

func TestProfiles(t *testing.T) {
	mc, ok := ProfileByName("minecraft")
	if !ok || !reflect.DeepEqual(mc.Mix, Mix{"int": 45, "fp": 10, "mem": 45}) || mc.MemSizeMB != 64 {
		t.Fatalf("minecraft %+v", mc)
	}
	src, _ := ProfileByName("source")
	if src.MemSizeMB != 16 || !reflect.DeepEqual(src.Mix, Mix{"int": 50, "fp": 35, "mem": 15}) {
		t.Fatalf("source %+v", src)
	}
	rust, _ := ProfileByName("rust")
	if !reflect.DeepEqual(rust.Mix, Mix{"int": 35, "fp": 35, "mem": 30}) || rust.MemSizeMB != 64 {
		t.Fatalf("rust %+v", rust)
	}
	seen := map[string]bool{}
	var names []string
	for _, p := range Profiles() {
		if seen[p.Name] {
			t.Fatalf("duplicate %s", p.Name)
		}
		seen[p.Name] = true
		names = append(names, p.Name)
		if !strings.Contains(p.Description, "синтетический") {
			t.Errorf("%s: description must say it is synthetic", p.Name)
		}
	}
	want := []string{"int", "fp", "avx", "mem", "source", "minecraft", "rust"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("order %v", names)
	}
}

func TestChainSingleCycle(t *testing.T) {
	c := newChain(1000, 1)
	seen := make([]bool, 1000)
	pos := uint32(0)
	for i := 0; i < 1000; i++ {
		if seen[pos] {
			t.Fatalf("revisited %d at step %d", pos, i)
		}
		seen[pos] = true
		pos = c[pos]
	}
	if pos != 0 {
		t.Fatalf("did not return to start: %d", pos)
	}
}

func TestKernelsDeterministic(t *testing.T) {
	for _, name := range []string{"int", "fp", "mem"} {
		a, err := New(name, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := New(name, 1<<20)
		sa, sb := a.Run(5), b.Run(5)
		if sa != sb || sa == 0 {
			t.Errorf("%s: sinks %d %d", name, sa, sb)
		}
	}
	if _, err := New("nope", 0); err == nil {
		t.Fatal("expected error for unknown kernel")
	}
}

func TestTickRuns(t *testing.T) {
	tk, err := NewTick(Mix{"int": 1, "fp": 1, "mem": 1}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Run() == 0 {
		t.Fatal("zero sink")
	}
}
