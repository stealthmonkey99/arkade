// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// InputRecord is one 8-byte input record. Player is who the source is in the
// game, 1 or 2; Physical is the seat, which picks the LCD and lamps; Index is
// the controller's attach slot. Source.ID is deliberately not carried, to
// keep the fixed 8-byte stride.
type InputRecord struct {
	Kind   uint8 // 0 keydown, 1 keyup, 2 keypress, 3 joystick
	Key    uint8 // ordinal into keyOrdinal, see KeyToOrdinal
	Player uint8 // 1 or 2: the seat's number, and 1 for a seatless source
	Joy    uint8 // 0 center, 1..8 = N,NE,E,SE,S,SW,W,NW

	Source   uint8 // 0 keyboard, 1 controller (input.SourceType)
	Physical uint8 // input.Seat.Byte(): the seat, or SeatNone
	Index    uint8 // input.Slot.Byte(): the attach slot, or SeatNone
	Flags    uint8 // bit 0 = virtual source, bit 1 = shift held
}

const (
	RecordKindKeydown        uint8 = 0
	RecordKindKeyup          uint8 = 1
	RecordKindKeypress       uint8 = 2
	RecordKindJoystickChange uint8 = 3
)

const (
	RecordSourceKeyboard   uint8 = 0
	RecordSourceController uint8 = 1
)

const (
	// RecordFlagVirtual marks a keyboard or virtual stick. A real stick emits
	// both a joystickchange and a direction keypress per push, so a cartridge
	// acting on both must gate on this or double-step.
	RecordFlagVirtual uint8 = 1 << 0
	RecordFlagShift   uint8 = 1 << 1
)

// SeatNone is Physical or Index for a source with no seat, a plain keyboard
// included. 0xFF because 0 is a real seat.
const SeatNone uint8 = 0xFF

func PackRecords(records []InputRecord) []byte {
	buf := make([]byte, len(records)*8)
	for i, r := range records {
		o := i * 8
		buf[o] = r.Kind
		buf[o+1] = r.Key
		buf[o+2] = r.Player
		buf[o+3] = r.Joy
		buf[o+4] = r.Source
		buf[o+5] = r.Physical
		buf[o+6] = r.Index
		buf[o+7] = r.Flags
	}
	return buf
}

// PackRecord returns PackRecords' eight bytes for one record as a
// little-endian uint64, the flight recorder's event payload.
func PackRecord(r InputRecord) uint64 {
	return uint64(r.Kind) |
		uint64(r.Key)<<8 |
		uint64(r.Player)<<16 |
		uint64(r.Joy)<<24 |
		uint64(r.Source)<<32 |
		uint64(r.Physical)<<40 |
		uint64(r.Index)<<48 |
		uint64(r.Flags)<<56
}

func UnpackRecord(w uint64) InputRecord {
	return InputRecord{
		Kind: uint8(w), Key: uint8(w >> 8), Player: uint8(w >> 16), Joy: uint8(w >> 24),
		Source: uint8(w >> 32), Physical: uint8(w >> 40), Index: uint8(w >> 48), Flags: uint8(w >> 56),
	}
}

func UnpackRecords(buf []byte) []InputRecord {
	n := len(buf) / 8
	out := make([]InputRecord, n)
	for i := 0; i < n; i++ {
		o := i * 8
		out[i] = InputRecord{
			Kind: buf[o], Key: buf[o+1], Player: buf[o+2], Joy: buf[o+3],
			Source: buf[o+4], Physical: buf[o+5], Index: buf[o+6], Flags: buf[o+7],
		}
	}
	return out
}

