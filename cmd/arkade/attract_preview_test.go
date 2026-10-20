// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// previewRig boots an awake rig on apps/clock with standby enabled but not
// idle, so a preview must take over from a running app.
func previewRig(t *testing.T) *testkit.Rig {
	t.Helper()
	registerFakeGame(t)
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "apps/clock",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected the rig to start ACTIVE, got %v", got)
	}
	return rig
}

func TestAttractPreviewShowsChosenGameAtOnce(t *testing.T) {
	rig := previewRig(t)

	rig.RT.RequestAttractPreview(holdLaunchSlug)
	rig.Tick(2) // drained at the top of a tick, attract loads the same tick

	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected the preview to force StateAttract, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected the attract app loaded, got %q", got)
	}
	if got := rig.RT.SuspendedSlug(); got != "apps/clock" {
		t.Fatalf("expected the running app parked for a resume, suspended = %q", got)
	}

	// Hold-to-launch reads the current slot (engine.AttractSlot), so launching
	// the fake game proves its slot is showing.
	joyADown(rig)
	rig.TickFor("1700ms")
	if got := rig.RT.CurrentSlug(); got != holdLaunchSlug {
		t.Fatalf("expected the previewed game's slot to be showing (hold should launch %q), got %q", holdLaunchSlug, got)
	}
}

func TestAttractPreviewIsOneShot(t *testing.T) {
	rig := previewRig(t)

	rig.RT.RequestAttractPreview(holdLaunchSlug)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected the attract app loaded, got %q", got)
	}

	// The preview focus must not survive: a natural attract starts on slot 0, the ambient starfield.
	joyADown(rig)
	rig.Tick(6) // a tap, well short of the hold-to-launch threshold
	joyAUp(rig)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected a press to resume the parked app, got %q", got)
	}

	rig.RT.RequestAttractPreview("") // an empty request is ignored outright
	rig.TickFor("5m1s")              // idle out to attract naturally
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected the idle timeout to load attract, got %q", got)
	}
	joyADown(rig)
	rig.TickFor("1700ms")
	if got := rig.RT.CurrentSlug(); got == holdLaunchSlug {
		t.Fatal("a stale preview focus survived: the natural attract started on the previewed game's slot")
	}
}

// TestAttractPreviewWithoutStandby: with standby disabled a preview is
// dropped. The HTTP layer already answers 409; this is the engine backstop.
func TestAttractPreviewWithoutStandby(t *testing.T) {
	registerFakeGame(t)
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/clock"}) // AttractAfter 0
	rig.Tick(1)

	rig.RT.RequestAttractPreview(holdLaunchSlug)
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected the preview ignored with standby disabled, current = %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive with standby disabled, got %v", got)
	}
}

// TestAttractPreviewUnknownSlug: an unregistered slug (a cartridge unmounted
// since the page loaded) changes nothing.
func TestAttractPreviewUnknownSlug(t *testing.T) {
	rig := previewRig(t)

	rig.RT.RequestAttractPreview("no-such-cartridge")
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected an unknown slug ignored, current = %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive after an ignored preview, got %v", got)
	}
}
