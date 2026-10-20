// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"
	"testing"
)

// steps runs a fade from..to over totalMs at the given ease, sampling the
// device level every tickMicros, and returns the whole ramp including both
// ends.
func steps(from, to uint8, totalMs int, ease Ease, tickMicros uint64) []uint8 {
	f := lampFade{
		from:   float64(from),
		to:     float64(to),
		total:  uint64(totalMs) * 1000,
		ease:   ease,
		active: true,
	}
	out := []uint8{f.level()}
	for f.elapsed < f.total {
		f.elapsed += tickMicros
		if f.elapsed > f.total {
			f.elapsed = f.total
		}
		out = append(out, f.level())
	}
	return out
}

func TestEasedFadesKeepTheirEnds(t *testing.T) {
	for _, ease := range []Ease{Linear, EaseOut, EaseInOut} {
		for _, dir := range [][2]uint8{{0, 255}, {255, 0}, {40, 200}, {200, 40}} {
			from, to := dir[0], dir[1]
			ramp := steps(from, to, 600, ease, 33333)
			if ramp[0] != from {
				t.Errorf("ease %d %d->%d: starts at %d", ease, from, to, ramp[0])
			}
			if last := ramp[len(ramp)-1]; last != to {
				t.Errorf("ease %d %d->%d: ends at %d", ease, from, to, last)
			}
			up := to > from
			for i := 1; i < len(ramp); i++ {
				if up && ramp[i] < ramp[i-1] || !up && ramp[i] > ramp[i-1] {
					t.Fatalf("ease %d %d->%d: not monotonic at step %d: %v", ease, from, to, i, ramp)
				}
			}
		}
	}
}

// EaseOut is past the middle at halfway, EaseInOut is exactly on it, Linear is the reference.
func TestEaseCurvesBendTheRightWay(t *testing.T) {
	if got := Linear.at(0.5); got != 0.5 {
		t.Errorf("Linear.at(0.5) = %v, want 0.5", got)
	}
	if got := EaseOut.at(0.5); got <= 0.6 {
		t.Errorf("EaseOut.at(0.5) = %v, want it well ahead of linear", got)
	}
	if got := EaseInOut.at(0.5); math.Abs(got-0.5) > 1e-12 {
		t.Errorf("EaseInOut.at(0.5) = %v, want 0.5", got)
	}
	if got := EaseInOut.at(0.25); got >= 0.25 {
		t.Errorf("EaseInOut.at(0.25) = %v, want it still easing in", got)
	}
	for _, e := range []Ease{Linear, EaseOut, EaseInOut} {
		if got := e.at(-1); got != 0 {
			t.Errorf("ease %d clamps low to %v, want 0", e, got)
		}
		if got := e.at(2); got != 1 {
			t.Errorf("ease %d clamps high to %v, want 1", e, got)
		}
	}
}

// Compares exact floats, not rounded levels, so fades tuned by feel keep their timing.
func TestLinearIsBitIdenticalToTheOldStepping(t *testing.T) {
	for from := 0; from <= 255; from += 5 {
		for to := 0; to <= 255; to += 5 {
			f := lampFade{from: float64(from), to: float64(to), total: 600000, active: true}
			for _, e := range []uint64{0, 1, 12345, 199999, 200000, 333333, 599999, 600000} {
				f.elapsed = e
				want := f.from + (f.to-f.from)*float64(f.elapsed)/float64(f.total)
				if got := f.value(); math.Float64bits(got) != math.Float64bits(want) {
					t.Fatalf("%d->%d at %dus: value = %v, want %v", from, to, e, got, want)
				}
			}
		}
	}
}
