// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import "github.com/stealthmonkey99/arkade/internal/input"

// Decoder turns one device's successive reports into input events: button
// transitions, joystick position changes with synthetic arrow-key edges on
// axis zero-crossings, and hat-switch changes. The first report only primes
// the state, so a button held at plug-in fires nothing.
type Decoder struct {
	spec *Spec
	keys []input.KeyName

	primedButtons bool
	buttons       int

	primedAxes bool
	x, y       int
	joystick   input.JoystickPosition

	primedDpad bool
	dpad       input.JoystickPosition
}

// NewDecoder builds a decoder for one device speaking spec.
func NewDecoder(spec *Spec) *Decoder {
	return &Decoder{spec: spec, keys: spec.Buttons()}
}

// Feed decodes one raw report and returns events ordered buttons (bit order),
// dpad, stick. Every keydown is followed by a keypress.
func (d *Decoder) Feed(data []byte) []input.Event {
	rep := d.spec.Decode(data)
	var out []input.Event

	emitDown := func(key input.KeyName) {
		out = append(out,
			input.Event{Kind: input.KindKeydown, Name: key},
			input.Event{Kind: input.KindKeypress, Name: key},
		)
	}
	emitUp := func(key input.KeyName) {
		out = append(out, input.Event{Kind: input.KindKeyup, Name: key})
	}

	if !d.primedButtons {
		d.primedButtons = true
		d.buttons = rep.Buttons
	} else if d.buttons != rep.Buttons {
		oldState, newState := d.buttons, rep.Buttons
		for i := 0; oldState^newState != 0; i++ {
			if (oldState^newState)&1 != 0 && i < len(d.keys) {
				if newState&1 != 0 {
					emitDown(d.keys[i])
				} else {
					emitUp(d.keys[i])
				}
			}
			oldState >>= 1
			newState >>= 1
		}
		d.buttons = rep.Buttons
	}

	if rep.HasDpad {
		if !d.primedDpad {
			d.primedDpad = true
			d.dpad = rep.Dpad
		} else if d.dpad != rep.Dpad {
			out = append(out, input.Event{Kind: input.KindJoystickChange, Position: rep.Dpad})
			if key, ok := cardinalKey(d.dpad); ok {
				emitUp(key)
			}
			if key, ok := cardinalKey(rep.Dpad); ok {
				emitDown(key)
			}
			d.dpad = rep.Dpad
		}
	}

	pos := posFromAxes(rep.X, rep.Y)
	if !d.primedAxes {
		d.primedAxes = true
	} else if d.joystick != pos {
		out = append(out, input.Event{Kind: input.KindJoystickChange, Position: pos})

		oldX, newX := d.x, rep.X
		if oldX < 0 && newX >= 0 {
			emitUp(input.KeyLeft)
		} else if oldX > 0 && newX <= 0 {
			emitUp(input.KeyRight)
		}
		if oldX >= 0 && newX < 0 {
			emitDown(input.KeyLeft)
		} else if oldX <= 0 && newX > 0 {
			emitDown(input.KeyRight)
		}

		oldY, newY := d.y, rep.Y
		if oldY < 0 && newY >= 0 {
			emitUp(input.KeyUp)
		} else if oldY > 0 && newY <= 0 {
			emitUp(input.KeyDown)
		}
		if oldY >= 0 && newY < 0 {
			emitDown(input.KeyUp)
		} else if oldY <= 0 && newY > 0 {
			emitDown(input.KeyDown)
		}
	}
	d.x, d.y = rep.X, rep.Y
	d.joystick = pos

	return out
}

func cardinalKey(pos input.JoystickPosition) (input.KeyName, bool) {
	switch pos {
	case input.JoyLeft:
		return input.KeyLeft, true
	case input.JoyRight:
		return input.KeyRight, true
	case input.JoyUp:
		return input.KeyUp, true
	case input.JoyDown:
		return input.KeyDown, true
	default:
		return "", false
	}
}
