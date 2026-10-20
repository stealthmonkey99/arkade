// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// The seat-claim gesture is SELECT plus a shoulder: R1 claims P1, the
// right-hand seat, and L1 claims P2. SELECT itself passes through, since
// swallowing it would need a synthetic release; the shoulder is swallowed
// while SELECT is held.
const (
	seatChordP1 = input.KeyR1 // right-hand seat, the red side
	seatChordP2 = input.KeyL1
)

// seatChord reports whether the event was consumed by the claim gesture and
// must not reach the cartridge.
func (m *Manager) seatChord(a *activeDevice, ev input.Event) bool {
	switch ev.Name {
	case input.KeySelect:
		switch ev.Kind {
		case input.KindKeydown:
			a.selectHeld = true
		case input.KindKeyup:
			a.selectHeld = false
		}
		return false
	case seatChordP1, seatChordP2:
		if !a.selectHeld {
			return false
		}
		// Only the press edge claims; keydown and keyup are swallowed too.
		if ev.Kind == input.KindKeypress {
			seat := input.SeatP1
			if ev.Name == seatChordP2 {
				seat = input.SeatP2
			}
			m.claimSeat(a, seat)
		}
		return true
	}
	return false
}

func (m *Manager) claimSeat(a *activeDevice, seat input.Seat) {
	m.AssignSeat(a.info.Path, seat)
	m.RememberSeat(a.info.StableID(), seat)
	m.logger("log", fmt.Sprintf("controller: %s claimed %v", a.profile.Name, seat))
	m.mu.Lock()
	onSeatClaim := m.onSeatClaim
	m.mu.Unlock()
	if onSeatClaim != nil {
		onSeatClaim(a.profile.Name, seat)
	}
}

// OnSeatClaim registers a callback for the claim gesture. It runs on the HID
// read goroutine and must not block.
func (m *Manager) OnSeatClaim(fn func(device string, seat input.Seat)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSeatClaim = fn
}

// RememberSeat records a device's seat by StableID so it takes that seat on
// every future attach.
func (m *Manager) RememberSeat(stableID string, seat input.Seat) {
	if stableID == "" || !seat.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seatMemory[stableID] = seat
}

// SeatMemory returns a copy of the remembered assignments, StableID to
// seat.
func (m *Manager) SeatMemory() map[string]input.Seat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]input.Seat, len(m.seatMemory))
	for k, v := range m.seatMemory {
		out[k] = v
	}
	return out
}

func (m *Manager) ForgetSeat(stableID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.seatMemory, stableID)
}

// AssignSeat puts a device at a seat without displacing its holder, unlike
// PairSeat, which swaps. A pad joining must not evict a wired stick, and
// a seat may hold both a stick and a pad.
func (m *Manager) AssignSeat(id string, seat input.Seat) {
	if !seat.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target, ok := m.active[id]
	if !ok || target.seat == seat {
		return
	}
	m.setSeatLocked(target, seat)
	m.logger("log", fmt.Sprintf("controller: %s assigned to %v", target.profile.Name, seat))
}
