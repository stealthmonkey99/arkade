// SPDX-License-Identifier: GPL-2.0-or-later

package attract

import "testing"

func near(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	return d > -eps && d < eps
}

func TestFadeEdgesHoldsThenFades(t *testing.T) {
	const dur = 8000.0
	cases := []struct {
		name string
		rem  float64
		want float64
	}{
		{"start", 0, 0},
		{"mid fade-in", ambientFadeMs / 2, ambientDim / 2},
		{"end of fade-in", ambientFadeMs, ambientDim},
		{"held", dur / 2, ambientDim},
		{"start of fade-out", dur - ambientFadeMs, ambientDim},
		{"mid fade-out", dur - ambientFadeMs/2, ambientDim / 2},
		{"end", dur, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fadeEdges(c.rem, dur, ambientDim)
			if !near(got, c.want) {
				t.Errorf("fadeEdges(%v, %v, ambientDim) = %v, want %v", c.rem, dur, got, c.want)
			}
		})
	}
}

func TestFadeEdgesZeroDurNeverDividesByIt(t *testing.T) {
	if got := fadeEdges(0, 0, ambientDim); got != ambientDim {
		t.Errorf("fadeEdges with a zero-length slot = %v, want ambientDim unchanged", got)
	}
}

func TestWordmarkOpacityFirstLapIdleTimeoutStartsFlat(t *testing.T) {
	const dur = 8000.0
	if got := wordmarkOpacity(0, dur, true, false); got != ambientDim {
		t.Errorf("first-lap, no boot settle, at rem=0: got %v, want ambientDim (no flash-up)", got)
	}
	if got := wordmarkOpacity(dur/2, dur, true, false); got != ambientDim {
		t.Errorf("first-lap, no boot settle, mid-slot: got %v, want ambientDim", got)
	}
	if got := wordmarkOpacity(dur, dur, true, false); got != 0 {
		t.Errorf("first-lap, no boot settle, at slot end: got %v, want 0 (fades out into the figure)", got)
	}
}

func TestWordmarkOpacityBootSettleEasesFromFull(t *testing.T) {
	const dur = 8000.0
	if got := wordmarkOpacity(0, dur, true, true); got != 1 {
		t.Errorf("boot transition at rem=0: got %v, want 1 (splash's own brightness)", got)
	}
	if got := wordmarkOpacity(bootSettleMs, dur, true, true); !near(got, ambientDim) {
		t.Errorf("boot transition once settled: got %v, want ambientDim", got)
	}
	mid := wordmarkOpacity(bootSettleMs/2, dur, true, true)
	if mid <= ambientDim || mid >= 1 {
		t.Errorf("boot transition mid-settle: got %v, want strictly between ambientDim and 1", mid)
	}
}

func TestWordmarkOpacityWrapFadesInEvenWithBootSettleFlagSet(t *testing.T) {
	const dur = 8000.0
	if got := wordmarkOpacity(0, dur, false, true); got != 0 {
		t.Errorf("wrap-around at rem=0: got %v, want 0 (fades in from black)", got)
	}
	if got := wordmarkOpacity(ambientFadeMs, dur, false, true); !near(got, ambientDim) {
		t.Errorf("wrap-around past the fade-in: got %v, want ambientDim", got)
	}
}
