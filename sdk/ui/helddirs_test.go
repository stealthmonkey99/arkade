// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestHeldDirsRealKeys(t *testing.T) {
	var h HeldDirs
	h.KeyDown(DirLeft)
	h.KeyDown(DirUp) // independent holds: rotate while thrusting
	if !h.Held(DirLeft) || !h.Held(DirUp) {
		t.Fatal("both real holds should register independently")
	}
	if h.Current() != DirUp {
		t.Errorf("Current = %v, want most recent (DirUp)", h.Current())
	}
	h.KeyUp(DirUp)
	if h.Held(DirUp) {
		t.Error("KeyUp should release DirUp")
	}
	if h.Current() != DirLeft {
		t.Errorf("Current after release = %v, want DirLeft fallback", h.Current())
	}
}

func TestHeldDirsVirtualPulses(t *testing.T) {
	var h HeldDirs
	h.Pulse(DirRight)
	if !h.Held(DirRight) {
		t.Fatal("pulse should grant held credit")
	}
	h.Update(200_000) // within the 250ms default credit
	if !h.Held(DirRight) {
		t.Error("credit should bridge an auto-repeat gap")
	}
	h.Update(100_000) // past expiry
	if h.Held(DirRight) {
		t.Error("credit should expire without further pulses")
	}
	if h.Current() != DirNone {
		t.Errorf("Current after expiry = %v, want DirNone", h.Current())
	}
}

// A browser key press arrives as keydown plus a virtual keypress; keyup must
// end the hold at once rather than leave PulseMicros of credit behind.
func TestHeldDirsKeyUpDropsVirtualCredit(t *testing.T) {
	var h HeldDirs
	h.KeyDown(DirLeft)
	h.Pulse(DirLeft)
	if !h.Held(DirLeft) {
		t.Fatal("press should hold DirLeft")
	}
	h.KeyUp(DirLeft)
	if h.Held(DirLeft) {
		t.Error("KeyUp must drop the virtual credit too, not just the real flag")
	}
	if h.Current() != DirNone {
		t.Errorf("Current after release = %v, want DirNone", h.Current())
	}
	// The joystick level is a separate signal: a key release must not
	// cancel a stick that is still pushed.
	h.SetJoy(DirRight)
	h.KeyUp(DirRight)
	if !h.Held(DirRight) {
		t.Error("KeyUp must not clear the joystick position level")
	}
}

func TestHeldDirsJoystickLevel(t *testing.T) {
	var h HeldDirs
	h.SetJoy(DirDown)
	h.Update(10_000_000) // joystick position never decays
	if !h.Held(DirDown) {
		t.Fatal("joystick level should persist across Update")
	}
	h.SetJoy(DirNone) // centered
	if h.Held(DirDown) {
		t.Error("centering should release the joystick direction")
	}
}

func TestHeldDirsReset(t *testing.T) {
	h := HeldDirs{PulseMicros: 400_000}
	h.KeyDown(DirLeft)
	h.Pulse(DirRight)
	h.SetJoy(DirUp)
	h.Reset()
	for _, d := range []Direction{DirUp, DirDown, DirLeft, DirRight} {
		if h.Held(d) {
			t.Errorf("Reset left %v held", d)
		}
	}
	if h.PulseMicros != 400_000 {
		t.Error("Reset should preserve the configured PulseMicros")
	}
}

func TestHeldDirsCurrentAllocatesNothing(t *testing.T) {
	var h HeldDirs
	h.KeyDown(DirUp)
	h.KeyDown(DirLeft)
	h.KeyUp(DirLeft)
	var sink Direction
	if n := testing.AllocsPerRun(100, func() { sink = h.Current() }); n != 0 {
		t.Errorf("Current allocated %v times per call", n)
	}
	if sink != DirUp {
		t.Errorf("Current = %v, want DirUp", sink)
	}
}
