// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

// The counter is the one thing the demo keeps, so test that it does: press
// twice, then Init a fresh game against the same storage, as a reload would.
func TestCounterSurvivesAReload(t *testing.T) {
	h := hosttest.New(t)
	h.Init(&demoGame{})
	h.AdvancePastSplash()
	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeySpace)...)

	reloaded := &demoGame{}
	h.Init(reloaded)
	if reloaded.counter != 2 {
		t.Errorf("counter after two presses and a reload = %d, want 2", reloaded.counter)
	}
}

func TestBlockStaysOnScreen(t *testing.T) {
	h := hosttest.New(t)
	g := &demoGame{}
	h.Init(g)
	h.AdvancePastSplash()
	for range 1000 {
		h.Tick(100_000)
		if g.x < 0 || g.x > cartlib.FBWidth-8 || g.y < 0 || g.y > cartlib.FBHeight-8 {
			t.Fatalf("block at (%d,%d) is off the %dx%d screen", g.x, g.y, cartlib.FBWidth, cartlib.FBHeight)
		}
	}
}
