//go:build windows

package topology

import (
	"os"
	"testing"
)

// TestCaptureGLPIEx saves the raw buffer of this machine as a fixture.
// Run with SMTBENCH_CAPTURE=1.
func TestCaptureGLPIEx(t *testing.T) {
	if os.Getenv("SMTBENCH_CAPTURE") != "1" {
		t.Skip("set SMTBENCH_CAPTURE=1")
	}
	buf, err := readGLPIEx()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("topotest/ryzen-5500x3d.glpiex.bin", buf, 0o644); err != nil {
		t.Fatal(err)
	}
}
