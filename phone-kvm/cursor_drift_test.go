//go:build windows

package main

import (
	"os"
	"testing"
	"time"
)

// TestCursorDriftProbe injects nothing and only watches the real cursor, to tell
// apart "our injection is flaky" from "something else on this machine moves the
// pointer". Any change reported here happens without us touching the mouse.
// Opt-in because it always passes; it only prints evidence.
func TestCursorDriftProbe(t *testing.T) {
	if os.Getenv("CURSOR_DRIFT_PROBE") == "" {
		t.Skip("set CURSOR_DRIFT_PROBE=1 to watch the cursor for 3s")
	}
	inj := newInjector(1.0)
	time.Sleep(300 * time.Millisecond)
	x0, y0, ok := inj.Cursor(time.Second)
	if !ok {
		t.Fatal("cannot read cursor")
	}
	last := [2]int32{x0, y0}
	changes := 0
	for i := 0; i < 60; i++ {
		time.Sleep(50 * time.Millisecond)
		x, y, ok := inj.Cursor(time.Second)
		if !ok {
			continue
		}
		if x != last[0] || y != last[1] {
			changes++
			t.Logf("sample %2d: cursor moved by itself %v -> (%d,%d)", i, last, x, y)
			last = [2]int32{x, y}
		}
	}
	t.Logf("3s watch: %d spontaneous changes, start (%d,%d)", changes, x0, y0)
}
