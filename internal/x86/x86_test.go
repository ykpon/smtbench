package x86

import "testing"

func TestBrandNonEmpty(t *testing.T) {
	if Brand() == "" {
		t.Fatal("empty brand")
	}
	if v := Vendor(); v != "GenuineIntel" && v != "AuthenticAMD" {
		t.Fatalf("vendor %q", v)
	}
}

func TestRDTSCMonotonic(t *testing.T) {
	a := RDTSC()
	b := RDTSC()
	if b <= a {
		t.Fatalf("%d <= %d", b, a)
	}
}
