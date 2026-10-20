// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"github.com/stealthmonkey99/arkade/internal/flight"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Record byte offsets within the packed 8-byte little-endian input record (wasm.PackRecord).
const (
	recByteKind = iota
	recByteKey
	recBytePlayer
	recByteJoy
	recByteSource
	recBytePhysical
	recByteIndex
	recByteFlags
)

// Record kinds and the key/joystick ordinals the combo needs. Ordinals are the ABI's
// (internal/cartridge/wasm/keys.go), redeclared because that package imports this one, and pinned by
// TestFlightComboOrdinalsMatchTheABI.
const (
	comboKindKeydown  uint8 = 0
	comboKindKeyup    uint8 = 1
	comboKindKeypress uint8 = 2
	comboKindJoystick uint8 = 3

	comboKeyUp    uint8 = 6
	comboKeyDown  uint8 = 7
	comboKeyLeft  uint8 = 8
	comboKeyRight uint8 = 9
	comboKeyAux1  uint8 = 20

	comboJoyN uint8 = 1
	comboJoyE uint8 = 3
	comboJoyS uint8 = 5
	comboJoyW uint8 = 7

	comboFlagVirtual uint8 = 1 << 0
)

// recordByte extracts one byte of a packed record.
func recordByte(packed uint64, index int) uint8 { return uint8(packed >> (8 * index)) }

type comboDir uint8

const (
	comboDirNone comboDir = iota
	comboDirUp
	comboDirDown
	comboDirLeft
	comboDirRight
)

const comboSteps = 8

var comboSequence = [comboSteps]comboDir{
	comboDirUp, comboDirUp, comboDirDown, comboDirDown,
	comboDirLeft, comboDirRight, comboDirLeft, comboDirRight,
}

// comboRotations accepts all four quarter-turn rotations of the sequence, since the cabinet remaps
// every stick by its mount before the detector sees it.
const comboRotations = 4

func comboRotate(d comboDir, r int) comboDir {
	cw := [...]comboDir{comboDirUp, comboDirRight, comboDirDown, comboDirLeft}
	for i, v := range cw {
		if v == d {
			return cw[(i+r)%len(cw)]
		}
	}
	return comboDirNone
}

// comboDetector mirrors cartlib.CheatCode over the flight recorder's record stream, because the
// guest-side combo is invisible to the host. Drift can only miss or mistime a dump, never reach the sim.
type comboDetector struct {
	idx     [comboRotations]int
	held    bool // Aux1 down on a device that reports release
	latched bool // the dev console keyboard's keypress-only stand-in
	// seat is the last seated device that fed the attempt; a keyboard leaves NoSeat.
	seat input.Seat
}

func (c *comboDetector) reset() { *c = comboDetector{} }

// feed consumes one packed record and reports (seat, true) once, on the record that completes the
// combo; seat is the physical seat, or NoSeat for a keyboard.
func (c *comboDetector) feed(packed uint64) (input.Seat, bool) {
	kind := recordByte(packed, recByteKind)
	key := recordByte(packed, recByteKey)

	if key == comboKeyAux1 {
		switch kind {
		case comboKindKeydown:
			c.held = true
			c.noteSeat(packed)
		case comboKindKeyup:
			c.held, c.latched = false, false
		case comboKindKeypress:
			if recordByte(packed, recByteFlags)&comboFlagVirtual != 0 {
				// The arming press also restarts the sequence.
				c.latched = true
				c.idx = [comboRotations]int{}
				c.noteSeat(packed)
			}
		}
		return input.NoSeat, false
	}

	d := comboRecordDir(packed)
	if d == comboDirNone {
		// Another key expires the keyboard latch (Aux1 is a modifier, not a mode); non-press records such as
		// a stick re-centring are ignored.
		if kind == comboKindKeypress && !comboIsDirKey(key) {
			c.latched = false
			c.idx = [comboRotations]int{}
		}
		return input.NoSeat, false
	}
	if !c.held && !c.latched {
		c.idx = [comboRotations]int{}
		return input.NoSeat, false
	}
	c.noteSeat(packed)

	// Each rotation advances or resets independently; a wrong press matching a rotation's first element
	// restarts it at 1.
	for r := 0; r < comboRotations; r++ {
		switch {
		case d == comboRotate(comboSequence[c.idx[r]], r):
			c.idx[r]++
			if c.idx[r] == comboSteps {
				c.idx = [comboRotations]int{}
				seat := c.seat
				c.seat = input.NoSeat
				return seat, true
			}
		case d == comboRotate(comboSequence[0], r):
			c.idx[r] = 1
		default:
			c.idx[r] = 0
		}
	}
	if c.progress() == 0 {
		// A wrong press ends a keyboard attempt; a held button is unaffected.
		c.latched = false
	}
	return input.NoSeat, false
}

func (c *comboDetector) progress() int {
	best := 0
	for _, n := range c.idx {
		if n > best {
			best = n
		}
	}
	return best
}

// noteSeat remembers a seated controller's seat; a keyboard, which carries no slot, leaves it alone.
func (c *comboDetector) noteSeat(packed uint64) {
	seat := input.SeatOfByte(recordByte(packed, recBytePhysical))
	if !seat.Valid() || recordByte(packed, recByteIndex) == input.NoSlot.Byte() {
		return
	}
	c.seat = seat
}

// comboRecordDir decodes one edge-triggered direction press. A real stick emits a joystick record and a
// non-virtual keypress twin, so only one of the two counts; diagonals and re-centre decode to none.
func comboRecordDir(packed uint64) comboDir {
	switch recordByte(packed, recByteKind) {
	case comboKindJoystick:
		switch recordByte(packed, recByteJoy) {
		case comboJoyN:
			return comboDirUp
		case comboJoyS:
			return comboDirDown
		case comboJoyW:
			return comboDirLeft
		case comboJoyE:
			return comboDirRight
		}
	case comboKindKeypress:
		if recordByte(packed, recByteFlags)&comboFlagVirtual != 0 {
			switch recordByte(packed, recByteKey) {
			case comboKeyUp:
				return comboDirUp
			case comboKeyDown:
				return comboDirDown
			case comboKeyLeft:
				return comboDirLeft
			case comboKeyRight:
				return comboDirRight
			}
		}
	}
	return comboDirNone
}

func comboIsDirKey(k uint8) bool {
	return k == comboKeyUp || k == comboKeyDown || k == comboKeyLeft || k == comboKeyRight
}

// noteFlightCombo feeds one packed record to the pad trigger and dumps on completion, at most once per
// session.
func (rt *Runtime) noteFlightCombo(packed uint64) {
	r := rt.flight
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	fired := false
	if r.active && !r.dumpedForCombo {
		if _, ok := r.combo.feed(packed); ok {
			r.dumpedForCombo = true
			fired = true
		}
	}
	r.mu.Unlock()
	if fired {
		rt.dumpAndLog(flight.ReasonCombo, "debug combo entered on the pads")
	}
}
