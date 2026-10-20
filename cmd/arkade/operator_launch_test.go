// SPDX-License-Identifier: GPL-2.0-or-later

package main

// RequestLaunchNow (the dashboard LAUNCH button) preempts whatever is on
// screen; RequestLaunch (USB hot-mount) is honored only from the shell menu.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
)

func TestOperatorLaunchPreemptsARunningApp(t *testing.T) {
	rig := previewRig(t) // awake, running apps/clock

	rig.RT.RequestLaunchNow(holdLaunchSlug)
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != holdLaunchSlug {
		t.Fatalf("operator launch from a running app landed on %q, want %q", got, holdLaunchSlug)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected the cabinet awake after an operator launch, got %v", got)
	}
}

func TestOperatorLaunchWakesOutOfAttract(t *testing.T) {
	rig := previewRig(t)

	rig.RT.RequestAttractPreview(holdLaunchSlug)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("setup: expected StateAttract, got %v", got)
	}
	if rig.RT.SuspendedSlug() == "" {
		t.Fatal("setup: expected an app parked behind the screensaver")
	}

	rig.RT.RequestLaunchNow(holdLaunchSlug)
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != holdLaunchSlug {
		t.Fatalf("operator launch from attract landed on %q, want %q", got, holdLaunchSlug)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected an operator launch to wake the cabinet, got %v", got)
	}
	// The parked app is torn down, not left to resume.
	if got := rig.RT.SuspendedSlug(); got != "" {
		t.Errorf("operator launch left %q parked behind the screensaver", got)
	}
}

func TestHotMountLaunchStillWillNotPreempt(t *testing.T) {
	rig := previewRig(t) // running apps/clock, not the shell menu

	before := rig.RT.CurrentSlug()
	rig.RT.RequestLaunch(holdLaunchSlug)
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != before {
		t.Fatalf("a USB hot-mount launch preempted %q and landed on %q — "+
			"that guarantee is the whole reason RequestLaunchNow is a separate channel",
			before, got)
	}
}
