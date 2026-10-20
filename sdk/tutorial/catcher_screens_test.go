// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The screens make host calls, so these tests install hosttest's in-process
// fake cabinet and drive the real Game through it: no TinyGo, no wasm.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

const frameMicros = 33_333 // the cabinet's 30 fps

// startCatcher is a fresh game on its title screen, load phase done.
func startCatcher(t *testing.T) (*hosttest.Host, *catcherGame) {
	h := hosttest.New(t)
	g := &catcherGame{}
	h.Init(g)
	h.AdvancePastSplash()
	return h, g
}

func TestGreenStartsARunThatEndsOnGameOver(t *testing.T) {
	h, g := startCatcher(t)
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != "CATCHER ֍" {
		t.Fatalf("title LCD line 1 = %q, want %q", line1, "CATCHER ֍")
	}

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	if g.machine.State() != stPlaying {
		t.Fatalf("green at the title left state %d, want stPlaying", g.machine.State())
	}

	// a catcher nobody steers misses its way to the end of the run
	for i := 0; i < 3000 && g.machine.State() == stPlaying; i++ {
		h.Tick(frameMicros)
	}
	if g.machine.State() != stOver {
		t.Fatalf("an unsteered run never reached game over (state %d)", g.machine.State())
	}
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != "GAME OVER" {
		t.Errorf("game-over LCD line 1 = %q, want %q", line1, "GAME OVER")
	}
}

func TestBMidRunOpensTheConfirmAndAnotherPressResumes(t *testing.T) {
	h, g := startCatcher(t)
	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	h.Tick(frameMicros)
	playing, _, _ := h.LCD(cartlib.SeatP1)

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyB)...)
	h.Input(hosttest.Release(cartlib.SeatP1, cartlib.KeyJoyB))
	if g.machine.State() != stConfirm {
		t.Fatalf("B mid-run left state %d, want stConfirm", g.machine.State())
	}
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 == playing {
		t.Errorf("the confirm left the playing LCD up (%q)", line1)
	}

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	if g.machine.State() != stPlaying {
		t.Fatalf("a press over the confirm left state %d, want stPlaying", g.machine.State())
	}
	// Pop re-enters the covered screen, and entering re-asserts its LCD
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != playing {
		t.Errorf("LCD line 1 after resuming = %q, want the playing line %q", line1, playing)
	}
	if h.Exits != 0 {
		t.Errorf("a cancelled confirm still asked to exit %d time(s)", h.Exits)
	}
}
