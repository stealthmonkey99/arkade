// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
)

func hasNonBlack(frame display.Matrix) bool {
	for _, row := range frame {
		for _, p := range row {
			if p != display.Black {
				return true
			}
		}
	}
	return false
}

func hintRowsLit(frame display.Matrix) bool {
	for y := sleepHintY; y < sleepHintY+2*sleepHintStep && y < len(frame); y++ {
		for _, p := range frame[y] {
			if p != display.Black {
				return true
			}
		}
	}
	return false
}

// TestSleepFadeFrameTimeline pins the sequence's phases. Note the Zzz is
// already mid-flight at t=0 — two of the three glyphs start a third and two
// thirds of the way through their loop, which is what makes the stream read
// as continuous from the first frame rather than starting empty.
func TestSleepFadeFrameTimeline(t *testing.T) {
	w, h := 64, 64

	first := sleepFadeFrame(w, h, 0)
	if !hasNonBlack(first) {
		t.Error("expected the Zzz stream to be mid-flight on the very first frame")
	}
	if hintRowsLit(first) {
		t.Error("expected the wake hint to still be dark at elapsed=0")
	}
	if hintRowsLit(sleepFadeFrame(w, h, sleepHintFadeInMs)) {
		t.Error("expected the wake hint to only start fading in after sleepHintFadeInMs")
	}
	if !hintRowsLit(sleepFadeFrame(w, h, sleepHintFullMs)) {
		t.Error("expected the wake hint to be lit once it has fully faded in")
	}
	if !hasNonBlack(sleepFadeFrame(w, h, sleepFadeStartMs)) {
		t.Error("expected the frame to still be lit when the closing fade begins")
	}
	if hasNonBlack(sleepFadeFrame(w, h, sleepTotalMs)) {
		t.Error("expected the sequence to be fully black by its total duration")
	}
	if hasNonBlack(sleepFadeFrame(w, h, sleepTotalMs+10_000)) {
		t.Error("expected well past the total duration to still render black")
	}
}

// The same pixels are lit either side of sleepFadeStartMs, only darker.
func TestSleepFadeFrameDims(t *testing.T) {
	w, h := 64, 64
	full := sleepFadeFrame(w, h, sleepFadeStartMs)
	dim := sleepFadeFrame(w, h, sleepFadeStartMs+sleepFadeMs/2)

	brighter := 0
	for y := range full {
		for x := range full[y] {
			a, b := full[y][x], dim[y][x]
			if a == display.Black {
				continue
			}
			if b == display.Black || b.R < a.R {
				brighter++
			}
		}
	}
	if brighter == 0 {
		t.Error("expected the closing fade to darken what the frame was painting")
	}
}

func TestClamp01(t *testing.T) {
	cases := map[float64]float64{-1: 0, 0: 0, 0.5: 0.5, 1: 1, 2: 1}
	for in, want := range cases {
		if got := clamp01(in); got != want {
			t.Errorf("clamp01(%v) = %v, want %v", in, got, want)
		}
	}
}
