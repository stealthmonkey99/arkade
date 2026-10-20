// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// Direction is a d-pad intent, the navigation vocabulary interactive widgets
// ([FocusManager], [Menu], [NameEntry]) speak. A cartridge's input record
// converts with [input.Record.Dir] or [input.Record.PressDir].
type Direction = input.Direction

// The four directions and DirNone, the inert zero value meaning "no
// direction held".
const (
	DirNone  = input.DirNone
	DirUp    = input.DirUp
	DirDown  = input.DirDown
	DirLeft  = input.DirLeft
	DirRight = input.DirRight
)

// Action is a non-directional intent: the buttons a focusable UI reacts to
// beyond movement. Pass one to a widget's Do method once per button press, not
// every frame the button is held.
type Action uint8

// The Action values. ActionNone, the zero value, means no action.
const (
	ActionNone Action = iota
	// ActionActivate is "select / confirm / press" (A / Enter / Start).
	ActionActivate
	// ActionBack is "cancel / dismiss" (B / Escape).
	ActionBack
	// ActionErase is "rub out the last thing I entered" (C), a text-editing
	// verb kept apart from ActionBack: the B button is reserved for back and
	// quit everywhere, and a widget that consumed it as a destructive edit
	// would swallow that gesture.
	ActionErase
)
