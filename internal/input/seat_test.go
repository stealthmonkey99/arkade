// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "testing"

func TestSeatOfRoundTripsTheTwoSeatsAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		index  int
		seat   Seat
		valid  bool
		number int
		abi    uint8
	}{
		{0, SeatP1, true, 1, 0},
		{1, SeatP2, true, 2, 1},
		{-1, NoSeat, false, 0, 0xFF},
		{2, NoSeat, false, 0, 0xFF},
		{99, NoSeat, false, 0, 0xFF},
	} {
		s := SeatOf(tc.index)
		if s != tc.seat || s.Valid() != tc.valid || s.Number() != tc.number || s.Byte() != tc.abi {
			t.Errorf("SeatOf(%d) = %v (valid %v, number %d, byte %#x), want %v (%v, %d, %#x)",
				tc.index, s, s.Valid(), s.Number(), s.Byte(), tc.seat, tc.valid, tc.number, tc.abi)
		}
		if tc.valid && s.Index() != tc.index {
			t.Errorf("SeatOf(%d).Index() = %d", tc.index, s.Index())
		}
	}
	if NoSeat.Index() != -1 {
		t.Errorf("NoSeat.Index() = %d, want -1", NoSeat.Index())
	}
	var zero Seat
	if zero != NoSeat {
		t.Error("the zero Seat is not NoSeat")
	}
}

func TestSlotOfRoundTripsAttachIndices(t *testing.T) {
	for _, index := range []int{0, 1, 2, 7, 254} {
		s := SlotOf(index)
		if s == NoSlot || s.Index() != index || s.Byte() != uint8(index) {
			t.Errorf("SlotOf(%d) = %v, Index %d, Byte %#x", index, s, s.Index(), s.Byte())
		}
	}
	if SlotOf(-1) != NoSlot || NoSlot.Index() != -1 || NoSlot.Byte() != 0xFF {
		t.Errorf("NoSlot: %v, Index %d, Byte %#x", SlotOf(-1), NoSlot.Index(), NoSlot.Byte())
	}
	if SlotOf(255).Byte() != 0xFF || SlotOf(1000).Byte() != 0xFF {
		t.Error("an attach index the ABI byte cannot hold must read as none, not wrap")
	}
	var zero Slot
	if zero != NoSlot {
		t.Error("the zero Slot is not NoSlot")
	}
}

func TestZeroPlayerSourceIsSeatless(t *testing.T) {
	src := PlayerSource{Type: SourceKeyboard}
	if src.Seat != NoSeat || src.Slot != NoSlot || src.Seat.Byte() != 0xFF || src.Slot.Byte() != 0xFF {
		t.Errorf("a keyboard source carries %v/%v", src.Seat, src.Slot)
	}
}

func TestSeatOfByteInvertsByte(t *testing.T) {
	for _, seat := range []Seat{SeatP1, SeatP2, NoSeat} {
		if got := SeatOfByte(seat.Byte()); got != seat {
			t.Errorf("SeatOfByte(%v.Byte()) = %v", seat, got)
		}
	}
	for _, b := range []uint8{2, 3, 0xFE} {
		if got := SeatOfByte(b); got != NoSeat {
			t.Errorf("SeatOfByte(%#x) = %v, want NoSeat", b, got)
		}
	}
}

func TestSeatsAddressOnePanelOrBoth(t *testing.T) {
	for _, tc := range []struct {
		seats Seats
		index int
		ok    bool
	}{
		{BothSeats, -1, true},
		{SeatsOf(SeatP1), 0, true},
		{SeatsOf(SeatP2), 1, true},
		{SeatsOf(SeatP1, SeatP2), -1, true},
		{SeatsOf(NoSeat), 0, false},
		{NoSeats, 0, false},
	} {
		index, ok := tc.seats.Index()
		if index != tc.index || ok != tc.ok {
			t.Errorf("%v.Index() = %d, %v; want %d, %v", tc.seats, index, ok, tc.index, tc.ok)
		}
	}
	if !BothSeats.Has(SeatP1) || !BothSeats.Has(SeatP2) || BothSeats.Has(NoSeat) {
		t.Error("BothSeats must hold exactly the two seats")
	}
	if one := SeatsOf(SeatP2); one.Has(SeatP1) || !one.Has(SeatP2) {
		t.Error("SeatsOf(SeatP2) must hold P2 alone")
	}
	var zero Seats
	if zero != NoSeats {
		t.Error("the zero Seats is not NoSeats")
	}
}
