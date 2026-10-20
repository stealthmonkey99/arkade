// SPDX-License-Identifier: GPL-2.0-or-later

package config_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// pushHeldDown sends one joystickchange: a real stick reports only on change,
// so "held" is silence afterwards. Seatless keeps the seat remap a no-op.
func pushHeldDown(rig *testkit.Rig) {
	rig.RT.Input.Push(input.Event{
		Kind:     input.KindJoystickChange,
		Position: input.JoyDown,
		Source: input.PlayerSource{
			Type: input.SourceController, ID: "test-stick",
			Slot: input.SlotOf(2), Seat: input.NoSeat,
		},
	})
}

func TestConfigMenuControllerHeldDirectionAutoRepeats(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")
	initial := testkit.ASCII(rig.Frame())

	pushHeldDown(rig)

	rig.Tick(20)
	mid := testkit.ASCII(rig.Frame())
	if mid == initial {
		t.Fatal("held direction did not move the focused button at all after ~667ms of holding down with no further events")
	}

	rig.Tick(20)
	later := testkit.ASCII(rig.Frame())
	if later == mid {
		t.Fatal("held direction stopped repeating: frame identical between ~667ms and ~1.33s of continuous hold")
	}
}

// Hold tracking arms only on KindJoystickChange, which the keyboard never emits.
func TestConfigMenuKeyboardHeldDirectionDoesNotDoubleStep(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")
	initial := testkit.ASCII(rig.Frame())

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	afterOne := testkit.ASCII(rig.Frame())
	if afterOne == initial {
		t.Fatal("one keyboard keypress should move the focused button, but the frame did not change")
	}

	rig.TickFor("1500ms")
	if got := testkit.ASCII(rig.Frame()); got != afterOne {
		t.Fatal("selection moved from ticking alone with no new keypress — keyboard input must never auto-repeat from Menu's hold-tracking")
	}

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if got := testkit.ASCII(rig.Frame()); got == afterOne {
		t.Fatal("second keyboard keypress did not move the focused button")
	}
}
