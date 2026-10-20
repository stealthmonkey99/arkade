// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// Status reflects state as of the last completed Tick; a Runtime that has never ticked reports the zero value.
func TestStatusZeroBeforeFirstTick(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	got := rig.RT.Status()
	if got.Slug != "" {
		t.Errorf("expected zero Status before any Tick, got Slug=%q", got.Slug)
	}
}

func TestStatusPopulatedAfterTick(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(5)

	got := rig.RT.Status()
	if got.Slug != "test/bounce" {
		t.Errorf("Slug = %q, want %q", got.Slug, "test/bounce")
	}
	if got.SuspendedSlug != "" {
		t.Errorf("SuspendedSlug = %q, want empty (standby disabled by default)", got.SuspendedSlug)
	}
	if got.StandbyState != "active" {
		t.Errorf("StandbyState = %q, want %q", got.StandbyState, "active")
	}
	if got.Ticks != 5 {
		t.Errorf("Ticks = %d, want 5", got.Ticks)
	}
	if got.UptimeMs <= 0 {
		t.Errorf("UptimeMs = %v, want > 0 after 5 ticks", got.UptimeMs)
	}
	if got.DurationMs <= 0 {
		t.Errorf("DurationMs = %v, want > 0 after 5 ticks", got.DurationMs)
	}
	if got.DroppedFrames != 0 {
		t.Errorf("DroppedFrames = %d, want 0 (rig paces the clock itself)", got.DroppedFrames)
	}
	if got.HeapMB < 0 || got.SysMB < 0 {
		t.Errorf("HeapMB/SysMB should never be negative: %v/%v", got.HeapMB, got.SysMB)
	}
	// hardware.Virtual (the rig's default suite) has no thermal sensor or
	// fan, so these must report the documented "unavailable" defaults.
	if got.TempF != nil {
		t.Errorf("TempF = %v, want nil (hardware.Virtual has no sensor)", *got.TempF)
	}
	if got.FanOn {
		t.Error("FanOn = true, want false (hardware.Virtual has no fan)")
	}
	// No InputSignal/motion has fired yet, so hardware.Virtual.LastMotion
	// reports "never" and MotionAgoMs must stay nil.
	if got.MotionAgoMs != nil {
		t.Errorf("MotionAgoMs = %v, want nil before any input", *got.MotionAgoMs)
	}
	if got.MotionActive {
		t.Error("MotionActive = true, want false before any input")
	}
}

func TestStatusTracksSuspendedApp(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/bounce",
		AttractAfter: 1000, // nanoseconds: any tick spacing trips it
	})
	rig.Tick(2)

	got := rig.RT.Status()
	if got.StandbyState != engine.StateAttract.String() {
		t.Fatalf("StandbyState = %q, want %q", got.StandbyState, engine.StateAttract.String())
	}
	if got.SuspendedSlug != "test/bounce" {
		t.Errorf("SuspendedSlug = %q, want %q", got.SuspendedSlug, "test/bounce")
	}
}

// A real keypress, not InjectMotion, is used because only it fires hardware.Virtual's InputSignal spy that backs LastMotion; InjectMotion feeds only the standby timers.
func TestStatusMotionAgoAfterInput(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Press(input.KeyUp)
	rig.Tick(1)

	got := rig.RT.Status()
	if got.MotionAgoMs == nil {
		t.Fatal("MotionAgoMs = nil, want non-nil after InjectMotion")
	}
	if *got.MotionAgoMs < 0 {
		t.Errorf("MotionAgoMs = %v, want >= 0", *got.MotionAgoMs)
	}
	if !got.MotionActive {
		t.Error("MotionActive = false, want true immediately after motion")
	}
}
