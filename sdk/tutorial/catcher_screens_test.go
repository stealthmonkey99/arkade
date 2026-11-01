// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The screens make host calls, so these tests install hosttest's in-process
// fake cabinet and drive the real Game through it: no TinyGo, no wasm.

import (
	"strconv"
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
	// the panel's banner already shows GAME OVER and the score, so the LCD
	// shows the table's best, which on an empty table is this run's
	want := "BEST " + strconv.Itoa(g.sim.score)
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != want {
		t.Errorf("game-over LCD line 1 = %q, want %q", line1, want)
	}
}

// The title invites both seats; once one player starts, the other seat's
// LCD and lamps go dark rather than keep inviting.
func TestAOnePlayerRunBlanksTheIdleSeat(t *testing.T) {
	h, _ := startCatcher(t)
	if line1, _, _ := h.LCD(cartlib.SeatP2); line1 != "CATCHER ֍" {
		t.Fatalf("seat 2's title LCD line 1 = %q, want %q", line1, "CATCHER ֍")
	}

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	h.Tick(frameMicros)
	if line1, line2, _ := h.LCD(cartlib.SeatP2); line1 != "" || line2 != "" {
		t.Errorf("seat 2's LCD during seat 1's run = %q / %q, want blank", line1, line2)
	}
	for lamp, level := range h.Lamps[cartlib.SeatP2.Index()] {
		if level != 0 {
			t.Errorf("seat 2's lamp %d is at %d during seat 1's run, want dark", lamp, level)
		}
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

// The browser's shared keyboard sits at no seat, and the score is on the LCD
// rather than the panel, so a keyboard run must still put it somewhere.
func TestAKeyboardRunShowsItsScoreOnBothLCDs(t *testing.T) {
	h, g := startCatcher(t)
	h.Input(hosttest.KeyboardPress(cartlib.KeyEnter))
	if g.machine.State() != stPlaying {
		t.Fatalf("Enter at the title left state %d, want stPlaying", g.machine.State())
	}
	for _, seat := range []cartlib.Seat{cartlib.SeatP1, cartlib.SeatP2} {
		if line1, _, _ := h.LCD(seat); line1 != "֍ 0" {
			t.Errorf("seat %d LCD line 1 = %q, want the score %q", seat.Index(), line1, "֍ 0")
		}
	}
}
