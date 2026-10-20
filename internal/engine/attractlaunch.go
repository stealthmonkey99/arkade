// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"time"
)

// attractLaunchHold is how long green (JoyA) must be held during attract to
// launch the showcased game; long enough that a stray tap never boots one.
const attractLaunchHold = 1500 * time.Millisecond

// AttractSlot is implemented by the attract app to report the game its current
// showcase slot displays. An empty slug means the ambient starfield slot,
// where a hold launches nothing.
type AttractSlot interface {
	AttractGameSlug() string
}

// attractHold times the green button held over a launchable attract slot;
// the zero value is nothing held.
type attractHold struct{ since time.Time }

// begin starts the hold at now unless one is already running, and reports
// whether it did.
func (h *attractHold) begin(now time.Time) bool {
	if !h.since.IsZero() {
		return false
	}
	h.since = now
	return true
}

func (h *attractHold) clear()      { h.since = time.Time{} }
func (h attractHold) active() bool { return !h.since.IsZero() }

// fraction is the hold's progress toward a launch, 0..1; 0 when nothing is
// held.
func (h attractHold) fraction(now time.Time) float64 {
	if !h.active() {
		return 0
	}
	return clamp01(float64(now.Sub(h.since)) / float64(attractLaunchHold))
}

// complete reports whether the hold has run long enough to launch.
func (h attractHold) complete(now time.Time) bool {
	return h.active() && now.Sub(h.since) >= attractLaunchHold
}

// attractLaunchSlug returns the showcased game's slug, or "" when not
// attracting or the slot has no launchable game.
func (rt *Runtime) attractLaunchSlug() string {
	if rt.standby.state != StateAttract {
		return ""
	}
	slot, ok := rt.cur.(AttractSlot)
	if !ok {
		return ""
	}
	return slot.AttractGameSlug()
}

// AttractHoldFraction reports the green-button hold-to-launch progress, 0..1
// (0 when nothing is held).
func (rt *Runtime) AttractHoldFraction() float64 {
	return rt.attractHold.fraction(rt.Clock.Now())
}

// stepAttractHold fires a completed hold, or drops one whose slot stopped
// being launchable.
func (rt *Runtime) stepAttractHold(now time.Time) {
	if !rt.attractHold.active() {
		return
	}
	slug := rt.attractLaunchSlug()
	switch {
	case slug == "":
		rt.attractHold.clear()
	case rt.attractHold.complete(now):
		rt.launchAttractGame(now, slug)
	}
}

// launchAttractGame wakes from attract into the showcased game. Unlike a tap
// it discards the app parked under standby and boots the game fresh.
func (rt *Runtime) launchAttractGame(now time.Time, slug string) {
	m, err := Lookup(slug)
	if err != nil {
		// Falls back to a normal wake rather than staying stuck in attract.
		rt.Log("error", fmt.Sprintf("standby: hold-launch %q not found: %v", slug, err))
		rt.leaveStandby(now, resumeParked)
		return
	}
	rt.leaveStandby(now, discardParked)
	rt.Log("info", fmt.Sprintf("standby: hold-launch %s", slug))
	rt.requestLoad(m)
}

// discardSuspended tears down the app parked by a standby takeover without
// resuming it. suspendCurrent kept its layers and strategies alive for a
// resume, so unloadCurrent releases them before the standby app is restored.
func (rt *Runtime) discardSuspended() {
	s := rt.suspended
	if s == nil {
		return
	}
	rt.suspended = nil
	cur, curCtx := rt.cur, rt.curCtx
	rt.cur, rt.curCtx = s.app, s.ctx
	rt.unloadCurrent()
	rt.cur, rt.curCtx = cur, curCtx
}
