// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// TestFlightComboOrdinalsMatchTheABI pins the ordinals the engine's debug
// combo hardcodes, since it cannot import this package.
func TestFlightComboOrdinalsMatchTheABI(t *testing.T) {
	keys := map[input.KeyName]uint8{
		input.KeyUp:      6,
		input.KeyDown:    7,
		input.KeyLeft:    8,
		input.KeyRight:   9,
		input.KeyJoyAux1: 20,
	}
	for name, want := range keys {
		got, ok := KeyToOrdinal(name)
		if !ok {
			t.Errorf("%s has no ordinal at all", name)
			continue
		}
		if got != want {
			t.Errorf("%s is ordinal %d; internal/engine/flightcombo.go hardcodes %d", name, got, want)
		}
	}

	joys := map[input.JoystickPosition]uint8{
		input.JoyUp: 1, input.JoyRight: 3, input.JoyDown: 5, input.JoyLeft: 7,
	}
	for pos, want := range joys {
		if got := joyToOrdinal(pos); got != want {
			t.Errorf("joystick %v is ordinal %d; internal/engine/flightcombo.go hardcodes %d", pos, got, want)
		}
	}

	if RecordKindKeydown != 0 || RecordKindKeyup != 1 || RecordKindKeypress != 2 || RecordKindJoystickChange != 3 {
		t.Error("record kind ordinals moved; internal/engine/flightcombo.go hardcodes 0/1/2/3")
	}
	if RecordFlagVirtual != 1 || SeatNone != 0xFF {
		t.Error("RecordFlagVirtual or SeatNone moved; internal/engine/flightcombo.go hardcodes 1 and 0xFF")
	}
}
