// SPDX-License-Identifier: GPL-2.0-or-later

// Package input is the cartridge ABI's input vocabulary: the decoded 8-byte
// input record (docs/CARTRIDGE.md, "Input records"), its key, joystick and
// flag ordinals, the [Seat] and [Slot] a record carries, and the [Direction]
// the sdk/ui widgets speak.
//
// sdk/cartlib re-exports every name here (cartlib.InputRecord is [Record]), so
// a cartridge does not need to import this package by name. Nothing here
// allocates.
package input

// Record is one decoded input record. A cartridge receives a slice of them
// per tick in Game.HandleInput; the host builds them, so every field holds
// one of the values documented on it.
type Record struct {
	// Kind is KindKeydown, KindKeyup, KindKeypress or KindJoystick.
	Kind uint8
	// Key is one of the Key* ordinals. Meaningless on a KindJoystick record.
	Key uint8
	// Player is the pairing slot: who is player one or two in the game.
	Player Slot
	// Joy is JoyCenter or one of JoyN..JoyNW. Set only on KindJoystick
	// records.
	Joy uint8
	// Source is SourceKeyboard or SourceController.
	Source uint8
	// Physical is the cabinet seat this input came from: which LCD and
	// button-light bank sit in front of this human. It is [SeatNone] for a
	// source with no seat, such as a keyboard, the browser or a microphone.
	// Player is who they are in the game; Physical is where they sit. Address
	// LCDs and lamps by Physical and game logic by Player.
	Physical Seat
	// Index is the controller slot, or [IndexNone] for a source that is not
	// a controller.
	Index uint8
	// Flags is a bitmask of FlagVirtual and FlagShift.
	Flags uint8
}

// IndexNone is the [Record.Index] of a source that is not a controller: a
// keyboard, the browser or the microphone.
const IndexNone uint8 = 0xFF

// Record kinds, byte 0.
const (
	KindKeydown uint8 = 0
	KindKeyup   uint8 = 1
	// KindKeypress is the canonical "button pressed" record. Every physical
	// press also sends a keydown, so act on the keypress alone.
	KindKeypress uint8 = 2
	KindJoystick uint8 = 3
)

// Source types, byte 4.
const (
	SourceKeyboard   uint8 = 0
	SourceController uint8 = 1
)

// Flag bits, byte 7.
const (
	// FlagVirtual marks a keyboard or keyboard-driven virtual stick, as
	// opposed to real HID hardware. A real joystick push emits a KindJoystick
	// record and a non-virtual direction keypress; [Record.PressDir] uses this
	// flag to count the push once.
	FlagVirtual uint8 = 1 << 0
	// FlagShift reports whether shift was held.
	FlagShift uint8 = 1 << 1
)

// Joystick position ordinals, byte 3: centre, then the eight compass points
// clockwise from north.
const (
	JoyCenter uint8 = 0
	JoyN      uint8 = 1
	JoyNE     uint8 = 2
	JoyE      uint8 = 3
	JoySE     uint8 = 4
	JoyS      uint8 = 5
	JoySW     uint8 = 6
	JoyW      uint8 = 7
	JoyNW     uint8 = 8
)

// Key ordinals, byte 1. The numbering is the wire format; new keys are only
// appended.
const (
	KeyBrightness uint8 = 0
	KeyRotate     uint8 = 1
	KeyQuit       uint8 = 2
	KeyEscape     uint8 = 3
	KeyEnter      uint8 = 4
	KeySpace      uint8 = 5
	KeyUp         uint8 = 6
	KeyDown       uint8 = 7
	KeyLeft       uint8 = 8
	KeyRight      uint8 = 9
	KeyHome       uint8 = 10
	KeyEnd        uint8 = 11
	KeyJoyA       uint8 = 12
	KeyJoyB       uint8 = 13
	KeyJoyC       uint8 = 14
	KeyJoyD       uint8 = 15
	KeyL1         uint8 = 16
	KeyR1         uint8 = 17
	KeyL2         uint8 = 18
	KeyR2         uint8 = 19
	KeyJoyAux1    uint8 = 20
	KeyJoyAux2    uint8 = 21
	KeyJoyAux3    uint8 = 22
	KeyJoyAux4    uint8 = 23
	KeySelect     uint8 = 24
	KeyStart      uint8 = 25
	KeyTurbo      uint8 = 26
	KeyClear      uint8 = 27

	// KeyMicWhistle, KeyMicClap and KeyMicBlow are microphone gestures
	// delivered as KindKeypress records. No audio crosses the boundary, only
	// the detected gesture.
	//
	// They arrive with Player [SlotP1], Physical [SeatNone], Source
	// SourceKeyboard and FlagVirtual set, because one microphone cannot tell
	// which seat made the sound; a two-player game routes the event to
	// whoever holds the turn. They are withheld unless the manifest declares
	// the "mic" capability. No cabinet emits KeyMicClap or KeyMicBlow.
	KeyMicWhistle uint8 = 28
	KeyMicClap    uint8 = 29
	KeyMicBlow    uint8 = 30

	// KeyMicCue0 through KeyMicCue7 are the cues of the sound model the
	// cartridge bundles (docs/ASSETS.md, "mic"; sdk/docs/reference-mic.md).
	// Cue i is the class of that model that names cue i; its meaning belongs
	// to the cartridge. Delivery and seat rules match KeyMicWhistle. With no
	// microphone, no cue is ever delivered and no error is raised.
	KeyMicCue0 uint8 = 31
	KeyMicCue1 uint8 = 32
	KeyMicCue2 uint8 = 33
	KeyMicCue3 uint8 = 34
	KeyMicCue4 uint8 = 35
	KeyMicCue5 uint8 = 36
	KeyMicCue6 uint8 = 37
	KeyMicCue7 uint8 = 38
)

