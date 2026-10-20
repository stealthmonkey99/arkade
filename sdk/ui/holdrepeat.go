// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// The cabinet's hold-to-repeat cadence for a held stick direction, in
// microseconds: the press edge is the first step, then nothing for the delay,
// then one step per interval.
const (
	HoldRepeatDelayMicros    = 220_000
	HoldRepeatIntervalMicros = 120_000
)

// HoldRepeat turns a held direction into repeated steps at the cabinet's
// cadence. The caller moves on the press edge itself (the keypress that
// accompanies a stick push), hands the stick level to Hold, and each frame
// moves Held() as many times as Update returns. Steps come from accumulated
// dtMicros, so frames the host skips are not counted and no catch-up burst
// fires on resume.
//
// It is a plain value with no allocation. Keep one per screen as a struct
// field and Reset it when the screen is re-entered, since a release during a
// gap in input is never seen and the stale hold would step on resume.
type HoldRepeat struct {
	// DelayMicros is the hold time before the first repeat step and
	// IntervalMicros the time between steps after it; zero means the
	// cabinet defaults HoldRepeatDelayMicros and HoldRepeatIntervalMicros.
	DelayMicros, IntervalMicros uint64
	// FastAfter > 0 switches the interval to FastIntervalMicros once more
	// than FastAfter repeat steps have fired during one hold, for long lists.
	FastAfter          int
	FastIntervalMicros uint64 // microseconds between steps once fast; zero never goes fast

	dir   Direction
	accum uint64
	steps int
}

// Hold records the stick level: a direction arms the hold, [DirNone] releases
// it. A change of direction restarts the delay; repeating the held direction
// is a no-op.
func (h *HoldRepeat) Hold(d Direction) {
	if d == h.dir {
		return
	}
	h.dir = d
	h.accum = 0
	h.steps = 0
}

// Held returns the direction being held, or [DirNone].
func (h *HoldRepeat) Held() Direction { return h.dir }

// Reset releases the hold, keeping the cadence fields.
func (h *HoldRepeat) Reset() { h.Hold(DirNone) }

// Update adds dtMicros to the hold and returns how many repeat steps are due
// this frame: zero while nothing is held or the delay has not passed. Call
// once per frame from Update.
func (h *HoldRepeat) Update(dtMicros uint64) int {
	if h.dir == DirNone {
		return 0
	}
	delay := h.DelayMicros
	if delay == 0 {
		delay = HoldRepeatDelayMicros
	}
	h.accum += dtMicros
	n := 0
	for h.accum >= delay {
		n++
		h.steps++
		h.accum -= h.interval()
	}
	return n
}

func (h *HoldRepeat) interval() uint64 {
	if h.FastAfter > 0 && h.steps > h.FastAfter && h.FastIntervalMicros > 0 {
		return h.FastIntervalMicros
	}
	if h.IntervalMicros == 0 {
		return HoldRepeatIntervalMicros
	}
	return h.IntervalMicros
}
