// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// CheatSteps is the length of the direction sequence.
const CheatSteps = 8

var cheatSequence = [CheatSteps]input.Direction{
	input.DirUp, input.DirUp, input.DirDown, input.DirDown,
	input.DirLeft, input.DirRight, input.DirLeft, input.DirRight,
}

// cheatRotations is four because the cabinet remaps each stick by its
// mount, so the physical gesture arrives rotated by a seat-dependent amount.
const cheatRotations = 4

// cheatRotate turns d by r quarter-turns clockwise. DirNone rotates to
// itself.
func cheatRotate(d input.Direction, r int) input.Direction {
	cw := [...]input.Direction{input.DirUp, input.DirRight, input.DirDown, input.DirLeft}
	for i, v := range cw {
		if v == d {
			return cw[(i+r)%len(cw)]
		}
	}
	return input.DirNone
}

var cheatTainted bool

// CheatTainted reports whether this cartridge instance has been marked as
// cheated, by a completed [CheatCode] or by [MarkCheated]. While it is true,
// [ScoreSubmit], [ScoreQualifies], [ScoreRank] and [NoteRecentName] refuse
// without calling the host; the score reads still work. Nothing clears it:
// the next launch from the shell starts a fresh instance. Read it to show
// the player that this session cannot post a score.
func CheatTainted() bool { return cheatTainted }

// MarkCheated taints the session as [CheatCode] does, for a debug
// affordance other than the shared combo. The taint is one-way.
func MarkCheated() { cheatTainted = true }

// CheatCode detects the shared debug combo, hold Aux1 and push UP UP DOWN
// DOWN LEFT RIGHT LEFT RIGHT, in a stream of input records, so every
// cartridge's debug shortcut has the same entry. The zero value is ready.
// It keeps state across frames, so hold it in a package variable:
//
//	var cheat cartlib.CheatCode
//
//	func handleRecord(r cartlib.InputRecord) {
//	    if seat, ok := cheat.Feed(r); ok {
//	        enterDebugMode(seat) // the physical seat, or SeatNone
//	        return
//	    }
//	    if !r.IsPress() {
//	        return
//	    }
//	    ...
//	}
//
// Feed every record, ahead of any press-only filter, because the Aux1 hold
// is tracked from keydown and keyup records. Feed only in phases where the
// shortcut does something visible: a completed combo sets [CheatTainted]
// whether or not the cartridge acts on it.
type CheatCode struct {
	// OnStep, if set, is called on each press that advances the sequence
	// except the completing one, with the new progress (1..CheatSteps-1),
	// so the cartridge can give entry feedback. Assign it in Init: a
	// package-level initializer that is not compile-time foldable never
	// runs.
	OnStep func(step int)

	idx     [cheatRotations]int // progress through each rotation of the sequence
	seat    Seat
	hasSeat bool
	held    bool // Aux1 down, from a source that also reports release
	latched bool // Aux1 armed by a keypress-only source; see Feed
}

// Feed consumes one input record. It returns (seat, true) once, on the
// record that completes the combo, and (SeatNone, false) otherwise. seat is
// the physical seat of the player who entered it, or [SeatNone] for a
// source with no seat; every peripheral call ignores SeatNone, so it can be
// passed straight to [SetText] or [SetButtonLights]. After firing, the
// detector rearms on the next press.
//
// Aux1 counts as held between its keydown and keyup. A source that reports
// only keypresses (the dev console keyboard) arms the combo with an Aux1
// keypress instead, and the arming lasts until a wrong direction, any other
// keypress, or an Aux1 keyup. Feed uses no clock, so a replayed record
// stream reaches the same state.
func (c *CheatCode) Feed(r InputRecord) (Seat, bool) {
	if r.Key == KeyJoyAux1 {
		switch r.Kind {
		case KindKeydown:
			c.held = true
			c.noteSeat(r)
		case KindKeyup:
			c.held, c.latched = false, false
		case KindKeypress:
			if r.Flags&FlagVirtual != 0 {
				// Re-arming restarts the sequence rather than resuming it.
				c.latched = true
				c.idx = [cheatRotations]int{}
				c.noteSeat(r)
			}
		}
		return SeatNone, false
	}

	d := r.PressDir()
	if d == input.DirNone {
		// Another key's press drops the keyboard latch. Non-press records,
		// such as a stick re-centring, are ignored, or every stick release
		// would break the combo.
		if r.IsPress() && r.Dir() == input.DirNone {
			c.latched = false
			c.idx = [cheatRotations]int{}
		}
		return SeatNone, false
	}
	if !c.held && !c.latched {
		c.idx = [cheatRotations]int{}
		return SeatNone, false
	}
	c.noteSeat(r)

	// A wrong press that is a rotation's first step restarts it at 1, so
	// "up up up down down ..." still matches. OnStep follows the leading
	// rotation only: every direction starts some rotation, so ticking on
	// any advance would tick on every press.
	before := c.Progress()
	for r := 0; r < cheatRotations; r++ {
		switch {
		case d == cheatRotate(cheatSequence[c.idx[r]], r):
			c.idx[r]++
			if c.idx[r] == CheatSteps {
				c.idx = [cheatRotations]int{}
				cheatTainted = true
				seat := c.Seat()
				c.hasSeat = false
				return seat, true
			}
		case d == cheatRotate(cheatSequence[0], r):
			c.idx[r] = 1
		default:
			c.idx[r] = 0
		}
	}
	if after := c.Progress(); after > before {
		c.step()
		return SeatNone, false
	}
	if c.Progress() > 0 {
		return SeatNone, false
	}
	c.latched = false
	return SeatNone, false
}

// Seat returns the physical seat of the player entering the combo, or
// [SeatNone] if no record with a real seat has been seen since the last
// fire or [CheatCode.Reset].
func (c *CheatCode) Seat() Seat {
	if !c.hasSeat {
		return SeatNone
	}
	return c.seat
}

// Progress returns how many steps of the sequence are matched,
// 0..CheatSteps-1.
func (c *CheatCode) Progress() int {
	best := 0
	for _, n := range c.idx {
		if n > best {
			best = n
		}
	}
	return best
}

// Reset drops all progress, the remembered seat and the Aux1 state,
// keeping OnStep. Call it on a scene change that should not carry a
// half-entered combo across. It does not clear [CheatTainted].
func (c *CheatCode) Reset() {
	c.idx = [cheatRotations]int{}
	c.seat, c.hasSeat = 0, false
	c.held, c.latched = false, false
}

func (c *CheatCode) step() {
	if c.OnStep != nil {
		c.OnStep(c.Progress())
	}
}

func (c *CheatCode) noteSeat(r InputRecord) {
	if r.Physical.Single() && r.Index != IndexNone {
		c.seat, c.hasSeat = r.Physical, true
	}
}
