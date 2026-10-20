// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// TestStandbyShellBackEntersAttractThenHardDark: BACK at the shell menu root
// suspends the shell behind attract, a second BACK goes hard dark, and only a
// button press wakes it.
func TestStandbyShellBackEntersAttractThenHardDark(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "shell",
		AttractAfter:   5 * time.Minute,
		StandbyAfter:   10 * time.Minute,
		HardDecayAfter: 10 * time.Minute,
	})
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected shell running, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive, got %v", got)
	}

	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract after shell BACK, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract loaded, got %q", got)
	}
	if got := rig.RT.SuspendedSlug(); got != "shell" {
		t.Fatalf("expected shell suspended behind attract, got %q", got)
	}

	// A deliberate dark plays the goodnight before the panel goes black.
	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark after attract BACK, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "" {
		t.Fatalf("expected no app loaded while dark, got %q", got)
	}
	if !rig.RT.StandbyHard() {
		t.Fatal("expected the dark to be hard")
	}
	rig.TickFor("5s") // ride out the goodnight
	assertAllBlack(t, rig)

	rig.Motion()
	rig.Tick(3)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected motion to leave hard dark alone, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "" {
		t.Fatalf("expected still no app loaded (motion must not wake hard dark), got %q", got)
	}

	rig.PressKeyboard(input.KeyEnter, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive after a real press, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected the suspended shell menu resumed, got %q", got)
	}
	if got := rig.RT.SuspendedSlug(); got != "" {
		t.Fatalf("expected nothing left suspended, got %q", got)
	}
}

// TestStandbyHardDarkDecaysAfterNoMotion: HardDecayAfter without presence
// clears the hard flag, after which motion wakes it.
func TestStandbyHardDarkDecaysAfterNoMotion(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "shell",
		AttractAfter:   time.Minute,
		StandbyAfter:   time.Minute,
		HardDecayAfter: 10 * time.Minute,
	})
	rig.Tick(3)

	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark, got %v", got)
	}
	if !rig.RT.StandbyHard() {
		t.Fatal("expected the dark to be hard")
	}

	// Waits on the clock alone; motion would push the decay out.
	rig.TickFor("9m59s")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected still StateDark before the decay window elapses, got %v", got)
	}
	if !rig.RT.StandbyHard() {
		t.Fatal("expected hard to still be set just before the decay window elapses")
	}

	rig.TickFor("2s")
	if rig.RT.StandbyHard() {
		t.Fatal("expected hard to have decayed to soft")
	}
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected decay to leave the state at StateDark, got %v", got)
	}

	rig.Motion()
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected motion to wake the now-soft dark to StateAttract, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract reloaded, got %q", got)
	}
}

// TestStandbyDisabledShellBackStillExitsEngine: with standby disabled, BACK
// at the shell menu root exits the engine; headless and test flows rely on it.
func TestStandbyDisabledShellBackStillExitsEngine(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	rig.Tick(1)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected shell running, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive (standby disabled), got %v", got)
	}

	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)

	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected standby to stay disabled/Active, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "" {
		t.Fatalf("expected the engine to quit (no app loaded, no shell fallback since shell itself quit), got %q", got)
	}

	// No further frames is the proxy for quitting.
	frames := rig.Capture.FrameCount()
	rig.Tick(3)
	if rig.Capture.FrameCount() != frames {
		t.Errorf("expected no new frames after quitting, painted %d more", rig.Capture.FrameCount()-frames)
	}
}

// pushRaw injects one controller event of any kind, unlike Rig.PressAs, which is keypress-only.
func pushRaw(rig *testkit.Rig, kind input.EventKind, name input.KeyName) {
	rig.RT.Input.Push(input.Event{
		Kind: kind,
		Name: name,
		Source: input.PlayerSource{
			Type: input.SourceController, ID: "usb-hw", Slot: input.SlotOf(0), Seat: input.SeatP1,
		},
	})
}

// TestStandbyBackChainSurvivesHardwareEventPairs replays the HID decoder's
// keydown+keypress then keyup per press: the release of the press that
// entered standby must not wake the cabinet.
func TestStandbyBackChainSurvivesHardwareEventPairs(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "shell",
		AttractAfter:   5 * time.Minute,
		StandbyAfter:   10 * time.Minute,
		HardDecayAfter: 10 * time.Minute,
	})
	rig.Tick(3)

	pushRaw(rig, input.KindKeydown, input.KeyJoyB)
	pushRaw(rig, input.KindKeypress, input.KeyJoyB)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract after shell back press, got %q", got)
	}
	pushRaw(rig, input.KindKeyup, input.KeyJoyB) // release lands after attract
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("release of the back press woke the cabinet: got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract, got %v", got)
	}

	// In attract, keydown must not wake, keypress deepens, release must not wake.
	pushRaw(rig, input.KindKeydown, input.KeyJoyB)
	pushRaw(rig, input.KindKeypress, input.KeyJoyB)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected hard dark after attract back press, got %v", got)
	}
	pushRaw(rig, input.KindKeyup, input.KeyJoyB)
	rig.Tick(3)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("release of the back press woke dark: got %v", got)
	}

	pushRaw(rig, input.KindKeydown, input.KeyJoyA)
	pushRaw(rig, input.KindKeypress, input.KeyJoyA)
	rig.Tick(2)
	pushRaw(rig, input.KindKeyup, input.KeyJoyA)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected button press to wake to the shell menu, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive after wake, got %v", got)
	}
}
