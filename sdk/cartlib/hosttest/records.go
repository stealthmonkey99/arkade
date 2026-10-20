// SPDX-License-Identifier: GPL-2.0-or-later

package hosttest

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// Encode packs records into cart_handle_input's wire format, eight bytes
// each in the order docs/CARTRIDGE.md gives: kind, key, player, joy, source,
// physical seat, controller index, flags.
func Encode(records []cartlib.InputRecord) []byte {
	wire := make([]byte, 8*len(records))
	for i, r := range records {
		o := 8 * i
		wire[o], wire[o+1], wire[o+2], wire[o+3] = r.Kind, r.Key, uint8(r.Player), r.Joy
		wire[o+4], wire[o+5], wire[o+6], wire[o+7] = r.Source, uint8(r.Physical), r.Index, r.Flags
	}
	return wire
}

// controller is the record shape of the cabinet's own panel at seat: the
// seat is paired as the slot of the same number and is controller index 0
// or 1. SeatBoth and SeatNone give a seatless keyboard-style record.
func controller(seat cartlib.Seat, kind, key, joy uint8) cartlib.InputRecord {
	if !seat.Single() {
		return cartlib.InputRecord{Kind: kind, Key: key, Joy: joy, Player: cartlib.SlotP1,
			Source: cartlib.SourceKeyboard, Physical: cartlib.SeatNone, Index: cartlib.IndexNone, Flags: cartlib.FlagVirtual}
	}
	return cartlib.InputRecord{Kind: kind, Key: key, Joy: joy, Player: cartlib.SlotOf(seat.Index()),
		Source: cartlib.SourceController, Physical: seat, Index: uint8(seat.Index())}
}

// Press is a panel button press at seat as the cabinet sends it: a keydown
// followed by a keypress. Feed both to [Host.Input] (`h.Input(Press(...)...)`)
// so hold trackers see the same stream the cabinet produces.
func Press(seat cartlib.Seat, key uint8) []cartlib.InputRecord {
	return []cartlib.InputRecord{
		controller(seat, cartlib.KindKeydown, key, cartlib.JoyCenter),
		controller(seat, cartlib.KindKeypress, key, cartlib.JoyCenter),
	}
}

// Release is the keyup that ends a [Press].
func Release(seat cartlib.Seat, key uint8) cartlib.InputRecord {
	return controller(seat, cartlib.KindKeyup, key, cartlib.JoyCenter)
}

// Stick is the joystick record for seat's stick moving to joy (JoyCenter
// for its return). A cabinet stick also sends a non-virtual arrow keypress
// for a cardinal push; [Host.Input] callers that drive widgets through
// [cartlib.InputRecord.PressDir] need only this record.
func Stick(seat cartlib.Seat, joy uint8) cartlib.InputRecord {
	return controller(seat, cartlib.KindJoystick, 0, joy)
}

// KeyboardPress is the dev console's keypress: virtual, seatless, slot one,
// with no keydown or keyup around it.
func KeyboardPress(key uint8) cartlib.InputRecord {
	return controller(cartlib.SeatNone, cartlib.KindKeypress, key, cartlib.JoyCenter)
}
