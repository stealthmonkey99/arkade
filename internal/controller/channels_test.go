// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
)

func TestPublishSurvivesADeviceBeyondTheSeats(t *testing.T) {
	spec := DualShock.Spec
	bus := NewChannelBus()
	bus.Offer(input.SeatP1, spec)

	report := make([]byte, 64)
	report[0] = 0x01

	for _, seat := range []input.Seat{input.NoSeat, 3, 4, 99} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Publish panicked for a device at seat %v: %v", seat, r)
				}
			}()
			bus.Publish(seat, spec, report)
		}()
	}

	bus.Publish(input.SeatP1, spec, report)
}
