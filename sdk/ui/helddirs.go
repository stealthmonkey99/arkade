// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// HeldDirs tracks which directions are currently held, the level signal
// hold-to-repeat consumers need (name entry scrolling, rotation, paddle slew).
// It unifies the three ways hosts report a hold:
//
//   - Real HID buttons send KindKeydown and KindKeyup pairs.
//   - A joystick sends a position level that persists until its next record.
//   - The dev console keyboard sends only KindKeypress records, so a held
//     arrow arrives as an auto-repeat pulse train; each pulse grants
//     PulseMicros of held credit.
//
// Feed every input record to [HeldDirs.Feed], all kinds and before any
// keypress-only filter, call [HeldDirs.Update] once per frame, and read
// [HeldDirs.Held] or [HeldDirs.Current]. Directions are tracked independently; Current
// collapses them to the most recently activated one. HeldDirs retains state
// across frames, so keep one per screen as a struct field or package
// variable. A zero value is ready to use, and no method allocates.
type HeldDirs struct {
	// PulseMicros is the held credit granted per Pulse, in microseconds. 0
	// defaults to 250,000.
	PulseMicros uint64

	real   [4]bool
	credit [4]uint64
	joy    Direction
	last   Direction
}

func dirIndex(d Direction) int {
	switch d {
	case DirUp:
		return 0
	case DirDown:
		return 1
	case DirLeft:
		return 2
	case DirRight:
		return 3
	}
	return -1
}

// KeyDown marks d physically pressed ([input.KindKeydown] from a real device).
// A d other than the four directions is ignored.
func (h *HeldDirs) KeyDown(d Direction) {
	if i := dirIndex(d); i >= 0 {
		h.real[i] = true
		h.last = d
	}
}

// KeyUp marks d physically released ([input.KindKeyup]) and drops that direction's
// outstanding pulse credit, so a browser key that sends keydown, a virtual
// keypress and keyup stops being held at release. The joystick level is
// unaffected; SetJoy([DirNone]) releases it.
func (h *HeldDirs) KeyUp(d Direction) {
	if i := dirIndex(d); i >= 0 {
		h.real[i] = false
		h.credit[i] = 0
	}
}

// Pulse grants PulseMicros of held credit for d (replacing any remaining credit), for a source that cannot
// report release.
func (h *HeldDirs) Pulse(d Direction) {
	if i := dirIndex(d); i >= 0 {
		p := h.PulseMicros
		if p == 0 {
			p = 250_000
		}
		h.credit[i] = p
		h.last = d
	}
}

// SetJoy records the joystick's position level ([DirNone] when centred). One
// direction is stored, so a caller that wants a diagonal to count picks the
// axis it means.
func (h *HeldDirs) SetJoy(d Direction) {
	h.joy = d
	if d != DirNone {
		h.last = d
	}
}

// Feed applies one input record of any kind: an arrow keydown or keyup marks
// that direction held or released, a virtual arrow keypress grants a pulse of
// held credit, and a joystick record sets the stick level ([DirNone] for centre
// and for diagonals). Every other record is ignored, so a cartridge can pass
// its whole record stream through unfiltered.
//
// Feed is [HeldDirs.KeyDown], [HeldDirs.KeyUp], [HeldDirs.Pulse] and
// [HeldDirs.SetJoy] driven by [input.Record.Dir]; call
// those directly only for a mapping Feed does not make, such as a diagonal
// that should count as one of its axes.
func (h *HeldDirs) Feed(r input.Record) {
	if r.Kind == input.KindJoystick {
		h.SetJoy(r.Dir())
		return
	}
	d := r.Dir()
	if d == DirNone {
		return
	}
	switch r.Kind {
	case input.KindKeydown:
		h.KeyDown(d)
	case input.KindKeyup:
		h.KeyUp(d)
	case input.KindKeypress:
		if r.Virtual() {
			h.Pulse(d)
		}
	}
}

// Update decays pulse credit by dtMicros microseconds. Call once per frame
// from Update.
func (h *HeldDirs) Update(dtMicros uint64) {
	for i := range h.credit {
		if h.credit[i] > dtMicros {
			h.credit[i] -= dtMicros
		} else {
			h.credit[i] = 0
		}
	}
}

// Held reports whether d is currently held by any signal (real key,
// unexpired virtual credit, or joystick position).
func (h *HeldDirs) Held(d Direction) bool {
	i := dirIndex(d)
	if i < 0 {
		return false
	}
	return h.real[i] || h.credit[i] > 0 || h.joy == d
}

// Current returns the most recently activated direction that is still held,
// or [DirNone], for consumers that need a single direction.
func (h *HeldDirs) Current() Direction {
	if h.last != DirNone && h.Held(h.last) {
		return h.last
	}
	for _, d := range heldOrder {
		if h.Held(d) {
			return d
		}
	}
	return DirNone
}

// heldOrder is the fallback precedence when the last-activated direction
// has been released, in dirIndex order.
var heldOrder = [4]Direction{DirUp, DirDown, DirLeft, DirRight}

// Reset clears all held state, keeping PulseMicros. Call on scene transitions
// so a stale hold cannot leak into a new screen.
func (h *HeldDirs) Reset() {
	*h = HeldDirs{PulseMicros: h.PulseMicros}
}
