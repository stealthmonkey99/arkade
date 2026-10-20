// SPDX-License-Identifier: GPL-2.0-or-later

package main

// These run against the real app registry. They live here because
// internal/engine's tests assume no shell app is registered.

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func TestStandbyStateMachine(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "apps/clock",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})

	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive at boot, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected apps/clock running, got %q", got)
	}

	// ACTIVE -> ATTRACT after idling past attractAfter.
	rig.TickFor("5m1s")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract after idling past attractAfter, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract app loaded, got %q", got)
	}

	// Motion resets the presence clock, so 9 more minutes stays under standbyAfter.
	rig.Motion()
	rig.TickFor("9m")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("motion should keep attract alive, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract still loaded, got %q", got)
	}

	// DARK: app unloaded, lights off, one black frame, then painting stops.
	framesBefore := rig.Capture.FrameCount()
	rig.TickFor("1m1s") // ~10m1s since the motion in step 2
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "" {
		t.Fatalf("expected no app loaded while dark, got %q", got)
	}
	assertAllBlack(t, rig)
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	for player := 0; player < 2; player++ {
		for _, name := range hardware.LightButtons {
			if hw.ButtonLit(player, name) {
				t.Errorf("expected all lights off while dark, player %d %s is lit", player, name)
			}
		}
	}
	stillDarkFrames := rig.Capture.FrameCount()
	rig.Tick(5)
	if rig.Capture.FrameCount() != stillDarkFrames {
		t.Errorf("expected no new frames while dark, painted %d more", rig.Capture.FrameCount()-stillDarkFrames)
	}
	if rig.Capture.FrameCount() <= framesBefore {
		t.Errorf("expected at least the one black frame to have painted on entering dark")
	}

	// Motion wakes DARK to ATTRACT, never to ACTIVE.
	rig.Motion()
	rig.Tick(1) // the load is applied at the start of the next tick
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected motion to wake DARK to StateAttract, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract app reloaded, got %q", got)
	}

	// A waking rotate keypress must not rotate; it never reaches Dispatch.
	before := rig.RT.Renderer.VirtualOrientation()
	rig.PressKeyboard(input.KeyRotate, false)
	rig.Tick(1)
	if after := rig.RT.Renderer.VirtualOrientation(); after != before {
		t.Fatalf("rotate keypress leaked through: orientation changed %v -> %v", before, after)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected any input to wake to StateActive, got %v", got)
	}

	// The wake resumes the suspended app without acting on the waking press.
	rig.Tick(1)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected suspended apps/clock resumed after waking, got %q", got)
	}
	rig.Tick(5)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected apps/clock to remain current (no leaked keypress acted on it), got %q", got)
	}

	// A later quit key must act, proving the resumed app is alive.
	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected a real quit press to exit the resumed app to the shell, got %q", got)
	}
}

// Other goldens rely on zero AttractAfter/StandbyAfter never leaving StateActive.
func TestStandbyDisabledByDefault(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/clock"})
	rig.Clock.Advance(24 * time.Hour)
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected standby disabled by default, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected apps/clock still running, got %q", got)
	}
}

func assertAllBlack(t *testing.T, rig *testkit.Rig) {
	t.Helper()
	frame := rig.Frame()
	for y, row := range frame {
		for x, px := range row {
			if px != display.Black {
				t.Fatalf("expected an all-black frame while dark, pixel (%d,%d)=%+v", x, y, px)
			}
		}
	}
}

// TestBaseOrientationCycleWithRealApps: the panel faces the room (CounterClockwise)
// while idle and the seated player (Clockwise) during play.
func TestBaseOrientationCycleWithRealApps(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:          "apps/clock",
		AttractAfter:      5 * time.Minute,
		StandbyAfter:      10 * time.Minute,
		RoomOrientation:   display.CounterClockwise,
		SeatedOrientation: display.Clockwise,
	})

	rig.Tick(1)
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("active play should face the seated player, got %v", got)
	}

	rig.TickFor("5m1s")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract after idling, got %v", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
		t.Fatalf("attract should face the room, got %v", got)
	}

	rig.Press(input.KeyJoyA)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("a press should wake to StateActive, got %v", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("waking should return to the seated orientation, got %v", got)
	}
}

// TestBootHandsOffToAttractWithRealApps: with nobody to pair, boot ends in
// attract facing the room, and the waking press lands on an already seated shell.
func TestBootHandsOffToAttractWithRealApps(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		AttractAfter:      5 * time.Minute,
		StandbyAfter:      10 * time.Minute,
		RoomOrientation:   display.CounterClockwise,
		SeatedOrientation: display.Clockwise,
	})
	if got := rig.RT.CurrentSlug(); got != "boot" {
		t.Fatalf("expected the boot app at power-on, got %q", got)
	}

	rig.Tick(80) // the 2500ms splash at 30fps, then the exit and the switch to attract
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("a silent pairing should leave the cabinet attracting, got %v (app %q)", got, rig.RT.CurrentSlug())
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected the attract reel after boot, got %q", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
		t.Fatalf("an untouched cabinet should face the room, got %v", got)
	}

	rig.Press(input.KeyJoyA)
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("the waking press should bring up the shell, got %q", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("the woken shell should already face the seated player, got %v", got)
	}
}
