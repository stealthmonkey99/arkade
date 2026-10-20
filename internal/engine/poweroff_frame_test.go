// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/goldens"
)

// powerOffSource is a stand-in for whatever was on the panel when the button
// was pressed: a lit band well off centre, so a collapse that fails to move
// it toward the middle row is obvious.
func powerOffSource(w, h int) display.Matrix {
	m := display.NewMatrix(w, h, display.Black)
	for y := 8; y < 16; y++ {
		for x := 4; x < w-4; x++ {
			m[y][x] = display.Pixel{R: 40, G: 90, B: 160, A: 255}
		}
	}
	return m
}

// litRows reports the first and last rows with any light in them, and how
// many pixels are lit overall.
func litRows(f display.Matrix) (first, last, count int) {
	first, last = -1, -1
	for y, row := range f {
		for _, p := range row {
			if p.R|p.G|p.B == 0 {
				continue
			}
			count++
			if first < 0 {
				first = y
			}
			last = y
		}
	}
	return
}

func TestPowerOffFrameTimeline(t *testing.T) {
	const w, h = 64, 64
	src := powerOffSource(w, h)

	for _, ms := range []float64{0, 200, powerOffHoldMs - 1} {
		if got := powerOffFrame(src, ms); !matricesEqual(got, src) {
			t.Errorf("t=%vms: frame moved, want the picture held untouched", ms)
		}
	}
	if same := powerOffFrame(src, 0); &same[0][0] == &src[0][0] {
		t.Error("the hold aliases the source frame, want a copy")
	}

	// The collapse: the lit band closes on the middle row, and brightens as
	// it does.
	prevSpan := h
	for _, ms := range []float64{500, 700, 900} {
		first, last, _ := litRows(powerOffFrame(src, ms))
		span := last - first
		if span >= prevSpan {
			t.Errorf("t=%vms: band spans %d rows, want it shorter than the %d before", ms, span, prevSpan)
		}
		prevSpan = span
		if mid := (first + last) / 2; mid < h/2-2 || mid > h/2+2 {
			t.Errorf("t=%vms: band centred on row %d, want it closing on %d", ms, mid, h/2)
		}
	}
	if !brighterThan(powerOffFrame(src, 900), powerOffFrame(src, 500)) {
		t.Error("the collapse does not brighten as it squeezes")
	}

	// The line: two rows either side of centre, shrinking to the middle.
	wide := powerOffFrame(src, powerOffShrinkAtMs+10)
	narrow := powerOffFrame(src, powerOffGlowAtMs-10)
	for name, f := range map[string]display.Matrix{"wide": wide, "narrow": narrow} {
		first, last, _ := litRows(f)
		if first != h/2-1 || last != h/2 {
			t.Errorf("%s line lights rows %d..%d, want %d..%d", name, first, last, h/2-1, h/2)
		}
	}
	_, _, wideCount := litRows(wide)
	_, _, narrowCount := litRows(narrow)
	if narrowCount >= wideCount {
		t.Errorf("line went from %d lit pixels to %d, want it shrinking", wideCount, narrowCount)
	}

	// The dot: a small halo at the centre, dying to black by the deadline.
	dot := powerOffFrame(src, powerOffGlowAtMs+10)
	first, last, count := litRows(dot)
	if first < h/2-3 || last > h/2+3 || count > 49 {
		t.Errorf("dot lights rows %d..%d over %d pixels, want a radius-3 halo at the centre", first, last, count)
	}
	for _, ms := range []float64{powerOffDeadMs, powerOffDeadMs + 500, powerOffQuitMs} {
		if _, _, n := litRows(powerOffFrame(src, ms)); n != 0 {
			t.Errorf("t=%vms: %d pixels still lit, want black", ms, n)
		}
	}
}

func TestPowerOffFrameGoldens(t *testing.T) {
	src := powerOffSource(64, 64)
	for _, tc := range []struct {
		name string
		ms   float64
	}{
		{"poweroff-collapse", 800},
		{"poweroff-line", 1200},
		{"poweroff-dot", 1600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goldens.Frame(t, tc.name, powerOffFrame(src, tc.ms))
		})
	}
}

func matricesEqual(a, b display.Matrix) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}

func brighterThan(a, b display.Matrix) bool {
	sum := func(m display.Matrix) int {
		total := 0
		for _, row := range m {
			for _, p := range row {
				total += int(p.R) + int(p.G) + int(p.B)
			}
		}
		return total
	}
	return sum(a) > sum(b)
}
