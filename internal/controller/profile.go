// SPDX-License-Identifier: GPL-2.0-or-later

// Package controller supports HID gamepads: per-device report decoding, a
// shared button/axis state machine, and a hot-plug manager that feeds decoded
// events into the input handler. HID I/O lives in the hiddev subpackage.
package controller

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// Report is one decoded HID input report.
type Report struct {
	// Buttons is a bitmask; bit i maps to Spec.Buttons()[i].
	Buttons int
	// X, Y are signed axis values (0 = centered).
	X, Y int
	// HasDpad marks devices with a separate hat switch (DualShock).
	HasDpad bool
	Dpad    input.JoystickPosition
}

// Profile is one supported device type: the identity a HID device is matched
// on, and the compiled Spec that decodes its reports. ProfileFromEnvelope is
// the only constructor, so Spec is never nil.
type Profile struct {
	Name string
	VID  uint16
	PID  uint16
	// Directional is "stick" or "dpad" (informational).
	Directional string
	// Bus restricts this profile to one transport, or "" for any.
	Bus  string
	Spec *Spec
}

func posFromAxes(x, y int) input.JoystickPosition {
	switch {
	case x < 0 && y < 0:
		return input.JoyUpLeft
	case x < 0 && y > 0:
		return input.JoyDownLeft
	case x < 0:
		return input.JoyLeft
	case x > 0 && y < 0:
		return input.JoyUpRight
	case x > 0 && y > 0:
		return input.JoyDownRight
	case x > 0:
		return input.JoyRight
	case y < 0:
		return input.JoyUp
	case y > 0:
		return input.JoyDown
	default:
		return input.JoyCenter
	}
}

// jsRound rounds half toward +Inf (-0.5 gives 0), which the axis divide
// filter depends on.
func jsRound(v float64) int {
	return int(math.Floor(v + 0.5))
}
