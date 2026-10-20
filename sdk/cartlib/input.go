// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// InputRecord is one decoded 8-byte input record (docs/CARTRIDGE.md, "Input
// records"). It is [input.Record], re-exported with the constants below so
// cartridge code needs only cartlib while sdk/ui widgets take the same
// type. Prefer its decodes ([input.Record.IsPress], [input.Record.IsQuit],
// [input.Record.Dir], [input.Record.PressDir], [input.Record.Virtual]) to
// hand-written switches.
type InputRecord = input.Record

// Seat is a physical cabinet position: [SeatP1], [SeatP2], [SeatBoth] or
// [SeatNone]. See [input.Seat].
type Seat = input.Seat

// Slot is a pairing slot: [SlotP1], [SlotP2] or [SlotNone]. See
// [input.Slot].
type Slot = input.Slot

// Direction is what [InputRecord.Dir] and [InputRecord.PressDir] return:
// [DirNone] or one of the four. It is the type sdk/ui's widgets take.
type Direction = input.Direction

// The four directions, and DirNone, the zero value.
const (
	DirNone  = input.DirNone
	DirUp    = input.DirUp
	DirDown  = input.DirDown
	DirLeft  = input.DirLeft
	DirRight = input.DirRight
)

// Seats. SeatBoth is a peripheral-call argument only; SeatNone is what a
// seatless record carries, and every peripheral call ignores it.
const (
	SeatP1   = input.SeatP1
	SeatP2   = input.SeatP2
	SeatBoth = input.SeatBoth
	SeatNone = input.SeatNone
)

// Pairing slots, as carried in [InputRecord].Player. The host never emits
// SlotNone: keyboard and system records read as SlotP1.
const (
	SlotNone = input.SlotNone
	SlotP1   = input.SlotP1
	SlotP2   = input.SlotP2
)

// IndexNone is [InputRecord].Index for a source that is not a controller.
const IndexNone = input.IndexNone

// Source types (record byte 4).
const (
	SourceKeyboard   = input.SourceKeyboard
	SourceController = input.SourceController
)

// Record flag bits (byte 7). See [input.FlagVirtual].
const (
	FlagVirtual = input.FlagVirtual
	FlagShift   = input.FlagShift
)

// Record kinds (byte 0). Act on KindKeypress: the cabinet sends a keydown
// and a keypress for every physical press.
const (
	KindKeydown  = input.KindKeydown
	KindKeyup    = input.KindKeyup
	KindKeypress = input.KindKeypress
	KindJoystick = input.KindJoystick
)

// Joystick position ordinals (byte 3): centre, then N, NE, E, SE, S, SW, W,
// NW.
const (
	JoyCenter = input.JoyCenter
	JoyN      = input.JoyN
	JoyNE     = input.JoyNE
	JoyE      = input.JoyE
	JoySE     = input.JoySE
	JoyS      = input.JoyS
	JoySW     = input.JoySW
	JoyW      = input.JoyW
	JoyNW     = input.JoyNW
)

// Key ordinals (byte 1). See the input package for the microphone keys'
// delivery rules ([input.KeyMicWhistle], [input.KeyMicCue0]).
const (
	KeyBrightness = input.KeyBrightness
	KeyRotate     = input.KeyRotate
	KeyQuit       = input.KeyQuit
	KeyEscape     = input.KeyEscape
	KeyEnter      = input.KeyEnter
	KeySpace      = input.KeySpace
	KeyUp         = input.KeyUp
	KeyDown       = input.KeyDown
	KeyLeft       = input.KeyLeft
	KeyRight      = input.KeyRight
	KeyHome       = input.KeyHome
	KeyEnd        = input.KeyEnd
	KeyJoyA       = input.KeyJoyA
	KeyJoyB       = input.KeyJoyB
	KeyJoyC       = input.KeyJoyC
	KeyJoyD       = input.KeyJoyD
	KeyL1         = input.KeyL1
	KeyR1         = input.KeyR1
	KeyL2         = input.KeyL2
	KeyR2         = input.KeyR2
	KeyJoyAux1    = input.KeyJoyAux1
	KeyJoyAux2    = input.KeyJoyAux2
	KeyJoyAux3    = input.KeyJoyAux3
	KeyJoyAux4    = input.KeyJoyAux4
	KeySelect     = input.KeySelect
	KeyStart      = input.KeyStart
	KeyTurbo      = input.KeyTurbo
	KeyClear      = input.KeyClear

	KeyMicWhistle = input.KeyMicWhistle
	KeyMicClap    = input.KeyMicClap
	KeyMicBlow    = input.KeyMicBlow

	KeyMicCue0 = input.KeyMicCue0
	KeyMicCue1 = input.KeyMicCue1
	KeyMicCue2 = input.KeyMicCue2
	KeyMicCue3 = input.KeyMicCue3
	KeyMicCue4 = input.KeyMicCue4
	KeyMicCue5 = input.KeyMicCue5
	KeyMicCue6 = input.KeyMicCue6
	KeyMicCue7 = input.KeyMicCue7
)

// ButtonMask names a set of one seat's six arcade-button lamps, one bit per
// lamp, for [SetButtonLights], [SetLampLevels] and [FadeLamps]. Only the low
// six bits mean anything.
type ButtonMask uint32

// The six lamp bits, in docs/CARTRIDGE.md's mask table order.
const (
	MaskJoyA    ButtonMask = 1 << 0
	MaskJoyB    ButtonMask = 1 << 1
	MaskJoyC    ButtonMask = 1 << 2
	MaskJoyD    ButtonMask = 1 << 3
	MaskJoyAux1 ButtonMask = 1 << 4
	MaskJoyAux2 ButtonMask = 1 << 5
)

// maxDecodedRecords is the largest cart_handle_input batch either host
// sends.
const maxDecodedRecords = 1024

// DecodeInputRecordsInto decodes buf's whole 8-byte records into out and
// returns how many it wrote, min(len(buf)/8, len(out)); trailing bytes are
// ignored. Allocation-free.
func DecodeInputRecordsInto(buf []byte, out []InputRecord) int {
	n := len(buf) / 8
	if n > len(out) {
		n = len(out)
	}
	for i := 0; i < n; i++ {
		o := i * 8
		out[i] = InputRecord{
			Kind: buf[o], Key: buf[o+1], Player: Slot(buf[o+2]), Joy: buf[o+3],
			Source: buf[o+4], Physical: Seat(buf[o+5]), Index: buf[o+6], Flags: buf[o+7],
		}
	}
	return n
}

// DecodeInputRecords is [DecodeInputRecordsInto] into a new slice, for
// tests and tooling. It allocates, so never call it after Init.
func DecodeInputRecords(buf []byte) []InputRecord {
	out := make([]InputRecord, len(buf)/8)
	DecodeInputRecordsInto(buf, out)
	return out
}

func quitPressed(records []InputRecord) bool {
	for i := range records {
		if records[i].IsPress() && records[i].IsQuit() {
			return true
		}
	}
	return false
}
