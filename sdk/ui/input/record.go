// SPDX-License-Identifier: GPL-2.0-or-later

package input

// IsPress reports whether r is the canonical "pressed" record, KindKeypress.
// It is true once per physical press; do not also act on the KindKeydown.
func (r Record) IsPress() bool { return r.Kind == KindKeypress }

// IsQuit reports whether r is about a quit key (KeyQuit, KeyEscape or KeyJoyB,
// the red button), whatever its Kind. Combine it with [Record.IsPress] to act
// once per press; a hold-to-confirm also needs the keydown and keyup records.
func (r Record) IsQuit() bool {
	if r.Kind == KindJoystick {
		return false
	}
	return r.Key == KeyQuit || r.Key == KeyEscape || r.Key == KeyJoyB
}

// Virtual reports whether r came from a keyboard or a keyboard-driven
// virtual stick (FlagVirtual) rather than real HID hardware.
func (r Record) Virtual() bool { return r.Flags&FlagVirtual != 0 }

// Dir is the direction r names, whatever its Kind: an arrow key on a
// keydown, keyup or keypress record, or a cardinal stick position on a
// KindJoystick record. Diagonals, the stick's centre and every other key are
// DirNone. This is the level decode that hold tracking wants; to act once per
// push use [Record.PressDir].
func (r Record) Dir() Direction {
	if r.Kind == KindJoystick {
		switch r.Joy {
		case JoyN:
			return DirUp
		case JoyS:
			return DirDown
		case JoyW:
			return DirLeft
		case JoyE:
			return DirRight
		}
		return DirNone
	}
	switch r.Key {
	case KeyUp:
		return DirUp
	case KeyDown:
		return DirDown
	case KeyLeft:
		return DirLeft
	case KeyRight:
		return DirRight
	}
	return DirNone
}

// PressDir is the direction r presses, counted once per physical push: a
// KindJoystick record or a virtual (keyboard) arrow keypress. A real stick
// also sends a non-virtual arrow keypress for the same push, which is
// DirNone here. Keydown, keyup, diagonals and the stick's return to centre
// are DirNone.
func (r Record) PressDir() Direction {
	switch {
	case r.Kind == KindJoystick:
		return r.Dir()
	case r.Kind == KindKeypress && r.Virtual():
		return r.Dir()
	}
	return DirNone
}
