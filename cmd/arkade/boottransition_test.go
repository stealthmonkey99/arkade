// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// litPixels counts non-black pixels and those outside the loading bar's box
// (placed in Runtime.Init); a frame lit only inside it is a bar flash.
func litPixels(frame display.Matrix) (lit, outsideBar int) {
	for y, row := range frame {
		for x, p := range row {
			if p.R < 20 && p.G < 20 && p.B < 20 {
				continue
			}
			lit++
			if y < 28 || y > 36 || x < 20 || x > 44 {
				outsideBar++
			}
		}
	}
	return lit, outsideBar
}

// Both tests walk the painted history, not the frame at the end of each tick:
// showLoading paints inside loadApp and hides the bar before the tick returns.

// TestBootShowsNoLoadingBar does not check blankness; the splash opens on the
// Å ring alone and is near-dark at first.
func TestBootShowsNoLoadingBar(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		AttractAfter: time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	rig.TickFor("1000ms")

	for i, f := range rig.Capture.Frames() {
		if lit, outsideBar := litPixels(f.Data); lit > 0 && outsideBar == 0 {
			t.Fatalf("loading-bar flash in painted frame %d of the boot splash: %d lit pixels, all inside the bar", i, lit)
		}
	}
}

// TestBootToAttractPaintsNoGap requires that no painted frame across the boot
// to attract transition is blank or bar-only, and that attract opens at the
// splash's brightness. Goldens cannot catch this; it is about which frames get painted.
func TestBootToAttractPaintsNoGap(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		AttractAfter: time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	rig.TickFor("2400ms") // into the splash, past the dark opening, before it exits
	rig.Capture.Reset()

	var switched bool
	for i := 0; i < 60 && !switched; i++ {
		rig.Tick(1)
		if rig.RT.CurrentSlug() == "attract" {
			switched = true
			if got := maxChannel(rig.Frame()); got < 250 {
				t.Errorf("attract opened at brightness %d; it should pick the splash up at full and settle down from there", got)
			}
		}
	}
	if !switched {
		t.Fatal("boot never handed off to attract")
	}

	// Bare starfield is lit but still a blink; both wordmarks light several hundred pixels.
	const wordmarkFloor = 200
	for i, f := range rig.Capture.Frames() {
		lit, outsideBar := litPixels(f.Data)
		if lit < wordmarkFloor {
			t.Fatalf("painted frame %d has only %d lit pixels: the wordmark is missing, which is a blink through the transition", i, lit)
		}
		if outsideBar == 0 {
			t.Fatalf("loading-bar flash in painted frame %d: %d lit pixels, all inside the bar", i, lit)
		}
	}

	// The full-brightness open is a ramp down to the ambient level.
	rig.TickFor("1500ms")
	if got := maxChannel(rig.Frame()); got > 180 {
		t.Errorf("attract should have settled to the ambient dim, still at %d", got)
	}
}

func maxChannel(frame display.Matrix) int {
	m := 0
	for _, row := range frame {
		for _, p := range row {
			for _, v := range []uint8{p.R, p.G, p.B} {
				if int(v) > m {
					m = int(v)
				}
			}
		}
	}
	return m
}
