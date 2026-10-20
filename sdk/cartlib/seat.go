// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// SeatOf converts a zero-based seat index to a [Seat]: 0 is SeatP1, 1 is
// SeatP2, and everything else, -1 included, is SeatNone. Address both seats
// with [SeatBoth].
func SeatOf(v int) Seat { return input.SeatOf(v) }

// SlotOf converts a zero-based player index to a [Slot]: 0 is SlotP1, 1 is
// SlotP2, and everything else, -1 included, is SlotNone.
func SlotOf(v int) Slot { return input.SlotOf(v) }
