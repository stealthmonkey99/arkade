// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "testing"

func TestSeatOfReexport(t *testing.T) {
	if SeatOf(0) != SeatP1 || SeatOf(1) != SeatP2 || SeatOf(-1) != SeatNone {
		t.Errorf("SeatOf(0,1,-1) = %#x,%#x,%#x", SeatOf(0), SeatOf(1), SeatOf(-1))
	}
}

func TestSlotOfReexport(t *testing.T) {
	if SlotOf(0) != SlotP1 || SlotOf(1) != SlotP2 || SlotOf(-1) != SlotNone {
		t.Errorf("SlotOf(0,1,-1) = %d,%d,%d", SlotOf(0), SlotOf(1), SlotOf(-1))
	}
}
