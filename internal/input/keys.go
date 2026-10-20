// SPDX-License-Identifier: GPL-2.0-or-later

// Package input models input events and the mapping-stack dispatch system.
// Controllers push Events into a Handler; apps push Mapping sets that route
// events into queues or trigger callbacks.
package input

import "github.com/stealthmonkey99/arkade/internal/display"

type KeyName string

const (
	KeyBrightness KeyName = "b"
	KeyRotate     KeyName = "r"
	KeyQuit       KeyName = "q"
	KeyEscape     KeyName = "escape"
	KeyEnter      KeyName = "return"
	KeySpace      KeyName = "space"
	KeyUp         KeyName = "up"
	KeyDown       KeyName = "down"
	KeyLeft       KeyName = "left"
	KeyRight      KeyName = "right"
	KeyHome       KeyName = "home"
	KeyEnd        KeyName = "end"
	KeyJoyA       KeyName = "joya"
	KeyJoyB       KeyName = "joyb"
	KeyJoyC       KeyName = "joyc"
	KeyJoyD       KeyName = "joyd"
	KeyL1         KeyName = "l1"
	KeyR1         KeyName = "r1"
	KeyL2         KeyName = "l2"
	KeyR2         KeyName = "r2"
	KeyJoyAux1    KeyName = "joyaux1"
	KeyJoyAux2    KeyName = "joyaux2"
	KeyJoyAux3    KeyName = "joyaux3"
	KeyJoyAux4    KeyName = "joyaux4"
	KeySelect     KeyName = "select"
	KeyStart      KeyName = "start"
	KeyTurbo      KeyName = "turbo"
	KeyClear      KeyName = "clear"

	// Microphone events travel the key path as SourceKeyboard with NoSeat.
	KeyMicWhistle KeyName = "mic.whistle"
	KeyMicClap    KeyName = "mic.clap"
	KeyMicBlow    KeyName = "mic.blow"

	// Cues are the classes of a cartridge's bundled sound model; their
	// meaning is whatever the bundle says.
	KeyMicCue0 KeyName = "mic.cue0"
	KeyMicCue1 KeyName = "mic.cue1"
	KeyMicCue2 KeyName = "mic.cue2"
	KeyMicCue3 KeyName = "mic.cue3"
	KeyMicCue4 KeyName = "mic.cue4"
	KeyMicCue5 KeyName = "mic.cue5"
	KeyMicCue6 KeyName = "mic.cue6"
	KeyMicCue7 KeyName = "mic.cue7"
)

// MicCueKeys are the cue keys in cue-index order, so MicCueKeys[i] is class
// i's key.
var MicCueKeys = [...]KeyName{
	KeyMicCue0, KeyMicCue1, KeyMicCue2, KeyMicCue3,
	KeyMicCue4, KeyMicCue5, KeyMicCue6, KeyMicCue7,
}

func MicCueKey(i int) KeyName {
	if i < 0 || i >= len(MicCueKeys) {
		return ""
	}
	return MicCueKeys[i]
}

func IsMicKey(k KeyName) bool {
	switch k {
	case KeyMicWhistle, KeyMicClap, KeyMicBlow:
		return true
	}
	for _, cue := range MicCueKeys {
		if k == cue {
			return true
		}
	}
	return false
}

type JoystickPosition string

const (
	JoyCenter    JoystickPosition = "center"
	JoyUp        JoystickPosition = "up"
	JoyDown      JoystickPosition = "down"
	JoyLeft      JoystickPosition = "left"
	JoyRight     JoystickPosition = "right"
	JoyUpLeft    JoystickPosition = "up-left"
	JoyUpRight   JoystickPosition = "up-right"
	JoyDownLeft  JoystickPosition = "down-left"
	JoyDownRight JoystickPosition = "down-right"
)

// ControllerPosition says which side of the cabinet a controller sits on.
type ControllerPosition string

const (
	PositionLeft    ControllerPosition = "left"
	PositionRight   ControllerPosition = "right"
	PositionUnknown ControllerPosition = "unknown"
)

type Orientation = display.Orientation

const (
	Upright          = display.Upright
	Clockwise        = display.Clockwise
	UpsideDown       = display.UpsideDown
	CounterClockwise = display.CounterClockwise
)

var keyOrder = []KeyName{KeyUp, KeyRight, KeyDown, KeyLeft}

var joyOrder = []JoystickPosition{
	JoyUp, JoyUpRight, JoyRight, JoyDownRight,
	JoyDown, JoyDownLeft, JoyLeft, JoyUpLeft,
}

// directionSteps is the clockwise quarter-turn remap for a controller mounted
// on the left or right of the cabinet: mount minus rotation, mod 4. Signs are
// pinned by two cabinet-validated cases: upright display remaps left -90 and
// right +90, and a display rotated toward a seat leaves that seat's remap as
// identity.
func directionSteps(position ControllerPosition, rotation Orientation) int {
	var mount int
	switch position {
	case PositionLeft:
		mount = -1
	case PositionRight:
		mount = 1
	default:
		return 0
	}
	var rot int
	switch rotation {
	case Clockwise:
		rot = 1
	case UpsideDown:
		rot = 2
	case CounterClockwise:
		rot = -1
	}
	return ((mount-rot)%4 + 4) % 4
}

// MapKeyDirection remaps a directional key for a side-mounted controller
// under the current display rotation.
func MapKeyDirection(position ControllerPosition, rotation Orientation, key KeyName) KeyName {
	steps := directionSteps(position, rotation)
	if steps == 0 {
		return key
	}
	i := indexOf(keyOrder, key)
	if i < 0 {
		return key
	}
	return keyOrder[(i+steps)%len(keyOrder)]
}

// MapJoyDirection is MapKeyDirection for stick positions: two ring steps per quarter turn.
func MapJoyDirection(position ControllerPosition, rotation Orientation, pos JoystickPosition) JoystickPosition {
	steps := directionSteps(position, rotation)
	if steps == 0 {
		return pos
	}
	i := indexOf(joyOrder, pos)
	if i < 0 {
		return pos
	}
	return joyOrder[(i+2*steps)%len(joyOrder)]
}

func indexOf[T comparable](list []T, v T) int {
	for i, item := range list {
		if item == v {
			return i
		}
	}
	return -1
}