// keyOrdinal is the record's key byte. It and the other wire tables reach
// the browser host through testdata/hostabi.json (hostabi_table_test.go).
var keyOrdinal = []input.KeyName{
	input.KeyBrightness, // 0
	input.KeyRotate,     // 1
	input.KeyQuit,       // 2
	input.KeyEscape,     // 3
	input.KeyEnter,      // 4
	input.KeySpace,      // 5
	input.KeyUp,         // 6
	input.KeyDown,       // 7
	input.KeyLeft,       // 8
	input.KeyRight,      // 9
	input.KeyHome,       // 10
	input.KeyEnd,        // 11
	input.KeyJoyA,       // 12
	input.KeyJoyB,       // 13
	input.KeyJoyC,       // 14
	input.KeyJoyD,       // 15
	input.KeyL1,         // 16
	input.KeyR1,         // 17
	input.KeyL2,         // 18
	input.KeyR2,         // 19
	input.KeyJoyAux1,    // 20
	input.KeyJoyAux2,    // 21
	input.KeyJoyAux3,    // 22
	input.KeyJoyAux4,    // 23
	input.KeySelect,     // 24
	input.KeyStart,      // 25
	input.KeyTurbo,      // 26
	input.KeyClear,      // 27

	// Append only: renumbering would change what every built .wasm reads.
	input.KeyMicWhistle, // 28
	input.KeyMicClap,    // 29
	input.KeyMicBlow,    // 30

	input.KeyMicCue0, // 31
	input.KeyMicCue1, // 32
	input.KeyMicCue2, // 33
	input.KeyMicCue3, // 34
	input.KeyMicCue4, // 35
	input.KeyMicCue5, // 36
	input.KeyMicCue6, // 37
	input.KeyMicCue7, // 38
}

func KeyToOrdinal(k input.KeyName) (uint8, bool) {
	for i, n := range keyOrdinal {
		if n == k {
			return uint8(i), true
		}
	}
	return 0, false
}

func OrdinalToKey(o uint8) (input.KeyName, bool) {
	if int(o) < len(keyOrdinal) {
		return keyOrdinal[o], true
	}
	return "", false
}

// joyOrdinal is the joy byte: 0 center, then 1..8.
var joyOrdinal = []input.JoystickPosition{
	input.JoyUp,        // 1 = N
	input.JoyUpRight,   // 2 = NE
	input.JoyRight,     // 3 = E
	input.JoyDownRight, // 4 = SE
	input.JoyDown,      // 5 = S
	input.JoyDownLeft,  // 6 = SW
	input.JoyLeft,      // 7 = W
	input.JoyUpLeft,    // 8 = NW
}

// buttonOrdinal names host_set_button_lights' mask bit i.
//
//	bit 0 = "joya"     bit 1 = "joyb"     bit 2 = "joyc"
//	bit 3 = "joyd"     bit 4 = "joyaux1"  bit 5 = "joyaux2"
var buttonOrdinal = hardware.LightButtons

func joyToOrdinal(p input.JoystickPosition) uint8 {
	for i, j := range joyOrdinal {
		if j == p {
			return uint8(i + 1)
		}
	}
	return 0
}

// EventRecords converts engine input events into the records a cartridge
// receives, in order, dropping any event the ABI does not model.
func EventRecords(events []input.Event) []InputRecord {
	records := make([]InputRecord, 0, len(events))
	for _, ev := range events {
		if rec, ok := eventToRecord(ev); ok {
			records = append(records, rec)
		}
	}
	return records
}

// eventToRecord converts an engine event, ok=false for a kind the ABI does
// not model. Player is never 0: a keyboard or system event reads as player 1.
func eventToRecord(ev input.Event) (InputRecord, bool) {
	var kind uint8
	switch ev.Kind {
	case input.KindKeydown:
		kind = RecordKindKeydown
	case input.KindKeyup:
		kind = RecordKindKeyup
	case input.KindKeypress:
		kind = RecordKindKeypress
	case input.KindJoystickChange:
		kind = RecordKindJoystickChange
	default:
		return InputRecord{}, false
	}

	key, _ := KeyToOrdinal(ev.Name) // 0 when unknown; joystick events carry no Name

	player := uint8(1)
	if ev.Source.Seat == input.SeatP2 {
		player = 2
	}

	var joy uint8
	if ev.Kind == input.KindJoystickChange {
		joy = joyToOrdinal(ev.Position)
	}

	source := RecordSourceKeyboard
	if ev.Source.Type == input.SourceController {
		source = RecordSourceController
	}

	var flags uint8
	if ev.Source.Virtual() {
		flags |= RecordFlagVirtual
	}
	if ev.Shift {
		flags |= RecordFlagShift
	}

	return InputRecord{
		Kind:     kind,
		Key:      key,
		Player:   player,
		Joy:      joy,
		Source:   source,
		Physical: ev.Source.Seat.Byte(),
		Index:    ev.Source.Slot.Byte(),
		Flags:    flags,
	}, true
}
