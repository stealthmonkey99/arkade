// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
)

func TestEventRecordCarriesTheSeatAndSlotBytes(t *testing.T) {
	press := func(src input.PlayerSource) input.Event {
		return input.Event{Kind: input.KindKeypress, Name: input.KeyJoyA, Source: src}
	}
	for _, tc := range []struct {
		name                    string
		src                     input.PlayerSource
		player, physical, index uint8
		source                  uint8
	}{
		{"plain keyboard", input.PlayerSource{Type: input.SourceKeyboard, ID: "keyboard"}, 1, SeatNone, SeatNone, RecordSourceKeyboard},
		{"microphone", input.PlayerSource{Type: input.SourceKeyboard, ID: "mic"}, 1, SeatNone, SeatNone, RecordSourceKeyboard},
		{"first stick", input.PlayerSource{Type: input.SourceController, Slot: input.SlotOf(0), Seat: input.SeatP1}, 1, 0, 0, RecordSourceController},
		{"second stick", input.PlayerSource{Type: input.SourceController, Slot: input.SlotOf(1), Seat: input.SeatP2}, 2, 1, 1, RecordSourceController},
		{"third pad claiming P1", input.PlayerSource{Type: input.SourceController, Slot: input.SlotOf(2), Seat: input.SeatP1}, 1, 0, 2, RecordSourceController},
		{"third pad unseated", input.PlayerSource{Type: input.SourceController, Slot: input.SlotOf(2)}, 1, SeatNone, 2, RecordSourceController},
		{"virtual keyboard stick", input.PlayerSource{Type: input.SourceController, ID: "virtual-1", Slot: input.SlotOf(1), Seat: input.SeatP2}, 2, 1, 1, RecordSourceController},
	} {
		rec, ok := eventToRecord(press(tc.src))
		if !ok {
			t.Fatalf("%s: keypress not modelled", tc.name)
		}
		if rec.Player != tc.player || rec.Physical != tc.physical || rec.Index != tc.index || rec.Source != tc.source {
			t.Errorf("%s: player %d physical %#x index %#x source %d, want %d %#x %#x %d",
				tc.name, rec.Player, rec.Physical, rec.Index, rec.Source, tc.player, tc.physical, tc.index, tc.source)
		}
	}
}
