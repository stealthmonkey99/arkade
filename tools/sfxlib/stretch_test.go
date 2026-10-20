// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

// crossings counts upward zero crossings per second over the middle of s.
func crossings(s []float64, rate int) float64 {
	lo, hi := len(s)/4, len(s)*3/4
	n := 0
	for i := lo + 1; i < hi; i++ {
		if s[i-1] < 0 && s[i] >= 0 {
			n++
		}
	}
	return float64(n) * float64(rate) / float64(hi-lo)
}

func TestPitchShiftMovesPitchAndKeepsLength(t *testing.T) {
	const rate = 24000
	for _, c := range []struct{ semitones, wantHz float64 }{{12, 400}, {-12, 100}, {4, 200 * 1.2599}, {-3, 200 * 0.8409}} {
		in := make([]float64, rate)
		for i := range in {
			in[i] = 0.5 * math.Sin(2*math.Pi*200*float64(i)/rate)
		}
		out := PitchShift(in, rate, c.semitones)
		if len(out) != len(in) {
			t.Errorf("%+v semitones: length %d, want %d", c.semitones, len(out), len(in))
		}
		if got := crossings(out, rate); math.Abs(got-c.wantHz) > c.wantHz*0.02 {
			t.Errorf("%+v semitones: %.1f Hz, want %.1f", c.semitones, got, c.wantHz)
		}
	}
}

func TestPitchShiftZeroIsUntouched(t *testing.T) {
	in := []float64{0.1, -0.2, 0.3}
	if out := PitchShift(in, 24000, 0); &out[0] != &in[0] {
		t.Error("a zero shift copied or changed the samples")
	}
}