// Seat is a physical cabinet position: the LCD, button lamps and controller
// port in front of one human. Records report it in [Record.Physical], and
// every peripheral call (LCD text, lamps, recent names) takes one.
type Seat uint8

// The Seat values: the two physical seats, plus the two non-seat sentinels.
const (
	// SeatP1 and SeatP2 are the two seats.
	SeatP1 Seat = 0
	SeatP2 Seat = 1
	// SeatBoth addresses both seats' hardware at once. It is an argument to
	// peripheral calls only and never appears in a record. Calls that name
	// one specific seat (recent names, LockPlayerToSeat) treat it as
	// [SeatNone].
	SeatBoth Seat = 0xFE
	// SeatNone is "no seat", what a record from a keyboard, the browser or
	// the microphone carries. Peripheral calls given SeatNone do nothing, so
	// a record's Physical can be passed straight through.
	SeatNone Seat = 0xFF
)

// Single reports whether s names exactly one seat ([SeatP1] or [SeatP2]).
func (s Seat) Single() bool { return s == SeatP1 || s == SeatP2 }

// Other returns the opposite seat for [SeatP1] or [SeatP2], and s unchanged for
// anything else.
func (s Seat) Other() Seat {
	switch s {
	case SeatP1:
		return SeatP2
	case SeatP2:
		return SeatP1
	}
	return s
}

// SeatOf converts a zero-based seat index to a Seat: 0 is [SeatP1], 1 is
// [SeatP2], and every other value, including -1, is [SeatNone]. A cartridge
// that means both seats writes [SeatBoth]. Prefer SeatOf to a non-constant Seat(v)
// conversion, which wraps an out-of-range int to an arbitrary byte.
func SeatOf(v int) Seat {
	switch v {
	case 0:
		return SeatP1
	case 1:
		return SeatP2
	}
	return SeatNone
}

// Index returns 0 for [SeatP1], 1 for [SeatP2] and -1 for anything else. It is
// the inverse of [SeatOf] for a single seat; check the result, or
// [Seat.Single], before indexing a per-seat [2]T array.
func (s Seat) Index() int {
	switch s {
	case SeatP1:
		return 0
	case SeatP2:
		return 1
	}
	return -1
}

// Slot is a pairing slot: whether a record's human is player one or player
// two in the game, independent of where they sit. See [Record.Player].
type Slot uint8

// The Slot values. Slots are one-based, so they do not match [Seat] ordinals.
const (
	// SlotNone is the zero value of an unset Slot. The host never emits it:
	// keyboard and system records read as SlotP1.
	SlotNone Slot = 0
	// SlotP1 is player one.
	SlotP1 Slot = 1
	// SlotP2 is player two.
	SlotP2 Slot = 2
)

// SlotOf converts a zero-based player index to a Slot: 0 is [SlotP1], 1 is
// [SlotP2], and every other value, including -1, is [SlotNone]. Slots are
// one-based, so Slot(v) of a zero-based index is off by one; use SlotOf.
func SlotOf(v int) Slot {
	switch v {
	case 0:
		return SlotP1
	case 1:
		return SlotP2
	}
	return SlotNone
}

// Index returns 0 for [SlotP1], 1 for [SlotP2] and -1 for anything else. It is
// the inverse of [SlotOf] for a real slot; check the result before indexing a
// per-player [2]T array.
func (s Slot) Index() int {
	switch s {
	case SlotP1:
		return 0
	case SlotP2:
		return 1
	}
	return -1
}

// Direction is a d-pad intent, the navigation vocabulary of the sdk/ui
// widgets. [DirNone] is the zero value.
type Direction uint8

// The Direction values: no direction, then the four d-pad directions.
const (
	DirNone Direction = iota
	DirUp
	DirDown
	DirLeft
	DirRight
)

// Horizontal reports whether d is [DirLeft] or [DirRight].
func (d Direction) Horizontal() bool { return d == DirLeft || d == DirRight }

// Vertical reports whether d is [DirUp] or [DirDown].
func (d Direction) Vertical() bool { return d == DirUp || d == DirDown }
