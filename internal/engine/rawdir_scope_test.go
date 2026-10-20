// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// The override belongs to the app instance that set it: the screensaver
// covering it does not inherit it, and the resume brings it back.
func TestRawDirectionsOverrideTravelsWithTheSuspendedApp(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Minute, 10*time.Minute)
	rig.tick(1)
	rig.rt.SetRawDirectionsOverride(true)

	rig.jump(time.Minute)
	rig.tick(1)
	if rig.rt.StandbyState() != StateAttract || rig.rt.SuspendedSlug() != "test/foreground" {
		t.Fatalf("setup: want the app parked behind attract, state %v suspended %q", rig.rt.StandbyState(), rig.rt.SuspendedSlug())
	}
	if rig.rt.curCtx.rawDirections() {
		t.Error("attract inherited the parked app's raw-directions override")
	}

	rig.press(input.KeyJoyC)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/foreground" {
		t.Fatalf("wake resumed %q, want the parked app", got)
	}
	if !rig.rt.curCtx.rawDirections() {
		t.Error("the resumed app lost its raw-directions override")
	}
}

// With nothing loaded there is no app to override, so nothing waits to
// ambush whichever app loads next.
func TestRawDirectionsOverrideWithNoAppIsDropped(t *testing.T) {
	rt := &Runtime{}
	rt.SetRawDirectionsOverride(true)
	rt.curCtx = &AppContext{}
	if rt.curCtx.rawDirections() {
		t.Error("an override set with no app loaded reached the next one")
	}
}
