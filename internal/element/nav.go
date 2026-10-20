// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Dir maps a direction key to the sdk/ui direction, DirNone for any other
// key.
func Dir(name input.KeyName) ui.Direction {
	switch name {
	case input.KeyUp:
		return ui.DirUp
	case input.KeyDown:
		return ui.DirDown
	case input.KeyLeft:
		return ui.DirLeft
	case input.KeyRight:
		return ui.DirRight
	}
	return ui.DirNone
}

// Key maps an sdk/ui direction back to its key name, "" for DirNone.
func Key(d ui.Direction) input.KeyName {
	switch d {
	case ui.DirUp:
		return input.KeyUp
	case ui.DirDown:
		return input.KeyDown
	case ui.DirLeft:
		return input.KeyLeft
	case ui.DirRight:
		return input.KeyRight
	}
	return ""
}

// StickDir is the direction a joystick position holds, for hold-to-repeat:
// diagonals count as their vertical axis so an off-axis wobble keeps a list
// scrolling, and centre is DirNone.
func StickDir(pos input.JoystickPosition) ui.Direction {
	switch pos {
	case input.JoyUp, input.JoyUpLeft, input.JoyUpRight:
		return ui.DirUp
	case input.JoyDown, input.JoyDownLeft, input.JoyDownRight:
		return ui.DirDown
	case input.JoyLeft:
		return ui.DirLeft
	case input.JoyRight:
		return ui.DirRight
	}
	return ui.DirNone
}

// IsSelect reports whether a key activates the focused control.
func IsSelect(name input.KeyName) bool {
	return name == input.KeyEnter || name == input.KeySpace || name == input.KeyJoyA
}

// ElapsedMicros is the tick's elapsed time in the sdk/ui widgets' unit.
func ElapsedMicros(stats clock.FrameTime) uint64 {
	return uint64(stats.ElapsedMs * 1000)
}
