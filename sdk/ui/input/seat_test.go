// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "testing"

func TestSeatOf(t *testing.T) {
	cases := []struct {
		v    int
		want Seat
	}{
		{0, SeatP1},
		{1, SeatP2},
		{-1, SeatNone},
		{2, SeatNone},
		{0xFE, SeatNone},
		{0xFF, SeatNone},
		{256, SeatNone},
	}
	for _, c := range cases {
		if got := SeatOf(c.v); got != c.want {
			t.Errorf("SeatOf(%d) = %#x, want %#x", c.v, got, c.want)
		}
	}
}

func TestSeatIndex(t *testing.T) {
	cases := []struct {
		s    Seat
		want int
	}{
		{SeatP1, 0},
		{SeatP2, 1},
		{SeatBoth, -1},
		{SeatNone, -1},
		{Seat(2), -1},
	}
	for _, c := range cases {
		if got := c.s.Index(); got != c.want {
			t.Errorf("Seat(%#x).Index() = %d, want %d", c.s, got, c.want)
		}
	}
	for i := 0; i < 2; i++ {
		if got := SeatOf(i).Index(); got != i {
			t.Errorf("SeatOf(%d).Index() = %d", i, got)
		}
	}
}

func TestSlotOf(t *testing.T) {
	cases := []struct {
		v    int
		want Slot
	}{
		{0, SlotP1},
		{1, SlotP2},
		{-1, SlotNone},
		{2, SlotNone},
		{256, SlotNone},
	}
	for _, c := range cases {
		if got := SlotOf(c.v); got != c.want {
			t.Errorf("SlotOf(%d) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestSlotIndex(t *testing.T) {
	cases := []struct {
		s    Slot
		want int
	}{
		{SlotP1, 0},
		{SlotP2, 1},
		{SlotNone, -1},
		{Slot(3), -1},
	}
	for _, c := range cases {
		if got := c.s.Index(); got != c.want {
			t.Errorf("Slot(%d).Index() = %d, want %d", c.s, got, c.want)
		}
	}
	for i := 0; i < 2; i++ {
		if got := SlotOf(i).Index(); got != i {
			t.Errorf("SlotOf(%d).Index() = %d", i, got)
		}
	}
}
