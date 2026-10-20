// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "fmt"

// Seat is one of the cabinet's two player positions: the LCD, lamps and
// controller port in front of one human. The zero value is NoSeat, so a
// source that never names a seat (a keyboard, the microphone, a third pad)
// has none.
type Seat int8

const (
	NoSeat Seat = 0
	SeatP1 Seat = 1
	SeatP2 Seat = 2
)

const SeatCount = 2

// SeatOf maps a zero-based seat index to a Seat: 0 is SeatP1, 1 is SeatP2,
// anything else is NoSeat. It is the only way to build a Seat from a number.
func SeatOf(index int) Seat {
	if index < 0 || index >= SeatCount {
		return NoSeat
	}
	return Seat(index + 1)
}

func (s Seat) Valid() bool { return s == SeatP1 || s == SeatP2 }

// Index is the zero-based seat index, the inverse of SeatOf: 0, 1, or -1 for
// NoSeat. Check Valid before indexing a per-seat array with it.
func (s Seat) Index() int {
	if !s.Valid() {
		return -1
	}
	return int(s) - 1
}

func (s Seat) Number() int {
	if !s.Valid() {
		return 0
	}
	return int(s)
}

// Byte is the seat as the cartridge ABI's input record carries it: 0 for
// SeatP1, 1 for SeatP2 and 0xFF for NoSeat.
func (s Seat) Byte() uint8 {
	if !s.Valid() {
		return 0xFF
	}
	return uint8(s.Index())
}

// SeatOfByte is Byte's inverse: 0 is SeatP1, 1 is SeatP2 and any other
// value, 0xFF included, is NoSeat.
func SeatOfByte(b uint8) Seat {
	if b >= SeatCount {
		return NoSeat
	}
	return Seat(b + 1)
}

func (s Seat) String() string {
	if !s.Valid() {
		return "no seat"
	}
	return fmt.Sprintf("P%d", s.Number())
}

// Seats is a set of seats: the address of a peripheral write, which lands on
// one panel or both. The zero value addresses nothing.
type Seats uint8

const (
	NoSeats   Seats = 0
	BothSeats Seats = 1<<SeatCount - 1
)

func SeatsOf(seats ...Seat) Seats {
	var out Seats
	for _, s := range seats {
		if s.Valid() {
			out |= 1 << uint(s.Index())
		}
	}
	return out
}

func (s Seats) Has(seat Seat) bool {
	return seat.Valid() && s&(1<<uint(seat.Index())) != 0
}

// Index is the address the hardware suite takes: -1 for both seats, 0 or 1
// for one. ok is false for an empty set, which addresses nothing.
func (s Seats) Index() (index int, ok bool) {
	switch s {
	case BothSeats:
		return -1, true
	case SeatsOf(SeatP1):
		return 0, true
	case SeatsOf(SeatP2):
		return 1, true
	}
	return 0, false
}

func (s Seats) String() string {
	switch s {
	case NoSeats:
		return "no seats"
	case BothSeats:
		return "both seats"
	}
	for _, seat := range []Seat{SeatP1, SeatP2} {
		if s.Has(seat) {
			return seat.String()
		}
	}
	return "no seats"
}

// Slot is a controller's position in attach order, which is fixed for as
// long as the device stays on its path and says nothing about where it sits:
// a pad can hold slot 2 and seat P1. The zero value is NoSlot, what a source
// that is not a controller carries.
type Slot int

const NoSlot Slot = 0

func SlotOf(index int) Slot {
	if index < 0 {
		return NoSlot
	}
	return Slot(index + 1)
}

// Index is the zero-based attach index, the inverse of SlotOf, or -1 for
// NoSlot.
func (s Slot) Index() int {
	if s == NoSlot {
		return -1
	}
	return int(s) - 1
}

// Byte is the slot as the cartridge ABI's input record carries it: the
// attach index, or 0xFF for NoSlot and for an index the byte cannot hold.
func (s Slot) Byte() uint8 {
	i := s.Index()
	if i < 0 || i >= 0xFF {
		return 0xFF
	}
	return uint8(i)
}

func (s Slot) String() string {
	if s == NoSlot {
		return "no slot"
	}
	return fmt.Sprintf("slot %d", s.Index())
}
