// SPDX-License-Identifier: GPL-2.0-or-later

package input

import (
	"fmt"
	"strings"
)

type EventKind string

const (
	KindKeypress       EventKind = "keypress"
	KindKeydown        EventKind = "keydown"
	KindKeyup          EventKind = "keyup"
	KindJoystickChange EventKind = "joystickchange"
)

type SourceType string

const (
	SourceKeyboard   SourceType = "keyboard"
	SourceController SourceType = "controller"
)

// PlayerSource identifies which device an event came from and where that
// device sits. A keyboard or the microphone has NoSlot and NoSeat; a
// controller has a Slot for life and a Seat that pairing can move.
type PlayerSource struct {
	Type SourceType
	// ID is the HID path, or "virtual-N" for the keyboard test controller.
	ID   string
	Slot Slot
	Seat Seat
}

// Virtual reports whether the source is the keyboard or a virtual controller
// rather than a real HID stick. A stick emits both a joystickchange and a
// direction keypress per push, so callers acting on both use this to move
// once.
func (s PlayerSource) Virtual() bool {
	return s.Type == SourceKeyboard || IsVirtualControllerID(s.ID)
}

const virtualIDPrefix = "virtual-"

func VirtualControllerID(index int) string { return fmt.Sprintf("%s%d", virtualIDPrefix, index) }

func IsVirtualControllerID(id string) bool { return strings.HasPrefix(id, virtualIDPrefix) }

// Event is a single input occurrence.
type Event struct {
	Kind     EventKind
	Name     KeyName          // for key events
	Position JoystickPosition // for joystickchange events
	Ctrl     bool
	Meta     bool
	Shift    bool
	Source   PlayerSource
}
