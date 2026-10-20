// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

const holdFrame = 33_333 // one 30 fps tick in microseconds

// stepsOver ticks h n frames and sums the steps it reports.
func stepsOver(h *HoldRepeat, n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += h.Update(holdFrame)
	}
	return total
}

func TestHoldRepeatWaitsForTheDelayThenRepeats(t *testing.T) {
	var h HoldRepeat
	if h.Update(holdFrame) != 0 {
		t.Fatal("nothing held, yet a step was reported")
	}
	h.Hold(DirDown)
	if got := stepsOver(&h, 6); got != 0 { // ~200ms of a 220ms delay
		t.Fatalf("stepped %d times before the delay elapsed", got)
	}
	if got := stepsOver(&h, 1); got != 1 { // ~233ms
		t.Fatalf("want exactly one step as the delay passes, got %d", got)
	}
	// 120ms cadence from here: ~800ms more is 6 or 7 steps.
	if got := stepsOver(&h, 24); got < 6 || got > 7 {
		t.Fatalf("want ~6 steps over 800ms at 120ms, got %d", got)
	}
}

func TestHoldRepeatReleaseAndDirectionChange(t *testing.T) {
	var h HoldRepeat
	h.Hold(DirDown)
	stepsOver(&h, 7)
	h.Hold(DirDown) // same level again must not restart the delay
	if got := stepsOver(&h, 4); got == 0 {
		t.Fatal("re-holding the same direction restarted the delay")
	}
	h.Hold(DirUp) // a new direction waits the full delay again
	if h.Held() != DirUp {
		t.Fatalf("Held = %v, want DirUp", h.Held())
	}
	if got := stepsOver(&h, 6); got != 0 {
		t.Fatalf("a direction change stepped %d times inside the delay", got)
	}
	h.Hold(DirNone)
	if got := stepsOver(&h, 30); got != 0 || h.Held() != DirNone {
		t.Fatalf("released hold still stepped %d times (held %v)", got, h.Held())
	}
}

func TestHoldRepeatResetKeepsCadence(t *testing.T) {
	h := HoldRepeat{DelayMicros: 100_000, IntervalMicros: 50_000}
	h.Hold(DirRight)
	stepsOver(&h, 4)
	h.Reset()
	if h.Held() != DirNone || h.Update(holdFrame) != 0 {
		t.Fatal("Reset did not release the hold")
	}
	h.Hold(DirRight)
	if got := stepsOver(&h, 4); got != 1 { // 133ms: past the 100ms delay once
		t.Fatalf("custom cadence lost across Reset: %d steps", got)
	}
}

func TestHoldRepeatFastAfter(t *testing.T) {
	h := HoldRepeat{FastAfter: 3, FastIntervalMicros: 30_000}
	h.Hold(DirDown)
	// Delay plus three slow steps: 220 + 3*120 = 580ms, so by 600ms the slow
	// phase is over and the 30ms cadence takes over.
	slow := stepsOver(&h, 18) // ~600ms
	fast := stepsOver(&h, 3)  // ~100ms more
	if slow < 3 || slow > 4 {
		t.Fatalf("want 3-4 slow steps by 600ms, got %d", slow)
	}
	if fast < 3 {
		t.Fatalf("want at least 3 fast steps in 100ms at 30ms, got %d", fast)
	}
}
