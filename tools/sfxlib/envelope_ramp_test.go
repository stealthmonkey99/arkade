// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func TestRampRisesSmoothlyBetweenItsEdges(t *testing.T) {
	for _, tc := range []struct {
		t, want float64
	}{
		{-1, 0}, {0, 0}, {0.5, 0.5}, {1, 1}, {2, 1},
	} {
		if got := Ramp(tc.t, 0, 1); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("Ramp(%.1f, 0, 1) = %.6f, want %.6f", tc.t, got, tc.want)
		}
	}
	const d = 1e-4
	if slope := (Ramp(d, 0, 1) - Ramp(0, 0, 1)) / d; slope > 1e-3 {
		t.Errorf("slope %.6f at the start, want ~0", slope)
	}
	if slope := (Ramp(1, 0, 1) - Ramp(1-d, 0, 1)) / d; slope > 1e-3 {
		t.Errorf("slope %.6f at the end, want ~0", slope)
	}
}

func TestRampIsMonotonic(t *testing.T) {
	prev := -1.0
	for i := 0; i <= 1000; i++ {
		got := Ramp(float64(i)/1000, 0.2, 0.8)
		if got < prev {
			t.Fatalf("Ramp fell from %.6f to %.6f at t=%.3f", prev, got, float64(i)/1000)
		}
		prev = got
	}
}

func TestRampWithNoWindowIsAStep(t *testing.T) {
	if got := Ramp(0.4999, 0.5, 0.5); got != 0 {
		t.Errorf("before a zero-width ramp: %.6f, want 0", got)
	}
	if got := Ramp(0.5001, 0.5, 0.5); got != 1 {
		t.Errorf("after a zero-width ramp: %.6f, want 1", got)
	}
	if got := Ramp(0.6, 0.5, 0.4); got != 1 {
		t.Errorf("past an inverted ramp: %.6f, want 1", got)
	}
}
