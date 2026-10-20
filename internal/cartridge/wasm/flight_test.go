// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

func TestPackRecordRoundTrip(t *testing.T) {
	cases := []wasm.InputRecord{
		{},
		{Kind: wasm.RecordKindKeypress, Key: 9, Player: 1, Source: wasm.RecordSourceController, Physical: 0, Index: 0},
		{Kind: wasm.RecordKindJoystickChange, Joy: 5, Player: 2, Source: wasm.RecordSourceController, Physical: 1, Index: 1},
		{Kind: wasm.RecordKindKeyup, Key: 20, Player: 1, Source: wasm.RecordSourceKeyboard,
			Physical: wasm.SeatNone, Index: wasm.SeatNone, Flags: wasm.RecordFlagVirtual | wasm.RecordFlagShift},
		// Every byte distinct, so a swapped pair of fields cannot pass.
		{Kind: 1, Key: 2, Player: 3, Joy: 4, Source: 5, Physical: 6, Index: 7, Flags: 8},
	}
	for _, rec := range cases {
		if got := wasm.UnpackRecord(wasm.PackRecord(rec)); got != rec {
			t.Errorf("round trip of %+v gave %+v", rec, got)
		}
	}
}

func TestPackRecordMatchesTheWireBytes(t *testing.T) {
	rec := wasm.InputRecord{Kind: 1, Key: 2, Player: 3, Joy: 4, Source: 5, Physical: 6, Index: 7, Flags: 8}
	wire := wasm.PackRecords([]wasm.InputRecord{rec})
	packed := wasm.PackRecord(rec)
	for i, want := range wire {
		if got := uint8(packed >> (8 * i)); got != want {
			t.Errorf("byte %d: PackRecord says %d, the wire says %d", i, got, want)
		}
	}
}
